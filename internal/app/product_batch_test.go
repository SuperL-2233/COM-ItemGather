package app

import (
	"fmt"
	"net/http"
	"testing"
)

func batchSelection(items ...Product) []map[string]any {
	selected := make([]map[string]any, 0, len(items))
	for _, item := range items {
		selected = append(selected, map[string]any{"id": item.ID, "version": item.Version})
	}
	return selected
}

func TestProductBatchUpdatesSelectedFieldsAndHistory(t *testing.T) {
	env := newTestEnvironment(t, nil)
	admin := newAdminClient(t, env)
	company := catalogCompany(t, admin, "批量修改的公司")
	category := newProductOption(t, admin, "categories", "批量新分类")
	state := newProductOption(t, admin, "statuses", "批量验收状态")
	first := catalogItem(t, admin, "products", "批量商品甲", map[string]any{"description": "需要保留的说明", "attributes": []Attribute{{Key: "材料", Value: "玻璃", Certainty: "asserted"}}})
	second := catalogItem(t, admin, "products", "批量商品乙", nil)
	untouched := catalogItem(t, admin, "products", "未选择的商品", nil)
	createTestUser(t, env.db, "batch-member", "member")
	member := newTestClient(t, env.server)
	member.login(t, "batch-member", "correct-horse-42")
	status, data := member.request(t, http.MethodPatch, "/api/products/batch", map[string]any{
		"items": batchSelection(first, second), "company_id": company.ID, "category": category.Value, "development_status": state.Value,
	})
	assertStatus(t, status, 200, data)
	if decodeTestJSON[struct {
		Updated int `json:"updated"`
	}](t, data).Updated != 2 {
		t.Fatal("应返回批量修改的商品数量")
	}
	for _, original := range []Product{first, second} {
		status, data = member.request(t, http.MethodGet, "/api/products/"+original.ID, nil)
		assertStatus(t, status, 200, data)
		item := decodeTestJSON[Product](t, data)
		if item.Version != original.Version+1 || item.Category != category.Value || item.CompanyID != company.ID || item.Company != company.ChineseName || item.DevelopmentStatus != state.Value || item.DevelopmentStatusLabel != state.Label {
			t.Fatalf("批量修改未完整应用: %+v", item)
		}
		if item.Description != original.Description || item.ChineseName != original.ChineseName || len(item.Attributes) != len(original.Attributes) || item.CreatedBy != original.CreatedBy {
			t.Fatal("批量修改不应覆盖未指定的字段")
		}
		status, data = member.request(t, http.MethodGet, "/api/products/"+item.ID+"/revisions", nil)
		assertStatus(t, status, 200, data)
		revisions := decodeTestJSON[struct {
			Items []Revision `json:"items"`
		}](t, data).Items
		if len(revisions) != 2 || revisions[0].ActorID != member.user.ID || revisions[0].Version != item.Version || revisions[0].Action != "update" {
			t.Fatalf("批量修改应记录操作者及版本历史: %s", data)
		}
	}
	first.Version++
	second.Version++
	status, data = member.request(t, http.MethodPatch, "/api/products/batch", map[string]any{"items": batchSelection(first, second), "category": "", "company_id": ""})
	assertStatus(t, status, 200, data)
	for _, original := range []Product{first, second} {
		status, data = member.request(t, http.MethodGet, "/api/products/"+original.ID, nil)
		assertStatus(t, status, 200, data)
		item := decodeTestJSON[Product](t, data)
		if item.Category != "" || item.CompanyID != "" || item.Company != "" || item.DevelopmentStatus != state.Value || item.Version != 3 {
			t.Fatalf("应允许清空分类和公司，并保留未指定的制作状态: %+v", item)
		}
	}
	status, data = member.request(t, http.MethodGet, "/api/products/"+untouched.ID, nil)
	assertStatus(t, status, 200, data)
	if item := decodeTestJSON[Product](t, data); item.Version != untouched.Version || item.Category != untouched.Category || item.CompanyID != untouched.CompanyID || item.DevelopmentStatus != untouched.DevelopmentStatus {
		t.Fatal("未选择的商品不应变化")
	}
	var events int
	if err := env.db.QueryRow("SELECT COUNT(*) FROM events WHERE event_type='product.updated'").Scan(&events); err != nil || events != 4 {
		t.Fatalf("应为每次商品修改发布同步事件: count=%d, err=%v", events, err)
	}
}

