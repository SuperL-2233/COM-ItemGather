package app

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSearchFilters(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	admin := newAdminClient(t, environment)
	idea := createTestIdea(t, admin, "search-filters", "绿色风暴")
	waitAIStatus(t, admin, idea.ID, "succeeded")
	createTestUser(t, environment.db, "author-two", "member")
	member := newTestClient(t, environment.server)
	member.login(t, "author-two", "correct-horse-42")
	createTestIdea(t, member, "other-author", "报纸包装")
	for _, client := range []*testClient{admin, member} {
		status, data := client.request(t, "POST", "/api/products", map[string]string{"chinese_name": "按创建者搜索的商品"})
		assertStatus(t, status, 201, data)
	}
	for _, kind := range []string{"all", "products"} {
		status, data := admin.request(t, "GET", "/api/search?author="+member.user.ID+"&kind="+kind, nil)
		assertStatus(t, status, 200, data)
		products := decodeTestJSON[struct {
			Products []Product `json:"products"`
		}](t, data).Products
		if len(products) != 1 || products[0].CreatedBy != member.user.ID {
			t.Fatalf("product creator filter failed for %s: %+v", kind, products)
		}
	}

	status, data := admin.request(t, "GET", "/api/search?q="+url.QueryEscape("青苹果")+"&kind=ideas&idea_type="+url.QueryEscape("名称")+"&direction="+url.QueryEscape("饮料")+"&author="+admin.user.ID, nil)
	assertStatus(t, status, 200, data)
	result := decodeTestJSON[struct {
		Ideas    []Idea    `json:"ideas"`
		Products []Product `json:"products"`
		Creators []User    `json:"creators"`
	}](t, data)
	if len(result.Ideas) != 1 || result.Ideas[0].ID != idea.ID || len(result.Products) != 0 || len(result.Creators) != 2 {
		t.Fatalf("unexpected filtered results %+v", result)
	}
	status, data = admin.request(t, "GET", "/api/search?kind=ideas&author="+member.user.ID, nil)
	assertStatus(t, status, 200, data)
	result = decodeTestJSON[struct {
		Ideas    []Idea    `json:"ideas"`
		Products []Product `json:"products"`
		Creators []User    `json:"creators"`
	}](t, data)
	if len(result.Ideas) != 1 || result.Ideas[0].AuthorID != member.user.ID {
		t.Fatal("creator filter did not find the other user's idea")
	}
	status, data = admin.request(t, "GET", "/api/search?q="+url.QueryEscape("绿色")+"&kind=products", nil)
	assertStatus(t, status, 200, data)
	result = decodeTestJSON[struct {
		Ideas    []Idea    `json:"ideas"`
		Products []Product `json:"products"`
		Creators []User    `json:"creators"`
	}](t, data)
	if len(result.Ideas) != 0 || len(result.Products) != 0 {
		t.Fatal("record type filter returned the wrong entity")
	}
}

