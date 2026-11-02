package app

import (
	"net/url"
	"strings"
	"testing"
)

func newProductOption(t *testing.T, client *testClient, kind, name string) ChoiceOption {
	t.Helper()
	status, data := client.request(t, "POST", "/api/product-options/"+kind, map[string]string{"name": name})
	assertStatus(t, status, 200, data)
	return decodeTestJSON[struct {
		Option ChoiceOption `json:"option"`
	}](t, data).Option
}
func TestProductOptionsAreSharedPersistentAndDeduplicated(t *testing.T) {
	env := newTestEnvironment(t, nil)
	admin := newAdminClient(t, env)
	createTestUser(t, env.db, "option-member", "member")
	member := newTestClient(t, env.server)
	member.login(t, "option-member", "correct-horse-42")
	a := newProductOption(t, admin, "categories", "  Drink Category  ")
	b := newProductOption(t, member, "categories", "drink category")
	if a != b || a.Value != "Drink Category" {
		t.Fatalf("category deduplication failed: %+v %+v", a, b)
	}
	state := newProductOption(t, member, "statuses", "等待模型返工")
	same := newProductOption(t, admin, "statuses", " 等待模型返工 ")
	if state != same || state.Value == "" || state.Label != "等待模型返工" {
		t.Fatal("status identity is not shared")
	}
	original := newProductOption(t, admin, "statuses", "设计中")
	if original.Value != "design" {
		t.Fatal("built-in status was duplicated")
	}
	status, data := member.request(t, "GET", "/api/product-options/categories", nil)
	assertStatus(t, status, 200, data)
	items := decodeTestJSON[struct {
		Items []ChoiceOption `json:"items"`
	}](t, data).Items
	if len(items) != 1 || items[0] != a {
		t.Fatalf("unused categories were not retained: %s", data)
	}
	for _, kind := range []string{"categories", "statuses"} {
		for _, name := range []string{" ", "+", "＋", "two\nlines", strings.Repeat("长", 101)} {
			status, data = admin.request(t, "POST", "/api/product-options/"+kind, map[string]string{"name": name})
			assertStatus(t, status, 422, data)
		}
	}
	anonymous := newTestClient(t, env.server)
	status, data = anonymous.request(t, "GET", "/api/product-options/statuses", nil)
	assertStatus(t, status, 401, data)
}
func TestCustomProductStatusSurvivesFiltersHistoryTrashAndExport(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	state := newProductOption(t, c, "statuses", "待联机验收")
	category := newProductOption(t, c, "categories", "状态验收分类")
	item := catalogItem(t, c, "products", "自定义状态商品", map[string]any{"development_status": state.Value, "category": category.Value})
	if item.DevelopmentStatus != state.Value || item.DevelopmentStatusLabel != state.Label {
		t.Fatalf("custom status not returned: %+v", item)
	}
	status, data := c.request(t, "GET", "/api/products?status="+url.QueryEscape(state.Value), nil)
	assertStatus(t, status, 200, data)
	page := decodeTestJSON[struct {
		Total int       `json:"total"`
		Items []Product `json:"items"`
	}](t, data)
	if page.Total != 1 || page.Items[0].DevelopmentStatusLabel != state.Label {
		t.Fatal("custom state filter failed")
	}
	status, data = c.request(t, "GET", "/api/products/filters", nil)
	assertStatus(t, status, 200, data)
	filters := decodeTestJSON[struct {
		Statuses []ChoiceOption `json:"statuses"`
	}](t, data)
	if len(filters.Statuses) != 5 || filters.Statuses[4] != state {
		t.Fatalf("status missing from filter choices: %s", data)
	}
	status, data = c.request(t, "PATCH", "/api/products/"+item.ID, map[string]any{"version": item.Version, "development_status": "completed"})
	assertStatus(t, status, 200, data)
	updated := decodeTestJSON[Product](t, data)
	if updated.DevelopmentStatus != "completed" || updated.DevelopmentStatusLabel != "已完成" {
		t.Fatal("built-in status update failed")
	}
	status, data = c.request(t, "GET", "/api/products/"+item.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[struct {
		Items []Revision `json:"items"`
	}](t, data).Items
	status, data = c.request(t, "POST", "/api/products/"+item.ID+"/revisions/"+revisions[len(revisions)-1].ID+"/restore", map[string]int{"version": updated.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/products/"+item.ID, nil)
	assertStatus(t, status, 200, data)
	restored := decodeTestJSON[Product](t, data)
	if restored.DevelopmentStatus != state.Value || restored.DevelopmentStatusLabel != state.Label || restored.Category != category.Value {
		t.Fatal("history did not restore custom selections")
	}
	status, data = c.request(t, "DELETE", "/api/products/"+item.ID, map[string]int{"version": restored.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/trash", nil)
	assertStatus(t, status, 200, data)
	trash := decodeTestJSON[struct {
		Products []Product `json:"products"`
	}](t, data)
	if len(trash.Products) != 1 || trash.Products[0].DevelopmentStatusLabel != state.Label {
		t.Fatal("trash lost custom state label")
	}
	status, data = c.request(t, "POST", "/api/products/"+item.ID+"/restore", map[string]int{"version": restored.Version + 1})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/export", nil)
	assertStatus(t, status, 200, data)
	export := decodeTestJSON[struct {
		Statuses   []ChoiceOption `json:"product_statuses"`
		Categories []ChoiceOption `json:"product_categories"`
		Products   []Product      `json:"products"`
	}](t, data)
	if len(export.Statuses) != 5 || len(export.Categories) != 1 || export.Products[0].DevelopmentStatus != state.Value {
		t.Fatal("export omitted option definitions")
	}
	status, data = c.request(t, "POST", "/api/products", map[string]string{"development_status": "unregistered status"})
	assertStatus(t, status, 422, data)
	status, data = c.request(t, "GET", "/api/products?status=unregistered", nil)
	assertStatus(t, status, 422, data)
}
