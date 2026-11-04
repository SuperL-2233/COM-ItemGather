package app

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func proxyMCPGrant(t *testing.T, env *testEnvironment, owner *testClient) testMCPCredentials {
	t.Helper()
	env.app.cfg.PublicURL = "https://ideas.example"
	clientID := registerTestMCPClient(t, owner)
	pending, verifier := beginTestMCPAuthorization(t, env, clientID)
	status, data := owner.request(t, "POST", "/api/mcp/oauth/requests/"+pending, map[string]bool{"allow": true})
	assertStatus(t, status, 200, data)
	callback, err := url.Parse(decodeTestJSON[map[string]string](t, data)["redirect_uri"])
	if err != nil || callback.Query().Get("iss") != env.app.oauthIssuer() {
		t.Fatalf("invalid public OAuth issuer: %s %v", data, err)
	}
	status, data = testMCPTokenForm(t, env, url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID},
		"code": {callback.Query().Get("code")}, "code_verifier": {verifier}, "redirect_uri": {"http://127.0.0.1:8888/callback"}, "resource": {env.app.mcpResource()}})
	assertStatus(t, status, 200, data)
	return decodeTestJSON[testMCPCredentials](t, data)
}

func proxyMCPRequest(t *testing.T, env *testEnvironment, token, host, origin, version string, message any) (int, []byte) {
	t.Helper()
	r, err := http.NewRequest("POST", env.server.URL+"/api/mcp", strings.NewReader(encodeJSON(message)))
	if err != nil {
		t.Fatal(err)
	}
	r.Host = host
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if version != "" {
		r.Header.Set("MCP-Protocol-Version", version)
	}
	response, err := env.server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

func TestMCPDiscoveryBehindReverseProxy(t *testing.T) {
	env := newTestEnvironment(t, nil)
	credentials := proxyMCPGrant(t, env, newAdminClient(t, env))
	for _, version := range []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			// The socket is loopback, while Nginx forwards the configured public
			// Host. Authentication succeeding must not be followed by an SDK 403.
			status, data := proxyMCPRequest(t, env, credentials.AccessToken, "ideas.example", "", "", map[string]any{
				"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
					"protocolVersion": version, "clientInfo": map[string]string{"name": "openai-mcp", "version": "1.0.0"}, "capabilities": map[string]any{}}})
			assertStatus(t, status, 200, data)
			initialized := decodeTestJSON[struct{ Result mcp.InitializeResult }](t, data)
			if initialized.Result.ProtocolVersion != version || initialized.Result.Capabilities.Tools == nil {
				t.Fatalf("tool capability negotiation failed: %s", data)
			}
			status, data = proxyMCPRequest(t, env, credentials.AccessToken, "ideas.example", "https://chatgpt.com", version,
				map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
			assertStatus(t, status, 202, data)
			status, data = proxyMCPRequest(t, env, credentials.AccessToken, "ideas.example", "https://chatgpt.com", version,
				map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}})
			assertStatus(t, status, 200, data)
			tools := decodeTestJSON[struct{ Result mcp.ListToolsResult }](t, data)
			if len(tools.Result.Tools) != 4 {
				t.Fatalf("discovery did not return the four read-only tools: %s", data)
			}
			for _, tool := range tools.Result.Tools {
				if tool.Title == "" || tool.OutputSchema == nil || tool.Meta["securitySchemes"] == nil {
					t.Fatalf("ChatGPT tool descriptor is incomplete: %s", encodeJSON(tool))
				}
				schemes := decodeTestJSON[[]struct {
					Type   string   `json:"type"`
					Scopes []string `json:"scopes"`
				}](t, []byte(encodeJSON(tool.Meta["securitySchemes"])))
				if len(schemes) != 1 || schemes[0].Type != "oauth2" || len(schemes[0].Scopes) != 1 || schemes[0].Scopes[0] != mcpReadScope {
					t.Fatalf("tool must advertise only read-scoped OAuth: %s", encodeJSON(tool))
				}
			}
		})
	}
	message := map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/list", "params": map[string]any{}}
	for _, test := range []struct {
		host, origin, token string
		status              int
	}{
		{"untrusted.example", "", credentials.AccessToken, 403},
		{"ideas.example.attacker.example", "", credentials.AccessToken, 403},
		{"ideas.example", "https://untrusted.example", credentials.AccessToken, 403},
		{"ideas.example", "https://chatgpt.com.attacker.example", credentials.AccessToken, 403},
		{"ideas.example", "https://chatgpt.com", "", 401},
		{"ideas.example", "https://ideas.example", credentials.AccessToken, 200},
	} {
		status, data := proxyMCPRequest(t, env, test.token, test.host, test.origin, "2025-11-25", message)
		assertStatus(t, status, test.status, data)
	}
}

type proxyMCPTransport struct{ token string }

func (transport proxyMCPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = "ideas.example"
	r.Header.Set("Origin", "https://chatgpt.com")
	r.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestMCPModernSDKDiscoveryBehindReverseProxy(t *testing.T) {
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	item := catalogItem(t, owner, "products", "反代发现商品", map[string]any{"description": "反向代理下读取的完整商品描述"})
	credentials := proxyMCPGrant(t, env, owner)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "proxy-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: env.server.URL + "/api/mcp",
		HTTPClient: &http.Client{Transport: proxyMCPTransport{credentials.AccessToken}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatalf("modern discovery failed behind the public Host: %v %v", tools, err)
	}
	for _, call := range []mcp.CallToolParams{
		{Name: "get_library_info", Arguments: map[string]any{}},
		{Name: "list_records", Arguments: map[string]any{}},
		{Name: "search", Arguments: map[string]any{"query": "反代发现商品"}},
		{Name: "fetch", Arguments: map[string]any{"id": "product:" + item.ID}},
	} {
		result, err := session.CallTool(ctx, &call)
		if err != nil || result.IsError {
			t.Fatalf("%s must match its advertised output schema: %v %v", call.Name, result, err)
		}
		structured := testMCPData[map[string]any](t, result)
		if len(result.Content) != 1 {
			t.Fatalf("%s must include a JSON text fallback", call.Name)
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok || encodeJSON(decodeTestJSON[map[string]any](t, []byte(content.Text))) != encodeJSON(structured) {
			t.Fatalf("%s JSON content and structuredContent differ", call.Name)
		}
		if call.Name == "fetch" {
			if !strings.Contains(structured["text"].(string), "反向代理下读取的完整商品描述") || structured["url"] != "https://ideas.example/products/"+item.ID {
				t.Fatal("fetch must provide the full document text and canonical citation URL")
			}
		}
	}
}
