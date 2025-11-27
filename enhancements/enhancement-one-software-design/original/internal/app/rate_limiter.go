package app

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

func (app *Application) RateLimit(next http.Handler) http.Handler {
	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	var (
		mu      sync.Mutex
		clients = make(map[string]*client)
	)

	go func() {
		for {
			time.Sleep(time.Minute)

			mu.Lock()

			for ip, client := range clients {
				if time.Since(client.lastSeen) > 3*time.Minute {
					delete(clients, ip)
				}
			}

			mu.Unlock()
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if app.config.Limiter.enabled {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				app.Logger.Error("error getting client IP", "error", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			mu.Lock()

			if _, found := clients[ip]; !found {
				clients[ip] = &client{
					limiter: rate.NewLimiter(rate.Limit(app.config.Limiter.rps), app.config.Limiter.burst),
				}
			}

			clients[ip].lastSeen = time.Now()

			if !clients[ip].limiter.Allow() {
				mu.Unlock()
				app.Logger.Info("rate limit exceeded", "client_ip", ip)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			mu.Unlock()
		}

		next.ServeHTTP(w, r)
	})
}
