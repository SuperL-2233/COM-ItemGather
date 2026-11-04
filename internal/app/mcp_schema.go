package app

func mcpObjectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

// Describe structured results explicitly so ChatGPT can import and validate
// the tools. Business record fields vary by kind and remain inside record.
func mcpOutputSchemas() map[string]any {
	text := map[string]any{"type": "string"}
	strings := map[string]any{"type": "array", "items": text}
	kind := map[string]any{"type": "string", "enum": mcpKinds}
	summary := mcpObjectSchema(map[string]any{
		"id": text, "kind": kind, "title": text, "summary": text, "url": text, "updated_at": text, "relations": strings,
	}, "id", "kind", "title", "summary", "url", "updated_at")
	page := mcpObjectSchema(map[string]any{
		"results":     map[string]any{"type": "array", "items": summary},
		"next_cursor": text,
		"limit":       map[string]any{"type": "integer", "minimum": 1, "maximum": 50},
	}, "results", "limit")
	choice := mcpObjectSchema(map[string]any{
		"value": text, "label": text, "color": text, "version": map[string]any{"type": "integer"},
	}, "value", "label", "version")
	choices := map[string]any{"type": "array", "items": choice}
	counts := map[string]any{}
	for _, name := range mcpKinds {
		counts[name] = map[string]any{"type": "integer", "minimum": 0}
	}
	return map[string]any{
		"search":       page,
		"list_records": page,
		"fetch": mcpObjectSchema(map[string]any{
			"id": text, "kind": kind, "title": text, "text": text, "url": text,
			"record": map[string]any{"type": "object"}, "related": page,
		}, "id", "kind", "title", "text", "url", "record", "related"),
		"get_library_info": mcpObjectSchema(map[string]any{
			"name": text, "counts": mcpObjectSchema(counts, mcpKinds...), "kinds": map[string]any{"type": "array", "items": kind},
			"categories": choices, "statuses": choices, "tags": strings, "use_categories": strings,
			"tags_truncated":    map[string]any{"type": "boolean"},
			"scope":             map[string]any{"type": "string", "const": mcpReadScope},
			"default_page_size": map[string]any{"type": "integer"},
			"max_page_size":     map[string]any{"type": "integer"},
			"fields":            map[string]any{"type": "object", "additionalProperties": text},
		}, "name", "counts", "kinds", "categories", "statuses", "tags", "use_categories", "tags_truncated", "scope", "default_page_size", "max_page_size", "fields"),
	}
}
