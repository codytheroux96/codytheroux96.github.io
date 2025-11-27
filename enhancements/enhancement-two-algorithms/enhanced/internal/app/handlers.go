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

	backend, err := app.Router.ResolveBackend(path)
	if err != nil {
		app.Logger.Warn("backend resolution failed", "path", path, "error", err)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	resp, err := app.performRequest(http.MethodGet, backend.TargetURL, r, nil)
	if err != nil {
		app.CircuitBreaker.OnFailure(backend.Server.Name)
		app.Logger.Error("GET request failed", "server", backend.Server.Name, "url", backend.TargetURL, "error", err)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
		app.CircuitBreaker.OnFailure(backend.Server.Name)
		app.Logger.Warn("server error from backend", "server", backend.Server.Name, "status", resp.StatusCode)
	} else {
		app.CircuitBreaker.OnSuccess(backend.Server.Name)
	}

	app.CircuitBreaker.OnRequestComplete(backend.Server.Name)

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

	app.Logger.Info("GET request completed",
		"server", backend.Server.Name,
		"status", resp.StatusCode,
		"path", path)

	if resp.StatusCode == http.StatusOK {
		app.Cache.Store(path, bodyBytes)
		app.Logger.Debug("Response cached", "path", path)
	}
}

func (app *Application) HandlePostRequest(w http.ResponseWriter, r *http.Request) {
	backend, err := app.Router.ResolveBackend(r.URL.Path)
	if err != nil {
		app.Logger.Warn("backend resolution failed", "path", r.URL.Path, "error", err)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		app.Logger.Error("failed to read request body", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	resp, err := app.performRequest(http.MethodPost, backend.TargetURL, r, bodyBytes)
	if err != nil {
		app.CircuitBreaker.OnFailure(backend.Server.Name)
		app.Logger.Error("POST request failed", "server", backend.Server.Name, "url", backend.TargetURL, "error", err)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
		app.CircuitBreaker.OnFailure(backend.Server.Name)
		app.Logger.Warn("server error from backend", "server", backend.Server.Name, "status", resp.StatusCode)
	} else {
		app.CircuitBreaker.OnSuccess(backend.Server.Name)
	}

	app.CircuitBreaker.OnRequestComplete(backend.Server.Name)

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		app.Logger.Error("failed to copy response body", "error", err)
	}

	app.Logger.Info("POST request completed",
		"server", backend.Server.Name,
		"status", resp.StatusCode,
		"path", r.URL.Path)
}

func (app *Application) performRequest(method, url string, originalReq *http.Request, body []byte) (*http.Response, error) {
	maxRetries := 3
	backoffTimes := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}

	var resp *http.Response
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}

		req, createErr := http.NewRequest(method, url, reqBody)
		if createErr != nil {
			app.Logger.Error("Failed to create request", "method", method, "url", url, "error", createErr)
			return nil, createErr
		}

		for key, values := range originalReq.Header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		app.Logger.Debug("Forwarding request",
			"method", method,
			"url", url,
			"attempt", attempt)

		resp, err = app.Client.Do(req)
		if err != nil {
			app.Logger.Warn("Request failed", "url", url, "error", err, "attempt", attempt)
			if attempt < maxRetries {
				time.Sleep(backoffTimes[attempt-1])
				continue
			}
			return nil, err
		}

		if resp.StatusCode >= 500 && resp.StatusCode <= 504 && attempt < maxRetries {
			app.Logger.Warn("Server error from backend", "status", resp.StatusCode, "attempt", attempt)
			resp.Body.Close()
			time.Sleep(backoffTimes[attempt-1])
			continue
		}

		break
	}

	return resp, err
}
