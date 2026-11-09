package app

import (
	"net/http"
	"strings"
)

func (a *App) batchUpdateProducts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"items"`
		Category          *string `json:"category"`
		DevelopmentStatus *string `json:"development_status"`
		CompanyID         *string `json:"company_id"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if len(body.Items) == 0 || len(body.Items) > 500 {
		writeError(w, fail(422, "validation", "每次请选择 1 至 500 件商品"))
		return
	}
	if body.Category == nil && body.DevelopmentStatus == nil && body.CompanyID == nil {
		writeError(w, fail(422, "validation", "请至少选择一项需要修改的内容"))
		return
	}
	seen := map[string]bool{}
	for _, item := range body.Items {
		if item.ID == "" || item.Version < 1 || seen[item.ID] {
			writeError(w, fail(422, "validation", "商品选择或版本不正确，请重新选择"))
			return
		}
		seen[item.ID] = true
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	items := make([]Product, 0, len(body.Items))
	for _, selected := range body.Items {
		item, loadErr := loadProduct(r.Context(), tx, selected.ID, false, false)
		if loadErr != nil {
			if apiErr, ok := loadErr.(*APIError); ok && apiErr.Status == 404 {
				loadErr = fail(404, "selection_changed", "部分所选商品已被删除，本次未保存。请重新选择商品后重试。")
			}
			writeError(w, loadErr)
			return
		}
		if item.ItemKind != "product" || item.ParentProductID != "" {
			writeError(w, fail(422, "validation", "批量修改请选择商品，口味／样式在所属商品下编辑"))
			return
		}
		if item.Version != selected.Version {
			writeError(w, fail(409, "version_conflict", "部分商品已被其他人修改，本次未保存。请重新选择商品后重试。"))
			return
		}
		items = append(items, item)
	}
	stamp := now()
	sets := []string{"version=version+1", "updated_at=?"}
	args := []any{stamp}
	var category, companyID, company, statusID, legacyStatus string
	if body.Category != nil {
		category, err = ensureProductCategory(r.Context(), tx, *body.Category)
		if err == nil {
			sets = append(sets, "category=?")
			args = append(args, category)
		}
	}
	if err == nil && body.CompanyID != nil {
		companyID, company, err = resolveCompany(r.Context(), tx, *body.CompanyID, "", currentUser(r).ID)
		if err == nil {
			sets = append(sets, "company_id=?", "company=?")
			args = append(args, nullString(companyID), company)
		}
	}
	if err == nil && body.DevelopmentStatus != nil {
		if *body.DevelopmentStatus == "" {
			err = fail(422, "validation", "请选择商品的制作状态")
		} else {
			statusID, legacyStatus, _, err = resolveProductStatus(r.Context(), tx, *body.DevelopmentStatus)
			if err == nil {
				sets = append(sets, "status_option_id=?", "development_status=?")
				args = append(args, statusID, legacyStatus)
			}
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	user := currentUser(r).ID
	type sharedUpdate struct {
		before productFields
		item   Product
	}
	sharedUpdates := []sharedUpdate{}
	for _, item := range items {
		before := productSnapshot(item)
		if body.Category != nil {
			item.Category = category
		}
		if body.CompanyID != nil {
			item.CompanyID, item.Company = companyID, company
		}
		if body.DevelopmentStatus != nil {
			item.DevelopmentStatus = statusID
		}
		parameters := append(append([]any{}, args...), item.ID, item.Version)
		result, updateErr := tx.ExecContext(r.Context(), "UPDATE products SET "+strings.Join(sets, ",")+" WHERE id=? AND version=? AND deleted_at IS NULL AND item_kind='product'", parameters...)
		if updateErr != nil {
			writeError(w, updateErr)
			return
		}
		count, countErr := result.RowsAffected()
		if countErr != nil {
			writeError(w, countErr)
			return
		}
		if count != 1 {
			writeError(w, fail(409, "version_conflict", "部分商品已变化，本次未保存。请重新选择商品后重试。"))
			return
		}
		item.Version++
		sharedUpdates = append(sharedUpdates, sharedUpdate{before, item})
		if err = addRevision(r.Context(), tx, "product", item.ID, item.Version, &user, "update", before, productSnapshot(item)); err == nil {
			err = a.publish(tx, "product.updated", "product", item.ID)
		}
		if err != nil {
			writeError(w, err)
			return
		}
	}
	for _, update := range sharedUpdates {
		if err = a.syncProductShared(r.Context(), tx, update.before, update.item, user, seen); err != nil {
			writeError(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"updated": len(items)})
}
