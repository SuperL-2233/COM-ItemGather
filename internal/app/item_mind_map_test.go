package app

import (
	"fmt"
	"slices"
	"testing"

	"com-itemgather/internal/security"
)

func readTestMindMap(t *testing.T, c *testClient) itemMindMap {
	t.Helper()
	status, data := c.request(t, "GET", "/api/item-mind-map", nil)
	assertStatus(t, status, 200, data)
	return decodeTestJSON[itemMindMap](t, data)
}

func TestItemMindMapAuthenticationAndEmptySnapshot(t *testing.T) {
	env := newTestEnvironment(t, nil)
	anonymous := newTestClient(t, env.server)
	status, data := anonymous.request(t, "GET", "/api/item-mind-map", nil)
	assertStatus(t, status, 401, data)
	c := newAdminClient(t, env)
	graph := readTestMindMap(t, c)
	if graph.Companies == nil || len(graph.Companies) != 0 || graph.Items == nil || len(graph.Items) != 0 || len(graph.Statuses) != 4 {
		t.Fatalf("empty graph must contain arrays and the shared statuses: %+v", graph)
	}
}

func TestItemMindMapIncludesOnlyProductsAndCurrentSharedStatuses(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	company := catalogCompany(t, c, "浮泡饮料")
	empty := catalogCompany(t, c, "空公司")
	archived := catalogCompany(t, c, "旧公司")
	status, data := c.request(t, "POST", "/api/product-options/statuses", map[string]string{"name": "待验收", "color": "#123abc"})
	assertStatus(t, status, 200, data)
	custom := decodeTestJSON[struct{ Option ChoiceOption }](t, data).Option
	goods := catalogItem(t, c, "products", "色乐", map[string]any{"company_id": company.ID, "english_name": "Color", "category": "饮料", "development_status": custom.Value})
	tool := catalogItem(t, c, "tools", "开瓶器", map[string]any{"company_id": company.ID, "development_status": "completed"})
	other := catalogItem(t, c, "other-items", "旧钥匙", nil)
	loose := catalogItem(t, c, "products", "", map[string]any{"english_name": "Loose Item"})
	trashed := catalogItem(t, c, "products", "回收站商品", map[string]any{"company_id": company.ID})
	oldItem := catalogItem(t, c, "products", "旧公司商品", nil)
	status, data = c.request(t, "DELETE", "/api/products/"+trashed.ID, map[string]int{"version": trashed.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "DELETE", "/api/companies/"+archived.ID, map[string]int{"version": archived.Version})
	assertStatus(t, status, 200, data)
	// Older imports or direct database edits may retain an archived company.
	if _, err := env.db.Exec("UPDATE products SET company_id=? WHERE id=?", archived.ID, oldItem.ID); err != nil {
		t.Fatal(err)
	}

	graph := readTestMindMap(t, c)
	if len(graph.Companies) != 2 || len(graph.Items) != 3 || len(graph.Statuses) != 5 {
		t.Fatalf("snapshot omitted active records or included trash: %+v", graph)
	}
	for _, id := range []string{company.ID, empty.ID} {
		if !slices.ContainsFunc(graph.Companies, func(value mindMapCompany) bool { return value.ID == id }) {
			t.Fatalf("company %s was omitted", id)
		}
	}
	byID := map[string]mindMapItem{}
	for _, item := range graph.Items {
		byID[item.ID] = item
	}
	if got := byID[goods.ID]; got.CompanyID != company.ID || got.ItemKind != "product" || got.EnglishName != "Color" || got.Category != "饮料" || got.DevelopmentStatus != custom.Value || got.DevelopmentStatusLabel != "待验收" || got.DevelopmentStatusColor != "#123abc" {
		t.Fatalf("product or custom status is incomplete: %+v", got)
	}
	for _, excluded := range []Product{tool, other, trashed} {
		if _, exists := byID[excluded.ID]; exists {
			t.Fatalf("non-product or trashed item was included: %+v", excluded)
		}
	}
	if byID[loose.ID].EnglishName != "Loose Item" || !byID[oldItem.ID].CompanyDeleted {
		t.Fatalf("unassigned or archived-company record is incomplete: %+v", graph.Items)
	}
	// Option changes do not increment product versions, so the graph must join
	// the current status table instead of reusing a saved item snapshot.
	status, data = c.request(t, "PATCH", "/api/product-options/statuses/"+custom.Value+"/color", map[string]any{"version": custom.Version, "color": "#abcdef"})
	assertStatus(t, status, 200, data)
	if !slices.ContainsFunc(readTestMindMap(t, c).Items, func(item mindMapItem) bool {
		return item.ID == goods.ID && item.DevelopmentStatusColor == "#abcdef"
	}) {
		t.Fatal("status color update was not reflected in item nodes")
	}
	removeStatusForTest(t, c, custom.Value, "completed")
	graph = readTestMindMap(t, c)
	if len(graph.Statuses) != 4 || !slices.ContainsFunc(graph.Items, func(item mindMapItem) bool {
		return item.ID == goods.ID && item.DevelopmentStatus == "completed" && item.DevelopmentStatusLabel == "已完成" && item.DevelopmentStatusColor == "#276749"
	}) {
		t.Fatalf("status replacement was not reflected: %+v", graph)
	}
}

func TestItemMindMapReadsAllItemsAndDatabaseChanges(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	source := catalogCompany(t, c, "来源公司")
	target := catalogCompany(t, c, "目标公司")
	tx, err := env.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for index := range 135 {
		_, err = tx.Exec(`INSERT INTO products(id,chinese_name,created_by,created_at,updated_at,company_id)
			VALUES(?,?,?,?,?,?)`, security.ID(), fmt.Sprintf("商品 %03d", index), c.user.ID, now(), now(), source.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	graph := readTestMindMap(t, c)
	if len(graph.Items) != 135 {
		t.Fatalf("graph was truncated by list pagination: %d", len(graph.Items))
	}
	status, data := c.request(t, "POST", "/api/companies/"+source.ID+"/merge", map[string]any{"version": source.Version, "target_id": target.ID, "target_version": target.Version})
	assertStatus(t, status, 200, data)
	// A direct database edit has no application event; the next snapshot must
	// still reflect it, which is also used by the browser's periodic refresh.
	if _, err := env.db.Exec("UPDATE product_statuses SET name=?,color=? WHERE id='concept'", "刚更新的状态", "#abc123"); err != nil {
		t.Fatal(err)
	}
	graph = readTestMindMap(t, c)
	if len(graph.Companies) != 1 || graph.Companies[0].ID != target.ID || len(graph.Items) != 135 {
		t.Fatalf("merge lost nodes or retained its archived company: %+v", graph.Companies)
	}
	for _, item := range graph.Items {
		if item.CompanyID != target.ID || item.DevelopmentStatus != "concept" || item.DevelopmentStatusLabel != "刚更新的状态" || item.DevelopmentStatusColor != "#abc123" {
			t.Fatalf("graph contains stale data after a move/status edit: %+v", item)
		}
	}
}
