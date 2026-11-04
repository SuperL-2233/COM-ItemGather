package app

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"com-itemgather/internal/security"
)

// Snapshot of https://chatgpt.com/oauth/client.json fetched on 2026-11-04.
// Keep both auth-method fields: the singular legacy preference is deliberately
// not a method supported by this server, while the plural list includes none.
func chatGPTClientFixture(t *testing.T) ([]byte, oauthClientMetadata) {
	t.Helper()
	raw, err := os.ReadFile("testdata/chatgpt_oauth_client.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw, decodeTestJSON[oauthClientMetadata](t, raw)
}

func TestMCPChatGPTCIMDAuthorization(t *testing.T) {
	raw, metadata := chatGPTClientFixture(t)
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	env.app.oauthHTTP = &http.Client{Transport: mcpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != metadata.ClientID || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("CIMD fetch must use the client document URL without account credentials")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
	})}
	verifier, err := security.RandomToken(48)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(verifier))
	parameters := url.Values{"response_type": {"code"}, "client_id": {metadata.ClientID}, "redirect_uri": {metadata.RedirectURIs[0]},
		"scope": {mcpReadScope}, "resource": {env.app.mcpResource()}, "state": {"chatgpt-cimd-state"}, "ui_locales": {"zh-CN"},
		"code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}}
	browser := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := browser.Get(env.server.URL + "/api/mcp/oauth/authorize?" + parameters.Encode())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	assertStatus(t, response.StatusCode, 303, body)
	consentURL, err := url.Parse(response.Header.Get("Location"))
	if err != nil || consentURL.Path != "/mcp/authorize" || consentURL.Query().Get("request") == "" {
		t.Fatal("ChatGPT must continue to the local consent page")
	}
	consentPath := "/api/mcp/oauth/requests/" + consentURL.Query().Get("request")
	status, data := owner.request(t, "GET", consentPath, nil)
	assertStatus(t, status, 200, data)
	details := decodeTestJSON[map[string]string](t, data)
	if details["client_name"] != "ChatGPT" || details["redirect_uri"] != metadata.RedirectURIs[0] || details["scope"] != mcpReadScope {
		t.Fatal("consent must identify ChatGPT and the read-only scope")
	}
	status, data = owner.request(t, "POST", consentPath, map[string]bool{"allow": true})
	assertStatus(t, status, 200, data)
	decision := decodeTestJSON[map[string]string](t, data)
	callback, err := url.Parse(decision["redirect_uri"])
	if err != nil {
		t.Fatal(err)
	}
	if callback.Scheme+"://"+callback.Host+callback.Path != metadata.RedirectURIs[0] || callback.Query().Get("state") != "chatgpt-cimd-state" || callback.Query().Get("iss") != env.server.URL || callback.Query().Get("code") == "" {
		t.Fatal("authorization must return the code, state and issuer to ChatGPT's registered callback")
	}
	// Redeem with none, as negotiated from the intersection of the server and
	// CIMD supported-method lists; no client secret or signed assertion is used.
	status, data = testMCPTokenForm(t, env, url.Values{"grant_type": {"authorization_code"}, "client_id": {metadata.ClientID},
		"code": {callback.Query().Get("code")}, "redirect_uri": {metadata.RedirectURIs[0]}, "code_verifier": {verifier}, "resource": {env.app.mcpResource()}})
	assertStatus(t, status, 200, data)
	credentials := decodeTestJSON[testMCPCredentials](t, data)
	if credentials.AccessToken == "" || credentials.RefreshToken == "" || credentials.Scope != mcpReadScope {
		t.Fatal("ChatGPT must receive only read-scoped access and refresh credentials")
	}
	testMCPData[map[string]any](t, testMCPCall(t, env, credentials.AccessToken, "get_library_info", map[string]any{}))
	status, data = testMCPTokenForm(t, env, url.Values{"grant_type": {"refresh_token"}, "client_id": {metadata.ClientID}, "refresh_token": {credentials.RefreshToken}, "resource": {env.app.mcpResource()}})
	assertStatus(t, status, 200, data)
	refreshed := decodeTestJSON[testMCPCredentials](t, data)
	if refreshed.AccessToken == "" || refreshed.RefreshToken == credentials.RefreshToken || refreshed.Scope != mcpReadScope {
		t.Fatal("CIMD refresh must rotate credentials and preserve the read-only scope")
	}
	status, data = owner.request(t, "GET", "/api/mcp/connections", nil)
	assertStatus(t, status, 200, data)
	connections := decodeTestJSON[struct {
		Items []MCPGrant `json:"items"`
	}](t, data)
	if len(connections.Items) != 1 || connections.Items[0].ClientID != metadata.ClientID {
		t.Fatal("the connection must remain bound to ChatGPT's CIMD client ID")
	}
	status, data = owner.request(t, "DELETE", "/api/mcp/connections/"+connections.Items[0].ID, nil)
	assertStatus(t, status, 200, data)
	status, data, _ = testMCPRequest(t, env, refreshed.AccessToken, "tools/list", map[string]any{})
	assertStatus(t, status, 401, data)
}

func TestMCPCIMDAuthMethodCompatibility(t *testing.T) {
	for _, test := range []struct {
		name      string
		preferred string
		supported []string
		allowed   bool
	}{
		{"ChatGPT legacy preference with public-client support", "private_key_jwt", []string{"none", "private_key_jwt"}, true},
		{"method order has no preference", "private_key_jwt", []string{"private_key_jwt", "none"}, true},
		{"legacy public client", "none", nil, true},
		{"default public client", "", nil, true},
		{"legacy signed-only client", "private_key_jwt", nil, false},
		{"no common method", "private_key_jwt", []string{"private_key_jwt"}, false},
		{"legacy preference cannot override supported methods", "none", []string{"private_key_jwt"}, false},
		{"empty supported list", "none", []string{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, metadata := chatGPTClientFixture(t)
			metadata.TokenEndpointAuthMethod = test.preferred
			metadata.TokenEndpointAuthMethodsSupported = test.supported
			if err := validateMCPClient(&metadata); (err == nil) != test.allowed {
				t.Fatalf("allowed=%v, validation error=%v", test.allowed, err)
			}
		})
	}
}

func TestMCPDCRAuthMethodCannotBeOverriddenByCIMD(t *testing.T) {
	env := newTestEnvironment(t, nil)
	client := newTestClient(t, env.server)
	_, metadata := chatGPTClientFixture(t)
	status, data := client.request(t, "POST", "/api/mcp/oauth/register", metadata)
	assertStatus(t, status, 400, data)
	metadata.TokenEndpointAuthMethod = "none"
	status, data = client.request(t, "POST", "/api/mcp/oauth/register", metadata)
	assertStatus(t, status, 201, data)
	registered := decodeTestJSON[oauthClientMetadata](t, data)
	if registered.TokenEndpointAuthMethod != "none" || registered.TokenEndpointAuthMethodsSupported != nil {
		t.Fatal("DCR must register the selected public-client method")
	}
}
