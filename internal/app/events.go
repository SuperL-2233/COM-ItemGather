package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Hub struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

func NewHub() *Hub { return &Hub{subscribers: make(map[chan struct{}]struct{})} }
func (h *Hub) Subscribe() (chan struct{}, func()) {
	channel := make(chan struct{}, 1)
	h.mu.Lock()
	h.subscribers[channel] = struct{}{}
	h.mu.Unlock()
	return channel, func() { h.mu.Lock(); delete(h.subscribers, channel); h.mu.Unlock() }
}
func (h *Hub) Notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for channel := range h.subscribers {
		select {
		case channel <- struct{}{}:
		default:
		}
	}
}
func writeEvent(w http.ResponseWriter, event map[string]any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %v\nevent: change\ndata: %s\n\n", event["id"], data)
	return err
}
func (a *App) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, fail(500, "sse_unsupported", "当前服务不支持实时事件"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	notifications, unsubscribe := a.hub.Subscribe()
	defer unsubscribe()
	lastID := int64(0)
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		lastID, _ = strconv.ParseInt(raw, 10, 64)
	} else {
		var latest sql.NullInt64
		a.db.QueryRowContext(r.Context(), "SELECT MAX(id) FROM events").Scan(&latest)
		if latest.Valid {
			lastID = latest.Int64
		}
	}
	sendPending := func() error {
		if !a.eventSessionActive(r) {
			return fail(401, "unauthenticated", "登录已失效")
		}
		rows, err := a.db.QueryContext(r.Context(), "SELECT id,event_type,entity_type,entity_id,created_at FROM events WHERE id>? ORDER BY id LIMIT 500", lastID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			event := map[string]any{}
			var id int64
			var eventType, entityType, entityID, created string
			if err := rows.Scan(&id, &eventType, &entityType, &entityID, &created); err != nil {
				return err
			}
			event["id"] = id
			event["type"] = eventType
			event["entity_type"] = entityType
			event["entity_id"] = entityID
			event["created_at"] = created
			if err := writeEvent(w, event); err != nil {
				return err
			}
			lastID = id
			flusher.Flush()
		}
		return rows.Err()
	}
	if err := sendPending(); err != nil {
		return
	}
	flusher.Flush()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-a.runContext.Done():
			return
		case <-notifications:
			if sendPending() != nil {
				return
			}
		case <-heartbeat.C:
			if !a.eventSessionActive(r) {
				return
			}
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (a *App) eventSessionActive(r *http.Request) bool {
	var active int
	err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.revoked_at IS NULL AND s.expires_at>? AND u.status='active'`, currentSession(r), now()).Scan(&active)
	return err == nil && active == 1
}
