package server_two

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
		app.logger.Error("failed to register server two with proxy", "error", err)
		os.Exit(1)
	}

	serverTwo := &http.Server{
		Addr:         ":2200",
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdown
		app.logger.Info("shutting server two down gracefully")

		if err := deregisterFromProxy(); err != nil {
			app.logger.Error("failed to deregister server two from proxy", "error", err)
		} else {
			app.logger.Info("successfully deregistered server two from proxy")
		}

		os.Exit(0)
	}()

	app.logger.Info("SERVER TWO IS RUNNING", "addr", serverTwo.Addr)

	if err := serverTwo.ListenAndServe(); err != nil {
		app.logger.Error(err.Error())
		os.Exit(1)
	}
}

func registerWithProxy() error {
	payload := map[string]interface{}{
		"name":     "server_two",
		"base_url": "http://localhost:2200",
		"routes":   []string{"/s2"},
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
		"name": "server_two",
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
