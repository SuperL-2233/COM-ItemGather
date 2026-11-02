package app

import (
	"com-itemgather/internal/security"
	"net/http"
)

type settingLink struct {
	ID         string `json:"id"`
	IdeaID     string `json:"idea_id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Status     string `json:"status"`
	Title      string `json:"title"`
	Path       string `json:"path"`
}

func (a *App) listSettingLinks(w http.ResponseWriter, r *http.Request) {
	ideaID := r.URL.Query().Get("idea_id")
	kind := r.URL.Query().Get("entity_type")
	id := r.URL.Query().Get("entity_id")
	if ideaID == "" && (id == "" || (kind != "company" && kind != "recipe")) {
		writeError(w, fail(422, "validation", "关联对象无效"))
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT l.id,l.idea_id,l.entity_type,l.entity_id,l.status,COALESCE(NULLIF(i.display_title,''),substr(i.content,1,80)) FROM setting_idea_links l JOIN ideas i ON i.id=l.idea_id WHERE i.deleted_at IS NULL AND ((?<>'' AND l.idea_id=?) OR (?='' AND l.entity_type=? AND l.entity_id=?)) ORDER BY l.created_at DESC`, ideaID, ideaID, ideaID, kind, id)
	if err != nil {
		writeError(w, err)
		return
	}
	items := []settingLink{}
	for rows.Next() {
		var l settingLink
		if err = rows.Scan(&l.ID, &l.IdeaID, &l.EntityType, &l.EntityID, &l.Status, &l.Title); err != nil {
			rows.Close()
			writeError(w, err)
			return
		}
		l.Path = "/ideas/" + l.IdeaID
		items = append(items, l)
	}
	rows.Close()
	visible := []settingLink{}
	for _, l := range items {
		if l.EntityType == "company" {
			c, e := loadCompany(r.Context(), a.db, l.EntityID, false)
			if e != nil {
				continue
			}
			if ideaID != "" {
				l.Title = companyName(c)
				l.Path = "/companies/" + c.ID
			}
		} else {
			recipe, e := loadRecipe(r.Context(), a.db, l.EntityID, false)
			if e != nil {
				continue
			}
			g, e := loadRecipeGroup(r.Context(), a.db, recipe.GroupID, false)
			if e != nil {
				continue
			}
			if ideaID != "" {
				l.Title = g.Output.ChineseName
				if l.Title == "" {
					l.Title = g.Output.EnglishName
				}
				if l.Title == "" {
					l.Title = "未命名产物"
				}
				l.Title += " · " + recipe.Name
				l.Path = "/recipes/" + g.ID + "#" + recipe.ID
			}
		}
		visible = append(visible, l)
	}
	writeJSON(w, 200, map[string]any{"items": visible})
}
func (a *App) createSettingLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IdeaID     string `json:"idea_id"`
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Status     string `json:"status"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.EntityType != "company" && body.EntityType != "recipe" {
		writeError(w, fail(422, "validation", "关联对象无效"))
		return
	}
	if body.Status == "" {
		body.Status = "candidate"
	}
	if !validLinkStatus(body.Status) {
		writeError(w, fail(422, "validation", "关联状态无效"))
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	var count int
	err = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM ideas WHERE id=? AND deleted_at IS NULL", body.IdeaID).Scan(&count)
	if err != nil {
		writeError(w, err)
		return
	}
	if count != 1 {
		writeError(w, fail(404, "not_found", "灵感不存在"))
		return
	}
	if body.EntityType == "company" {
		_, err = loadCompany(r.Context(), tx, body.EntityID, false)
	} else {
		var rec Recipe
		rec, err = loadRecipe(r.Context(), tx, body.EntityID, false)
		if err == nil {
			_, err = loadRecipeGroup(r.Context(), tx, rec.GroupID, false)
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	var exists int
	err = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM setting_idea_links WHERE idea_id=? AND entity_type=? AND entity_id=?", body.IdeaID, body.EntityType, body.EntityID).Scan(&exists)
	if err != nil {
		writeError(w, err)
		return
	}
	if exists > 0 {
		writeError(w, fail(409, "link_exists", "已有关联"))
		return
	}
	id := security.ID()
	_, err = tx.ExecContext(r.Context(), "INSERT INTO setting_idea_links(id,idea_id,entity_type,entity_id,status,created_at) VALUES(?,?,?,?,?,?)", id, body.IdeaID, body.EntityType, body.EntityID, body.Status, now())
	if err == nil {
		err = a.publish(tx, "link.changed", "setting_link", id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 201, map[string]any{"id": id})
}
func (a *App) updateSettingLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if !validLinkStatus(body.Status) {
		writeError(w, fail(422, "validation", "关联状态无效"))
		return
	}
	result, err := a.db.ExecContext(r.Context(), "UPDATE setting_idea_links SET status=? WHERE id=?", body.Status, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		writeError(w, fail(404, "not_found", "关联不存在"))
		return
	}
	a.emit("link.changed", "setting_link", "")
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) deleteSettingLink(w http.ResponseWriter, r *http.Request) {
	result, err := a.db.ExecContext(r.Context(), "DELETE FROM setting_idea_links WHERE id=?", r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		writeError(w, fail(404, "not_found", "关联不存在"))
		return
	}
	a.emit("link.changed", "setting_link", "")
	writeJSON(w, 200, map[string]any{"ok": true})
}
