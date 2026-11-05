package app

import (
	"context"
	"fmt"
	"net/http"
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
	otherItems := []Product{}
	onlyProducts := []Product{}
	for _, p := range products {
		if p.ItemKind == "tool" {
			tools = append(tools, p)
		} else if p.ItemKind == "other" {
			otherItems = append(otherItems, p)
		} else {
			onlyProducts = append(onlyProducts, p)
		}
	}
	extra["ideas"] = ideas
	extra["products"] = onlyProducts
	extra["tools"] = tools
	extra["other_items"] = otherItems
	writeJSON(w, 200, extra)
}

func (a *App) creativeCatalog(ctx context.Context, userID string) (map[string]any, error) {
	ideaIDs, err := queryIDs(ctx, a.db, "SELECT id FROM ideas ORDER BY created_at")
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
	productIDs, err := queryIDs(ctx, a.db, "SELECT id FROM products ORDER BY created_at")
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
	tools := []Product{}
	otherItems := []Product{}
	onlyProducts := []Product{}
	for _, item := range products {
		if item.ItemKind == "tool" {
			tools = append(tools, item)
		} else if item.ItemKind == "other" {
			otherItems = append(otherItems, item)
		} else {
			onlyProducts = append(onlyProducts, item)
		}
	}
	extra["ideas"] = ideas
	extra["products"] = onlyProducts
	extra["tools"] = tools
	extra["other_items"] = otherItems
	extra["links"] = links
	return extra, nil
}

func (a *App) exportData(w http.ResponseWriter, r *http.Request) {
	extra, err := a.creativeCatalog(r.Context(), currentUser(r).ID)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="com-itemgather-%s.json"`, time.Now().UTC().Format("20060102")))
	extra["format_version"] = 3
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
