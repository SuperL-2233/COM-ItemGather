package app

import (
	"net/http"

	"com-itemgather/internal/security"
)

func (a *App) createLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IdeaID    string `json:"idea_id"`
		ProductID string `json:"product_id"`
		Status    string `json:"status"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Status == "" {
		body.Status = "candidate"
	}
	if !validLinkStatus(body.Status) {
		writeError(w, fail(422, "validation", "关联状态无效"))
		return
	}
	var count int
	a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM ideas i,products p WHERE i.id=? AND p.id=? AND i.deleted_at IS NULL AND p.deleted_at IS NULL", body.IdeaID, body.ProductID).Scan(&count)
	if count != 1 {
		writeError(w, fail(404, "not_found", "灵感或商品不存在"))
		return
	}
	id := security.ID()
	timestamp := now()
	_, err := a.db.ExecContext(r.Context(), "INSERT INTO idea_product_links(id,idea_id,product_id,status,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", id, body.IdeaID, body.ProductID, body.Status, currentUser(r).ID, timestamp, timestamp)
	if err != nil {
		writeError(w, fail(409, "link_exists", "这条灵感已经关联到该商品"))
		return
	}
	a.emit("link.changed", "link", "")
	writeJSON(w, 201, map[string]any{"id": id, "idea_id": body.IdeaID, "product_id": body.ProductID, "status": body.Status, "created_at": timestamp, "updated_at": timestamp})
}
func validLinkStatus(status string) bool {
	return status == "candidate" || status == "adopted" || status == "rejected"
}
func (a *App) updateLink(w http.ResponseWriter, r *http.Request) {
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
	result, err := a.db.ExecContext(r.Context(), "UPDATE idea_product_links SET status=?,updated_at=? WHERE id=?", body.Status, now(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(404, "not_found", "关联不存在"))
		return
	}
	a.emit("link.changed", "link", "")
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) deleteLink(w http.ResponseWriter, r *http.Request) {
	result, err := a.db.ExecContext(r.Context(), "DELETE FROM idea_product_links WHERE id=?", r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(404, "not_found", "关联不存在"))
		return
	}
	a.emit("link.changed", "link", "")
	writeJSON(w, 200, map[string]any{"ok": true})
}
