package app

import (
	"slices"
	"testing"
)

func optionStateForTest(t *testing.T, c *testClient, kind string) optionState {
	t.Helper()
	status, data := c.request(t, "GET", "/api/product-options/"+kind, nil)
	assertStatus(t, status, 200, data)
	return decodeTestJSON[optionState](t, data)
}

func TestOptionOrderIsSharedAndRejectsStaleOrInvalidLists(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	for _, kind := range []string{"categories", "statuses"} {
		t.Run(kind, func(t *testing.T) {
			for _, name := range []string{"排序乙", "排序甲", "排序丙"} {
				newProductOption(t, c, kind, name)
			}
			before := optionStateForTest(t, c, kind)
			last := before.Items[len(before.Items)-3:]
			if last[0].Label != "排序乙" || last[1].Label != "排序甲" || last[2].Label != "排序丙" {
				t.Fatal("new options did not append in creation order")
			}
			order := []string{}
			for _, item := range before.Items {
				order = append(order, item.Value)
			}
			slices.Reverse(order)
			status, data := c.request(t, "PATCH", "/api/product-options/"+kind+"/order", map[string]any{"order": order, "revision": before.Revision})
			assertStatus(t, status, 200, data)
			after := optionStateForTest(t, c, kind)
			got := []string{}
			for _, item := range after.Items {
				got = append(got, item.Value)
			}
			if !slices.Equal(got, order) {
				t.Fatalf("sort not retained: got %v want %v", got, order)
			}
			status, data = c.request(t, "PATCH", "/api/product-options/"+kind+"/order", map[string]any{"order": order, "revision": before.Revision})
			assertStatus(t, status, 409, data)
			if testError(t, data) != "option_changed" {
				t.Fatal("stale order overwrote shared options")
			}
			for _, invalid := range [][]string{order[:len(order)-1], append(slices.Clone(order[:len(order)-1]), order[0]), append(slices.Clone(order[:len(order)-1]), "missing")} {
				status, data = c.request(t, "PATCH", "/api/product-options/"+kind+"/order", map[string]any{"order": invalid, "revision": after.Revision})
				assertStatus(t, status, 422, data)
			}
		})
	}
	item := catalogItem(t, c, "products", "排序后创建", nil)
	if item.DevelopmentStatus != "concept" {
		t.Fatal("display order changed the default status")
	}
	for _, name := range []string{"排序甲", "排序乙", "排序丙"} {
		catalogItem(t, c, "products", name, map[string]any{"category": name})
	}
	status, data := c.request(t, "GET", "/api/products/filters", nil)
	assertStatus(t, status, 200, data)
	filters := decodeTestJSON[struct {
		Categories []string `json:"categories"`
	}](t, data)
	if !slices.Equal(filters.Categories, []string{"排序丙", "排序甲", "排序乙"}) {
		t.Fatalf("filters ignored category order: %v", filters.Categories)
	}
	anon := newTestClient(t, env.server)
	status, data = anon.request(t, "PATCH", "/api/product-options/statuses/order", map[string]any{})
	assertStatus(t, status, 401, data)
}

func TestStatusColorsAreSharedWithoutChangingRecordVersions(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	status, data := c.request(t, "POST", "/api/product-options/statuses", map[string]string{"name": "颜色验收", "color": "#089ABC"})
	assertStatus(t, status, 200, data)
	option := decodeTestJSON[struct {
		Option ChoiceOption `json:"option"`
	}](t, data).Option
	if option.Color != "#089abc" {
		t.Fatal("custom status color was not normalized")
	}
	product := catalogItem(t, c, "products", "颜色商品", map[string]any{"development_status": option.Value})
	group := catalogGroup(t, c, product)
	status, data = c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 1, DevelopmentStatus: option.Value})
	assertStatus(t, status, 201, data)
	recipe := decodeTestJSON[Recipe](t, data)
	if product.DevelopmentStatusColor != option.Color || recipe.DevelopmentStatusColor != option.Color {
		t.Fatal("record responses omitted shared color")
	}
	for _, invalid := range []string{"red", "#123", "#12345678", "#zzzzzz", "var(--text)"} {
		status, data = c.request(t, "PATCH", "/api/product-options/statuses/"+option.Value+"/color", map[string]any{"color": invalid, "version": option.Version})
		assertStatus(t, status, 422, data)
	}
	status, data = c.request(t, "PATCH", "/api/product-options/statuses/"+option.Value+"/color", map[string]any{"color": "#d14362", "version": option.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "PATCH", "/api/product-options/statuses/"+option.Value+"/color", map[string]any{"color": "#000000", "version": option.Version})
	assertStatus(t, status, 409, data)
	status, data = c.request(t, "GET", "/api/products/"+product.ID, nil)
	assertStatus(t, status, 200, data)
	updated := decodeTestJSON[Product](t, data)
	if updated.DevelopmentStatusColor != "#d14362" || updated.Version != product.Version {
		t.Fatal("color update changed record content/version or failed to propagate")
	}
	status, data = c.request(t, "GET", "/api/recipe-groups/"+group.ID, nil)
	assertStatus(t, status, 200, data)
	updatedGroup := decodeTestJSON[RecipeGroup](t, data)
	if updatedGroup.Recipes[0].DevelopmentStatusColor != "#d14362" || updatedGroup.Recipes[0].Version != recipe.Version {
		t.Fatal("recipe color/version is inconsistent")
	}
	removeStatusForTest(t, c, option.Value, "concept")
	status, data = c.request(t, "PATCH", "/api/product-options/statuses/"+option.Value+"/color", map[string]any{"color": "#000000", "version": option.Version})
	assertStatus(t, status, 404, data)
}
