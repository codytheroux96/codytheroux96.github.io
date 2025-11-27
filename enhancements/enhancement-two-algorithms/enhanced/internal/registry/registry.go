package registry

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	servers map[string]Server
	mu      sync.RWMutex
	logger  *slog.Logger
}

type Server struct {
	Name         string    `json:"name"`
	BaseURL      string    `json:"base_url"`
	Prefixes     []string  `json:"routes"`
	RegisteredAt time.Time `json:"registered_at"`
}

func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		servers: make(map[string]Server),
		logger:  logger,
	}
}

func (r *Registry) Register(s Server) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.servers[s.Name]; exists {
		return fmt.Errorf("server '%s' already registered", s.Name)
	}

	r.servers[s.Name] = s
	return nil
}

func (r *Registry) Deregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.servers[name]; !exists {
		return fmt.Errorf("server '%s' does not exist... cannot deregister", name)
	}

	delete(r.servers, name)
	return nil
}

func (r *Registry) ListRegistered() []Server {
	r.mu.RLock()
	defer r.mu.RUnlock()

	servers := make([]Server, 0, len(r.servers))
	for _, server := range r.servers {
		servers = append(servers, server)
	}

	return servers
}

// ServersForPath returns the longest matching prefix and all servers that handle that prefix
func (r *Registry) ServersForPath(requestPath string) (string, []Server) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	longestPrefix := ""
	var matchingServers []Server

	// First pass: find the longest matching prefix
	for _, server := range r.servers {
		for _, prefix := range server.Prefixes {
			if strings.HasPrefix(requestPath, prefix) && len(prefix) > len(longestPrefix) {
				longestPrefix = prefix
			}
		}
	}

	// Second pass: collect all servers that match the longest prefix
	if longestPrefix != "" {
		for _, server := range r.servers {
			for _, prefix := range server.Prefixes {
				if prefix == longestPrefix {
					matchingServers = append(matchingServers, server)
					break // Don't add the same server multiple times
				}
			}
		}
	}

	return longestPrefix, matchingServers
}
