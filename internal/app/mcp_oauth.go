package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"com-itemgather/internal/security"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/server"
)

func (a *App) mcpProtectedMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, 200, map[string]any{"resource": a.mcpResource(), "resource_name": "COM 创意库（只读）",
		"authorization_servers": []string{a.oauthIssuer()}, "scopes_supported": []string{mcpReadScope}, "bearer_methods_supported": []string{"header"}})
}

func (a *App) mcpAuthorizationMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	base := a.oauthIssuer() + "/api/mcp/oauth"
	writeJSON(w, 200, map[string]any{"issuer": a.oauthIssuer(), "authorization_endpoint": base + "/authorize", "token_endpoint": base + "/token",
		"registration_endpoint": base + "/register", "revocation_endpoint": base + "/revoke", "scopes_supported": []string{mcpReadScope},
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported": []string{"none"}, "revocation_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported": []string{"S256"}, "client_id_metadata_document_supported": true,
		"authorization_response_iss_parameter_supported": true})
}

func (a *App) validateMCPAuthorization(r *http.Request) (*server.AuthorizeRequest, oauthClientMetadata, error) {
	var client oauthClientMetadata
	values := r.URL.Query()
	for _, items := range values {
		if len(items) != 1 {
			return nil, client, errors.New("授权参数不能重复")
		}
	}
	if values.Get("resource") != a.mcpResource() || (values.Get("scope") != "" && values.Get("scope") != mcpReadScope) || len(values.Get("state")) > 2048 {
		return nil, client, errors.New("请求的资源或权限范围无效")
	}
	request, err := a.oauth.ValidationAuthorizeRequest(r)
	if err != nil {
		return nil, client, errors.New("需要授权码流程与 S256 PKCE")
	}
	challenge, err := base64.RawURLEncoding.DecodeString(request.CodeChallenge)
	if err != nil || len(challenge) != 32 || base64.RawURLEncoding.EncodeToString(challenge) != request.CodeChallenge {
		return nil, client, errors.New("PKCE challenge 无效")
	}
	client, err = a.oauthClient(r.Context(), request.ClientID)
	if err != nil {
		return nil, client, errors.New("无法验证 OAuth 客户端资料")
	}
	if err := validateMCPRedirect(encodeJSON(client.RedirectURIs), request.RedirectURI); err != nil {
		return nil, client, errors.New("回调地址未注册")
	}
	request.Scope = mcpReadScope
	return request, client, nil
}

func (a *App) authorizeMCP(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.RawQuery) > 8192 {
		oauthError(w, "invalid_request", "授权请求过长")
		return
	}
	if _, _, err := a.validateMCPAuthorization(r); err != nil {
		// An unvalidated callback must never receive a redirect, even an error.
		oauthError(w, "invalid_request", err.Error())
		return
	}
	raw, err := security.RandomToken(32)
	if err != nil {
		writeError(w, err)
		return
	}
	_, err = a.db.ExecContext(r.Context(), "DELETE FROM mcp_oauth_requests WHERE expires_at<? OR consumed_at IS NOT NULL", now())
	if err == nil {
		_, err = a.db.ExecContext(r.Context(), "INSERT INTO mcp_oauth_requests(token_hash,parameters,expires_at) VALUES(?,?,?)", tokenHash(raw), r.URL.RawQuery, time.Now().UTC().Add(10*time.Minute).Format(time.RFC3339Nano))
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, "/mcp/authorize?request="+url.QueryEscape(raw), http.StatusSeeOther)
}

func (a *App) pendingMCPAuthorization(r *http.Request) (*server.AuthorizeRequest, oauthClientMetadata, error) {
	var parameters string
	if len(r.PathValue("request")) > 128 {
		return nil, oauthClientMetadata{}, fail(404, "not_found", "授权请求不存在或已过期，请从客户端重新连接")
	}
	err := a.oauthDB(r.Context()).QueryRowContext(r.Context(), `SELECT parameters FROM mcp_oauth_requests WHERE token_hash=? AND expires_at>? AND consumed_at IS NULL`, tokenHash(r.PathValue("request")), now()).Scan(&parameters)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = fail(404, "not_found", "授权请求不存在或已过期，请从客户端重新连接")
		}
		return nil, oauthClientMetadata{}, err
	}
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, a.oauthIssuer()+"/api/mcp/oauth/authorize?"+parameters, nil)
	return a.validateMCPAuthorization(request)
}

