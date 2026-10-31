package app

import (
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"com-itemgather/internal/security"
)

func tokenHash(token string) string { return security.TokenHash(token) }

type loginAttempt struct {
	failures int
	resetAt  time.Time
}
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{attempts: make(map[string]loginAttempt)} }
func (l *loginLimiter) allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	item := l.attempts[key]
	if now.After(item.resetAt) {
		delete(l.attempts, key)
		return true
	}
	return item.failures < 5
}
func (l *loginLimiter) failed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	item := l.attempts[key]
	if time.Now().After(item.resetAt) {
		item = loginAttempt{resetAt: time.Now().Add(15 * time.Minute)}
	}
	item.failures++
	l.attempts[key] = item
}
func (l *loginLimiter) success(key string) { l.mu.Lock(); delete(l.attempts, key); l.mu.Unlock() }

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r, a.cfg.PublicURL) {
		writeError(w, fail(403, "origin", "请求来源无效"))
		return
	}
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		Remember   bool   `json:"remember"`
		DeviceName string `json:"device_name"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host == "" {
		host = r.RemoteAddr
	}
	key := host + "|" + strings.ToLower(clean(body.Username, 100))
	if !a.limiter.allowed(key) {
		writeError(w, fail(429, "rate_limited", "登录尝试过多，请稍后再试"))
		return
	}
	var user User
	var passwordHash string
	err := a.db.QueryRowContext(r.Context(), "SELECT id,username,nickname,password_hash,role,status,created_at FROM users WHERE username=? COLLATE NOCASE", clean(body.Username, 100)).Scan(&user.ID, &user.Username, &passwordHash, &user.Role, &user.Status, &user.CreatedAt)
	if err != nil || user.Status != "active" || !security.VerifyPassword(body.Password, passwordHash) {
		a.limiter.failed(key)
		writeError(w, fail(401, "invalid_credentials", "用户名或密码错误"))
		return
	}
	a.limiter.success(key)
	csrf, err := a.newSession(w, r, user, body.Remember, body.DeviceName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": user, "csrf_token": csrf})
}

func (a *App) createSession(w http.ResponseWriter, r *http.Request, user User, remember bool, deviceName string) error {
	_, err := a.newSession(w, r, user, remember, deviceName)
	return err
}

func (a *App) newSession(w http.ResponseWriter, r *http.Request, user User, remember bool, deviceName string) (string, error) {
	token, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	csrf, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	ttl := a.cfg.SessionTTL
	if remember {
		ttl = a.cfg.RememberTTL
	}
	timestamp := now()
	expires := time.Now().UTC().Add(ttl).Format(time.RFC3339Nano)
	if deviceName == "" {
		deviceName = clean(r.UserAgent(), 160)
	}
	if deviceName == "" {
		deviceName = "未知设备"
	}
	_, err = a.db.ExecContext(r.Context(), "INSERT INTO sessions(token_hash,user_id,csrf_token,device_name,remember,created_at,last_seen_at,expires_at) VALUES(?,?,?,?,?,?,?,?)", tokenHash(token), user.ID, csrf, clean(deviceName, 160), remember, timestamp, timestamp, expires)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{Name: "com_session", Value: token, Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookie, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(ttl), MaxAge: int(ttl.Seconds())})
	return csrf, nil
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	csrf := ""
	a.db.QueryRowContext(r.Context(), "SELECT csrf_token FROM sessions WHERE token_hash=?", currentSession(r)).Scan(&csrf)
	writeJSON(w, 200, map[string]any{"user": user, "csrf_token": csrf})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	a.db.ExecContext(r.Context(), "UPDATE sessions SET revoked_at=? WHERE token_hash=?", now(), currentSession(r))
	http.SetCookie(w, &http.Cookie{Name: "com_session", Value: "", Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) createInvitation(w http.ResponseWriter, r *http.Request) {
	if err := a.requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	token, err := security.RandomToken(32)
	if err != nil {
		writeError(w, err)
		return
	}
	id := security.ID()
	expires := time.Now().UTC().Add(24 * time.Hour)
	_, err = a.db.ExecContext(r.Context(), "INSERT INTO invitations(id,token_hash,created_by,expires_at,created_at) VALUES(?,?,?,?,?)", id, tokenHash(token), currentUser(r).ID, expires.Format(time.RFC3339Nano), now())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "url": a.cfg.PublicURL + "/register?token=" + token, "expires_at": expires.Format(time.RFC3339Nano)})
}
func (a *App) invitationInfo(w http.ResponseWriter, r *http.Request) {
	var expires string
	var used sql.NullString
	err := a.db.QueryRowContext(r.Context(), "SELECT expires_at,used_at FROM invitations WHERE token_hash=?", tokenHash(r.PathValue("token"))).Scan(&expires, &used)
	parsed, _ := time.Parse(time.RFC3339Nano, expires)
	if err != nil || used.Valid || !parsed.After(time.Now()) {
		writeError(w, fail(404, "invalid_invitation", "邀请链接无效或已过期"))
		return
	}
	writeJSON(w, 200, map[string]any{"valid": true, "expires_at": expires})
}
func (a *App) listInvitations(w http.ResponseWriter, r *http.Request) {
	if err := a.requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT i.id,i.expires_at,COALESCE(i.used_at,''),COALESCE(u.nickname,''),i.created_at FROM invitations i LEFT JOIN users u ON u.id=i.used_by ORDER BY i.created_at DESC`)
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, expires, used, usedBy, created string
		rows.Scan(&id, &expires, &used, &usedBy, &created)
		items = append(items, map[string]any{"id": id, "expires_at": expires, "used_at": used, "used_by": usedBy, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (a *App) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	if err := a.requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	_, err := a.db.ExecContext(r.Context(), "UPDATE invitations SET used_at=? WHERE id=? AND used_at IS NULL", now(), r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"ok": err == nil})
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r, a.cfg.PublicURL) {
		writeError(w, fail(403, "origin", "请求来源无效"))
		return
	}
	var body struct {
		Token      string `json:"token"`
		Username   string `json:"username"`
		Nickname   string `json:"nickname"`
		Password   string `json:"password"`
		Remember   bool   `json:"remember"`
		DeviceName string `json:"device_name"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Username = clean(body.Username, 50)
	body.Nickname = clean(body.Nickname, 80)
	if len([]rune(body.Username)) < 2 || !security.ValidPassword(body.Password) || body.Nickname == "" {
		writeError(w, fail(422, "validation", "用户名至少 2 个字符，密码至少 10 个字符，昵称不能为空"))
		return
	}
	hash, err := security.HashPassword(body.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	user := User{ID: security.ID(), Username: body.Username, Nickname: body.Nickname, Role: "member", Status: "active", CreatedAt: now()}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	var inviteID, expires string
	var used sql.NullString
	err = tx.QueryRowContext(r.Context(), "SELECT id,expires_at,used_at FROM invitations WHERE token_hash=?", tokenHash(body.Token)).Scan(&inviteID, &expires, &used)
	parsed, _ := time.Parse(time.RFC3339Nano, expires)
	if err != nil || used.Valid || !parsed.After(time.Now()) {
		writeError(w, fail(400, "invalid_invitation", "邀请链接无效、已过期或已使用"))
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO users(id,username,nickname,password_hash,role,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", user.ID, user.Username, user.Nickname, hash, user.Role, user.CreatedAt, user.CreatedAt); err != nil {
		writeError(w, fail(409, "username_taken", "用户名已被使用"))
		return
	}
	result, err := tx.ExecContext(r.Context(), "UPDATE invitations SET used_at=?,used_by=? WHERE id=? AND used_at IS NULL", now(), user.ID, inviteID)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(409, "invalid_invitation", "邀请已被使用"))
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	if err = a.createSession(w, r, user, body.Remember, body.DeviceName); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"user": user})
}

func (a *App) createPasswordReset(w http.ResponseWriter, r *http.Request) {
	if err := a.requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	token, err := security.RandomToken(32)
	if err != nil {
		writeError(w, err)
		return
	}
	id := security.ID()
	expires := time.Now().UTC().Add(time.Hour)
	_, err = a.db.ExecContext(r.Context(), "INSERT INTO password_reset_tokens(id,token_hash,user_id,created_by,expires_at,created_at) VALUES(?,?,?,?,?,?)", id, tokenHash(token), body.UserID, currentUser(r).ID, expires.Format(time.RFC3339Nano), now())
	if err != nil {
		writeError(w, fail(400, "invalid_user", "用户不存在"))
		return
	}
	writeJSON(w, 201, map[string]any{"url": a.cfg.PublicURL + "/reset-password?token=" + token, "expires_at": expires})
}
func (a *App) resetPassword(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r, a.cfg.PublicURL) {
		writeError(w, fail(403, "origin", "请求来源无效"))
		return
	}
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if !security.ValidPassword(body.Password) {
		writeError(w, fail(422, "validation", "密码至少需要 10 个字符"))
		return
	}
	hash, err := security.HashPassword(body.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	var id, userID, expires string
	var used sql.NullString
	err = tx.QueryRowContext(r.Context(), "SELECT id,user_id,expires_at,used_at FROM password_reset_tokens WHERE token_hash=?", tokenHash(body.Token)).Scan(&id, &userID, &expires, &used)
	parsed, _ := time.Parse(time.RFC3339Nano, expires)
	if err != nil || used.Valid || !parsed.After(time.Now()) {
		writeError(w, fail(400, "invalid_reset", "重置链接无效、已过期或已使用"))
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE users SET password_hash=?,updated_at=? WHERE id=?", hash, now(), userID); err != nil {
		writeError(w, err)
		return
	}
	tx.ExecContext(r.Context(), "UPDATE password_reset_tokens SET used_at=? WHERE id=?", now(), id)
	tx.ExecContext(r.Context(), "UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL", now(), userID)
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) updateProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	nickname := clean(body.Nickname, 80)
	if nickname == "" {
		writeError(w, fail(422, "validation", "昵称不能为空"))
		return
	}
	user := currentUser(r)
	_, err := a.db.ExecContext(r.Context(), "UPDATE users SET nickname=?,updated_at=? WHERE id=?", nickname, now(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	user.Nickname = nickname
	writeJSON(w, 200, map[string]any{"user": user})
}
func (a *App) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		RevokeOthers    bool   `json:"revoke_others"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if !security.ValidPassword(body.NewPassword) {
		writeError(w, fail(422, "validation", "新密码至少需要 10 个字符"))
		return
	}
	user := currentUser(r)
	var hash string
	a.db.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id=?", user.ID).Scan(&hash)
	if !security.VerifyPassword(body.CurrentPassword, hash) {
		writeError(w, fail(400, "invalid_password", "当前密码错误"))
		return
	}
	hash, err := security.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, err)
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	tx.ExecContext(r.Context(), "UPDATE users SET password_hash=?,updated_at=? WHERE id=?", hash, now(), user.ID)
	if body.RevokeOthers {
		tx.ExecContext(r.Context(), "UPDATE sessions SET revoked_at=? WHERE user_id=? AND token_hash<>? AND revoked_at IS NULL", now(), user.ID, currentSession(r))
	}
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) listSessions(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), "SELECT token_hash,device_name,remember,created_at,last_seen_at,expires_at FROM sessions WHERE user_id=? AND revoked_at IS NULL AND expires_at>? ORDER BY last_seen_at DESC", currentUser(r).ID, now())
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	items := []Session{}
	for rows.Next() {
		var item Session
		var hash string
		rows.Scan(&hash, &item.DeviceName, &item.Remember, &item.CreatedAt, &item.LastSeenAt, &item.ExpiresAt)
		item.ID = hash
		item.Current = hash == currentSession(r)
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (a *App) revokeSession(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	result, err := a.db.ExecContext(r.Context(), "UPDATE sessions SET revoked_at=? WHERE token_hash=? AND user_id=?", now(), r.PathValue("hash"), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeError(w, fail(404, "not_found", "设备会话不存在"))
		return
	}
	if r.PathValue("hash") == currentSession(r) {
		http.SetCookie(w, &http.Cookie{Name: "com_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.cfg.SecureCookie, SameSite: http.SameSiteLaxMode})
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	a.db.ExecContext(r.Context(), "UPDATE sessions SET revoked_at=? WHERE user_id=? AND token_hash<>? AND revoked_at IS NULL", now(), currentUser(r).ID, currentSession(r))
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) listUsers(w http.ResponseWriter, r *http.Request) {
	if err := a.requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,username,nickname,role,status,created_at FROM users ORDER BY created_at")
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	items := []User{}
	for rows.Next() {
		var user User
		rows.Scan(&user.ID, &user.Username, &user.Nickname, &user.Role, &user.Status, &user.CreatedAt)
		items = append(items, user)
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (a *App) updateUser(w http.ResponseWriter, r *http.Request) {
	if err := a.requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Status != "active" && body.Status != "disabled" {
		writeError(w, fail(422, "validation", "账号状态无效"))
		return
	}
	if r.PathValue("id") == currentUser(r).ID && body.Status == "disabled" {
		writeError(w, fail(400, "self_disable", "不能停用自己的账号"))
		return
	}
	_, err := a.db.ExecContext(r.Context(), "UPDATE users SET status=?,updated_at=? WHERE id=?", body.Status, now(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if body.Status == "disabled" {
		a.db.ExecContext(r.Context(), "UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL", now(), r.PathValue("id"))
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) systemStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ai": map[string]any{"configured": a.cfg.AIAPIKey != "", "base_url": a.cfg.AIBaseURL, "model": a.cfg.AIModel, "timeout_seconds": int(a.cfg.AITimeout.Seconds()), "max_retries": a.cfg.AIMaxRetries, "workers": a.cfg.AIWorkers}, "backup": map[string]any{"directory": a.cfg.BackupDir, "interval": a.cfg.BackupInterval.String(), "keep": a.cfg.BackupKeep}, "upload": map[string]any{"max_file_bytes": a.cfg.MaxUploadBytes, "max_attachments": a.cfg.MaxAttachments, "max_storage_bytes": a.cfg.MaxStorageBytes}})
}
func ensureNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fail(404, "not_found", "记录不存在")
	}
	return err
}
