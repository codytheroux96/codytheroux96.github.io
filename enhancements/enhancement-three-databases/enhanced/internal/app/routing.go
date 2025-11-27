package app

import (
	"fmt"
	"strings"
	"sync"

	"github.com/codytheroux96/go-reverse-proxy/internal/registry"
)

// ResilientRouter handles routing with health checks and load balancing
type ResilientRouter struct {
	app             *Application
	roundRobinIndex map[string]int // per-prefix round-robin counter
	mu              sync.Mutex     // protects roundRobinIndex
}

// NewResilientRouter creates a new resilient router
func NewResilientRouter(app *Application) *ResilientRouter {
	return &ResilientRouter{
		app:             app,
		roundRobinIndex: make(map[string]int),
	}
}

// BackendInfo represents information about a selected backend
type BackendInfo struct {
	Server    registry.Server
	TargetURL string
	Prefix    string
}

// ResolveBackend finds a healthy backend for the given request path
func (rr *ResilientRouter) ResolveBackend(requestPath string) (*BackendInfo, error) {
	// 1) Find longest prefix match and candidate servers
	prefix, candidates, found := rr.app.Registry.ServersForPath(requestPath)
	if prefix == "" || !found || len(candidates) == 0 {
		rr.app.Logger.Debug("no route found", "path", requestPath)
		return nil, fmt.Errorf("no_route")
	}

	// 2) Filter for healthy servers that pass circuit breaker check
	var healthyServers []registry.Server
	for _, server := range candidates {
		isHealthy := rr.app.HealthMonitor.IsHealthy(server.Name)
		allowedByBreaker := rr.app.CircuitBreaker.AllowRequest(server.Name)

		if isHealthy && allowedByBreaker {
			healthyServers = append(healthyServers, server)
			rr.app.Logger.Debug("server eligible",
				"server", server.Name,
				"healthy", isHealthy,
				"breaker_allowed", allowedByBreaker)
		} else {
			rr.app.Logger.Debug("server filtered out",
				"server", server.Name,
				"healthy", isHealthy,
				"breaker_allowed", allowedByBreaker)
		}
	}

	if len(healthyServers) == 0 {
		rr.app.Logger.Warn("no healthy backends available",
			"path", requestPath,
			"prefix", prefix,
			"total_candidates", len(candidates))
		return nil, fmt.Errorf("no_healthy_backends")
	}

	// 3) Round-robin selection within healthy servers for this prefix
	rr.mu.Lock()
	index := rr.roundRobinIndex[prefix] % len(healthyServers)
	rr.roundRobinIndex[prefix]++
	rr.mu.Unlock()

	chosen := healthyServers[index]

	// 4) Construct target URL
	trimmedPath := strings.TrimPrefix(requestPath, prefix)
	targetURL := chosen.BaseURL + trimmedPath

	rr.app.Logger.Info("backend selected",
		"path", requestPath,
		"prefix", prefix,
		"server", chosen.Name,
		"target_url", targetURL,
		"healthy_count", len(healthyServers),
		"total_count", len(candidates))

	return &BackendInfo{
		Server:    chosen,
		TargetURL: targetURL,
		Prefix:    prefix,
	}, nil
}
