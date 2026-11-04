package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"com-itemgather/internal/security"
)

type oauthClientMetadata struct {
	ClientID                          string   `json:"client_id,omitempty"`
	ClientName                        string   `json:"client_name"`
	RedirectURIs                      []string `json:"redirect_uris"`
	TokenEndpointAuthMethod           string   `json:"token_endpoint_auth_method,omitempty"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	GrantTypes                        []string `json:"grant_types,omitempty"`
	ResponseTypes                     []string `json:"response_types,omitempty"`
}

func validMCPRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && isLoopbackHost(u.Hostname()))
}

func isLoopbackHost(host string) bool {
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func validateMCPRedirect(registered, redirect string) error {
	if !validMCPRedirect(redirect) {
		return errors.New("invalid redirect URI")
	}
	var allowed []string
	if err := json.Unmarshal([]byte(registered), &allowed); err != nil {
		return err
	}
	actual, _ := url.Parse(redirect)
	for _, raw := range allowed {
		if raw == redirect {
			return nil
		}
		expected, err := url.Parse(raw)
		// RFC 8252 permits ephemeral ports only for literal loopback callbacks.
		if err == nil && expected.Scheme == "http" && actual.Scheme == "http" && isLoopbackHost(expected.Hostname()) &&
			expected.Hostname() == actual.Hostname() && expected.EscapedPath() == actual.EscapedPath() && expected.RawQuery == actual.RawQuery {
			return nil
		}
	}
	return errors.New("redirect URI is not registered")
}

func validateMCPClient(item *oauthClientMetadata) error {
	item.ClientName = strings.TrimSpace(item.ClientName)
	if len([]rune(item.ClientName)) > 100 || len(item.RedirectURIs) < 1 || len(item.RedirectURIs) > 10 {
		return errors.New("client name or redirect URIs are invalid")
	}
	if item.TokenEndpointAuthMethod != "" && item.TokenEndpointAuthMethod != "none" {
		return errors.New("only public clients with PKCE are supported")
	}
	if len(item.TokenEndpointAuthMethodsSupported) > 0 && !slices.Contains(item.TokenEndpointAuthMethodsSupported, "none") {
		return errors.New("client must support public-client token exchange")
	}
	for _, value := range item.GrantTypes {
		if value != "authorization_code" && value != "refresh_token" {
			return errors.New("unsupported grant type")
		}
	}
	for _, value := range item.ResponseTypes {
		if value != "code" {
			return errors.New("only authorization codes are supported")
		}
	}
	for _, raw := range item.RedirectURIs {
		if !validMCPRedirect(raw) {
			return errors.New("redirect URIs must use HTTPS or HTTP with a literal loopback address")
		}
	}
	if item.ClientName == "" {
		u, _ := url.Parse(item.RedirectURIs[0])
		item.ClientName = u.Hostname()
	}
	return nil
}

func (a *App) oauthClient(ctx context.Context, id string) (oauthClientMetadata, error) {
	var item oauthClientMetadata
	if id == "" || len(id) > 2048 {
		return item, errors.New("invalid client ID")
	}
	var redirects, metadataAt string
	err := a.oauthDB(ctx).QueryRowContext(ctx, "SELECT name,redirect_uris,metadata_at FROM mcp_oauth_clients WHERE id=?", id).Scan(&item.ClientName, &redirects, &metadataAt)
	if err == nil {
		if err := json.Unmarshal([]byte(redirects), &item.RedirectURIs); err != nil {
			return item, err
		}
		item.ClientID = id
		at, _ := time.Parse(time.RFC3339Nano, metadataAt)
		if !strings.HasPrefix(id, "https://") || time.Since(at) < time.Hour {
			return item, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return item, err
	}
	u, err := url.Parse(id)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path == "" || u.User != nil || u.Fragment != "" {
		return item, errors.New("unknown OAuth client")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, id, nil)
	if err != nil {
		return item, err
	}
	req.Header.Set("Accept", "application/json")
	response, err := a.oauthHTTP.Do(req)
	if err != nil {
		return item, fmt.Errorf("client metadata unavailable: %w", err)
	}
	defer response.Body.Close()
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != 200 || mediaType != "application/json" {
		return item, errors.New("client metadata unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 || json.Unmarshal(data, &item) != nil || item.ClientID != id {
		return item, errors.New("invalid client metadata document")
	}
	if err := validateMCPClient(&item); err != nil {
		return item, err
	}
	_, err = a.oauthDB(ctx).ExecContext(ctx, `INSERT INTO mcp_oauth_clients(id,name,redirect_uris,metadata_at,created_at) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name,redirect_uris=excluded.redirect_uris,metadata_at=excluded.metadata_at`, id, item.ClientName, encodeJSON(item.RedirectURIs), now(), now())
	return item, err
}

// Resolve and validate at dial time so a metadata URL cannot reach private
// services through either redirects or a DNS rebinding between check and dial.
func newMCPMetadataClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
				if err != nil || len(addresses) == 0 {
					return nil, errors.New("cannot resolve client metadata host")
				}
				for _, ip := range addresses {
					if !publicMCPAddress(ip.Unmap()) {
						return nil, errors.New("client metadata must use a public address")
					}
				}
				dialer := net.Dialer{Timeout: 4 * time.Second}
				return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
			}},
	}
}

func publicMCPAddress(ip netip.Addr) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(block).Contains(ip) {
			return false
		}
	}
	return true
}

func (a *App) registerMCPClient(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	var item oauthClientMetadata
	decoder := json.NewDecoder(r.Body)
	if decoder.Decode(&item) != nil || decoder.Decode(new(any)) != io.EOF {
		oauthError(w, "invalid_client_metadata", "客户端资料格式无效")
		return
	}
	if err := validateMCPClient(&item); err != nil {
		oauthError(w, "invalid_client_metadata", err.Error())
		return
	}
	id, err := security.RandomToken(24)
	if err != nil {
		writeError(w, err)
		return
	}
	item.ClientID = "mcp_" + id
	_, err = a.db.ExecContext(r.Context(), "INSERT INTO mcp_oauth_clients(id,name,redirect_uris,metadata_at,created_at) VALUES(?,?,?,?,?)", item.ClientID, item.ClientName, encodeJSON(item.RedirectURIs), now(), now())
	if err != nil {
		writeError(w, err)
		return
	}
	item.TokenEndpointAuthMethod = "none"
	item.GrantTypes = []string{"authorization_code", "refresh_token"}
	item.ResponseTypes = []string{"code"}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, item)
}
