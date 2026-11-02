package app

import (
	"net/url"
	"slices"
	"testing"
)

func TestProductFilters(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	products := []map[string]any{
		{"chinese_name": "筛选目标", "company": "青苹果公司", "category": "饮料", "development_status": "design", "attributes": []Attribute{{Key: "口味", Value: "酸甜", Certainty: "asserted"}}},
		{"chinese_name": "公司相同分类不同", "company": "青苹果公司", "category": "罐头", "development_status": "completed", "attributes": []Attribute{{Key: "口感", Value: "爽脆", Certainty: "asserted"}}},
		{"chinese_name": "分类相同公司不同", "company": "其他公司", "category": "饮料", "development_status": "production", "attributes": []Attribute{{Key: "包装", Value: "瓶装", Certainty: "asserted"}}},
		{"chinese_name": "已删除商品", "company": "删除公司", "category": "删除分类", "development_status": "concept", "attributes": []Attribute{{Key: "删除属性", Value: "无", Certainty: "asserted"}}},
	}
	ids := make([]string, 0, len(products))
	for _, body := range products {
		status, data := client.request(t, "POST", "/api/products", body)
		assertStatus(t, status, 201, data)
		ids = append(ids, decodeTestJSON[Product](t, data).ID)
	}
	status, data := client.request(t, "DELETE", "/api/products/"+ids[3], map[string]int{"version": 1})
	assertStatus(t, status, 200, data)

	status, data = client.request(t, "GET", "/api/products/filters", nil)
	assertStatus(t, status, 200, data)
	options := decodeTestJSON[struct {
		Companies  []string `json:"companies"`
		Categories []string `json:"categories"`
		Attributes []string `json:"attributes"`
	}](t, data)
	if len(options.Companies) != 2 || len(options.Categories) != 2 || len(options.Attributes) != 3 ||
		!slices.Contains(options.Companies, "青苹果公司") || !slices.Contains(options.Categories, "饮料") || !slices.Contains(options.Attributes, "口味") ||
		slices.Contains(options.Companies, "删除公司") || slices.Contains(options.Categories, "删除分类") || slices.Contains(options.Attributes, "删除属性") {
		t.Fatalf("unexpected product filter options: %+v", options)
	}

	tests := []struct {
		query string
		total int
		name  string
	}{
		{"company=" + url.QueryEscape("青苹果公司"), 2, ""},
		{"category=" + url.QueryEscape("饮料"), 2, ""},
		{"status=design", 1, "筛选目标"},
		{"attribute=" + url.QueryEscape("口味"), 1, "筛选目标"},
		{"company=" + url.QueryEscape("青苹果公司") + "&category=" + url.QueryEscape("饮料") + "&status=design&attribute=" + url.QueryEscape("口味"), 1, "筛选目标"},
		{"company=" + url.QueryEscape("青苹果公司") + "&attribute=" + url.QueryEscape("包装"), 0, ""},
		{"company=" + url.QueryEscape("青苹果"), 0, ""},
	}
	for _, test := range tests {
		status, data = client.request(t, "GET", "/api/products?"+test.query, nil)
		assertStatus(t, status, 200, data)
		page := decodeTestJSON[struct {
			Items []Product `json:"items"`
			Total int       `json:"total"`
		}](t, data)
		if page.Total != test.total || len(page.Items) != test.total || test.name != "" && page.Items[0].ChineseName != test.name {
			t.Fatalf("unexpected products for %s: %+v", test.query, page)
		}
	}
}