func (a *App) showMCPAuthorization(w http.ResponseWriter, r *http.Request) {
	request, client, err := a.pendingMCPAuthorization(r)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"client_name": client.ClientName, "client_id": request.ClientID, "redirect_uri": request.RedirectURI, "scope": mcpReadScope})
}

func (a *App) decideMCPAuthorization(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Allow *bool `json:"allow"`
	}
	if err := decodeBody(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	if input.Allow == nil {
		writeError(w, fail(422, "validation", "请选择是否授权"))
		return
	}
	// Resolve CIMD before opening the transaction; consent revalidates the
	// registered redirect and the actual login, rather than trusting the page.
	if _, _, err := a.pendingMCPAuthorization(r); err != nil {
		writeError(w, err)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	ctx := context.WithValue(r.Context(), oauthDBKey, tx)
	request, client, err := a.pendingMCPAuthorization(r.WithContext(ctx))
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := tx.ExecContext(ctx, "UPDATE mcp_oauth_requests SET consumed_at=? WHERE token_hash=? AND consumed_at IS NULL", now(), tokenHash(r.PathValue("request")))
	if err != nil {
		writeError(w, err)
		return
	}
	if count, _ := result.RowsAffected(); count != 1 {
		writeError(w, fail(409, "already_used", "授权请求已处理"))
		return
	}
	data := map[string]any{"iss": a.oauthIssuer()}
	if !*input.Allow {
		data["error"] = "access_denied"
	} else {
		request.UserID = security.ID()
		_, err = tx.ExecContext(ctx, `INSERT INTO mcp_oauth_grants(id,user_id,client_id,client_name,resource,scope,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?)`,
			request.UserID, currentUser(r).ID, request.ClientID, client.ClientName, a.mcpResource(), mcpReadScope, now(), time.Now().UTC().Add(mcpGrantTTL).Format(time.RFC3339Nano))
		if err != nil {
			writeError(w, err)
			return
		}
		token, err := a.oauth.GetAuthorizeToken(ctx, request)
		if err != nil {
			writeError(w, err)
			return
		}
		data["code"] = token.GetCode()
	}
	redirect, err := a.oauth.GetRedirectURI(request, data)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"redirect_uri": redirect})
}

func parseMCPTokenForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") || r.URL.RawQuery != "" || r.ParseForm() != nil {
		oauthError(w, "invalid_request", "请使用 form-urlencoded 请求正文")
		return false
	}
	for _, values := range r.PostForm {
		if len(values) != 1 {
			oauthError(w, "invalid_request", "请求参数不能重复")
			return false
		}
	}
	if r.PostForm.Get("client_secret") != "" || r.Header.Get("Authorization") != "" {
		oauthError(w, "invalid_client", "客户端应使用 PKCE 公共客户端认证")
		return false
	}
	return true
}