func TestSearchMultiSelectAndToolFilters(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	admin := newAdminClient(t, environment)
	createTestUser(t, environment.db, "search-member", "member")
	member := newTestClient(t, environment.server)
	member.login(t, "search-member", "correct-horse-42")
	for index, client := range []*testClient{admin, member} {
		status, data := client.request(t, "POST", "/api/products", map[string]string{"chinese_name": "多选搜索商品"})
		assertStatus(t, status, 201, data)
		use, state, source := "主动", "concept", "购买"
		if index == 1 {
			use, state, source = "被动", "design", "任务奖励"
		}
		status, data = client.request(t, "POST", "/api/tools", map[string]any{"chinese_name": "多选搜索道具", "use_category": use, "development_status": state, "tool_settings": ToolSettings{Sources: []AcquisitionSource{{Method: source}, {Method: "NPC 赠送"}}}})
		assertStatus(t, status, 201, data)
		idea := createTestIdea(t, client, fmt.Sprintf("multi-search-%d", index), "多选搜索灵感")
		waitAIStatus(t, client, idea.ID, "succeeded")
		if index == 1 {
			if _, err := environment.db.Exec(`UPDATE ideas SET idea_types_json='["世界观"]',product_directions_json='["零食"]' WHERE id=?`, idea.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	parameters := url.Values{"q": {"多选搜索"}, "kind": {"products", "tools"}, "author": {admin.user.ID, member.user.ID}}
	status, data := admin.request(t, "GET", "/api/search?"+parameters.Encode(), nil)
	assertStatus(t, status, 200, data)
	result := decodeTestJSON[struct {
		Products, Tools []Product
		Ideas           []Idea
	}](t, data)
	if len(result.Products) != 2 || len(result.Tools) != 2 || len(result.Ideas) != 0 {
		t.Fatalf("record types/authors should OR within a group: %s", data)
	}
	parameters.Set("kind", "ideas")
	parameters["idea_type"] = []string{"名称", "世界观"}
	parameters["direction"] = []string{"饮料", "零食"}
	status, data = admin.request(t, "GET", "/api/search?"+parameters.Encode(), nil)
	assertStatus(t, status, 200, data)
	result = decodeTestJSON[struct {
		Products, Tools []Product
		Ideas           []Idea
	}](t, data)
	if len(result.Ideas) != 2 {
		t.Fatalf("idea metadata should allow multiple values: %s", data)
	}
	parameters.Set("idea_type", "名称")
	parameters.Set("direction", "零食")
	status, data = admin.request(t, "GET", "/api/search?"+parameters.Encode(), nil)
	assertStatus(t, status, 200, data)
	result = decodeTestJSON[struct {
		Products, Tools []Product
		Ideas           []Idea
	}](t, data)
	if len(result.Ideas) != 0 {
		t.Fatal("different filter groups must AND")
	}
	parameters = url.Values{"q": {"多选搜索"}, "use_category": {"主动", "被动"}, "source": {"购买", "任务奖励", "NPC 赠送"}, "status": {"concept", "design"}}
	status, data = admin.request(t, "GET", "/api/tools?"+parameters.Encode(), nil)
	assertStatus(t, status, 200, data)
	page := decodeTestJSON[struct {
		Items []Product
		Total int
	}](t, data)
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("tool filter OR should not duplicate matching sources: %s", data)
	}
	parameters.Set("source", "购买")
	status, data = admin.request(t, "GET", "/api/tools?"+parameters.Encode(), nil)
	assertStatus(t, status, 200, data)
	page = decodeTestJSON[struct {
		Items []Product
		Total int
	}](t, data)
	if page.Total != 1 || page.Items[0].UseCategory != "主动" {
		t.Fatalf("tool filters should AND across groups: %s", data)
	}
	status, data = admin.request(t, "GET", "/api/search?kind=products&kind=unknown", nil)
	assertStatus(t, status, 422, data)
	status, data = admin.request(t, "GET", "/api/search?"+strings.Repeat("author=x&", 51), nil)
	assertStatus(t, status, 422, data)
}

func TestLiveSearchBoundsAndRecipeSummaries(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	admin := newAdminClient(t, environment)
	tx, err := environment.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	stamp := now()
	// A library with 1,000 records of each kind, and two recipes per output.
	for index := 0; index < 1000; index++ {
		id := fmt.Sprintf("live-search-%04d", index)
		for _, entry := range []struct {
			query string
			args  []any
		}{
			{`INSERT INTO ideas(id,content,author_id,created_at,updated_at) VALUES(?,?,?,?,?)`, []any{id, "性能搜索样本", admin.user.ID, stamp, stamp}},
			{`INSERT INTO companies(id,chinese_name,created_by,created_at,updated_at) VALUES(?,?,?,?,?)`, []any{id, "性能搜索样本", admin.user.ID, stamp, stamp}},
			{`INSERT INTO products(id,chinese_name,item_kind,created_by,created_at,updated_at) VALUES(?,?,'product',?,?,?)`, []any{id, "性能搜索样本", admin.user.ID, stamp, stamp}},
			{`INSERT INTO products(id,chinese_name,item_kind,created_by,created_at,updated_at) VALUES(?,?,'tool',?,?,?)`, []any{id + "-tool", "性能搜索样本", admin.user.ID, stamp, stamp}},
			{`INSERT INTO recipe_groups(id,output_id,created_by,created_at,updated_at) VALUES(?,?,?,?,?)`, []any{id, id, admin.user.ID, stamp, stamp}},
			{`INSERT INTO recipes(id,group_id,notes,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?)`, []any{id, id, "独特材料检索", admin.user.ID, stamp, stamp}},
			{`INSERT INTO recipes(id,group_id,created_by,created_at,updated_at) VALUES(?,?,?,?,?)`, []any{id + "-recipe", id, admin.user.ID, stamp, stamp}},
			{`INSERT INTO recipe_ingredients(id,recipe_id,product_id,quantity,position) VALUES(?,?,?,1,0)`, []any{id, id, id + "-tool"}},
		} {
			if _, err = tx.Exec(entry.query, entry.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	status, data := admin.request(t, "GET", "/api/search?q="+url.QueryEscape("性能搜索样本"), nil)
	assertStatus(t, status, 200, data)
	t.Logf("global live search across 5,000 entries: %s, response %d bytes", time.Since(start), len(data))
	result := decodeTestJSON[struct {
		Ideas           []Idea
		Products, Tools []Product
		Companies       []Company
		Recipes         []RecipeGroup
		HasMore         map[string]bool `json:"has_more"`
	}](t, data)
	for kind, count := range map[string]int{"ideas": len(result.Ideas), "products": len(result.Products), "tools": len(result.Tools), "companies": len(result.Companies), "recipes": len(result.Recipes)} {
		if count != searchResultLimit || !result.HasMore[kind] {
			t.Fatalf("%s was not bounded: count=%d more=%v", kind, count, result.HasMore)
		}
	}
	for _, group := range result.Recipes {
		if group.RecipeCount == nil || *group.RecipeCount != 2 || len(group.Recipes) != 0 {
			t.Fatalf("search should return correct recipe counts without ingredient graphs: %+v", group)
		}
	}
	status, data = admin.request(t, "GET", "/api/search?kind=recipes&q="+url.QueryEscape("独特材料检索"), nil)
	assertStatus(t, status, 200, data)
	if len(decodeTestJSON[struct{ Recipes []RecipeGroup }](t, data).Recipes) != searchResultLimit {
		t.Fatal("summary search lost recipe metadata matching")
	}
	status, data = admin.request(t, "GET", "/api/recipe-groups/live-search-0000", nil)
	assertStatus(t, status, 200, data)
	group := decodeTestJSON[RecipeGroup](t, data)
	if len(group.Recipes) != 2 || len(group.Recipes[0].Ingredients) != 1 {
		t.Fatal("detail should still include complete recipes and materials")
	}
	status, data = admin.request(t, "GET", "/api/search?q="+url.QueryEscape("不存在的搜索关键词"), nil)
	assertStatus(t, status, 200, data)
	result = decodeTestJSON[struct {
		Ideas           []Idea
		Products, Tools []Product
		Companies       []Company
		Recipes         []RecipeGroup
		HasMore         map[string]bool `json:"has_more"`
	}](t, data)
	if len(result.Ideas)+len(result.Products)+len(result.Tools)+len(result.Companies)+len(result.Recipes) != 0 {
		t.Fatal("unmatched query should be empty")
	}
}

func TestAPIRateLimit(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	environment.app.apiLimiter = newRequestLimiter(3)
	client := newAdminClient(t, environment)
	status, data := client.request(t, "GET", "/api/auth/me", nil)
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/ideas", nil)
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/ideas", nil)
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/ideas", nil)
	assertStatus(t, status, 429, data)
	if testError(t, data) != "rate_limited" {
		t.Fatal("expected general API rate limit")
	}
}
