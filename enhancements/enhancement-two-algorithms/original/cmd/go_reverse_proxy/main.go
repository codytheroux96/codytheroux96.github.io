package main

import (
	"net/http"
	"os"
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
	app := app.NewApplication()
	app.Logger.Info("MESSAGE FROM MAIN SERVER: APPLICATION IS RUNNING!!!")

	proxyServer := &http.Server{
		Addr:         ":8443",
		Handler:      app.RateLimit(app.Routes()),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		server_one.Serve()
	}()

	go func() {
		server_two.Serve()
	}()

	redirectServer := &http.Server{
		Addr:    ":8080",
		Handler: http.HandlerFunc(redirectHandler),
	}

	go func() {
		if err := redirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.Logger.Error("Redirect server failed", "error", err)
		}
	}()

	if err := proxyServer.ListenAndServeTLS("cert/cert.pem", "cert/key.pem"); err != nil {
		app.Logger.Error(err.Error())
		os.Exit(1)
	}

	select {}
}
