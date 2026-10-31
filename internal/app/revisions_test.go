package app

import "testing"

func TestHistoryRestoreAdvancesContentVersion(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	idea := createTestIdea(t, client, "history-content-version", "第一版正文")
	idea = waitAIStatus(t, client, idea.ID, "succeeded")
	status, data := client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, "content": "第二版正文"})
	assertStatus(t, status, 200, data)
	edited := decodeTestJSON[Idea](t, data)
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[map[string][]Revision](t, data)["items"]
	first := revisions[len(revisions)-1]
	status, data = client.request(t, "POST", "/api/ideas/"+idea.ID+"/revisions/"+first.ID+"/restore", map[string]int{"version": edited.Version})
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID, nil)
	assertStatus(t, status, 200, data)
	restored := decodeTestJSON[Idea](t, data)
	if restored.Content != "第一版正文" || restored.ContentVersion != edited.ContentVersion+1 || restored.AIStatus != "idle" {
		t.Fatalf("restored revision has stale content version: %+v", restored)
	}
}
