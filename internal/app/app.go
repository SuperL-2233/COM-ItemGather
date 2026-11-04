package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"com-itemgather/internal/config"
)

type contextKey string

const userKey contextKey = "user"
const sessionKey contextKey = "session"

type App struct {
	db         *sql.DB
	cfg        config.Config
	log        *log.Logger
	mux        *http.ServeMux
	hub        *Hub
	ai         *AIWorker
	limiter    *loginLimiter
	apiLimiter *requestLimiter
	runContext context.Context
}

type APIError struct {
	Status  int
	Code    string
	Message string
	Details any
}

func (e *APIError) Error() string { return e.Message }
func fail(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

func New(db *sql.DB, cfg config.Config, logger *log.Logger) *App {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	a := &App{db: db, cfg: cfg, log: logger, mux: http.NewServeMux(), hub: NewHub(), limiter: newLoginLimiter(), apiLimiter: newRequestLimiter(300), runContext: context.Background()}
	a.routes()
	a.ai = NewAIWorker(a)
	return a
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	if !a.rateLimit(w, r) {
		return
	}
	a.mux.ServeHTTP(w, r)
}

func (a *App) Start(ctx context.Context) error { a.runContext = ctx; return a.ai.Start(ctx) }
func (a *App) DB() *sql.DB                     { return a.db }

func (a *App) routes() {
	handle := func(pattern string, handler http.HandlerFunc, auth bool) {
		if auth {
			handler = a.requireAuth(handler)
		}
		a.mux.HandleFunc(pattern, handler)
	}
	handle("GET /api/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]any{"status": "ok"}) }, false)
	handle("POST /api/auth/login", a.login, false)
	handle("POST /api/auth/register", a.register, false)
	handle("POST /api/auth/reset-password", a.resetPassword, false)
	handle("GET /api/auth/invitations/{token}", a.invitationInfo, false)
	handle("GET /api/auth/me", a.me, true)
	handle("POST /api/auth/logout", a.logout, true)
	handle("PATCH /api/auth/profile", a.updateProfile, true)
	handle("POST /api/auth/change-password", a.changePassword, true)
	handle("GET /api/sessions", a.listSessions, true)
	handle("DELETE /api/sessions/{hash}", a.revokeSession, true)
	handle("POST /api/sessions/revoke-others", a.revokeOtherSessions, true)
	handle("GET /api/admin/users", a.listUsers, true)
	handle("PATCH /api/admin/users/{id}", a.updateUser, true)
	handle("POST /api/admin/invitations", a.createInvitation, true)
	handle("GET /api/admin/invitations", a.listInvitations, true)
	handle("DELETE /api/admin/invitations/{id}", a.revokeInvitation, true)
	handle("POST /api/admin/password-resets", a.createPasswordReset, true)
	handle("GET /api/system/status", a.systemStatus, true)
	handle("POST /api/system/backup", a.manualBackup, true)
	handle("POST /api/agent-tokens", a.createAgentToken, true)
	handle("GET /api/agent-tokens", a.listAgentTokens, true)
	handle("DELETE /api/agent-tokens/{id}", a.revokeAgentToken, true)
	handle("GET /api/agent/v1/catalog", a.requireAgentAuth(a.agentCatalog), false)
	handle("GET /api/ideas", a.listIdeas, true)
	handle("POST /api/ideas", a.createIdea, true)
	handle("GET /api/ideas/{id}", a.getIdea, true)
	handle("PATCH /api/ideas/{id}", a.updateIdea, true)
	handle("DELETE /api/ideas/{id}", a.deleteIdea, true)
	handle("POST /api/ideas/{id}/restore", a.restoreIdea, true)
	handle("DELETE /api/ideas/{id}/permanent", a.permanentDeleteIdea, true)
	handle("POST /api/ideas/{id}/favorite", a.favoriteIdea, true)
	handle("POST /api/ideas/{id}/ai", a.retryAI, true)
	handle("GET /api/ideas/{id}/revisions", a.listRevisions("idea"), true)
	handle("POST /api/ideas/{id}/revisions/{revision}/restore", a.restoreRevision("idea"), true)
	handle("GET /api/products", a.listProducts, true)
	handle("POST /api/products", a.createProduct, true)
	handle("GET /api/product-options/categories/usage", a.productCategoryUsage, true)
	handle("DELETE /api/product-options/categories", a.deleteProductCategory, true)
	handle("GET /api/product-options/statuses/usage", a.productStatusUsage, true)
	handle("DELETE /api/product-options/statuses", a.deleteProductStatus, true)
	handle("PATCH /api/product-options/{kind}/order", a.reorderProductOptions, true)
	handle("PATCH /api/product-options/statuses/{id}/color", a.updateProductStatusColor, true)
	handle("GET /api/product-options/{kind}", a.productOptions, true)
	handle("POST /api/product-options/{kind}", a.productOptions, true)
	handle("GET /api/products/filters", a.listProductFilters, true)
	handle("GET /api/products/{id}", a.getProduct, true)
	handle("PATCH /api/products/{id}", a.updateProduct, true)
	handle("DELETE /api/products/{id}", a.deleteProduct, true)
	handle("POST /api/products/{id}/restore", a.restoreProduct, true)
	handle("DELETE /api/products/{id}/permanent", a.permanentDeleteProduct, true)
	handle("GET /api/products/{id}/revisions", a.listRevisions("product"), true)
	handle("POST /api/products/{id}/revisions/{revision}/restore", a.restoreRevision("product"), true)
	for _, entry := range []struct{ path, kind string }{{"companies", "company"}, {"recipe-groups", "recipe_group"}, {"recipes", "recipe"}} {
		base := "/api/" + entry.path
		handle("DELETE "+base+"/{id}", a.catalogLifecycle(entry.kind, "delete"), true)
		handle("POST "+base+"/{id}/restore", a.catalogLifecycle(entry.kind, "restore"), true)
		handle("DELETE "+base+"/{id}/permanent", a.catalogLifecycle(entry.kind, "permanent"), true)
		handle("GET "+base+"/{id}/revisions", a.listRevisions(entry.kind), true)
		if entry.kind != "recipe_group" {
			handle("POST "+base+"/{id}/revisions/{revision}/restore", a.restoreCatalogRevision(entry.kind), true)
		}
	}
	handle("GET /api/companies", a.listCompanies, true)
	handle("POST /api/companies", a.saveCompany, true)
	handle("GET /api/companies/{id}", a.getCompany, true)
	handle("PATCH /api/companies/{id}", a.saveCompany, true)
	handle("POST /api/companies/{id}/merge", a.mergeCompany, true)
	handle("GET /api/tool-categories", a.toolCategories, true)
	handle("POST /api/tool-categories", a.toolCategories, true)
	handle("GET /api/tools", a.listProducts, true)
	handle("POST /api/tools", a.createProduct, true)
	handle("GET /api/tools/{id}", a.getProduct, true)
	handle("PATCH /api/tools/{id}", a.updateProduct, true)
	handle("DELETE /api/tools/{id}", a.deleteProduct, true)
	handle("POST /api/tools/{id}/restore", a.restoreProduct, true)
	handle("DELETE /api/tools/{id}/permanent", a.permanentDeleteProduct, true)
	handle("GET /api/tools/{id}/revisions", a.listRevisions("product"), true)
	handle("POST /api/tools/{id}/revisions/{revision}/restore", a.restoreRevision("product"), true)
	handle("GET /api/recipe-groups", a.listRecipeGroups, true)
	handle("POST /api/recipe-groups", a.createRecipeGroup, true)
	handle("GET /api/recipe-groups/{id}", a.getRecipeGroup, true)
	handle("POST /api/recipe-groups/{group}/recipes", a.saveRecipe, true)
	handle("PATCH /api/recipes/{id}", a.saveRecipe, true)
	handle("GET /api/setting-links", a.listSettingLinks, true)
	handle("POST /api/setting-links", a.createSettingLink, true)
	handle("PATCH /api/setting-links/{id}", a.updateSettingLink, true)
	handle("DELETE /api/setting-links/{id}", a.deleteSettingLink, true)
	handle("POST /api/links", a.createLink, true)
	handle("PATCH /api/links/{id}", a.updateLink, true)
	handle("DELETE /api/links/{id}", a.deleteLink, true)
	handle("GET /api/search", a.globalSearch, true)
	handle("GET /api/tags", a.listTags, true)
	handle("PATCH /api/tags/{id}", a.renameTag, true)
	handle("DELETE /api/tags/{id}", a.deleteTag, true)
	handle("GET /api/trash", a.listTrash, true)
	handle("GET /api/export", a.exportData, true)
	handle("POST /api/attachments", a.uploadAttachment, true)
	handle("GET /api/attachments/{id}", a.downloadAttachment, true)
	handle("DELETE /api/attachments/{id}", a.deleteAttachment, true)
	handle("GET /api/events", a.events, true)
	a.mux.HandleFunc("/", a.serveStatic)
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("com_session")
		if err != nil || cookie.Value == "" {
			writeError(w, fail(401, "unauthenticated", "请先登录"))
			return
		}
		hash := tokenHash(cookie.Value)
		var user User
		var sessionHash, csrf, expires, revoked, status string
		err = a.db.QueryRowContext(r.Context(), `SELECT s.token_hash,s.csrf_token,s.expires_at,COALESCE(s.revoked_at,''),u.id,u.username,u.nickname,u.role,u.status,u.created_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, hash).Scan(&sessionHash, &csrf, &expires, &revoked, &user.ID, &user.Username, &user.Nickname, &user.Role, &status, &user.CreatedAt)
		if err != nil || revoked != "" || status != "active" {
			writeError(w, fail(401, "unauthenticated", "登录已失效，请重新登录"))
			return
		}
		expiresAt, _ := time.Parse(time.RFC3339Nano, expires)
		if !expiresAt.After(time.Now()) {
			writeError(w, fail(401, "unauthenticated", "登录已过期，请重新登录"))
			return
		}
		if expected := r.Header.Get("X-User-ID"); expected != "" && expected != user.ID {
			writeError(w, fail(409, "account_changed", "登录账号已变化，请刷新页面后重试"))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if subtleCompare(r.Header.Get("X-CSRF-Token"), csrf) == false {
				writeError(w, fail(403, "csrf", "请求校验失败，请刷新页面后重试"))
				return
			}
		}
		user.Status = status
		a.db.ExecContext(r.Context(), "UPDATE sessions SET last_seen_at=? WHERE token_hash=?", now(), hash)
		ctx := context.WithValue(r.Context(), userKey, user)
		ctx = context.WithValue(ctx, sessionKey, sessionHash)
		next(w, r.WithContext(ctx))
	}
}

func currentUser(r *http.Request) User      { return r.Context().Value(userKey).(User) }
func currentSession(r *http.Request) string { return r.Context().Value(sessionKey).(string) }
func (a *App) requireAdmin(r *http.Request) *APIError {
	if currentUser(r).Role != "admin" {
		return fail(403, "forbidden", "需要管理员权限")
	}
	return nil
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) *APIError {
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fail(400, "invalid_json", "请求内容或字段格式不正确")
	}
	if decoder.More() {
		return fail(400, "invalid_json", "请求只能包含一个 JSON 对象")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	// Live search routinely cancels obsolete requests; the client has gone away.
	if errors.Is(err, context.Canceled) {
		return
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		log.Printf("internal API error: %v", err)
		apiErr = fail(500, "internal_error", "服务器处理失败")
	}
	body := map[string]any{"code": apiErr.Code, "message": apiErr.Message}
	if apiErr.Details != nil {
		body["details"] = apiErr.Details
	}
	writeJSON(w, apiErr.Status, map[string]any{"error": body})
}
func handleError(w http.ResponseWriter, err error) {
	if err != nil {
		writeError(w, err)
	}
}
func (a *App) publish(tx *sql.Tx, eventType, entityType, id string) error {
	_, err := tx.Exec("INSERT INTO events(event_type,entity_type,entity_id,created_at) VALUES(?,?,?,?)", eventType, entityType, id, now())
	return err
}
func (a *App) afterCommit() { a.hub.Notify() }
func (a *App) emit(eventType, entityType, id string) {
	if _, err := a.db.Exec("INSERT INTO events(event_type,entity_type,entity_id,created_at) VALUES(?,?,?,?)", eventType, entityType, id, now()); err == nil {
		a.hub.Notify()
	}
}
func (a *App) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		writeError(w, fail(405, "method_not_allowed", "不支持此方法"))
		return
	}
	root := a.cfg.WebDist
	path := filepath.Join(root, filepath.Clean(r.URL.Path))
	if r.URL.Path == "/" {
		path = filepath.Join(root, "index.html")
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		path = filepath.Join(root, "index.html")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "Frontend build not found. Run npm run build in web/.", 503)
		return
	}
	if strings.HasSuffix(path, "index.html") {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if strings.HasSuffix(path, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else if strings.HasSuffix(path, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else if strings.HasSuffix(path, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else if strings.HasSuffix(path, ".webmanifest") {
		w.Header().Set("Content-Type", "application/manifest+json")
	}
	http.ServeContent(w, r, filepath.Base(path), time.Time{}, strings.NewReader(string(data)))
}
func sameOrigin(r *http.Request, publicURL string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	public, _ := url.Parse(publicURL)
	return parsed.Host == r.Host || (public != nil && parsed.Host == public.Host)
}
func subtleCompare(a, b string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	result := byte(0)
	for i := range a {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
