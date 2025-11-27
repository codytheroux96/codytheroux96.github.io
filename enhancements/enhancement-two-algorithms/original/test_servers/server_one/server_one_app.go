package server_one

import (
	"log/slog"
	"net/http"
	"os"
)

type application struct {
	logger *slog.Logger
}

func newApplication() *application {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	return &application{
		logger: logger,
	}
}

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			app.handleHealthcheck(w, r)
		default:
			app.logger.Error("unsupported HTTP method", slog.String("method", r.Method), slog.String("url", r.URL.String()))
			http.Error(w, "unsupported HTTP method", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			app.handleList(w, r)
		default:
			app.logger.Error("unsupported HTTP method", slog.String("method", r.Method), slog.String("url", r.URL.String()))
			http.Error(w, "unsupported HTTP method", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			app.handleEcho(w, r)
		default:
			app.logger.Error("unsupported HTTP method", slog.String("method", r.Method), slog.String("url", r.URL.String()))
			http.Error(w, "unsupported HTTP method", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		app.handleHeaders(w, r)
	})

	return mux
}
