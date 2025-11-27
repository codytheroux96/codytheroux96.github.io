package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/codytheroux96/go-reverse-proxy/internal/db"
	"github.com/lib/pq"
)

type PostgreSQLRegistry struct {
	queries *db.Queries
	db      *sql.DB
	logger  *slog.Logger
}

func NewPostgreSQLRegistry(databaseURL string, logger *slog.Logger) (*PostgreSQLRegistry, error) {
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgreSQLRegistry{
		queries: db.New(database),
		db:      database,
		logger:  logger,
	}, nil
}

func (r *PostgreSQLRegistry) Register(s Server) error {
	ctx := context.Background()

	// Convert []string to pq.StringArray for PostgreSQL
	prefixes := pq.StringArray(s.Prefixes)

	service, err := r.queries.RegisterService(ctx, db.RegisterServiceParams{
		Name:     s.Name,
		BaseUrl:  s.BaseURL,
		Prefixes: prefixes,
	})
	if err != nil {
		r.logger.Error("Failed to register service", "error", err, "service", s.Name)
		return fmt.Errorf("failed to register service: %w", err)
	}

	r.logger.Info("Service registered", "service", s.Name, "base_url", s.BaseURL, "prefixes", s.Prefixes)
	_ = service // Use the returned service if needed
	return nil
}

func (r *PostgreSQLRegistry) Deregister(name string) error {
	ctx := context.Background()

	err := r.queries.DeleteService(ctx, name)
	if err != nil {
		r.logger.Error("Failed to deregister service", "error", err, "service", name)
		return fmt.Errorf("failed to deregister service: %w", err)
	}

	r.logger.Info("Service deregistered", "service", name)
	return nil
}

func (r *PostgreSQLRegistry) GetServers() ([]Server, error) {
	ctx := context.Background()

	services, err := r.queries.GetAllServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %w", err)
	}

	servers := make([]Server, len(services))
	for i, service := range services {
		registeredAt := service.CreatedAt.Time
		if !service.CreatedAt.Valid {
			registeredAt = time.Now() // fallback
		}
		servers[i] = Server{
			Name:         service.Name,
			BaseURL:      service.BaseUrl,
			Prefixes:     []string(service.Prefixes),
			RegisteredAt: registeredAt,
		}
	}

	return servers, nil
}

func (r *PostgreSQLRegistry) GetServer(name string) (*Server, error) {
	ctx := context.Background()

	service, err := r.queries.GetService(ctx, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("server '%s' not found", name)
		}
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	registeredAt := service.CreatedAt.Time
	if !service.CreatedAt.Valid {
		registeredAt = time.Now() // fallback
	}

	server := &Server{
		Name:         service.Name,
		BaseURL:      service.BaseUrl,
		Prefixes:     []string(service.Prefixes),
		RegisteredAt: registeredAt,
	}

	return server, nil
}

func (r *PostgreSQLRegistry) ServersForPath(requestPath string) (string, []Server, bool) {
	ctx := context.Background()

	// Get all services and find the longest prefix match
	services, err := r.queries.GetAllServices(ctx)
	if err != nil {
		r.logger.Error("Failed to get services for path matching", "error", err)
		return "", nil, false
	}

	longestPrefix := ""
	var matchingServers []Server

	// Find the longest matching prefix
	for _, service := range services {
		prefixes := []string(service.Prefixes)
		for _, prefix := range prefixes {
			if strings.HasPrefix(requestPath, prefix) && len(prefix) > len(longestPrefix) {
				longestPrefix = prefix
			}
		}
	}

	// Collect all servers that match the longest prefix
	if longestPrefix != "" {
		for _, service := range services {
			prefixes := []string(service.Prefixes)
			for _, prefix := range prefixes {
				if prefix == longestPrefix {
					registeredAt := service.CreatedAt.Time
					if !service.CreatedAt.Valid {
						registeredAt = time.Now() // fallback
					}
					server := Server{
						Name:         service.Name,
						BaseURL:      service.BaseUrl,
						Prefixes:     prefixes,
						RegisteredAt: registeredAt,
					}
					matchingServers = append(matchingServers, server)
					break // Don't add the same server multiple times
				}
			}
		}
	}

	return longestPrefix, matchingServers, len(matchingServers) > 0
}

func (r *PostgreSQLRegistry) Close() error {
	return r.db.Close()
}

// HTTP Handlers
func (r *PostgreSQLRegistry) HandleRegister(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var srv Server
	if err := json.NewDecoder(req.Body).Decode(&srv); err != nil {
		http.Error(w, "invalid payload in request", http.StatusBadRequest)
		return
	}

	srv.RegisteredAt = time.Now()

	if err := r.Register(srv); err != nil {
		r.logger.Error("Failed to register server", "error", err)
		http.Error(w, "failed to register server", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered", "server": srv.Name})
}

func (r *PostgreSQLRegistry) HandleDeregister(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := req.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name parameter required", http.StatusBadRequest)
		return
	}

	if err := r.Deregister(name); err != nil {
		r.logger.Error("Failed to deregister server", "error", err)
		http.Error(w, "failed to deregister server", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "deregistered", "server": name})
}

func (r *PostgreSQLRegistry) HandleRegistryList(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	servers, err := r.GetServers()
	if err != nil {
		r.logger.Error("Failed to get servers", "error", err)
		http.Error(w, "failed to get servers", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(servers)
}
