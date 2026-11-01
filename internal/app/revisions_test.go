package app

import "testing"

func TestLegacyProductRevisionKeepsFinalBrandName(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	status, data := client.request(t, "POST", "/api/products", map[string]string{"chinese_name": "新中文名"})
	assertStatus(t, status, 201, data)
	product := decodeTestJSON[Product](t, data)
	legacy := `{"internal_name":"旧内部名","final_brand_name":"旧中文名","company":"旧公司","category":"","description":"","development_status":"concept","attributes":[]}`
	_, err := environment.db.Exec("INSERT INTO revisions(id,entity_type,entity_id,version,action,after_data,created_at) VALUES(?,?,?,?,?,?,?)", "legacy-product-revision", "product", product.ID, 1, "update", legacy, now())
	if err != nil {
		t.Fatal(err)
	}
	status, data = client.request(t, "POST", "/api/products/"+product.ID+"/revisions/legacy-product-revision/restore", map[string]int{"version": product.Version})
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/products/"+product.ID, nil)
	assertStatus(t, status, 200, data)
	restored := decodeTestJSON[Product](t, data)
	if restored.ChineseName != "旧中文名" || restored.Company != "旧公司" {
		t.Fatalf("legacy product revision was not restored: %+v", restored)
	}
}

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