func (a *App) exchangeMCPToken(w http.ResponseWriter, r *http.Request) {
	if !parseMCPTokenForm(w, r) {
		return
	}
	resource, scope := r.PostForm.Get("resource"), r.PostForm.Get("scope")
	if resource != a.mcpResource() && !(resource == "" && r.PostForm.Get("grant_type") == "refresh_token") {
		oauthError(w, "invalid_target", "令牌仅可用于本站创意库 MCP")
		return
	}
	if scope != "" && scope != mcpReadScope {
		oauthError(w, "invalid_scope", "只支持创意库只读权限")
		return
	}
	// All store operations for an exchange use one transaction. Code consumption
	// and refresh rotation commit together, including across service restarts.
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	ctx := context.WithValue(r.Context(), oauthDBKey, tx)
	r = r.WithContext(ctx)
	grantType, request, err := a.oauth.ValidationTokenRequest(r)
	if err != nil || !a.oauth.CheckGrantType(grantType) {
		oauthError(w, "invalid_request", "需要有效的授权码或刷新令牌")
		return
	}
	if grantType == oauth2.AuthorizationCode {
		verifier := request.CodeVerifier
		if len(verifier) < 43 || len(verifier) > 128 || strings.IndexFunc(verifier, func(c rune) bool {
			return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c))
		}) >= 0 {
			oauthError(w, "invalid_request", "PKCE verifier 格式无效")
			return
		}
	}
	if grantType == oauth2.Refreshing {
		var clientID, used, grantID string
		err = tx.QueryRowContext(ctx, `SELECT g.client_id,COALESCE(t.refresh_used_at,''),g.id FROM mcp_oauth_tokens t JOIN mcp_oauth_grants g ON g.id=t.grant_id WHERE t.refresh_hash=?`, tokenHash(request.Refresh)).Scan(&clientID, &used, &grantID)
		if err != nil || clientID != request.ClientID {
			oauthError(w, "invalid_grant", "刷新令牌无效")
			return
		}
		if used != "" {
			// Reuse of a rotated credential revokes its entire grant family.
			_, err = tx.ExecContext(ctx, "UPDATE mcp_oauth_grants SET revoked_at=? WHERE id=? AND revoked_at IS NULL", now(), grantID)
			if err == nil {
				err = tx.Commit()
			}
			if err != nil {
				writeError(w, err)
				return
			}
			oauthError(w, "invalid_grant", "刷新令牌已使用，请重新授权")
			return
		}
	}
	token, err := a.oauth.GetAccessToken(ctx, grantType, request)
	if err != nil {
		oauthError(w, "invalid_grant", "凭据无效、已过期，或 PKCE 校验失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, 200, a.oauth.GetTokenData(token))
}

type MCPGrant struct {
	ID         string `json:"id"`
	ClientName string `json:"client_name"`
	ClientID   string `json:"client_id"`
	Scope      string `json:"scope"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	RevokedAt  string `json:"revoked_at,omitempty"`
}

func (a *App) listMCPGrants(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,client_name,client_id,scope,created_at,expires_at,COALESCE(last_used_at,''),COALESCE(revoked_at,'')
		FROM mcp_oauth_grants WHERE user_id=? AND ((revoked_at IS NULL AND expires_at>?) OR id IN
		(SELECT id FROM mcp_oauth_grants WHERE user_id=? ORDER BY created_at DESC,id DESC LIMIT 20)) ORDER BY created_at DESC,id DESC`, currentUser(r).ID, now(), currentUser(r).ID)
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	items := []MCPGrant{}
	for rows.Next() {
		var item MCPGrant
		if err := rows.Scan(&item.ID, &item.ClientName, &item.ClientID, &item.Scope, &item.CreatedAt, &item.ExpiresAt, &item.LastUsedAt, &item.RevokedAt); err != nil {
			writeError(w, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "server_url": a.mcpResource()})
}

func (a *App) revokeMCPGrant(w http.ResponseWriter, r *http.Request) {
	result, err := a.db.ExecContext(r.Context(), "UPDATE mcp_oauth_grants SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at IS NULL", now(), r.PathValue("id"), currentUser(r).ID)
	if err != nil {
		writeError(w, err)
		return
	}
	if count, _ := result.RowsAffected(); count != 1 {
		writeError(w, fail(404, "not_found", "连接不存在或已撤销"))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) revokeMCPToken(w http.ResponseWriter, r *http.Request) {
	if !parseMCPTokenForm(w, r) {
		return
	}
	_, err := a.db.ExecContext(r.Context(), `UPDATE mcp_oauth_grants SET revoked_at=? WHERE client_id=? AND id IN
		(SELECT grant_id FROM mcp_oauth_tokens WHERE access_hash=? OR refresh_hash=?) AND revoked_at IS NULL`, now(), r.PostForm.Get("client_id"), tokenHash(r.PostForm.Get("token")), tokenHash(r.PostForm.Get("token")))
	if err != nil {
		writeError(w, err)
		return
	}
	// RFC 7009 does not reveal whether a credential existed.
	w.WriteHeader(200)
}

func (a *App) requireMCPAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && origin != a.oauthIssuer() {
			writeError(w, fail(403, "invalid_origin", "不允许此来源访问 MCP"))
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			a.mcpAuthChallenge(w)
			return
		}
		token, err := a.oauth.Manager.LoadAccessToken(r.Context(), parts[1])
		if err != nil || token == nil || token.GetScope() != mcpReadScope {
			a.mcpAuthChallenge(w)
			return
		}
		if _, err := a.db.ExecContext(r.Context(), "UPDATE mcp_oauth_grants SET last_used_at=? WHERE id=?", now(), token.GetUserID()); err != nil {
			writeError(w, err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		next.ServeHTTP(w, r)
	})
}

func (a *App) mcpAuthChallenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+a.oauthIssuer()+`/.well-known/oauth-protected-resource/api/mcp", scope="`+mcpReadScope+`"`)
	writeError(w, fail(401, "mcp_unauthenticated", "请连接 MCP 并授予创意库只读权限"))
}
