package app

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type requestWindow struct {
	started time.Time
	count   int
}

type requestLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	requests  map[string]requestWindow
	lastSweep time.Time
}

func newRequestLimiter(limit int) *requestLimiter {
	return &requestLimiter{limit: limit, window: time.Minute, requests: map[string]requestWindow{}, lastSweep: time.Now()}
}

func (l *requestLimiter) allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Sub(l.lastSweep) >= l.window {
		for key, window := range l.requests {
			if now.Sub(window.started) >= l.window {
				delete(l.requests, key)
			}
		}
		l.lastSweep = now
	}
	window := l.requests[key]
	if now.Sub(window.started) >= l.window {
		window = requestWindow{started: now}
	}
	if window.count >= l.limit {
		retryAfter := int(window.started.Add(l.window).Sub(now).Seconds()) + 1
		return false, retryAfter
	}
	window.count++
	l.requests[key] = window
	return true, 0
}

func (a *App) rateLimit(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/events" || r.URL.Path == "/api/health" {
		return true
	}
	key := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		key = host
	}
	if cookie, err := r.Cookie("com_session"); err == nil && cookie.Value != "" {
		key = "session:" + tokenHash(cookie.Value)
	} else {
		key = "address:" + key
	}
	allowed, retryAfter := a.apiLimiter.allow(key)
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeError(w, fail(429, "rate_limited", "请求过于频繁，请稍后重试"))
	}
	return allowed
}
