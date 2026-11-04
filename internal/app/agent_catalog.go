package app

import (
	"encoding/json"
	"net/http"
)

func (a *App) agentCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := a.creativeCatalog(r.Context(), "", false)
	if err != nil {
		writeError(w, err)
		return
	}
	raw, err := json.Marshal(catalog)
	if err != nil {
		writeError(w, err)
		return
	}
	var snapshot map[string]any
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		writeError(w, err)
		return
	}
	sanitized, _ := sanitizeAgentCatalog(snapshot)
	snapshot = sanitized.(map[string]any)
	snapshot["schema_version"] = 1
	snapshot["generated_at"] = now()
	writeJSON(w, 200, snapshot)
}

func sanitizeAgentCatalog(value any) (any, bool) {
	switch item := value.(type) {
	case map[string]any:
		// Remove deleted records before stripping their deletion metadata, including
		// the products embedded in recipe outputs and ingredients.
		if deletedAt, _ := item["deleted_at"].(string); deletedAt != "" {
			return nil, false
		}
		for key, child := range item {
			switch key {
			case "client_id", "author_id", "author_nickname", "created_by", "ai_status", "ai_error", "favorite", "url", "thumbnail_url", "deleted_at":
				delete(item, key)
			default:
				if sanitized, keep := sanitizeAgentCatalog(child); keep {
					item[key] = sanitized
				} else {
					delete(item, key)
				}
			}
		}
	case []any:
		items := make([]any, 0, len(item))
		for _, child := range item {
			if sanitized, keep := sanitizeAgentCatalog(child); keep {
				items = append(items, sanitized)
			}
		}
		return items, true
	}
	return value, true
}
