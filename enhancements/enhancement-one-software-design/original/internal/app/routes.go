package app

import (
	"net/http"
)

func (app *Application) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", app.reverseProxyHandler)

	mux.HandleFunc("/register", app.Registry.HandleRegister)
	mux.HandleFunc("/deregister", app.Registry.HandleDeregister)
	mux.HandleFunc("/registry", app.Registry.HandleRegistryList)

	return mux
}
