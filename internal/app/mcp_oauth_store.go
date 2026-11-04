package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"com-itemgather/internal/security"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
	"github.com/go-oauth2/oauth2/v4/server"
)

const mcpReadScope = "creative-library:read"
const mcpAccessTTL = 15 * time.Minute
const mcpGrantTTL = 30 * 24 * time.Hour
const oauthDBKey contextKey = "mcp_oauth_db"

type oauthDatabase interface {
	querier
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (a *App) oauthDB(ctx context.Context) oauthDatabase {
	if q, ok := ctx.Value(oauthDBKey).(oauthDatabase); ok {
		return q
	}
	return a.db
}

// UserID in the OAuth library identifies a grant, not an account. The grant
// binds every code/access/refresh token to its user, client, resource and scope.
type mcpTokenStore struct{ app *App }

func (s mcpTokenStore) Create(ctx context.Context, info oauth2.TokenInfo) error {
	raw, err := json.Marshal(info)
	if err != nil {
		return err
	}
	var saved models.Token
	if err := json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	// Only hashes are persisted, including in the library's serialized model.
	saved.Code, saved.Access, saved.Refresh = "", "", ""
	_, err = s.app.oauthDB(ctx).ExecContext(ctx, `INSERT INTO mcp_oauth_tokens(id,grant_id,code_hash,access_hash,refresh_hash,data) VALUES(?,?,?,?,?,?)`,
		security.ID(), info.GetUserID(), nullableTokenHash(info.GetCode()), nullableTokenHash(info.GetAccess()), nullableTokenHash(info.GetRefresh()), encodeJSON(saved))
	return err
}

func nullableTokenHash(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return nullString(tokenHash(value))
}

func (s mcpTokenStore) get(ctx context.Context, column, unused, value string) (oauth2.TokenInfo, error) {
	if value == "" || len(value) > 512 {
		return nil, nil
	}
	var data, resource string
	err := s.app.oauthDB(ctx).QueryRowContext(ctx, `SELECT t.data,g.resource FROM mcp_oauth_tokens t
		JOIN mcp_oauth_grants g ON g.id=t.grant_id JOIN users u ON u.id=g.user_id
		WHERE t.`+column+`=? AND t.`+unused+` IS NULL AND g.revoked_at IS NULL AND g.expires_at>? AND u.status='active'`, tokenHash(value), now()).Scan(&data, &resource)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if resource != s.app.mcpResource() {
		return nil, nil
	}
	var item models.Token
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		return nil, err
	}
	// Restore only the credential supplied by this caller. Old access tokens
	// retain their original 15-minute lifetime when a refresh rotates.
	switch column {
	case "code_hash":
		item.Code = value
	case "access_hash":
		item.Access = value
	case "refresh_hash":
		item.Refresh = value
	}
	return &item, nil
}

func (s mcpTokenStore) GetByCode(ctx context.Context, value string) (oauth2.TokenInfo, error) {
	return s.get(ctx, "code_hash", "code_used_at", value)
}
func (s mcpTokenStore) GetByAccess(ctx context.Context, value string) (oauth2.TokenInfo, error) {
	return s.get(ctx, "access_hash", "access_revoked_at", value)
}
func (s mcpTokenStore) GetByRefresh(ctx context.Context, value string) (oauth2.TokenInfo, error) {
	return s.get(ctx, "refresh_hash", "refresh_used_at", value)
}
func (s mcpTokenStore) consume(ctx context.Context, column, used, value string) error {
	result, err := s.app.oauthDB(ctx).ExecContext(ctx, `UPDATE mcp_oauth_tokens SET `+used+`=? WHERE `+column+`=? AND `+used+` IS NULL`, now(), tokenHash(value))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return errors.New("OAuth credential already consumed")
	}
	return err
}
func (s mcpTokenStore) RemoveByCode(ctx context.Context, value string) error {
	return s.consume(ctx, "code_hash", "code_used_at", value)
}
func (s mcpTokenStore) RemoveByAccess(ctx context.Context, value string) error {
	return s.consume(ctx, "access_hash", "access_revoked_at", value)
}
func (s mcpTokenStore) RemoveByRefresh(ctx context.Context, value string) error {
	return s.consume(ctx, "refresh_hash", "refresh_used_at", value)
}

type mcpClientStore struct{ app *App }

func (s mcpClientStore) GetByID(ctx context.Context, id string) (oauth2.ClientInfo, error) {
	item, err := s.app.oauthClient(ctx, id)
	if err != nil {
		return nil, err
	}
	return &models.Client{ID: id, Domain: encodeJSON(item.RedirectURIs), Public: true}, nil
}

type mcpCodeGenerator struct{}

func (mcpCodeGenerator) Token(context.Context, *oauth2.GenerateBasic) (string, error) {
	return security.RandomToken(32)
}

type mcpAccessGenerator struct{}

func (mcpAccessGenerator) Token(_ context.Context, _ *oauth2.GenerateBasic, refresh bool) (string, string, error) {
	access, err := security.RandomToken(32)
	if err != nil || !refresh {
		return access, "", err
	}
	raw, err := security.RandomToken(32)
	return access, raw, err
}

func (a *App) newOAuthServer() *server.Server {
	m := manage.NewDefaultManager()
	m.MapClientStorage(mcpClientStore{a})
	m.MapTokenStorage(mcpTokenStore{a})
	m.MapAuthorizeGenerate(mcpCodeGenerator{})
	m.MapAccessGenerate(mcpAccessGenerator{})
	m.SetValidateURIHandler(validateMCPRedirect)
	m.SetAuthorizeCodeExp(5 * time.Minute)
	m.SetAuthorizeCodeTokenCfg(&manage.Config{AccessTokenExp: mcpAccessTTL, RefreshTokenExp: mcpGrantTTL, IsGenerateRefresh: true})
	m.SetRefreshTokenCfg(&manage.RefreshingConfig{AccessTokenExp: mcpAccessTTL, IsGenerateRefresh: true, IsRemoveRefreshing: true})
	s := server.NewServer(&server.Config{TokenType: "Bearer", AllowedResponseTypes: []oauth2.ResponseType{oauth2.Code},
		AllowedGrantTypes:           []oauth2.GrantType{oauth2.AuthorizationCode, oauth2.Refreshing},
		AllowedCodeChallengeMethods: []oauth2.CodeChallengeMethod{oauth2.CodeChallengeS256}, ForcePKCE: true}, m)
	s.ClientInfoHandler = server.ClientFormHandler
	s.RefreshingScopeHandler = func(request *oauth2.TokenGenerateRequest, previous string) (bool, error) {
		return request.Scope == mcpReadScope && previous == mcpReadScope, nil
	}
	return s
}

func (a *App) mcpResource() string { return strings.TrimRight(a.cfg.PublicURL, "/") + "/api/mcp" }
func (a *App) oauthIssuer() string { return strings.TrimRight(a.cfg.PublicURL, "/") }

func oauthError(w http.ResponseWriter, code, description string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": code, "error_description": description})
}
