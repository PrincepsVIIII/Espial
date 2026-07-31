package auth

import (
	"sync"
	"time"
)

type LoginLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]limitEntry
}

type limitEntry struct {
	started time.Time
	count   int
}

func NewLoginLimiter(limit int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{limit: limit, window: window, entries: make(map[string]limitEntry)}
}

func (limiter *LoginLimiter) Allow(key string, now time.Time) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	entry := limiter.entries[key]
	if entry.started.IsZero() || now.Sub(entry.started) >= limiter.window {
		entry = limitEntry{started: now}
	}
	if entry.count >= limiter.limit {
		return false
	}
	entry.count++
	limiter.entries[key] = entry

	if len(limiter.entries) > 1024 {
		for entryKey, candidate := range limiter.entries {
			if now.Sub(candidate.started) >= limiter.window {
				delete(limiter.entries, entryKey)
			}
		}
	}
	return true
}
