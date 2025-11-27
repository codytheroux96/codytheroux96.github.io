package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/codytheroux96/go-reverse-proxy/internal/app"
	"github.com/codytheroux96/go-reverse-proxy/test_servers/server_one"
	"github.com/codytheroux96/go-reverse-proxy/test_servers/server_two"
)

func redirectHandler(w http.ResponseWriter, r *http.Request) {
	target := "https://localhost:8443" + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	http.Redirect(w, r, target, http.StatusTemporaryRedirect)
}

func main() {
	// Try PostgreSQL first, fallback to in-memory
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres@localhost/reverse_proxy?sslmode=disable"
	}

	application, err := app.NewApplicationWithPostgreSQL(databaseURL)
	if err != nil {
		// Fallback to in-memory registry
		fmt.Printf("PostgreSQL connection failed, using in-memory registry: %v\n", err)
		application = app.NewApplicationWithInMemoryRegistry()
	} else {
		fmt.Println("Using PostgreSQL-backed registry")
	}

	application.Logger.Info("MESSAGE FROM MAIN SERVER: APPLICATION IS RUNNING!!!")

	application.Start()

	proxyServer := &http.Server{
		Addr:         ":8443",
		Handler:      application.RateLimit(application.Routes()),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		application.Logger.Info("Starting test server one on :4200")
		server_one.Serve()
	}()

	go func() {
		application.Logger.Info("Starting test server two on :2200")
		server_two.Serve()
	}()

	redirectServer := &http.Server{
		Addr:    ":8080",
		Handler: http.HandlerFunc(redirectHandler),
	}

	go func() {
		application.Logger.Info("Starting redirect server on :8080")
		if err := redirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			application.Logger.Error("Redirect server failed", "error", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		application.Logger.Info("Shutdown signal received, gracefully shutting down...")
		application.Shutdown()
		os.Exit(0)
	}()

	application.Logger.Info("Starting reverse proxy server on :8443")
	if err := proxyServer.ListenAndServeTLS("cert/cert.pem", "cert/key.pem"); err != nil {
		application.Logger.Error("Proxy server failed", "error", err)
		application.Shutdown()
		os.Exit(1)
	}
}
