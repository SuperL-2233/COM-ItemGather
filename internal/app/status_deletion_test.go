package app

import (
	"net/url"
	"testing"
)

func statusUsageForTest(t *testing.T, c *testClient, id string) statusUsage {
	t.Helper()
	status, data := c.request(t, "GET", "/api/product-options/statuses/usage?id="+url.QueryEscape(id), nil)
	assertStatus(t, status, 200, data)
	return decodeTestJSON[statusUsage](t, data)
}

func removeStatusForTest(t *testing.T, c *testClient, id, replacement string) {
	t.Helper()
	usage := statusUsageForTest(t, c, id)
	status, data := c.request(t, "DELETE", "/api/product-options/statuses", map[string]string{"id": id, "replacement": replacement, "token": usage.Token})
	assertStatus(t, status, 200, data)
}

func TestStatusDeletionReplacesItemsRecipesAndTrash(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	replacement := newProductOption(t, c, "statuses", "统一复核")
	product := catalogItem(t, c, "products", "色乐汽水", map[string]any{"development_status": "design", "description": "保留商品描述"})
	trashed := catalogItem(t, c, "products", "回收站商品", map[string]any{"development_status": "design"})
	tool := catalogItem(t, c, "tools", "配方工具", map[string]any{"development_status": "design"})
	status, data := c.request(t, "DELETE", "/api/products/"+trashed.ID, map[string]int{"version": trashed.Version})
	assertStatus(t, status, 200, data)
	group := catalogGroup(t, c, product)
	recipes := []Recipe{}
	for range 2 {
		status, data = c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 2, DevelopmentStatus: "design", Ingredients: []Ingredient{{ProductID: tool.ID, Quantity: 3}}})
		assertStatus(t, status, 201, data)
		recipes = append(recipes, decodeTestJSON[Recipe](t, data))
	}
	status, data = c.request(t, "DELETE", "/api/recipes/"+recipes[1].ID, map[string]int{"version": recipes[1].Version})
	assertStatus(t, status, 200, data)
	usage := statusUsageForTest(t, c, "design")
	if usage.ItemCount != 3 || usage.RecipeCount != 2 {
		t.Fatalf("usage missed active or trashed records: %+v", usage)
	}
	removeStatusForTest(t, c, "design", replacement.Value)
	for _, original := range []Product{product, trashed, tool} {
		item, err := loadProduct(env.app.runContext, env.db, original.ID, true, false)
		if err != nil {
			t.Fatal(err)
		}
		wantVersion := original.Version + 1
		if original.ID == trashed.ID {
			wantVersion++
		}
		if item.DevelopmentStatus != replacement.Value || item.DevelopmentStatusLabel != replacement.Label || item.Version != wantVersion || item.Description != original.Description || item.ChineseName != original.ChineseName {
			t.Fatalf("replacement changed unrelated item data or missed status: %+v", item)
		}
		if (item.DeletedAt != "") != (original.ID == trashed.ID) {
			t.Fatal("status deletion changed trash membership")
		}
	}
	for index, original := range recipes {
		item, err := loadRecipe(env.app.runContext, env.db, original.ID, true)
		if err != nil {
			t.Fatal(err)
		}
		if item.DevelopmentStatus != replacement.Value || item.DevelopmentStatusLabel != replacement.Label || item.Version != original.Version+1+index || item.OutputQuantity != 2 || len(item.Ingredients) != 1 || item.Ingredients[0].Quantity != 3 {
			t.Fatalf("recipe replacement failed: %+v", item)
		}
	}
	status, data = c.request(t, "GET", "/api/products/"+product.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[struct {
		Items []Revision `json:"items"`
	}](t, data).Items
	if revisions[0].Action != "replace_status" {
		t.Fatal("status replacement did not create history")
	}
	path := "/api/products/" + product.ID + "/revisions/" + revisions[len(revisions)-1].ID + "/restore"
	status, data = c.request(t, "POST", path, map[string]int{"version": product.Version + 1})
	assertStatus(t, status, 409, data)
	if testError(t, data) != "status_deleted" {
		t.Fatalf("history silently restored a deleted state: %s", data)
	}
	if revived := newProductOption(t, c, "statuses", "设计中"); revived.Value != "design" {
		t.Fatal("explicit recreation lost the status identity")
	}
	status, data = c.request(t, "POST", path, map[string]int{"version": product.Version + 1})
	assertStatus(t, status, 200, data)
	rows, err := env.db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("status replacement broke foreign keys")
	}
}

func TestStatusDeletionReconfirmsAndKeepsAWorkingDefault(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	usage := statusUsageForTest(t, c, "concept")
	product := catalogItem(t, c, "products", "新增引用", nil)
	status, data := c.request(t, "DELETE", "/api/product-options/statuses", map[string]string{"id": "concept", "replacement": "design", "token": usage.Token})
	assertStatus(t, status, 409, data)
	if testError(t, data) != "status_changed" {
		t.Fatal("new references did not require confirmation")
	}
	usage = statusUsageForTest(t, c, "concept")
	status, data = c.request(t, "PATCH", "/api/products/"+product.ID, map[string]any{"version": product.Version, "description": "引用数量不变但内容已变化"})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "DELETE", "/api/product-options/statuses", map[string]string{"id": "concept", "replacement": "design", "token": usage.Token})
	assertStatus(t, status, 409, data)
	usage = statusUsageForTest(t, c, "concept")
	for _, invalid := range []string{"", "concept", "unknown"} {
		status, data = c.request(t, "DELETE", "/api/product-options/statuses", map[string]string{"id": "concept", "replacement": invalid, "token": usage.Token})
		assertStatus(t, status, 422, data)
	}
	removeStatusForTest(t, c, "concept", "design")
	status, data = c.request(t, "POST", "/api/products", map[string]string{"chinese_name": "删除默认后创建"})
	assertStatus(t, status, 201, data)
	created := decodeTestJSON[Product](t, data)
	if created.DevelopmentStatus != "design" {
		t.Fatal("new product still used the deleted default")
	}
	status, data = c.request(t, "POST", "/api/products", map[string]string{"development_status": "concept"})
	assertStatus(t, status, 409, data)
	group := catalogGroup(t, c, created)
	status, data = c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 1})
	assertStatus(t, status, 201, data)
	if decodeTestJSON[Recipe](t, data).DevelopmentStatus != "design" {
		t.Fatal("new recipe still used the deleted default")
	}
	for _, id := range []string{"production", "completed"} {
		removeStatusForTest(t, c, id, "")
	}
	usage = statusUsageForTest(t, c, "design")
	status, data = c.request(t, "DELETE", "/api/product-options/statuses", map[string]string{"id": "design", "token": usage.Token})
	assertStatus(t, status, 422, data)
	if testError(t, data) != "last_status" {
		t.Fatal("last active state was not protected")
	}
	state := newProductOption(t, c, "statuses", "唯一剩余状态")
	removeStatusForTest(t, c, "design", state.Value)
	status, data = c.request(t, "POST", "/api/tools", map[string]string{"chinese_name": "全部内置状态删除后创建"})
	assertStatus(t, status, 201, data)
	if decodeTestJSON[Product](t, data).DevelopmentStatus != state.Value {
		t.Fatal("new tool failed with only a custom state remaining")
	}
	anonymous := newTestClient(t, env.server)
	status, data = anonymous.request(t, "DELETE", "/api/product-options/statuses", map[string]string{"id": state.Value, "token": "token"})
	assertStatus(t, status, 401, data)
}
