package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/codytheroux96/go-reverse-proxy/internal/registry"
)

// RegistryInterface defines what a registry must implement
type RegistryInterface interface {
	Register(server registry.Server) error
	Deregister(name string) error
	GetServers() ([]registry.Server, error)
	GetServer(name string) (*registry.Server, error)
	ServersForPath(path string) (string, []registry.Server, bool)
	HandleRegister(w http.ResponseWriter, r *http.Request)
	HandleDeregister(w http.ResponseWriter, r *http.Request)
	HandleRegistryList(w http.ResponseWriter, r *http.Request)
}

type RateLimiterConfig struct {
	enabled bool
	rps     float64
	burst   int
}

type Application struct {
	Logger *slog.Logger
	Cache  *ResponseCache
	config struct {
		Limiter RateLimiterConfig
	}
	Client         *http.Client
	Registry       RegistryInterface
	HealthMonitor  *HealthMonitor
	CircuitBreaker *CircuitBreakerManager
	Router         *ResilientRouter
	ctx            context.Context
	cancelFunc     context.CancelFunc
}

func NewApplication() *Application {
	return NewApplicationWithInMemoryRegistry()
}

func NewApplicationWithInMemoryRegistry() *Application {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	registry := registry.NewRegistry(logger)
	return newApplication(logger, registry)
}

func NewApplicationWithPostgreSQL(databaseURL string) (*Application, error) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	registry, err := registry.NewPostgreSQLRegistry(databaseURL, logger)
	if err != nil {
		return nil, err
	}
	return newApplication(logger, registry), nil
}

func newApplication(logger *slog.Logger, reg RegistryInterface) *Application {

	// Create context for the application lifecycle
	ctx, cancel := context.WithCancel(context.Background())

	// Configure cache with TTL and byte capacity
	cacheTTL := 30 * time.Second
	cacheMaxBytes := 10 * 1024 * 1024 // 10 MB cache capacity

	app := &Application{
		Logger: logger,
		Cache:  NewResponseCache(cacheTTL, cacheMaxBytes, logger),
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
		Registry:       reg,
		HealthMonitor:  NewHealthMonitor(reg, logger),
		CircuitBreaker: NewCircuitBreakerManager(logger),
		ctx:            ctx,
		cancelFunc:     cancel,
	}

	app.Router = NewResilientRouter(app)

	go app.Cache.Cleanup(app, 15*time.Second)

	app.config.Limiter = RateLimiterConfig{
		enabled: true,
		rps:     50,
		burst:   250,
	}

	return app
}

func (app *Application) Start() {
	app.Logger.Info("starting application components")

	go func() {
		app.HealthMonitor.Start(app.ctx)
	}()
}

func (app *Application) Shutdown() {
	app.Logger.Info("shutting down application")

	app.cancelFunc()

	app.HealthMonitor.Stop()
}

func (app *Application) LogRequest(r *http.Request) {
	app.Logger.Info("Incoming Request", "method", r.Method, "path", r.URL.Path)
}
