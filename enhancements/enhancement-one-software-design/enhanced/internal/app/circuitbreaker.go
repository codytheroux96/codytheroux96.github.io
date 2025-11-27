package app

import (
	"log/slog"
	"sync"
	"time"
)

// BreakerState represents the current state of a circuit breaker
type BreakerState int

const (
	// Closed state allows all requests through
	Closed BreakerState = iota
	// Open state blocks all requests
	Open
	// HalfOpen state allows limited requests through for testing
	HalfOpen
)

// String returns a string representation of the breaker state
func (s BreakerState) String() string {
	switch s {
	case Closed:
		return "Closed"
	case Open:
		return "Open"
	case HalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

const (
	// FailuresToOpen is the number of failures needed to open the breaker
	FailuresToOpen = 5
	// OpenCooldown is how long to wait before transitioning to half-open
	OpenCooldown = 30 * time.Second
)

// Breaker represents the state of a circuit breaker for a single server
type Breaker struct {
	State        BreakerState `json:"state"`
	Failures     int          `json:"failures"`
	LastOpenTime time.Time    `json:"last_open_time"`
	InFlight     int          `json:"in_flight"` // Number of requests currently in flight during HalfOpen
}

// CircuitBreakerManager manages circuit breakers for all backend servers
type CircuitBreakerManager struct {
	breakers map[string]*Breaker
	mu       sync.RWMutex
	logger   *slog.Logger
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(logger *slog.Logger) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*Breaker),
		logger:   logger,
	}
}

// AllowRequest checks if a request should be allowed through the circuit breaker
func (cbm *CircuitBreakerManager) AllowRequest(serverName string) bool {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	breaker, exists := cbm.breakers[serverName]
	if !exists {
		// Create a new breaker in Closed state for unknown servers
		breaker = &Breaker{
			State:    Closed,
			Failures: 0,
		}
		cbm.breakers[serverName] = breaker
	}

	switch breaker.State {
	case Closed:
		// Allow all requests when closed
		return true

	case Open:
		// Check if we should transition to half-open
		if time.Since(breaker.LastOpenTime) >= OpenCooldown {
			cbm.logger.Info("transitioning breaker to half-open",
				"server", serverName,
				"cooldown_elapsed", time.Since(breaker.LastOpenTime))
			breaker.State = HalfOpen
			breaker.InFlight = 0
			return true
		}
		// Block requests during open state
		cbm.logger.Debug("breaker open, blocking request",
			"server", serverName,
			"time_remaining", OpenCooldown-time.Since(breaker.LastOpenTime))
		return false

	case HalfOpen:
		// Allow only one probe request at a time
		if breaker.InFlight == 0 {
			breaker.InFlight++
			cbm.logger.Debug("allowing probe request in half-open state", "server", serverName)
			return true
		}
		cbm.logger.Debug("probe request already in flight, blocking", "server", serverName)
		return false

	default:
		return false
	}
}

// OnSuccess records a successful request and potentially closes the breaker
func (cbm *CircuitBreakerManager) OnSuccess(serverName string) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	breaker, exists := cbm.breakers[serverName]
	if !exists {
		return
	}

	// Reset failure count on success
	breaker.Failures = 0

	// Handle state transitions based on current state
	switch breaker.State {
	case HalfOpen:
		// Transition back to closed on successful probe
		breaker.State = Closed
		breaker.InFlight = 0
		cbm.logger.Info("breaker closed after successful probe",
			"server", serverName)

	case Open:
		// This shouldn't happen if AllowRequest is working correctly
		cbm.logger.Warn("success recorded for open breaker", "server", serverName)

	case Closed:
		// Normal operation, no state change needed
		cbm.logger.Debug("success recorded for closed breaker", "server", serverName)
	}
}

// OnFailure records a failed request and potentially opens the breaker
func (cbm *CircuitBreakerManager) OnFailure(serverName string) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	breaker, exists := cbm.breakers[serverName]
	if !exists {
		breaker = &Breaker{
			State:    Closed,
			Failures: 0,
		}
		cbm.breakers[serverName] = breaker
	}

	breaker.Failures++

	switch breaker.State {
	case HalfOpen:
		// Failed probe - go back to open
		breaker.State = Open
		breaker.LastOpenTime = time.Now()
		breaker.InFlight = 0
		cbm.logger.Warn("probe failed, breaker opened",
			"server", serverName,
			"failures", breaker.Failures)

	case Closed:
		// Check if we should transition to open
		if breaker.Failures >= FailuresToOpen {
			breaker.State = Open
			breaker.LastOpenTime = time.Now()
			cbm.logger.Warn("breaker opened due to failures",
				"server", serverName,
				"failures", breaker.Failures,
				"threshold", FailuresToOpen)
		} else {
			cbm.logger.Debug("failure recorded",
				"server", serverName,
				"failures", breaker.Failures,
				"threshold", FailuresToOpen)
		}

	case Open:
		// Already open, just log the additional failure
		cbm.logger.Debug("additional failure on open breaker",
			"server", serverName,
			"failures", breaker.Failures)
	}
}

// OnRequestComplete should be called when a request completes in HalfOpen state
func (cbm *CircuitBreakerManager) OnRequestComplete(serverName string) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	breaker, exists := cbm.breakers[serverName]
	if !exists {
		return
	}

	if breaker.State == HalfOpen && breaker.InFlight > 0 {
		breaker.InFlight--
		cbm.logger.Debug("request completed in half-open state",
			"server", serverName,
			"in_flight", breaker.InFlight)
	}
}

// GetBreakerState returns the current state of a circuit breaker
func (cbm *CircuitBreakerManager) GetBreakerState(serverName string) BreakerState {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	breaker, exists := cbm.breakers[serverName]
	if !exists {
		return Closed // Default state for unknown servers
	}

	return breaker.State
}

// GetBreakerInfo returns detailed information about a circuit breaker
func (cbm *CircuitBreakerManager) GetBreakerInfo(serverName string) (Breaker, bool) {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	breaker, exists := cbm.breakers[serverName]
	if !exists {
		return Breaker{}, false
	}

	// Return a copy to avoid race conditions
	return *breaker, true
}

// GetAllBreakers returns the state of all circuit breakers
func (cbm *CircuitBreakerManager) GetAllBreakers() map[string]Breaker {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	result := make(map[string]Breaker)
	for name, breaker := range cbm.breakers {
		result[name] = *breaker
	}

	return result
}

// RemoveBreaker removes a circuit breaker for a server (useful when deregistering)
func (cbm *CircuitBreakerManager) RemoveBreaker(serverName string) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	delete(cbm.breakers, serverName)
	cbm.logger.Info("removed circuit breaker for server", "server", serverName)
}

// ResetBreaker manually resets a circuit breaker to closed state
func (cbm *CircuitBreakerManager) ResetBreaker(serverName string) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	breaker, exists := cbm.breakers[serverName]
	if !exists {
		return
	}

	oldState := breaker.State
	breaker.State = Closed
	breaker.Failures = 0
	breaker.InFlight = 0

	cbm.logger.Info("manually reset circuit breaker",
		"server", serverName,
		"old_state", oldState.String(),
		"new_state", breaker.State.String())
}
