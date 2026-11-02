package app

import (
	"net/url"
	"testing"
)

func categoryPreview(t *testing.T, c *testClient, name string) categoryUsage {
	t.Helper()
	status, data := c.request(t, "GET", "/api/product-options/categories/usage?name="+url.QueryEscape(name), nil)
	assertStatus(t, status, 200, data)
	return decodeTestJSON[categoryUsage](t, data)
}
func categoryDelete(t *testing.T, c *testClient, usage categoryUsage, replacement string) (int, []byte) {
	t.Helper()
	return c.request(t, "DELETE", "/api/product-options/categories", map[string]string{"name": usage.Name, "token": usage.Token, "replacement": replacement})
}
func TestUnusedCategoryDeletionAndExplicitRecreation(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	name := newProductOption(t, c, "categories", "待删除 / 分类?").Value
	preview := categoryPreview(t, c, name)
	status, data := categoryDelete(t, c, preview, "")
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/product-options/categories", nil)
	assertStatus(t, status, 200, data)
	if len(decodeTestJSON[struct {
		Items []ChoiceOption `json:"items"`
	}](t, data).Items) != 0 {
		t.Fatal("deleted option remains visible")
	}
	status, data = c.request(t, "POST", "/api/products", map[string]string{"category": name})
	assertStatus(t, status, 409, data)
	if testError(t, data) != "category_deleted" {
		t.Fatal("stale form recreated a deleted category")
	}
	newProductOption(t, c, "categories", name)
	revived := categoryPreview(t, c, name)
	if revived.Token == preview.Token {
		t.Fatal("delete/recreate reused an old deletion token")
	}
	status, data = categoryDelete(t, c, preview, "")
	assertStatus(t, status, 409, data)
	status, data = categoryDelete(t, c, revived, "")
	assertStatus(t, status, 200, data)
	anonymous := newTestClient(t, env.server)
	status, data = categoryDelete(t, anonymous, revived, "")
	assertStatus(t, status, 401, data)
}
func TestCategoryDeletionClearsAllItemsAndPreservesHistory(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	product := catalogItem(t, c, "products", "清分类商品", map[string]any{"category": "删除商品类别", "description": "保留说明"})
	tool := catalogItem(t, c, "tools", "清分类道具", map[string]any{"category": "删除商品类别"})
	status, data := c.request(t, "DELETE", "/api/tools/"+tool.ID, map[string]int{"version": tool.Version})
	assertStatus(t, status, 200, data)
	preview := categoryPreview(t, c, product.Category)
	if preview.ItemCount != 2 {
		t.Fatal("archived item was omitted from usage")
	}
	status, data = categoryDelete(t, c, preview, "")
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/products/"+product.ID, nil)
	assertStatus(t, status, 200, data)
	after := decodeTestJSON[Product](t, data)
	if after.Category != "" || after.Description != "保留说明" || after.Version != product.Version+1 {
		t.Fatal("deletion did not preserve other item fields and version")
	}
	status, data = c.request(t, "GET", "/api/trash", nil)
	assertStatus(t, status, 200, data)
	trash := decodeTestJSON[struct {
		Tools []Product `json:"tools"`
	}](t, data)
	if len(trash.Tools) != 1 || trash.Tools[0].Category != "" || trash.Tools[0].Version != tool.Version+2 {
		t.Fatal("deleted tool classification was not cleared")
	}
	status, data = c.request(t, "PATCH", "/api/products/"+product.ID, map[string]any{"version": product.Version, "category": product.Category})
	assertStatus(t, status, 409, data)
	status, data = c.request(t, "GET", "/api/products/"+product.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[struct {
		Items []Revision `json:"items"`
	}](t, data).Items
	if revisions[0].Action != "remove_category" {
		t.Fatal("classification removal lacks history")
	}
	before := decodeTestJSON[productFields](t, revisions[0].Before)
	if before.Category != product.Category {
		t.Fatal("old category was not preserved in history")
	}
	status, data = c.request(t, "POST", "/api/products/"+product.ID+"/revisions/"+revisions[len(revisions)-1].ID+"/restore", map[string]int{"version": after.Version})
	assertStatus(t, status, 409, data)
}
func TestCategoryReplacementUpdatesRecipesAndMergesQuantities(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	source := newProductOption(t, c, "categories", "旧材料类").Value
	target := newProductOption(t, c, "categories", "新材料类").Value
	output := catalogItem(t, c, "products", "合成分类产物", map[string]any{"category": source})
	group := catalogGroup(t, c, output)
	status, data := c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 1, Ingredients: []Ingredient{{Category: source, Quantity: 2}, {Category: target, Quantity: 3}}})
	assertStatus(t, status, 201, data)
	recipe := decodeTestJSON[Recipe](t, data)
	preview := categoryPreview(t, c, source)
	if preview.RecipeCount != 1 || preview.ItemCount != 1 {
		t.Fatalf("wrong usage: %+v", preview)
	}
	status, data = categoryDelete(t, c, preview, "")
	assertStatus(t, status, 422, data)
	status, data = categoryDelete(t, c, preview, source)
	assertStatus(t, status, 422, data)
	status, data = categoryDelete(t, c, preview, "不存在的类别")
	assertStatus(t, status, 422, data)
	status, data = categoryDelete(t, c, preview, target)
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/recipe-groups/"+group.ID, nil)
	assertStatus(t, status, 200, data)
	after := decodeTestJSON[RecipeGroup](t, data)
	if after.Output.Category != target || len(after.Recipes[0].Ingredients) != 1 || after.Recipes[0].Ingredients[0].Quantity != 5 || after.Recipes[0].Ingredients[0].Category != target || after.Recipes[0].Version != recipe.Version+1 {
		t.Fatalf("replacement not consistent: %s", data)
	}
	status, data = c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 1, Ingredients: []Ingredient{{Category: source, Quantity: 1}}})
	assertStatus(t, status, 409, data)
	status, data = c.request(t, "GET", "/api/recipes/"+recipe.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	if decodeTestJSON[struct {
		Items []Revision `json:"items"`
	}](t, data).Items[0].Action != "replace_category" {
		t.Fatal("recipe replacement lacks history")
	}
}
func TestCategoryDeletionRequiresCurrentUsageAndRollsBackOnOverflow(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	source := newProductOption(t, c, "categories", "并发类别").Value
	target := newProductOption(t, c, "categories", "替代类别").Value
	output := catalogItem(t, c, "products", "并发类别商品", map[string]any{"category": source})
	preview := categoryPreview(t, c, source)
	status, data := c.request(t, "PATCH", "/api/products/"+output.ID, map[string]any{"version": output.Version, "description": "刚更新的说明"})
	assertStatus(t, status, 200, data)
	status, data = categoryDelete(t, c, preview, "")
	assertStatus(t, status, 409, data)
	if testError(t, data) != "category_changed" {
		t.Fatal("usage conflict omitted")
	}
	group := catalogGroup(t, c, output)
	status, data = c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 1, Ingredients: []Ingredient{{Category: source, Quantity: 1000000000}, {Category: target, Quantity: 1}}})
	assertStatus(t, status, 201, data)
	preview = categoryPreview(t, c, source)
	status, data = categoryDelete(t, c, preview, target)
	assertStatus(t, status, 422, data)
	if testError(t, data) != "quantity_overflow" {
		t.Fatal("ingredient overflow was ignored")
	}
	status, data = c.request(t, "GET", "/api/products/"+output.ID, nil)
	assertStatus(t, status, 200, data)
	unchanged := decodeTestJSON[Product](t, data)
	if unchanged.Category != source || unchanged.Version != output.Version+1 || unchanged.Description != "刚更新的说明" {
		t.Fatal("failed deletion partially updated an item")
	}
	if fresh := categoryPreview(t, c, source); fresh.Token != preview.Token {
		t.Fatal("failed transaction changed the category or references")
	}
}

