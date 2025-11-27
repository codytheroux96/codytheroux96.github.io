package server_one

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

func (app *application) handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Server One Is Up And Healthy\n"))
}

func (app *application) handleList(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Server One Is Serving A Fake List\n"))
}

func (app *application) handleEcho(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.logger.Error("unsupported HTTP method", slog.String("method", r.Method))
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		app.logger.Error("failed to read body", slog.String("error", err.Error()))
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if !json.Valid(body) {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func (app *application) handleHeaders(w http.ResponseWriter, r *http.Request) {
	headersMap := make(map[string][]string)

	for name, value := range r.Header {
		app.logger.Info("headers received", slog.String("key", name), slog.Any("values", value))
		headersMap[name] = value
	}

	jsonData, err := json.Marshal(headersMap)
	if err != nil {
		app.logger.Error("failed to marshal headers", slog.String("error", err.Error()))
		http.Error(w, "failed to marshal headers", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}