func TestProductBatchRejectsInvalidSelectionsWithoutWrites(t *testing.T) {
	env := newTestEnvironment(t, nil)
	client := newAdminClient(t, env)
	first := catalogItem(t, client, "products", "有效商品", nil)
	second := catalogItem(t, client, "products", "版本过期的商品", nil)
	tool := catalogItem(t, client, "tools", "不应修改的道具", nil)
	deleted := catalogItem(t, client, "products", "已删除的商品", nil)
	status, data := client.request(t, http.MethodDelete, "/api/products/"+deleted.ID, map[string]int{"version": deleted.Version})
	assertStatus(t, status, 200, data)
	deletedCompany := catalogCompany(t, client, "已删除的公司")
	status, data = client.request(t, http.MethodDelete, "/api/companies/"+deletedCompany.ID, map[string]int{"version": deletedCompany.Version})
	assertStatus(t, status, 200, data)
	deletedCategory := newProductOption(t, client, "categories", "已删除的分类")
	deletedStatus := newProductOption(t, client, "statuses", "已删除的状态")
	if _, err := env.db.Exec("UPDATE product_categories SET deleted_at=? WHERE name=?", now(), deletedCategory.Value); err != nil {
		t.Fatal(err)
	}
	if _, err := env.db.Exec("UPDATE product_statuses SET deleted_at=? WHERE id=?", now(), deletedStatus.Value); err != nil {
		t.Fatal(err)
	}
	tooMany := make([]map[string]any, 501)
	for i := range tooMany {
		tooMany[i] = map[string]any{"id": fmt.Sprint(i), "version": 1}
	}
	cases := []struct {
		name   string
		body   map[string]any
		status int
	}{
		{"empty", map[string]any{"items": batchSelection()}, 422},
		{"no changes", map[string]any{"items": batchSelection(first)}, 422},
		{"duplicate", map[string]any{"items": batchSelection(first, first), "category": "新分类"}, 422},
		{"too many", map[string]any{"items": tooMany, "category": "新分类"}, 422},
		{"missing version", map[string]any{"items": []map[string]any{{"id": first.ID}}, "category": "新分类"}, 422},
		{"stale version", map[string]any{"items": batchSelection(first, Product{ID: second.ID, Version: 2}), "category": "新分类"}, 409},
		{"missing", map[string]any{"items": batchSelection(first, Product{ID: "missing", Version: 1}), "category": "新分类"}, 404},
		{"deleted", map[string]any{"items": batchSelection(first, deleted), "category": "新分类"}, 404},
		{"tool", map[string]any{"items": batchSelection(first, tool), "category": "新分类"}, 422},
		{"company", map[string]any{"items": batchSelection(first), "category": "新分类", "company_id": "missing"}, 404},
		{"deleted company", map[string]any{"items": batchSelection(first), "company_id": deletedCompany.ID}, 404},
		{"status", map[string]any{"items": batchSelection(first), "category": "新分类", "development_status": "missing"}, 422},
		{"empty status", map[string]any{"items": batchSelection(first), "development_status": ""}, 422},
		{"deleted status", map[string]any{"items": batchSelection(first), "development_status": deletedStatus.Value}, 409},
		{"deleted category", map[string]any{"items": batchSelection(first), "category": deletedCategory.Value}, 409},
		{"unsupported field", map[string]any{"items": batchSelection(first), "description": "不允许修改"}, 400},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			status, data := client.request(t, http.MethodPatch, "/api/products/batch", test.body)
			assertStatus(t, status, test.status, data)
			if test.name == "deleted" && testError(t, data) != "selection_changed" {
				t.Fatal("所选商品已删除时应提示重新选择")
			}
			status, data = client.request(t, http.MethodGet, "/api/products/"+first.ID, nil)
			assertStatus(t, status, 200, data)
			if item := decodeTestJSON[Product](t, data); item.Version != 1 || item.Category != "" {
				t.Fatal("失败的批量修改不应部分保存")
			}
			var categories, changes int
			if err := env.db.QueryRow("SELECT COUNT(*) FROM product_categories WHERE name='新分类'").Scan(&categories); err != nil {
				t.Fatal(err)
			}
			if err := env.db.QueryRow("SELECT COUNT(*) FROM events WHERE event_type='product.updated'").Scan(&changes); err != nil {
				t.Fatal(err)
			}
			if categories != 0 || changes != 0 {
				t.Fatal("失败请求不应留下分类或同步事件")
			}
		})
	}
	anonymous := newTestClient(t, env.server)
	status, data = anonymous.request(t, http.MethodPatch, "/api/products/batch", map[string]any{"items": batchSelection(first), "category": "新分类"})
	assertStatus(t, status, 401, data)
	client.csrf = ""
	status, data = client.request(t, http.MethodPatch, "/api/products/batch", map[string]any{"items": batchSelection(first), "category": "新分类"})
	assertStatus(t, status, 403, data)
}

func TestProductBatchRollsBackEarlierWrites(t *testing.T) {
	env := newTestEnvironment(t, nil)
	client := newAdminClient(t, env)
	first := catalogItem(t, client, "products", "第一件商品", nil)
	second := catalogItem(t, client, "products", "第二件商品", nil)
	if _, err := env.db.Exec("CREATE TRIGGER reject_batch_update BEFORE UPDATE ON products WHEN OLD.id='" + second.ID + "' BEGIN SELECT RAISE(ABORT,'batch test failure'); END"); err != nil {
		t.Fatal(err)
	}
	status, data := client.request(t, http.MethodPatch, "/api/products/batch", map[string]any{"items": batchSelection(first, second), "category": "应回滚的分类"})
	assertStatus(t, status, 500, data)
	for _, original := range []Product{first, second} {
		status, data = client.request(t, http.MethodGet, "/api/products/"+original.ID, nil)
		assertStatus(t, status, 200, data)
		if item := decodeTestJSON[Product](t, data); item.Version != 1 || item.Category != "" {
			t.Fatal("后续写入失败时，应回滚先前商品的修改")
		}
	}
	var categories, revisions, events int
	if err := env.db.QueryRow("SELECT COUNT(*) FROM product_categories WHERE name='应回滚的分类'").Scan(&categories); err != nil {
		t.Fatal(err)
	}
	if err := env.db.QueryRow("SELECT COUNT(*) FROM revisions WHERE action='update'").Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err := env.db.QueryRow("SELECT COUNT(*) FROM events WHERE event_type='product.updated'").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if categories != 0 || revisions != 0 || events != 0 {
		t.Fatal("分类、版本历史与同步事件必须随批量写入一起回滚")
	}
}
