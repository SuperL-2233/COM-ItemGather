package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"com-itemgather/internal/security"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testMCPCredentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	ClientID     string
	Code         string
}

func registerTestMCPClient(t *testing.T, owner *testClient) string {
	t.Helper()
	status, data := owner.request(t, "POST", "/api/mcp/oauth/register", map[string]any{
		"client_name": "测试 MCP 客户端", "redirect_uris": []string{"http://127.0.0.1:7777/callback"}, "token_endpoint_auth_method": "none",
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}})
	assertStatus(t, status, 201, data)
	return decodeTestJSON[oauthClientMetadata](t, data).ClientID
}

func beginTestMCPAuthorization(t *testing.T, env *testEnvironment, clientID string) (string, string) {
	t.Helper()
	verifier, err := security.RandomToken(48)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(verifier))
	parameters := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {"http://127.0.0.1:8888/callback"},
		"scope": {mcpReadScope}, "resource": {env.app.mcpResource()}, "state": {"test-state"}, "code_challenge_method": {"S256"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(env.server.URL + "/api/mcp/oauth/authorize?" + parameters.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 303 {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("authorize: %d %s", response.StatusCode, data)
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || location.Path != "/mcp/authorize" || location.Query().Get("request") == "" {
		t.Fatal("authorization must continue through the consent page")
	}
	return location.Query().Get("request"), verifier
}

func finishTestMCPAuthorization(t *testing.T, owner *testClient, pending string, allow bool) string {
	t.Helper()
	status, data := owner.request(t, "GET", "/api/mcp/oauth/requests/"+pending, nil)
	assertStatus(t, status, 200, data)
	status, data = owner.request(t, "POST", "/api/mcp/oauth/requests/"+pending, map[string]bool{"allow": allow})
	assertStatus(t, status, 200, data)
	result := decodeTestJSON[map[string]string](t, data)
	redirect, err := url.Parse(result["redirect_uri"])
	if err != nil || redirect.Host != "127.0.0.1:8888" || redirect.Query().Get("state") != "test-state" || redirect.Query().Get("iss") != owner.base {
		t.Fatalf("invalid callback: %s", data)
	}
	if !allow && redirect.Query().Get("error") != "access_denied" {
		t.Fatal("denying consent must return access_denied")
	}
	return redirect.Query().Get("code")
}

func testMCPTokenForm(t *testing.T, env *testEnvironment, values url.Values) (int, []byte) {
	t.Helper()
	response, err := env.server.Client().PostForm(env.server.URL+"/api/mcp/oauth/token", values)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("OAuth responses must not be cached")
	}
	return response.StatusCode, data
}

func newTestMCPGrant(t *testing.T, env *testEnvironment, owner *testClient) testMCPCredentials {
	t.Helper()
	id := registerTestMCPClient(t, owner)
	pending, verifier := beginTestMCPAuthorization(t, env, id)
	code := finishTestMCPAuthorization(t, owner, pending, true)
	status, data := testMCPTokenForm(t, env, url.Values{"grant_type": {"authorization_code"}, "client_id": {id}, "code": {code},
		"redirect_uri": {"http://127.0.0.1:8888/callback"}, "code_verifier": {verifier}, "resource": {env.app.mcpResource()}})
	assertStatus(t, status, 200, data)
	credentials := decodeTestJSON[testMCPCredentials](t, data)
	credentials.ClientID, credentials.Code = id, code
	if credentials.AccessToken == "" || credentials.RefreshToken == "" || credentials.Scope != mcpReadScope || credentials.ExpiresIn != 900 {
		t.Fatalf("invalid token response: %s", data)
	}
	return credentials
}

func testMCPRequest(t *testing.T, env *testEnvironment, token, method string, params any) (int, []byte, http.Header) {
	t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
	r, _ := http.NewRequest("POST", env.server.URL+"/api/mcp", strings.NewReader(encodeJSON(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-11-25")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := env.server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data, response.Header
}

func testMCPCall(t *testing.T, env *testEnvironment, token, name string, arguments any) *mcp.CallToolResult {
	t.Helper()
	status, data, _ := testMCPRequest(t, env, token, "tools/call", map[string]any{"name": name, "arguments": arguments})
	assertStatus(t, status, 200, data)
	var result struct {
		Result *mcp.CallToolResult `json:"result"`
		Error  json.RawMessage     `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.Result == nil || len(result.Error) > 0 {
		t.Fatalf("MCP call failed: %s", data)
	}
	return result.Result
}

func testMCPData[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	if result.IsError {
		t.Fatalf("tool failed: %s", encodeJSON(result))
	}
	return decodeTestJSON[T](t, []byte(encodeJSON(result.StructuredContent)))
}

type testMCPTransport struct{ token string }

func (transport testMCPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestMCPOAuthAndSDKCompatibility(t *testing.T) {
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	status, data, headers := testMCPRequest(t, env, "", "tools/list", map[string]any{})
	assertStatus(t, status, 401, data)
	if !strings.Contains(headers.Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource/api/mcp") {
		t.Fatal("401 must advertise protected resource metadata")
	}
	for _, path := range []string{"/.well-known/oauth-protected-resource/api/mcp", "/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server"} {
		status, data = owner.request(t, "GET", path, nil)
		assertStatus(t, status, 200, data)
	}
	credentials := newTestMCPGrant(t, env, owner)
	status, data, _ = testMCPRequest(t, env, credentials.AccessToken, "initialize", map[string]any{"protocolVersion": "2025-11-25", "clientInfo": map[string]string{"name": "legacy", "version": "1"}, "capabilities": map[string]any{}})
	assertStatus(t, status, 200, data)
	if !bytes.Contains(data, []byte(`"protocolVersion":"2025-11-25"`)) {
		t.Fatalf("legacy negotiation failed: %s", data)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "integration-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: env.app.mcpResource(), HTTPClient: &http.Client{Transport: testMCPTransport{credentials.AccessToken}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.InitializeResult().ProtocolVersion != "2026-07-28" {
		t.Fatalf("current SDK should select 2026-07-28: %s", session.InitializeResult().ProtocolVersion)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatalf("tool discovery: %v %v", tools, err)
	}
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatal("every tool must declare read-only, closed-world behavior")
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"fetch", "get_library_info", "list_records", "search"}) {
		t.Fatal(names)
	}
	info, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_library_info", Arguments: map[string]any{}})
	if err != nil || info.IsError {
		t.Fatalf("SDK tool call failed: %v %v", info, err)
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "write_record", Arguments: map[string]any{}}); err == nil {
		t.Fatal("no write tool should be callable")
	}
	for _, arguments := range []map[string]any{{"limit": 51}, {"limit": 0}, {"kinds": []string{"account"}}, {"sql": "SELECT * FROM users"}} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_records", Arguments: arguments})
		if err == nil && !result.IsError {
			t.Fatalf("invalid arguments accepted: %v", arguments)
		}
	}
	// No plaintext credentials may survive in SQLite, including serialized data.
	rows, err := env.db.Query("SELECT COALESCE(code_hash,''),COALESCE(access_hash,''),COALESCE(refresh_hash,''),data FROM mcp_oauth_tokens")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var code, access, refresh, serialized string
		if err := rows.Scan(&code, &access, &refresh, &serialized); err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{credentials.Code, credentials.AccessToken, credentials.RefreshToken} {
			if strings.Contains(code+access+refresh+serialized, raw) {
				t.Fatal("plaintext credential persisted")
			}
		}
	}
	rows.Close()
	for _, path := range []string{"/api/export", "/api/admin/users", "/api/ideas"} {
		r, _ := http.NewRequest("GET", env.server.URL+path, nil)
		r.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		response, err := env.server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatalf("MCP token must not authenticate ordinary API: %s", path)
		}
	}
}

func TestMCPRefreshRevocationAndOwnership(t *testing.T) {
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	credentials := newTestMCPGrant(t, env, owner)
	refresh := func(token, clientID, scope string) (int, []byte) {
		return testMCPTokenForm(t, env, url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {token}, "scope": {scope}, "resource": {env.app.mcpResource()}})
	}
	for _, input := range [][2]string{{"wrong-client", mcpReadScope}, {credentials.ClientID, "creative-library:write"}} {
		status, data := refresh(credentials.RefreshToken, input[0], input[1])
		assertStatus(t, status, 400, data)
	}
	status, data := refresh(credentials.RefreshToken, credentials.ClientID, mcpReadScope)
	assertStatus(t, status, 200, data)
	next := decodeTestJSON[testMCPCredentials](t, data)
	if next.RefreshToken == credentials.RefreshToken || next.AccessToken == credentials.AccessToken {
		t.Fatal("refresh must rotate both credentials")
	}
	status, data, _ = testMCPRequest(t, env, next.AccessToken, "tools/list", map[string]any{})
	assertStatus(t, status, 200, data)
	status, data = refresh(credentials.RefreshToken, credentials.ClientID, "")
	assertStatus(t, status, 400, data)
	status, data, _ = testMCPRequest(t, env, next.AccessToken, "tools/list", map[string]any{})
	assertStatus(t, status, 401, data)
	status, data = refresh(next.RefreshToken, credentials.ClientID, "")
	assertStatus(t, status, 400, data)
	credentials = newTestMCPGrant(t, env, owner)
	status, data = owner.request(t, "GET", "/api/mcp/connections", nil)
	assertStatus(t, status, 200, data)
	connections := decodeTestJSON[struct {
		Items []MCPGrant `json:"items"`
	}](t, data)
	grantID := connections.Items[0].ID
	other := createTestUser(t, env.db, "other-mcp-user", "member")
	outsider := newTestClient(t, env.server)
	outsider.login(t, other.Username, "correct-horse-42")
	status, data = outsider.request(t, "DELETE", "/api/mcp/connections/"+grantID, nil)
	assertStatus(t, status, 404, data)
	status, data = owner.request(t, "DELETE", "/api/mcp/connections/"+grantID, nil)
	assertStatus(t, status, 200, data)
	status, data, _ = testMCPRequest(t, env, credentials.AccessToken, "tools/list", map[string]any{})
	assertStatus(t, status, 401, data)
	status, data = refresh(credentials.RefreshToken, credentials.ClientID, "")
	assertStatus(t, status, 400, data)
}

func TestMCPOAuthConsentPKCEAndReplay(t *testing.T) {
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	id := registerTestMCPClient(t, owner)
	pending, verifier := beginTestMCPAuthorization(t, env, id)
	anonymous := newTestClient(t, env.server)
	status, data := anonymous.request(t, "POST", "/api/mcp/oauth/requests/"+pending, map[string]bool{"allow": true})
	assertStatus(t, status, 401, data)
	csrf := owner.csrf
	owner.csrf = "wrong"
	status, data = owner.request(t, "POST", "/api/mcp/oauth/requests/"+pending, map[string]bool{"allow": true})
	assertStatus(t, status, 403, data)
	owner.csrf = csrf
	code := finishTestMCPAuthorization(t, owner, pending, true)
	status, data = owner.request(t, "POST", "/api/mcp/oauth/requests/"+pending, map[string]bool{"allow": true})
	assertStatus(t, status, 404, data)
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {id}, "code": {code}, "redirect_uri": {"http://127.0.0.1:8888/callback"}, "resource": {env.app.mcpResource()}, "code_verifier": {strings.Repeat("x", 43)}}
	status, data = testMCPTokenForm(t, env, form)
	assertStatus(t, status, 400, data)
	form.Set("code_verifier", verifier)
	// Failed exchange rolls back consumption so the rightful client can retry.
	status, data = testMCPTokenForm(t, env, form)
	assertStatus(t, status, 200, data)
	status, data = testMCPTokenForm(t, env, form)
	assertStatus(t, status, 400, data)
	pending, _ = beginTestMCPAuthorization(t, env, id)
	if code := finishTestMCPAuthorization(t, owner, pending, false); code != "" {
		t.Fatal("denied consent must not issue a code")
	}
	pending, _ = beginTestMCPAuthorization(t, env, id)
	if _, err := env.db.Exec("UPDATE mcp_oauth_requests SET expires_at='2000-01-01' WHERE token_hash=?", tokenHash(pending)); err != nil {
		t.Fatal(err)
	}
	status, data = owner.request(t, "GET", "/api/mcp/oauth/requests/"+pending, nil)
	assertStatus(t, status, 404, data)
}

func TestMCPConcurrentCodeExchange(t *testing.T) {
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	id := registerTestMCPClient(t, owner)
	pending, verifier := beginTestMCPAuthorization(t, env, id)
	code := finishTestMCPAuthorization(t, owner, pending, true)
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {id}, "code": {code}, "redirect_uri": {"http://127.0.0.1:8888/callback"}, "resource": {env.app.mcpResource()}, "code_verifier": {verifier}}
	var wait sync.WaitGroup
	results := make(chan int, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response, err := env.server.Client().PostForm(env.server.URL+"/api/mcp/oauth/token", form)
			if err != nil {
				results <- 0
				return
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			results <- response.StatusCode
		}()
	}
	wait.Wait()
	close(results)
	statuses := []int{}
	for status := range results {
		statuses = append(statuses, status)
	}
	slices.Sort(statuses)
	if !slices.Equal(statuses, []int{200, 400}) {
		t.Fatalf("authorization code redeemed more than once: %v", statuses)
	}
}

func TestMCPRedirectAndMetadataSecurity(t *testing.T) {
	registered := encodeJSON([]string{"https://client.example/callback", "http://127.0.0.1:7777/callback"})
	for _, uri := range []string{"https://client.example/callback", "http://127.0.0.1:8888/callback"} {
		if err := validateMCPRedirect(registered, uri); err != nil {
			t.Fatal(err)
		}
	}
	for _, uri := range []string{"https://client.example/callback/evil", "https://client.example.evil/callback", "https://client.example/callback?evil=1", "http://127.0.0.1:8888/evil", "http://localhost:8888/callback", "https://user@client.example/callback", "https://client.example/callback#fragment", "javascript:alert(1)"} {
		if err := validateMCPRedirect(registered, uri); err == nil {
			t.Fatalf("unsafe redirect accepted: %s", uri)
		}
	}
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::1", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "0.0.0.1", "240.0.0.1"} {
		if publicMCPAddress(netip.MustParseAddr(raw).Unmap()) {
			t.Fatalf("private/reserved metadata address accepted: %s", raw)
		}
	}
}

func TestMCPQueriesPaginationAndReadOnly(t *testing.T) {
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	credentials := newTestMCPGrant(t, env, owner)
	company := catalogCompany(t, owner, "检索泡泡公司")
	for i := range 43 {
		catalogItem(t, owner, "products", fmt.Sprintf("分页青苹果 %02d", i), map[string]any{"company_id": company.ID, "category": "水果"})
	}
	if _, err := env.db.Exec("UPDATE products SET updated_at='2026-01-01T00:00:00Z'"); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := env.db.QueryRow("SELECT COUNT(*) FROM revisions").Scan(&before); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	args := map[string]any{"query": "青苹果", "kinds": []string{"product"}, "company_id": company.ID, "limit": 20}
	for {
		page := testMCPData[mcpPage](t, testMCPCall(t, env, credentials.AccessToken, "search", args))
		for _, item := range page.Results {
			if seen[item.ID] {
				t.Fatal("duplicate row across cursor pages")
			}
			seen[item.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		args["cursor"] = page.NextCursor
	}
	if len(seen) != 43 {
		t.Fatalf("pagination lost records: %d", len(seen))
	}
	args["query"] = "different filter"
	if !testMCPCall(t, env, credentials.AccessToken, "search", args).IsError {
		t.Fatal("cursor from another query should be rejected")
	}
	page := testMCPData[mcpPage](t, testMCPCall(t, env, credentials.AccessToken, "search", map[string]any{"query": "泡泡", "kinds": []string{"product"}}))
	if len(page.Results) != 20 || page.NextCursor == "" {
		t.Fatal("company aliases and default pagination should work")
	}
	detail := testMCPData[map[string]any](t, testMCPCall(t, env, credentials.AccessToken, "fetch", map[string]any{"id": page.Results[0].ID}))
	for _, private := range []string{`"created_by"`, `"author_id"`, `"favorite"`, `"ai_status"`, `"thumbnail_url"`, `"file_path"`} {
		if strings.Contains(encodeJSON(detail), private) {
			t.Fatalf("private field exposed: %s", private)
		}
	}
	var after int
	if err := env.db.QueryRow("SELECT COUNT(*) FROM revisions").Scan(&after); err != nil || before != after {
		t.Fatal("read tools changed business history")
	}
}

func TestMCPDeletedNestedRecordsAndRelations(t *testing.T) {
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	credentials := newTestMCPGrant(t, env, owner)
	material := catalogItem(t, owner, "products", "已删除的秘密材料", map[string]any{"description": "绝不应返回的材料说明"})
	live := catalogItem(t, owner, "tools", "保留的道具", nil)
	output := catalogItem(t, owner, "products", "已删除的秘密产物", nil)
	group := catalogGroup(t, owner, output)
	status, data := owner.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{Name: "保留的配方", OutputQuantity: 2, Ingredients: []Ingredient{{ProductID: material.ID, Quantity: 3}, {ProductID: live.ID, Quantity: 1}}})
	assertStatus(t, status, 201, data)
	recipe := decodeTestJSON[Recipe](t, data)
	idea := createTestIdea(t, owner, "mcp-relation", "配方灵感")
	status, data = owner.request(t, "POST", "/api/setting-links", map[string]string{"idea_id": idea.ID, "entity_type": "recipe", "entity_id": recipe.ID})
	assertStatus(t, status, 201, data)
	for _, product := range []Product{material, output} {
		status, data = owner.request(t, "DELETE", "/api/products/"+product.ID, map[string]int{"version": product.Version})
		assertStatus(t, status, 200, data)
	}
	result := testMCPData[map[string]any](t, testMCPCall(t, env, credentials.AccessToken, "fetch", map[string]any{"id": "recipe:" + recipe.ID}))
	raw := encodeJSON(result)
	for _, secret := range []string{material.ChineseName, material.Description, output.ChineseName, `"created_by"`, `"deleted_at"`} {
		if strings.Contains(raw, secret) {
			t.Fatalf("deleted/private content exposed: %s", secret)
		}
	}
	if !strings.Contains(raw, material.ID) || !strings.Contains(raw, `"quantity":3`) || !strings.Contains(raw, "candidate") {
		t.Fatal("recipe references/quantities and inspiration relation state must survive")
	}
	search := testMCPData[mcpPage](t, testMCPCall(t, env, credentials.AccessToken, "search", map[string]any{"query": "秘密"}))
	if len(search.Results) != 0 {
		t.Fatal("deleted ingredient/output content must not remain searchable")
	}
	if !testMCPCall(t, env, credentials.AccessToken, "fetch", map[string]any{"id": "product:" + material.ID}).IsError {
		t.Fatal("deleted record should be inaccessible")
	}
	status, data = owner.request(t, "DELETE", "/api/recipe-groups/"+group.ID, map[string]int{"version": group.Version})
	assertStatus(t, status, 200, data)
	if !testMCPCall(t, env, credentials.AccessToken, "fetch", map[string]any{"id": "recipe:" + recipe.ID}).IsError {
		t.Fatal("recipe in deleted group should be inaccessible")
	}
	related := testMCPData[mcpPage](t, testMCPCall(t, env, credentials.AccessToken, "list_records", map[string]any{"related_to": "idea:" + idea.ID}))
	if len(related.Results) != 0 {
		t.Fatal("deleted group recipe should not survive in relations")
	}
	status, data = owner.request(t, "GET", "/api/export", nil)
	assertStatus(t, status, 200, data)
	if !bytes.Contains(data, []byte(material.ChineseName)) || !bytes.Contains(data, []byte(output.ChineseName)) || !bytes.Contains(data, []byte(`"deleted_at"`)) {
		t.Fatal("authenticated manual export must still include trash and deleted references")
	}
}

type mcpRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn mcpRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestMCPCIMDAndAuthorizationValidation(t *testing.T) {
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	metadataID := "https://client.example/oauth/client.json"
	metadata := oauthClientMetadata{ClientID: metadataID, ClientName: "CIMD 客户端", RedirectURIs: []string{"http://127.0.0.1/callback"}, TokenEndpointAuthMethodsSupported: []string{"none", "private_key_jwt"}}
	env.app.oauthHTTP = &http.Client{Transport: mcpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != metadataID || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("metadata fetching must not forward account credentials")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(encodeJSON(metadata))), Request: r}, nil
	})}
	pending, _ := beginTestMCPAuthorization(t, env, metadataID)
	if code := finishTestMCPAuthorization(t, owner, pending, true); code == "" {
		t.Fatal("CIMD public clients must authorize without DCR or a secret")
	}
	id := registerTestMCPClient(t, owner)
	params := url.Values{"response_type": {"code"}, "client_id": {id}, "redirect_uri": {"http://127.0.0.1:7777/callback"}, "resource": {env.app.mcpResource()},
		"code_challenge": {strings.Repeat("A", 43)}, "code_challenge_method": {"S256"}, "scope": {mcpReadScope}}
	for _, change := range [][2]string{{"resource", "https://other.example/api/mcp"}, {"scope", "creative-library:write"}, {"redirect_uri", "http://127.0.0.1:7777/evil"}, {"code_challenge_method", "plain"}, {"code_challenge", ""}} {
		query, _ := url.ParseQuery(params.Encode())
		query.Set(change[0], change[1])
		status, data := owner.request(t, "GET", "/api/mcp/oauth/authorize?"+query.Encode(), nil)
		assertStatus(t, status, 400, data)
	}
	response, err := newMCPMetadataClient().Get("https://127.0.0.1/client.json")
	if err == nil {
		response.Body.Close()
		t.Fatal("metadata client must reject private network access before dialing")
	}
}

func TestMCPExpiryAccountStatusAndRemovedSnapshot(t *testing.T) {
	env := newTestEnvironment(t, nil)
	owner := newAdminClient(t, env)
	credentials := newTestMCPGrant(t, env, owner)
	if _, err := env.db.Exec(`UPDATE mcp_oauth_tokens SET data=json_set(data,'$.AccessCreateAt',?) WHERE access_hash=?`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), tokenHash(credentials.AccessToken)); err != nil {
		t.Fatal(err)
	}
	status, data, _ := testMCPRequest(t, env, credentials.AccessToken, "tools/list", map[string]any{})
	assertStatus(t, status, 401, data)
	status, data = testMCPTokenForm(t, env, url.Values{"grant_type": {"refresh_token"}, "client_id": {credentials.ClientID}, "refresh_token": {credentials.RefreshToken}})
	assertStatus(t, status, 200, data)
	refreshed := decodeTestJSON[testMCPCredentials](t, data)
	if _, err := env.db.Exec("UPDATE users SET status='disabled' WHERE id=?", owner.user.ID); err != nil {
		t.Fatal(err)
	}
	status, data, _ = testMCPRequest(t, env, refreshed.AccessToken, "tools/list", map[string]any{})
	assertStatus(t, status, 401, data)
	status, data = testMCPTokenForm(t, env, url.Values{"grant_type": {"refresh_token"}, "client_id": {credentials.ClientID}, "refresh_token": {refreshed.RefreshToken}})
	assertStatus(t, status, 400, data)
	if _, err := env.db.Exec("UPDATE users SET status='active' WHERE id=?", owner.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.db.Exec("UPDATE mcp_oauth_grants SET expires_at='2000-01-01' WHERE client_id=?", credentials.ClientID); err != nil {
		t.Fatal(err)
	}
	status, data, _ = testMCPRequest(t, env, refreshed.AccessToken, "tools/list", map[string]any{})
	assertStatus(t, status, 401, data)
	for _, path := range []string{"/api/agent/v1/catalog", "/api/agent-tokens"} {
		status, data = owner.request(t, "GET", path, nil)
		assertStatus(t, status, 404, data)
	}
	// Browser cookies alone do not grant MCP access.
	status, data = owner.request(t, "POST", "/api/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	assertStatus(t, status, 401, data)
}
