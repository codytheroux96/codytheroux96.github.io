package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/codytheroux96/go-reverse-proxy/internal/registry"
)

const (
	HealthInterval     = 5 * time.Second
	UnhealthyThreshold = 3
	HealthCheckTimeout = 1 * time.Second
	HealthCheckPath    = "/health"
)

// HealthStatus represents the health state of a backend server
type HealthStatus struct {
	IsHealthy           bool          `json:"is_healthy"`
	LastChecked         time.Time     `json:"last_checked"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	LastResponseTime    time.Duration `json:"last_response_time"`
}

// HealthMonitor manages health checking for all registered backends
type HealthMonitor struct {
	registry  *registry.Registry
	healthMap map[string]*HealthStatus
	mu        sync.RWMutex
	logger    *slog.Logger
	client    *http.Client
	stopCh    chan struct{}
	stopped   chan struct{}
}

// NewHealthMonitor creates a new health monitor instance
func NewHealthMonitor(reg *registry.Registry, logger *slog.Logger) *HealthMonitor {
	return &HealthMonitor{
		registry:  reg,
		healthMap: make(map[string]*HealthStatus),
		logger:    logger,
		client: &http.Client{
			Timeout: HealthCheckTimeout,
		},
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

// Start begins the health monitoring process
func (hm *HealthMonitor) Start(ctx context.Context) {
	hm.logger.Info("starting health monitor", "interval", HealthInterval)

	ticker := time.NewTicker(HealthInterval)
	defer ticker.Stop()
	defer close(hm.stopped)

	// Initial health check
	hm.checkAllServers(ctx)

	for {
		select {
		case <-ticker.C:
			hm.checkAllServers(ctx)
		case <-hm.stopCh:
			hm.logger.Info("health monitor stopped")
			return
		case <-ctx.Done():
			hm.logger.Info("health monitor context cancelled")
			return
		}
	}
}

// Stop gracefully shuts down the health monitor
func (hm *HealthMonitor) Stop() {
	close(hm.stopCh)
	<-hm.stopped
}

// checkAllServers performs health checks on all registered servers
func (hm *HealthMonitor) checkAllServers(ctx context.Context) {
	servers := hm.registry.ListRegistered()
	if len(servers) == 0 {
		return
	}

	hm.logger.Debug("performing health checks", "server_count", len(servers))

	// Use a WaitGroup to perform health checks in parallel
	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Add(1)
		go func(s registry.Server) {
			defer wg.Done()
			hm.checkServerHealth(ctx, s)
		}(server)
	}
	wg.Wait()
}

// checkServerHealth performs a health check on a single server
func (hm *HealthMonitor) checkServerHealth(ctx context.Context, server registry.Server) {
	start := time.Now()
	healthURL := server.BaseURL + HealthCheckPath

	// Create request with context for timeout
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		hm.updateHealthStatus(server.Name, false, time.Since(start))
		hm.logger.Error("failed to create health check request",
			"server", server.Name, "error", err)
		return
	}

	resp, err := hm.client.Do(req)
	responseTime := time.Since(start)

	if err != nil {
		hm.updateHealthStatus(server.Name, false, responseTime)
		hm.logger.Debug("health check failed",
			"server", server.Name, "error", err, "response_time", responseTime)
		return
	}
	defer resp.Body.Close()

	isHealthy := resp.StatusCode >= 200 && resp.StatusCode < 300
	hm.updateHealthStatus(server.Name, isHealthy, responseTime)

	if isHealthy {
		hm.logger.Debug("health check passed",
			"server", server.Name, "status", resp.StatusCode, "response_time", responseTime)
	} else {
		hm.logger.Warn("health check failed",
			"server", server.Name, "status", resp.StatusCode, "response_time", responseTime)
	}
}

// updateHealthStatus updates the health status for a server
func (hm *HealthMonitor) updateHealthStatus(serverName string, isHealthy bool, responseTime time.Duration) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	status, exists := hm.healthMap[serverName]
	if !exists {
		status = &HealthStatus{
			IsHealthy:           false,
			ConsecutiveFailures: 0,
		}
		hm.healthMap[serverName] = status
	}

	status.LastChecked = time.Now()
	status.LastResponseTime = responseTime

	if isHealthy {
		status.ConsecutiveFailures = 0
		wasUnhealthy := !status.IsHealthy
		status.IsHealthy = true

		if wasUnhealthy {
			hm.logger.Info("server recovered",
				"server", serverName, "response_time", responseTime)
		}
	} else {
		status.ConsecutiveFailures++
		wasHealthy := status.IsHealthy

		if status.ConsecutiveFailures >= UnhealthyThreshold {
			status.IsHealthy = false
			if wasHealthy {
				hm.logger.Warn("server marked unhealthy",
					"server", serverName,
					"consecutive_failures", status.ConsecutiveFailures)
			}
		}
	}

	hm.logger.Debug("health status updated",
		"server", serverName,
		"healthy", status.IsHealthy,
		"failures", status.ConsecutiveFailures,
		"response_time", responseTime)
}

// IsHealthy returns whether a server is currently healthy
func (hm *HealthMonitor) IsHealthy(serverName string) bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	status, exists := hm.healthMap[serverName]
	if !exists {
		// Default to unhealthy for unknown servers
		return false
	}

	return status.IsHealthy
}

// GetHealthStatus returns the complete health status for a server
func (hm *HealthMonitor) GetHealthStatus(serverName string) (HealthStatus, bool) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	status, exists := hm.healthMap[serverName]
	if !exists {
		return HealthStatus{}, false
	}

	// Return a copy to avoid race conditions
	return *status, true
}

// GetAllHealthStatuses returns health status for all servers
func (hm *HealthMonitor) GetAllHealthStatuses() map[string]HealthStatus {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make(map[string]HealthStatus)
	for name, status := range hm.healthMap {
		result[name] = *status
	}

	return result
}

// RemoveServer removes health tracking for a server (useful when deregistering)
func (hm *HealthMonitor) RemoveServer(serverName string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	delete(hm.healthMap, serverName)
	hm.logger.Info("removed health tracking for server", "server", serverName)
}
