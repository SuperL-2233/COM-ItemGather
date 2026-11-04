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
	removeAgentPrivateFields(snapshot)
	snapshot["schema_version"] = 1
	snapshot["generated_at"] = now()
	writeJSON(w, 200, snapshot)
}

func removeAgentPrivateFields(value any) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			switch key {
			case "client_id", "author_id", "author_nickname", "created_by", "ai_status", "ai_error", "favorite", "url", "thumbnail_url", "deleted_at":
				delete(item, key)
			default:
				removeAgentPrivateFields(child)
			}
		}
	case []any:
		for _, child := range item {
			removeAgentPrivateFields(child)
		}
	}
}
