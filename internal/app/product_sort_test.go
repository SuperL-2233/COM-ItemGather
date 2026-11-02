package app

import (
	"fmt"
	"net/url"
	"slices"
	"testing"
)

func TestProductSorting(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	timestamps := []string{
		"2026-01-01T00:00:00Z",
		"2026-01-01T00:00:00.1Z",
		"2026-01-01T00:00:00.100001Z",
		"2026-01-01T00:00:00.11Z",
		"2026-01-01T00:00:00.110001Z",
		"2026-01-01T00:00:00.12Z",
	}
	fixtures := []struct {
		chinese, english string
		created, updated int
	}{
		{"阿梨2", "", 3, 0},
		{"波板糖", "", 0, 3},
		{"橙汽水", "", 1, 2},
		{"", "English Only", 4, 5},
		{"阿梨10", "", 2, 1},
		{"", "", 5, 4},
	}
	ids := []string{}
	for _, fixture := range fixtures {
		status, data := client.request(t, "POST", "/api/products", map[string]any{
			"chinese_name": fixture.chinese, "english_name": fixture.english,
			"description": "sorting-fixture", "category": "排序测试",
		})
		assertStatus(t, status, 201, data)
		id := decodeTestJSON[Product](t, data).ID
		ids = append(ids, id)
		if _, err := environment.db.Exec("UPDATE products SET created_at=?,updated_at=? WHERE id=?", timestamps[fixture.created], timestamps[fixture.updated], id); err != nil {
			t.Fatal(err)
		}
	}
	for _, collection := range []string{"products", "tools"} {
		status, data := client.request(t, "POST", "/api/"+collection, map[string]string{"chinese_name": "不应混入的物品", "description": "sorting-fixture", "category": "排序测试"})
		assertStatus(t, status, 201, data)
		if collection == "products" {
			status, data = client.request(t, "DELETE", "/api/products/"+decodeTestJSON[Product](t, data).ID, map[string]int{"version": 1})
			assertStatus(t, status, 200, data)
		}
	}
	status, data := client.request(t, "POST", "/api/products", map[string]string{"chinese_name": "其他分类", "description": "sorting-fixture", "category": "其他"})
	assertStatus(t, status, 201, data)

	assertPages := func(t *testing.T, order string, want []string) {
		t.Helper()
		got := []string{}
		for offset := 0; offset < len(want); offset += 2 {
			path := fmt.Sprintf("/api/products?sort=%s&q=sorting-fixture&category=%s&status=concept&limit=2&offset=%d", url.QueryEscape(order), url.QueryEscape("排序测试"), offset)
			status, data := client.request(t, "GET", path, nil)
			assertStatus(t, status, 200, data)
			page := decodeTestJSON[struct {
				Items []Product `json:"items"`
				Total int       `json:"total"`
			}](t, data)
			if page.Total != len(want) || len(page.Items) != 2 {
				t.Fatalf("unexpected page for %s offset %d: %+v", order, offset, page)
			}
			for _, item := range page.Items {
				got = append(got, item.ID)
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("sort %s: got %v, want %v", order, got, want)
		}
	}
	for _, test := range []struct {
		order string
		want  []int
	}{
		{"name_asc", []int{0, 4, 1, 2, 3, 5}},
		{"name_desc", []int{2, 1, 4, 0, 3, 5}},
		{"created_asc", []int{1, 2, 4, 0, 3, 5}},
		{"created_desc", []int{5, 3, 0, 4, 2, 1}},
		{"updated_asc", []int{0, 4, 2, 1, 5, 3}},
		{"updated_desc", []int{3, 5, 1, 2, 4, 0}},
		{"", []int{5, 3, 0, 4, 2, 1}},
	} {
		t.Run(test.order, func(t *testing.T) {
			want := []string{}
			for _, index := range test.want {
				want = append(want, ids[index])
			}
			assertPages(t, test.order, want)
		})
	}
	t.Run("ties stay stable across pages", func(t *testing.T) {
		for _, id := range ids {
			if _, err := environment.db.Exec("UPDATE products SET chinese_name='同名商品',english_name='',created_at=?,updated_at=? WHERE id=?", timestamps[0], timestamps[0], id); err != nil {
				t.Fatal(err)
			}
		}
		want := slices.Clone(ids)
		slices.Sort(want)
		for _, order := range []string{"name_asc", "created_asc", "updated_asc"} {
			assertPages(t, order, want)
		}
		slices.Reverse(want)
		for _, order := range []string{"name_desc", "created_desc", "updated_desc"} {
			assertPages(t, order, want)
		}
	})
	for _, order := range []string{"unknown", "p.created_at DESC; DROP TABLE products"} {
		status, data := client.request(t, "GET", "/api/products?sort="+url.QueryEscape(order), nil)
		assertStatus(t, status, 400, data)
		if testError(t, data) != "invalid_sort" {
			t.Fatalf("unexpected error: %s", data)
		}
	}
	status, data = client.request(t, "GET", "/api/products", nil)
	assertStatus(t, status, 200, data)
}
