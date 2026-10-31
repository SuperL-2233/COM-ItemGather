package app

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"com-itemgather/internal/backup"
)

func (a *App) globalSearch(w http.ResponseWriter, r *http.Request) {
	parameters := r.URL.Query()
	query := strings.TrimSpace(parameters.Get("q"))
	kind := parameters.Get("kind")
	ideaType := clean(parameters.Get("idea_type"), 80)
	direction := clean(parameters.Get("direction"), 80)
	author := clean(parameters.Get("author"), 100)
	if kind == "" {
		kind = "all"
	}
	if len([]rune(query)) > 200 || (kind != "all" && kind != "ideas" && kind != "products") {
		writeError(w, fail(422, "validation", "搜索条件无效"))
		return
	}
	creatorRows, err := a.db.QueryContext(r.Context(), "SELECT id,nickname FROM users WHERE status='active' ORDER BY nickname COLLATE NOCASE")
	if err != nil {
		writeError(w, err)
		return
	}
	creators := []map[string]string{}
	for creatorRows.Next() {
		var id, nickname string
		if err = creatorRows.Scan(&id, &nickname); err != nil {
			creatorRows.Close()
			writeError(w, err)
			return
		}
		creators = append(creators, map[string]string{"id": id, "nickname": nickname})
	}
	creatorRows.Close()
	ideas := []Idea{}
	products := []Product{}
	if query == "" && ideaType == "" && direction == "" && author == "" {
		writeJSON(w, 200, map[string]any{"ideas": ideas, "products": products, "creators": creators})
		return
	}
	like := "%" + escapeLike(query) + "%"
	if kind != "products" {
		sql := `SELECT i.id FROM ideas i WHERE i.deleted_at IS NULL AND (?='' OR i.original_content LIKE ? ESCAPE '\' OR i.content LIKE ? ESCAPE '\' OR i.display_title LIKE ? ESCAPE '\' OR i.summary LIKE ? ESCAPE '\' OR i.name_candidates_json LIKE ? ESCAPE '\' OR i.tags_json LIKE ? ESCAPE '\' OR EXISTS(SELECT 1 FROM idea_attributes a WHERE a.idea_id=i.id AND (a.key LIKE ? ESCAPE '\' OR a.value LIKE ? ESCAPE '\')))`
		arguments := []any{query, like, like, like, like, like, like, like, like}
		if ideaType != "" {
			sql += " AND EXISTS(SELECT 1 FROM json_each(i.idea_types_json) WHERE json_each.value=?)"
			arguments = append(arguments, ideaType)
		}
		if direction != "" {
			sql += " AND EXISTS(SELECT 1 FROM json_each(i.product_directions_json) WHERE json_each.value=?)"
			arguments = append(arguments, direction)
		}
		if author != "" {
			sql += " AND i.author_id=?"
			arguments = append(arguments, author)
		}
		ideaRows, queryErr := a.db.QueryContext(r.Context(), sql+" ORDER BY i.updated_at DESC LIMIT 50", arguments...)
		if queryErr != nil {
			writeError(w, queryErr)
			return
		}
		ideaIDs := []string{}
		for ideaRows.Next() {
			var id string
			if err = ideaRows.Scan(&id); err != nil {
				ideaRows.Close()
				writeError(w, err)
				return
			}
			ideaIDs = append(ideaIDs, id)
		}
		if err = ideaRows.Err(); err != nil {
			ideaRows.Close()
			writeError(w, err)
			return
		}
		ideaRows.Close()
		for _, id := range ideaIDs {
			item, loadErr := loadIdea(r.Context(), a.db, id, currentUser(r).ID, false, false)
			if loadErr != nil {
				writeError(w, loadErr)
				return
			}
			ideas = append(ideas, item)
		}
	}
	if kind != "ideas" && ideaType == "" && direction == "" && author == "" {
		productRows, queryErr := a.db.QueryContext(r.Context(), `SELECT p.id FROM products p WHERE p.deleted_at IS NULL AND (?='' OR p.internal_name LIKE ? ESCAPE '\' OR p.final_brand_name LIKE ? ESCAPE '\' OR p.category LIKE ? ESCAPE '\' OR p.description LIKE ? ESCAPE '\' OR EXISTS(SELECT 1 FROM product_attributes a WHERE a.product_id=p.id AND (a.key LIKE ? ESCAPE '\' OR a.value LIKE ? ESCAPE '\'))) ORDER BY p.updated_at DESC LIMIT 50`, query, like, like, like, like, like, like)
		if queryErr != nil {
			writeError(w, queryErr)
			return
		}
		productIDs := []string{}
		for productRows.Next() {
			var id string
			if err = productRows.Scan(&id); err != nil {
				productRows.Close()
				writeError(w, err)
				return
			}
			productIDs = append(productIDs, id)
		}
		if err = productRows.Err(); err != nil {
			productRows.Close()
			writeError(w, err)
			return
		}
		productRows.Close()
		for _, id := range productIDs {
			item, loadErr := loadProduct(r.Context(), a.db, id, false, false)
			if loadErr != nil {
				writeError(w, loadErr)
				return
			}
			products = append(products, item)
		}
	}
	writeJSON(w, 200, map[string]any{"ideas": ideas, "products": products, "creators": creators})
}

