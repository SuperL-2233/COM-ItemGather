package app

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"com-itemgather/internal/security"
)

func TestInvitationAuthenticationAndDevices(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	admin := newAdminClient(t, environment)
	anonymous := newTestClient(t, environment.server)
	status, data := anonymous.request(t, "GET", "/api/ideas", nil)
	assertStatus(t, status, 401, data)
	status, data = anonymous.request(t, "POST", "/api/auth/register", map[string]any{"token": "invalid", "username": "outsider", "nickname": "外部用户", "password": "correct-horse-42"})
	assertStatus(t, status, 400, data)
	status, data = admin.request(t, "POST", "/api/admin/invitations", map[string]any{})
	assertStatus(t, status, 201, data)
	invitation := decodeTestJSON[map[string]any](t, data)
	parsed, _ := url.Parse(invitation["url"].(string))
	token := parsed.Query().Get("token")
	if token == "" {
		t.Fatal("missing invitation token")
	}
	var storedHash string
	environment.db.QueryRow("SELECT token_hash FROM invitations WHERE id=?", invitation["id"]).Scan(&storedHash)
	if storedHash == token || storedHash != security.TokenHash(token) {
		t.Fatal("invitation token was not hashed")
	}
	member := newTestClient(t, environment.server)
	registration := map[string]any{"token": token, "username": "member", "nickname": "成员", "password": "correct-horse-42", "remember": true}
	status, data = member.request(t, "POST", "/api/auth/register", registration)
	assertStatus(t, status, 201, data)
	member.refreshCSRF(t)
	status, data = member.request(t, "GET", "/api/auth/me", nil)
	assertStatus(t, status, 200, data)
	second := newTestClient(t, environment.server)
	status, data = second.request(t, "POST", "/api/auth/register", map[string]any{"token": token, "username": "second", "nickname": "第二人", "password": "correct-horse-42"})
	assertStatus(t, status, 400, data)
	status, data = admin.request(t, "POST", "/api/admin/invitations", map[string]any{})
	assertStatus(t, status, 201, data)
	expired := decodeTestJSON[map[string]any](t, data)
	environment.db.Exec("UPDATE invitations SET expires_at=? WHERE id=?", time.Now().Add(-time.Hour).Format(time.RFC3339Nano), expired["id"])
	expiredURL, _ := url.Parse(expired["url"].(string))
	status, data = anonymous.request(t, "POST", "/api/auth/register", map[string]any{"token": expiredURL.Query().Get("token"), "username": "expired", "nickname": "过期", "password": "correct-horse-42"})
	assertStatus(t, status, 400, data)
	status, data = member.request(t, "POST", "/api/admin/invitations", map[string]any{})
	assertStatus(t, status, 403, data)
	device2 := newTestClient(t, environment.server)
	device2.login(t, "member", "correct-horse-42")
	status, data = member.request(t, "GET", "/api/sessions", nil)
	assertStatus(t, status, 200, data)
	sessions := decodeTestJSON[map[string][]Session](t, data)["items"]
	if len(sessions) != 2 {
		t.Fatalf("expected two device sessions, got %d", len(sessions))
	}
	var revoked string
	for _, session := range sessions {
		if !session.Current {
			revoked = session.ID
		}
	}
	status, data = member.request(t, "DELETE", "/api/sessions/"+revoked, nil)
	assertStatus(t, status, 200, data)
	status, data = device2.request(t, "GET", "/api/auth/me", nil)
	assertStatus(t, status, 401, data)
	status, data = member.request(t, "GET", "/api/auth/me", nil)
	assertStatus(t, status, 200, data)
	if !strings.Contains(storedHash, "=") {
		t.Log("token hashes use URL-safe encoding")
	}
}
func TestCSRFRequired(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	client.csrf = "wrong"
	status, data := client.request(t, "POST", "/api/ideas", map[string]string{"content": "test"})
	assertStatus(t, status, 403, data)
	if testError(t, data) != "csrf" {
		t.Fatal("expected csrf error")
	}
}
func TestPasswordReset(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	admin := newAdminClient(t, environment)
	createTestUser(t, environment.db, "member", "member")
	status, data := admin.request(t, "POST", "/api/admin/password-resets", map[string]string{"user_id": func() string {
		var id string
		environment.db.QueryRow("SELECT id FROM users WHERE username='member'").Scan(&id)
		return id
	}()})
	assertStatus(t, status, 201, data)
	raw := decodeTestJSON[map[string]any](t, data)["url"].(string)
	parsed, _ := url.Parse(raw)
	client := newTestClient(t, environment.server)
	status, data = client.request(t, "POST", "/api/auth/reset-password", map[string]string{"token": parsed.Query().Get("token"), "password": "new-password-42"})
	assertStatus(t, status, 200, data)
	client.login(t, "member", "new-password-42")
	status, data = client.request(t, "GET", "/api/auth/me", nil)
	assertStatus(t, status, 200, data)
	_ = http.MethodGet
}
