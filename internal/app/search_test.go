package app

import (
	"net/url"
	"testing"
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
