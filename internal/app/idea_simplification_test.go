package app

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestAIResultClassificationContract(t *testing.T) {
	valid := AIResult{DisplayTitle: "测试标题", Summary: "", IdeaTypes: []string{"玩法", "包装", "玩法"}, ProductDirections: []string{"饮料"}}
	result, err := parseAIResult(encodeJSON(valid))
	if err != nil || !slices.Equal(result.IdeaTypes, []string{"玩法", "包装"}) || !slices.Equal(result.ProductDirections, []string{"饮料"}) {
		t.Fatalf("valid classifications rejected: %+v %v", result, err)
	}
	empty := `{"display_title":"绿色风暴","summary":"","idea_types":["名称"],"product_directions":[]}`
	if result, err = parseAIResult(empty); err != nil || len(result.ProductDirections) != 0 {
		t.Fatalf("unknown product direction must be allowed to stay empty: %+v %v", result, err)
	}
	for name, fields := range map[string]map[string]any{
		"unknown idea type":         {"idea_types": []string{"饮料"}},
		"unknown product direction": {"product_directions": []string{"青苹果"}},
		"free form idea type":       {"idea_types": []string{"玩法设计"}},
		"null idea types":           {"idea_types": nil},
		"null directions":           {"product_directions": nil},
		"invalid idea type":         {"idea_types": "玩法"},
		"retired names":             {"name_candidates": []string{}},
		"retired tags":              {"tags": []string{}},
		"retired attributes":        {"attributes": []Attribute{}},
	} {
		t.Run(name, func(t *testing.T) {
			payload := map[string]any{"display_title": "标题", "summary": "", "idea_types": []string{}, "product_directions": []string{}}
			for key, value := range fields {
				payload[key] = value
			}
			if _, err := parseAIResult(encodeJSON(payload)); err == nil {
				t.Fatalf("invalid AI classification accepted: %s", encodeJSON(payload))
			}
		})
	}
	if _, err = parseAIResult(empty + ` {"extra":true}`); err == nil {
		t.Fatal("multiple AI objects accepted")
	}
}

func TestRetiredIdeaFieldsAreAbsentAndCannotBeWritten(t *testing.T) {
	env := newTestEnvironment(t, nil)
	client := newAdminClient(t, env)
	idea := createTestIdea(t, client, "simplified", "精简灵感验收")
	idea = waitAIStatus(t, client, idea.ID, "succeeded")
	assertNoRetiredFields := func(raw json.RawMessage) {
		t.Helper()
		fields := decodeTestJSON[map[string]json.RawMessage](t, raw)
		for _, field := range []string{"name_candidates", "tags", "attributes"} {
			if _, exists := fields[field]; exists {
				t.Fatalf("idea still exposes %s: %s", field, raw)
			}
		}
	}
	for _, path := range []string{"/api/ideas/" + idea.ID, "/api/ideas", "/api/search?kind=ideas", "/api/export"} {
		status, data := client.request(t, "GET", path, nil)
		assertStatus(t, status, 200, data)
		if path == "/api/ideas/"+idea.ID {
			assertNoRetiredFields(data)
			continue
		}
		result := decodeTestJSON[map[string]json.RawMessage](t, data)
		key := "ideas"
		if path == "/api/ideas" {
			key = "items"
		}
		items := decodeTestJSON[[]json.RawMessage](t, result[key])
		if len(items) != 1 {
			t.Fatalf("expected one idea from %s: %s", path, data)
		}
		assertNoRetiredFields(items[0])
	}
	for _, field := range []string{"name_candidates", "tags", "attributes"} {
		status, data := client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, field: []string{}})
		assertStatus(t, status, 400, data)
	}
	status, data := client.request(t, "GET", "/api/tags", nil)
	assertStatus(t, status, 404, data)
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	for _, revision := range decodeTestJSON[map[string][]Revision](t, data)["items"] {
		assertNoRetiredFields(revision.After)
	}
}