func TestCategoryReplacementMatchesDatabaseIdentity(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	source := newProductOption(t, c, "categories", "Фрукты").Value
	other := newProductOption(t, c, "categories", "фрукты").Value
	target := newProductOption(t, c, "categories", "TARGET").Value
	output := catalogItem(t, c, "products", "分类名称匹配产物", nil)
	group := catalogGroup(t, c, output)
	status, data := c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 1, Ingredients: []Ingredient{{Category: source, Quantity: 2}, {Category: other, Quantity: 3}, {Category: target, Quantity: 4}}})
	assertStatus(t, status, 201, data)
	if _, err := env.db.Exec("UPDATE recipe_ingredients SET category='target' WHERE category='TARGET'"); err != nil {
		t.Fatal(err)
	}
	status, data = categoryDelete(t, c, categoryPreview(t, c, source), target)
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/recipe-groups/"+group.ID, nil)
	assertStatus(t, status, 200, data)
	after := decodeTestJSON[RecipeGroup](t, data)
	if len(after.Recipes[0].Ingredients) != 2 || after.Recipes[0].Ingredients[0].Category != target || after.Recipes[0].Ingredients[0].Quantity != 6 || after.Recipes[0].Ingredients[1].Category != other {
		t.Fatal("replacement changed a different category")
	}
}
