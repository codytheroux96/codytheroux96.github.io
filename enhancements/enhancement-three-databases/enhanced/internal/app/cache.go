package app

import (
	"log/slog"
	"sync"
	"time"
)

// Node represents a cache entry in the doubly linked list
type Node struct {
	key       string
	value     []byte
	sizeBytes int
	expiresAt time.Time
	prev      *Node
	next      *Node
}

// ResponseCache is an LRU cache with TTL and byte-based capacity
type ResponseCache struct {
	mu        sync.RWMutex
	items     map[string]*Node
	head      *Node // Most Recently Used (MRU)
	tail      *Node // Least Recently Used (LRU)
	maxBytes  int
	usedBytes int
	ttl       time.Duration
	Logger    *slog.Logger
}

// NewResponseCache creates a new LRU cache with TTL and byte capacity
func NewResponseCache(ttl time.Duration, maxBytes int, logger *slog.Logger) *ResponseCache {
	return &ResponseCache{
		items:     make(map[string]*Node),
		head:      nil,
		tail:      nil,
		maxBytes:  maxBytes,
		usedBytes: 0,
		ttl:       ttl,
		Logger:    logger,
	}
}

// Get retrieves a value from the cache and moves it to MRU position
func (rc *ResponseCache) Get(key string) ([]byte, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	node, exists := rc.items[key]
	if !exists {
		rc.Logger.Debug("Cache miss", "key", key, "reason", "not_found")
		return nil, false
	}

	// Check if expired
	if time.Now().After(node.expiresAt) {
		rc.Logger.Debug("Cache miss", "key", key, "reason", "expired")
		rc.detachNode(node)
		delete(rc.items, key)
		rc.usedBytes -= node.sizeBytes
		return nil, false
	}

	// Move to head (MRU position)
	rc.moveToHead(node)

	// Return a copy of the value to prevent external modification
	valueCopy := make([]byte, len(node.value))
	copy(valueCopy, node.value)

	rc.Logger.Debug("Cache hit", "key", key, "size", node.sizeBytes)
	return valueCopy, true
}

// Store adds or updates a value in the cache
func (rc *ResponseCache) Store(key string, value []byte) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	size := rc.approximateSize(key, value)
	now := time.Now()

	if existingNode, exists := rc.items[key]; exists {
		// Update existing node
		rc.Logger.Debug("Cache update", "key", key, "old_size", existingNode.sizeBytes, "new_size", size)

		rc.usedBytes -= existingNode.sizeBytes

		// Create a copy of the value
		existingNode.value = make([]byte, len(value))
		copy(existingNode.value, value)
		existingNode.sizeBytes = size
		existingNode.expiresAt = now.Add(rc.ttl)

		rc.moveToHead(existingNode)
		rc.usedBytes += size
	} else {
		// Create new node
		rc.Logger.Debug("Cache store", "key", key, "size", size)

		// Create a copy of the value
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)

		newNode := &Node{
			key:       key,
			value:     valueCopy,
			sizeBytes: size,
			expiresAt: now.Add(rc.ttl),
		}

		rc.items[key] = newNode
		rc.insertAtHead(newNode)
		rc.usedBytes += size
	}

	// Evict nodes while over capacity
	rc.evictToCapacity()
}

// Cleanup periodically removes expired entries (for compatibility)
func (rc *ResponseCache) Cleanup(app *Application, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rc.cleanupExpired()
		case <-app.ctx.Done():
			rc.Logger.Info("Cache cleanup stopped")
			return
		}
	}
}

// cleanupExpired removes expired entries proactively
func (rc *ResponseCache) cleanupExpired() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	now := time.Now()
	expiredCount := 0

	// Walk from tail (LRU) towards head, removing expired entries
	current := rc.tail
	for current != nil {
		prev := current.prev

		if now.After(current.expiresAt) {
			rc.Logger.Debug("Removing expired cache entry", "key", current.key)
			rc.detachNode(current)
			delete(rc.items, current.key)
			rc.usedBytes -= current.sizeBytes
			expiredCount++
		}

		current = prev
	}

	if expiredCount > 0 {
		rc.Logger.Info("Cache cleanup completed",
			"expired_entries", expiredCount,
			"remaining_entries", len(rc.items),
			"used_bytes", rc.usedBytes)
	}
}

// evictToCapacity removes LRU entries until under capacity
func (rc *ResponseCache) evictToCapacity() {
	evictedCount := 0

	for rc.usedBytes > rc.maxBytes && rc.tail != nil {
		evictNode := rc.tail
		rc.Logger.Debug("Evicting LRU entry",
			"key", evictNode.key,
			"size", evictNode.sizeBytes,
			"used_bytes", rc.usedBytes,
			"max_bytes", rc.maxBytes)

		rc.detachNode(evictNode)
		delete(rc.items, evictNode.key)
		rc.usedBytes -= evictNode.sizeBytes
		evictedCount++
	}

	if evictedCount > 0 {
		rc.Logger.Info("Cache eviction completed",
			"evicted_entries", evictedCount,
			"used_bytes", rc.usedBytes,
			"max_bytes", rc.maxBytes)
	}
}

// insertAtHead adds a node at the head (MRU position)
func (rc *ResponseCache) insertAtHead(node *Node) {
	node.prev = nil
	node.next = rc.head

	if rc.head != nil {
		rc.head.prev = node
	}

	rc.head = node

	if rc.tail == nil {
		rc.tail = node
	}
}

// moveToHead moves an existing node to the head position
func (rc *ResponseCache) moveToHead(node *Node) {
	if node == rc.head {
		return // Already at head
	}

	rc.detachNode(node)
	rc.insertAtHead(node)
}

// detachNode removes a node from the doubly linked list
func (rc *ResponseCache) detachNode(node *Node) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		// This is the head node
		rc.head = node.next
	}

	if node.next != nil {
		node.next.prev = node.prev
	} else {
		// This is the tail node
		rc.tail = node.prev
	}
}

// approximateSize calculates the approximate memory usage of a cache entry
func (rc *ResponseCache) approximateSize(key string, value []byte) int {
	// Approximate size: key length + value length + overhead for node structure
	const nodeOverhead = 64 // Approximate overhead for pointers, timestamps, etc.
	return len(key) + len(value) + nodeOverhead
}

// GetStats returns cache statistics for monitoring
func (rc *ResponseCache) GetStats() map[string]interface{} {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return map[string]interface{}{
		"entries":    len(rc.items),
		"used_bytes": rc.usedBytes,
		"max_bytes":  rc.maxBytes,
		"hit_ratio":  rc.calculateHitRatio(),
	}
}

// calculateHitRatio calculates cache hit ratio (simplified implementation)
func (rc *ResponseCache) calculateHitRatio() float64 {
	if len(rc.items) == 0 {
		return 0.0
	}
	return float64(len(rc.items)) / float64(rc.maxBytes/1024) // Rough approximation
}
