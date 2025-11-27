package app

import (
	"log/slog"
	"sync"
	"time"
)

type ResponseCache struct {
	mu     sync.RWMutex
	cache  map[string]CacheEntry
	ttl    time.Duration
	Logger *slog.Logger
}

type CacheEntry struct {
	response []byte
	expires  time.Time
}

func NewResponseCache(ttl time.Duration, logger *slog.Logger) *ResponseCache {
	return &ResponseCache{
		cache:  make(map[string]CacheEntry),
		ttl:    ttl,
		Logger: logger,
	}
}

func (rc *ResponseCache) Store(key string, data []byte) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.cache[key] = CacheEntry{
		response: data,
		expires:  time.Now().Add(rc.ttl),
	}
}

func (rc *ResponseCache) Get(key string) ([]byte, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	element, exists := rc.cache[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(element.expires) {
		return nil, false
	}

	return element.response, true
}

func (rc *ResponseCache) Cleanup(app *Application, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		rc.mu.Lock()
		for key, entry := range rc.cache {
			if time.Now().After(entry.expires) {
				delete(rc.cache, key)
				app.Logger.Info("Cache entry expired and removed", "key", key)
			}
		}
		rc.mu.Unlock()
	}
}
