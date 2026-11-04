package app

import (
	"net/http"
	"strings"
	"time"

	"com-itemgather/internal/security"
)

const agentTokenTTL = 15 * time.Minute

type AgentToken struct {
	ID         string `json:"id"`
	Token      string `json:"token,omitempty"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	RevokedAt  string `json:"revoked_at,omitempty"`
}

func (a *App) createAgentToken(w http.ResponseWriter, r *http.Request) {
	raw, err := security.RandomToken(32)
	if err != nil {
		writeError(w, err)
		return
	}
	created := time.Now().UTC()
	item := AgentToken{ID: security.ID(), Token: raw, CreatedAt: created.Format(time.RFC3339Nano), ExpiresAt: created.Add(agentTokenTTL).Format(time.RFC3339Nano)}
	_, err = a.db.ExecContext(r.Context(), "INSERT INTO agent_tokens(id,token_hash,created_by,created_at,expires_at) VALUES(?,?,?,?,?)", item.ID, tokenHash(raw), currentUser(r).ID, item.CreatedAt, item.ExpiresAt)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, item)
}

func (a *App) listAgentTokens(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,created_at,expires_at,COALESCE(last_used_at,''),COALESCE(revoked_at,'') FROM agent_tokens WHERE created_by=? ORDER BY created_at DESC LIMIT 20", currentUser(r).ID)
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	items := []AgentToken{}
	for rows.Next() {
		var item AgentToken
		if err = rows.Scan(&item.ID, &item.CreatedAt, &item.ExpiresAt, &item.LastUsedAt, &item.RevokedAt); err != nil {
			writeError(w, err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *App) revokeAgentToken(w http.ResponseWriter, r *http.Request) {
	result, err := a.db.ExecContext(r.Context(), "UPDATE agent_tokens SET revoked_at=? WHERE id=? AND created_by=? AND revoked_at IS NULL", now(), r.PathValue("id"), currentUser(r).ID)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(404, "not_found", "AI 只读令牌不存在或已撤销"))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) requireAgentAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			writeError(w, fail(401, "agent_unauthenticated", "AI 只读令牌无效或已过期"))
			return
		}
		var id, expires, revoked, status string
		err := a.db.QueryRowContext(r.Context(), `SELECT t.id,t.expires_at,COALESCE(t.revoked_at,''),u.status FROM agent_tokens t JOIN users u ON u.id=t.created_by WHERE t.token_hash=?`, tokenHash(parts[1])).Scan(&id, &expires, &revoked, &status)
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, expires)
		if err != nil || parseErr != nil || revoked != "" || status != "active" || !expiresAt.After(time.Now()) {
			writeError(w, fail(401, "agent_unauthenticated", "AI 只读令牌无效或已过期"))
			return
		}
		a.db.ExecContext(r.Context(), "UPDATE agent_tokens SET last_used_at=? WHERE id=?", now(), id)
		next(w, r)
	}
}
