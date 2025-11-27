package server_one

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Serve() {
	app := newApplication()

	if err := registerWithProxy(); err != nil {
		app.logger.Error("failed to register server one with proxy", "error", err)
		os.Exit(1)
	}

	serverOne := &http.Server{
		Addr:         ":4200",
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdown
		app.logger.Info("shutting server one down gracefully")

		if err := deregisterFromProxy(); err != nil {
			app.logger.Error("failed to deregister server one from proxy", "error", err)
		} else {
			app.logger.Info("successfully deregistered server one from proxy")
		}

		os.Exit(0)
	}()

	app.logger.Info("SERVER ONE IS RUNNING", "addr", serverOne.Addr)

	if err := serverOne.ListenAndServe(); err != nil {
		app.logger.Error(err.Error())
		os.Exit(1)
	}
}

func registerWithProxy() error {
	payload := map[string]interface{}{
		"name":     "server_one",
		"base_url": "http://localhost:4200",
		"routes":   []string{"/s1"},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post("https://localhost:8443/register", "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to call proxy register endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration failed, status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func deregisterFromProxy() error {
	payload := map[string]string{
		"name": "server_one",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post("https://localhost:8443/deregister", "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to call proxy register endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("deregistration failed, status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
