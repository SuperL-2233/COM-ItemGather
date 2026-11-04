package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"time"

	"com-itemgather/internal/backup"
)

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
	extra, err := a.catalogExport(r.Context(), false)
	if err != nil {
		writeError(w, err)
		return
	}
	tools := []Product{}
	onlyProducts := []Product{}
	for _, p := range products {
		if p.ItemKind == "tool" {
			tools = append(tools, p)
		} else {
			onlyProducts = append(onlyProducts, p)
		}
	}
	extra["ideas"] = ideas
	extra["products"] = onlyProducts
	extra["tools"] = tools
	writeJSON(w, 200, extra)
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
	a.changeTag(w, r, id, &name)
}
func (a *App) deleteTag(w http.ResponseWriter, r *http.Request) {
	a.changeTag(w, r, r.PathValue("id"), nil)
}

func (a *App) changeTag(w http.ResponseWriter, r *http.Request, id string, name *string) {
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	var previous string
	if err := tx.QueryRowContext(r.Context(), "SELECT name FROM tags WHERE id=?", id).Scan(&previous); err != nil {
		writeError(w, ensureNoRows(err))
		return
	}
	if name != nil && previous == *name {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	rows, err := tx.QueryContext(r.Context(), "SELECT idea_id FROM idea_tags WHERE tag_id=? ORDER BY idea_id", id)
	if err != nil {
		writeError(w, err)
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		writeError(w, err)
		return
	}
	if name == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM tags WHERE id=?", id)
	} else {
		var exists int
		if err = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM tags WHERE name=? COLLATE NOCASE AND id<>?", *name, id).Scan(&exists); err == nil && exists > 0 {
			writeError(w, fail(409, "tag_exists", "标签名已存在"))
			return
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "UPDATE tags SET name=? WHERE id=?", *name, id)
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	userID := currentUser(r).ID
	for _, ideaID := range ids {
		if err := a.updateIdeaTagsTx(r.Context(), tx, ideaID, userID); err != nil {
			writeError(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) updateIdeaTagsTx(ctx context.Context, tx *sql.Tx, ideaID, userID string) error {
	item, err := loadIdea(ctx, tx, ideaID, userID, true, false)
	if err != nil {
		return err
	}
	before := ideaSnapshot(item)
	tagRows, err := tx.QueryContext(ctx, "SELECT t.name FROM idea_tags it JOIN tags t ON t.id=it.tag_id WHERE it.idea_id=? ORDER BY t.name COLLATE NOCASE", ideaID)
	if err != nil {
		return err
	}
	defer tagRows.Close()
	tags := []string{}
	for tagRows.Next() {
		var name string
		if err := tagRows.Scan(&name); err != nil {
			return err
		}
		tags = append(tags, name)
	}
	if err := tagRows.Err(); err != nil {
		return err
	}
	tagRows.Close()
	item.Tags = tags
	if !slices.Contains(item.ManualFields, "tags") {
		item.ManualFields = append(item.ManualFields, "tags")
	}
	slices.Sort(item.ManualFields)
	item.Version++
	item.UpdatedAt = now()
	if _, err := tx.ExecContext(ctx, "UPDATE ideas SET tags_json=?,manual_fields_json=?,version=?,updated_at=? WHERE id=?", encodeJSON(item.Tags), encodeJSON(item.ManualFields), item.Version, item.UpdatedAt, ideaID); err != nil {
		return err
	}
	if err := addRevision(ctx, tx, "idea", ideaID, item.Version, &userID, "update_tags", before, ideaSnapshot(item)); err != nil {
		return err
	}
	return a.publish(tx, "idea.updated", "idea", ideaID)
}

func (a *App) creativeCatalog(ctx context.Context, userID string, includeDeleted bool) (map[string]any, error) {
	where := " WHERE deleted_at IS NULL"
	if includeDeleted {
		where = ""
	}
	ideaIDs, err := queryIDs(ctx, a.db, "SELECT id FROM ideas"+where+" ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	ideas := []Idea{}
	for _, id := range ideaIDs {
		item, loadErr := loadIdea(ctx, a.db, id, userID, true, false)
		if loadErr != nil {
			return nil, loadErr
		}
		ideas = append(ideas, item)
	}
	productIDs, err := queryIDs(ctx, a.db, "SELECT id FROM products"+where+" ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	products := []Product{}
	for _, id := range productIDs {
		item, loadErr := loadProduct(ctx, a.db, id, true, false)
		if loadErr != nil {
			return nil, loadErr
		}
		products = append(products, item)
	}
	linkQuery := "SELECT l.id,l.idea_id,l.product_id,l.status,l.created_at,l.updated_at FROM idea_product_links l"
	if !includeDeleted {
		linkQuery += " JOIN ideas i ON i.id=l.idea_id JOIN products p ON p.id=l.product_id WHERE i.deleted_at IS NULL AND p.deleted_at IS NULL"
	}
	linkRows, err := a.db.QueryContext(ctx, linkQuery+" ORDER BY l.created_at")
	if err != nil {
		return nil, err
	}
	links := []Link{}
	for linkRows.Next() {
		var link Link
		if err = linkRows.Scan(&link.ID, &link.IdeaID, &link.ProductID, &link.Status, &link.CreatedAt, &link.UpdatedAt); err != nil {
			linkRows.Close()
			return nil, err
		}
		links = append(links, link)
	}
	linkRows.Close()
	extra, err := a.catalogExport(ctx, true)
	if err != nil {
		return nil, err
	}
	if !includeDeleted {
		companies := []Company{}
		activeCompanies := map[string]bool{}
		for _, item := range extra["companies"].([]Company) {
			if item.DeletedAt == "" {
				companies = append(companies, item)
				activeCompanies[item.ID] = true
			}
		}
		extra["companies"] = companies
		groups := []RecipeGroup{}
		activeGroups := map[string]bool{}
		for _, item := range extra["recipe_groups"].([]RecipeGroup) {
			if item.DeletedAt == "" {
				groups = append(groups, item)
				activeGroups[item.ID] = true
			}
		}
		extra["recipe_groups"] = groups
		recipes := []Recipe{}
		activeRecipes := map[string]bool{}
		for _, item := range extra["recipes"].([]Recipe) {
			if item.DeletedAt == "" && activeGroups[item.GroupID] {
				recipes = append(recipes, item)
				activeRecipes[item.ID] = true
			}
		}
		extra["recipes"] = recipes
		activeIdeas := map[string]bool{}
		for _, item := range ideas {
			activeIdeas[item.ID] = true
		}
		settingLinks := []settingLink{}
		for _, link := range extra["setting_links"].([]settingLink) {
			if activeIdeas[link.IdeaID] && ((link.EntityType == "company" && activeCompanies[link.EntityID]) || (link.EntityType == "recipe" && activeRecipes[link.EntityID])) {
				settingLinks = append(settingLinks, link)
			}
		}
		extra["setting_links"] = settingLinks
	}
	tools := []Product{}
	onlyProducts := []Product{}
	for _, item := range products {
		if item.ItemKind == "tool" {
			tools = append(tools, item)
		} else {
			onlyProducts = append(onlyProducts, item)
		}
	}
	extra["ideas"] = ideas
	extra["products"] = onlyProducts
	extra["tools"] = tools
	extra["links"] = links
	return extra, nil
}

func (a *App) exportData(w http.ResponseWriter, r *http.Request) {
	extra, err := a.creativeCatalog(r.Context(), currentUser(r).ID, true)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="com-itemgather-%s.json"`, time.Now().UTC().Format("20060102")))
	extra["format_version"] = 2
	extra["exported_at"] = now()
	extra["attachments_note"] = "附件原件包含在服务器备份归档中。"
	writeJSON(w, 200, extra)
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