func (a *App) listTrash(w http.ResponseWriter, r *http.Request) {
	ideas := []Idea{}
	rows, err := a.db.QueryContext(r.Context(), "SELECT id FROM ideas WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC")
	if err != nil {
		writeError(w, err)
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		item, loadErr := loadIdea(r.Context(), a.db, id, currentUser(r).ID, true, false)
		if loadErr == nil {
			ideas = append(ideas, item)
		}
	}
	products := []Product{}
	productRows, err := a.db.QueryContext(r.Context(), "SELECT id FROM products WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC")
	if err != nil {
		writeError(w, err)
		return
	}
	ids = nil
	for productRows.Next() {
		var id string
		productRows.Scan(&id)
		ids = append(ids, id)
	}
	productRows.Close()
	for _, id := range ids {
		item, loadErr := loadProduct(r.Context(), a.db, id, true, false)
		if loadErr == nil {
			products = append(products, item)
		}
	}
	writeJSON(w, 200, map[string]any{"ideas": ideas, "products": products})
}

func (a *App) listTags(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), "SELECT t.id,t.name,COUNT(it.idea_id) FROM tags t LEFT JOIN idea_tags it ON it.tag_id=t.id GROUP BY t.id,t.name ORDER BY t.name COLLATE NOCASE")
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name string
		var count int
		rows.Scan(&id, &name, &count)
		items = append(items, map[string]any{"id": id, "name": name, "idea_count": count})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (a *App) renameTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	name := clean(body.Name, 80)
	if name == "" {
		writeError(w, fail(422, "validation", "标签名不能为空"))
		return
	}
	id := r.PathValue("id")
	if _, err := a.db.ExecContext(r.Context(), "UPDATE tags SET name=? WHERE id=?", name, id); err != nil {
		writeError(w, fail(409, "tag_exists", "标签名已存在"))
		return
	}
	a.refreshTagJSON(id)
	a.hub.Notify()
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) deleteTag(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := a.db.QueryContext(r.Context(), "SELECT idea_id FROM idea_tags WHERE tag_id=?", id)
	if err != nil {
		writeError(w, err)
		return
	}
	ideaIDs := []string{}
	for rows.Next() {
		var ideaID string
		rows.Scan(&ideaID)
		ideaIDs = append(ideaIDs, ideaID)
	}
	rows.Close()
	if _, err := a.db.ExecContext(r.Context(), "DELETE FROM tags WHERE id=?", id); err != nil {
		writeError(w, err)
		return
	}
	for _, ideaID := range ideaIDs {
		a.refreshIdeaTags(ideaID)
	}
	a.hub.Notify()
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) refreshTagJSON(tagID string) {
	rows, err := a.db.Query("SELECT idea_id FROM idea_tags WHERE tag_id=?", tagID)
	if err != nil {
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, ideaID := range ids {
		a.refreshIdeaTags(ideaID)
	}
}
func (a *App) refreshIdeaTags(ideaID string) {
	tagRows, err := a.db.Query("SELECT t.name FROM idea_tags it JOIN tags t ON t.id=it.tag_id WHERE it.idea_id=? ORDER BY t.name COLLATE NOCASE", ideaID)
	if err != nil {
		return
	}
	tags := []string{}
	for tagRows.Next() {
		var name string
		tagRows.Scan(&name)
		tags = append(tags, name)
	}
	tagRows.Close()
	a.db.Exec("UPDATE ideas SET tags_json=? WHERE id=?", encodeJSON(tags), ideaID)
}

func (a *App) exportData(w http.ResponseWriter, r *http.Request) {
	ideaIDs := []string{}
	rows, err := a.db.QueryContext(r.Context(), "SELECT id FROM ideas ORDER BY created_at")
	if err != nil {
		writeError(w, err)
		return
	}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ideaIDs = append(ideaIDs, id)
	}
	rows.Close()
	ideas := []Idea{}
	for _, id := range ideaIDs {
		item, loadErr := loadIdea(r.Context(), a.db, id, currentUser(r).ID, true, false)
		if loadErr != nil {
			writeError(w, loadErr)
			return
		}
		ideas = append(ideas, item)
	}
	productIDs := []string{}
	rows, err = a.db.QueryContext(r.Context(), "SELECT id FROM products ORDER BY created_at")
	if err != nil {
		writeError(w, err)
		return
	}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		productIDs = append(productIDs, id)
	}
	rows.Close()
	products := []Product{}
	for _, id := range productIDs {
		item, loadErr := loadProduct(r.Context(), a.db, id, true, false)
		if loadErr != nil {
			writeError(w, loadErr)
			return
		}
		products = append(products, item)
	}
	links := []Link{}
	linkRows, err := a.db.QueryContext(r.Context(), "SELECT id,idea_id,product_id,status,created_at,updated_at FROM idea_product_links ORDER BY created_at")
	if err != nil {
		writeError(w, err)
		return
	}
	defer linkRows.Close()
	for linkRows.Next() {
		var link Link
		linkRows.Scan(&link.ID, &link.IdeaID, &link.ProductID, &link.Status, &link.CreatedAt, &link.UpdatedAt)
		links = append(links, link)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="com-itemgather-%s.json"`, time.Now().UTC().Format("20060102")))
	writeJSON(w, 200, map[string]any{"format_version": 1, "exported_at": now(), "ideas": ideas, "products": products, "links": links, "attachments_note": "附件原件包含在服务器备份归档中。"})
}
func (a *App) manualBackup(w http.ResponseWriter, r *http.Request) {
	if err := a.requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	path, err := backup.New(a.db, a.cfg.AttachmentDir, a.cfg.BackupDir, a.cfg.BackupKeep, a.cfg.BackupRetentionDays).Create(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"path": path})
}
