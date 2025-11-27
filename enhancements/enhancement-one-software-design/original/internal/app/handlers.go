package app

import (
	"bytes"
	"io"
	"net/http"
	"time"
)

func (app *Application) reverseProxyHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app.HandleGetRequest(w, r)
	case http.MethodPost:
		app.HandlePostRequest(w, r)
	default:
		http.Error(w, "unsupported http method", http.StatusMethodNotAllowed)
	}
}

func (app *Application) HandleGetRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if cachedResp, found := app.Cache.Get(path); found {
		w.WriteHeader(http.StatusOK)
		w.Write(cachedResp)
		app.Logger.Info("Cache hit", "path", path)
		return
	}

	backendURL, err := app.determineBackendURL(path)
	if err != nil {
		http.Error(w, "invalid backend path", http.StatusNotFound)
		return
	}

	maxRetries := 3
	backoffTimes := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}

	var resp *http.Response
	var finalErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		app.Logger.Info("Forwarding GET request", "url", backendURL, "attempt", attempt)

		req, err := http.NewRequest(http.MethodGet, backendURL, nil)
		if err != nil {
			app.Logger.Error("Failed to create GET request", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		for key, values := range r.Header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		resp, finalErr = app.Client.Do(req)
		if finalErr != nil {
			app.Logger.Warn("Request failed", "url", backendURL, "error", finalErr, "attempt", attempt)
			if attempt < maxRetries {
				time.Sleep(backoffTimes[attempt-1])
			}
			continue
		}

		if resp.StatusCode >= 500 && resp.StatusCode <= 504 {
			app.Logger.Warn("Server error from backend", "status", resp.StatusCode, "attempt", attempt)
			if attempt < maxRetries {
				resp.Body.Close()
				time.Sleep(backoffTimes[attempt-1])
				continue
			}
		}

		break
	}

	if finalErr != nil {
		app.Logger.Error("Final failure on GET request", "url", backendURL, "error", finalErr)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	if resp == nil || resp.StatusCode >= 500 {
		app.Logger.Error("Final failure - bad backend status", "url", backendURL, "status", resp.StatusCode)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		app.Logger.Error("Failed to read response body", "error", err)
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(resp.StatusCode)
	w.Write(bodyBytes)
	app.Logger.Info("request succeeded")

	if resp.StatusCode == http.StatusOK {
		app.Cache.Store(path, bodyBytes)
		app.Logger.Info("Response cached", "path", path)
	}

}

func (app *Application) HandlePostRequest(w http.ResponseWriter, r *http.Request) {
	backendURL, err := app.determineBackendURL(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid backend path", http.StatusNotFound)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		app.Logger.Error("failed to read request body", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	maxRetries := 3
	backoffTimes := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}

	var resp *http.Response
	var finalErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		bodyReader := bytes.NewReader(bodyBytes)

		req, err := http.NewRequest(http.MethodPost, backendURL, bodyReader)
		if err != nil {
			app.Logger.Error("failed to create POST request", "error", err)
			http.Error(w, "failed to create request", http.StatusInternalServerError)
			return
		}

		for key, values := range r.Header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		resp, finalErr = app.Client.Do(req)
		if finalErr != nil {
			app.Logger.Warn("Request failed", "url", backendURL, "error", finalErr, "attempt", attempt)
			if attempt < maxRetries {
				time.Sleep(backoffTimes[attempt-1])
			}
			continue
		}

		if resp.StatusCode >= 500 && resp.StatusCode <= 504 {
			app.Logger.Warn("Server error from backend", "status", resp.StatusCode, "attempt", attempt)
			if attempt < maxRetries {
				resp.Body.Close()
				time.Sleep(backoffTimes[attempt-1])
				continue
			}
		}

		break
	}

	if finalErr != nil {
		app.Logger.Error("POST request failed - no response received", "url", backendURL, "error", finalErr)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	if resp == nil || resp.StatusCode >= 500 {
		app.Logger.Error("Final failure - bad backend status", "url", backendURL, "status", resp.StatusCode)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		app.Logger.Error("failed to copy response body", "error", err)
	}
	app.Logger.Info("request succeeded")
}
