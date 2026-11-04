package app

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func agentRequest(t *testing.T, environment *testEnvironment, token, path string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, environment.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := environment.server.Client().Do(req)
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

func TestAgentTokenCatalogLifecycle(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	createTestIdea(t, client, "agent-visible", "可提供给 AI 的创意")
	deleted := createTestIdea(t, client, "agent-deleted", "回收站内容不应出现在快照")
	deleted = waitAIStatus(t, client, deleted.ID, "succeeded")
	status, data := client.request(t, http.MethodDelete, "/api/ideas/"+deleted.ID, map[string]int{"version": deleted.Version})
	assertStatus(t, status, 200, data)

	status, data = agentRequest(t, environment, "", "/api/agent/v1/catalog")
	assertStatus(t, status, 401, data)
	status, data = client.request(t, http.MethodPost, "/api/agent-tokens", nil)
	assertStatus(t, status, 201, data)
	created := decodeTestJSON[AgentToken](t, data)
	createdAt, _ := time.Parse(time.RFC3339Nano, created.CreatedAt)
	expiresAt, _ := time.Parse(time.RFC3339Nano, created.ExpiresAt)
	if created.Token == "" || expiresAt.Sub(createdAt) != 15*time.Minute {
		t.Fatalf("令牌或有效期不正确: %+v", created)
	}
	var storedHash string
	if err := environment.db.QueryRow("SELECT token_hash FROM agent_tokens WHERE id=?", created.ID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash != tokenHash(created.Token) || storedHash == created.Token {
		t.Fatal("数据库没有只保存令牌哈希")
	}

	status, data = agentRequest(t, environment, created.Token, "/api/agent/v1/catalog")
	assertStatus(t, status, 200, data)
	snapshot := decodeTestJSON[map[string]any](t, data)
	for _, key := range []string{"ideas", "products", "tools", "companies", "recipe_groups", "recipes", "links", "setting_links", "product_categories", "product_statuses", "tool_categories", "schema_version", "generated_at"} {
		if _, ok := snapshot[key]; !ok {
			t.Errorf("快照缺少字段 %s", key)
		}
	}
	ideas := snapshot["ideas"].([]any)
	if len(ideas) != 1 {
		t.Fatalf("快照应只包含未删除灵感: %s", data)
	}
	idea := ideas[0].(map[string]any)
	if idea["original_content"] != "可提供给 AI 的创意" {
		t.Fatalf("快照灵感内容不正确: %+v", idea)
	}
	for _, key := range []string{"client_id", "author_id", "author_nickname", "ai_status", "ai_error", "favorite", "deleted_at"} {
		if _, ok := idea[key]; ok {
			t.Errorf("快照不应包含字段 %s", key)
		}
	}
	status, data = agentRequest(t, environment, created.Token, "/api/ideas")
	assertStatus(t, status, 401, data)

	status, data = client.request(t, http.MethodGet, "/api/agent-tokens", nil)
	assertStatus(t, status, 200, data)
	listed := decodeTestJSON[struct {
		Items []AgentToken `json:"items"`
	}](t, data)
	if len(listed.Items) != 1 || listed.Items[0].Token != "" || listed.Items[0].LastUsedAt == "" {
		t.Fatalf("令牌列表泄露原文或缺少使用记录: %+v", listed.Items)
	}

	createTestUser(t, environment.db, "other", "member")
	other := newTestClient(t, environment.server)
	other.login(t, "other", "correct-horse-42")
	status, data = other.request(t, http.MethodDelete, "/api/agent-tokens/"+created.ID, nil)
	assertStatus(t, status, 404, data)

	if _, err := environment.db.Exec("UPDATE agent_tokens SET expires_at=? WHERE id=?", time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), created.ID); err != nil {
		t.Fatal(err)
	}
	status, data = agentRequest(t, environment, created.Token, "/api/agent/v1/catalog")
	assertStatus(t, status, 401, data)
	if _, err := environment.db.Exec("UPDATE agent_tokens SET expires_at=? WHERE id=?", time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), created.ID); err != nil {
		t.Fatal(err)
	}
	status, data = client.request(t, http.MethodDelete, "/api/agent-tokens/"+created.ID, nil)
	assertStatus(t, status, 200, data)
	status, data = agentRequest(t, environment, created.Token, "/api/agent/v1/catalog")
	assertStatus(t, status, 401, data)
}
