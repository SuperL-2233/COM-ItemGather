package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strings"
)

type statusUsage struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	ItemCount   int               `json:"item_count"`
	RecipeCount int               `json:"recipe_count"`
	Token       string            `json:"token"`
	Version     int               `json:"-"`
	Items       []optionReference `json:"-"`
	Recipes     []optionReference `json:"-"`
}

func loadStatusUsage(ctx context.Context, q querier, id string) (statusUsage, error) {
	var usage statusUsage
	err := q.QueryRowContext(ctx, "SELECT id,name,version FROM product_statuses WHERE id=? AND deleted_at IS NULL", id).Scan(&usage.ID, &usage.Label, &usage.Version)
	if err != nil {
		if err == sql.ErrNoRows {
			err = fail(404, "status_deleted", "该制作状态不存在或已被删除")
		}
		return usage, err
	}
	usage.Items, err = optionReferences(ctx, q, "SELECT id,version FROM products WHERE COALESCE(status_option_id,development_status)=? ORDER BY id", id)
	if err != nil {
		return usage, err
	}
	usage.Recipes, err = optionReferences(ctx, q, "SELECT id,version FROM recipes WHERE COALESCE(status_option_id,development_status)=? ORDER BY id", id)
	if err != nil {
		return usage, err
	}
	usage.ItemCount, usage.RecipeCount = len(usage.Items), len(usage.Recipes)
	digest := sha256.Sum256([]byte(encodeJSON(struct {
		ID, Label      string
		Version        int
		Items, Recipes []optionReference
	}{usage.ID, usage.Label, usage.Version, usage.Items, usage.Recipes})))
	usage.Token = hex.EncodeToString(digest[:])
	return usage, nil
}

func (a *App) productStatusUsage(w http.ResponseWriter, r *http.Request) {
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	usage, err := loadStatusUsage(r.Context(), tx, r.URL.Query().Get("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, usage)
}

func (a *App) deleteProductStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string `json:"id"`
		Replacement string `json:"replacement"`
		Token       string `json:"token"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Token == "" {
		writeError(w, fail(422, "validation", "请先查看制作状态使用情况，再确认删除"))
		return
	}
	body.ID, body.Replacement = strings.TrimSpace(body.ID), strings.TrimSpace(body.Replacement)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	usage, err := loadStatusUsage(r.Context(), tx, body.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	if usage.Token != body.Token {
		writeError(w, &APIError{Status: 409, Code: "status_changed", Message: "制作状态使用情况已变化，请确认最新使用情况后再次删除", Details: map[string]any{"usage": usage}})
		return
	}
	var count int
	if err = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM product_statuses WHERE deleted_at IS NULL").Scan(&count); err != nil {
		writeError(w, err)
		return
	}
	if count <= 1 {
		writeError(w, fail(422, "last_status", "至少保留一个制作状态，请先新建其他状态"))
		return
	}
	if usage.ItemCount+usage.RecipeCount > 0 && body.Replacement == "" {
		writeError(w, fail(422, "status_in_use", "该制作状态正在使用，请选择替代状态"))
		return
	}
	if body.Replacement == "" {
		err = tx.QueryRowContext(r.Context(), "SELECT id FROM product_statuses WHERE id<>? AND deleted_at IS NULL ORDER BY position,created_at,id LIMIT 1", usage.ID).Scan(&body.Replacement)
		if err != nil {
			writeError(w, err)
			return
		}
	}
	if body.Replacement == usage.ID {
		writeError(w, fail(422, "validation", "替代状态不能是当前状态"))
		return
	}
	replacement, legacy, label, err := resolveProductStatus(r.Context(), tx, body.Replacement)
	if err != nil {
		writeError(w, err)
		return
	}
	user, stamp := currentUser(r).ID, now()
	for _, ref := range usage.Items {
		item, e := loadProduct(r.Context(), tx, ref.ID, true, false)
		if e != nil {
			writeError(w, e)
			return
		}
		before := productSnapshot(item)
		item.DevelopmentStatus, item.DevelopmentStatusLabel = replacement, label
		item.Version++
		item.UpdatedAt = stamp
		_, err = tx.ExecContext(r.Context(), "UPDATE products SET status_option_id=?,development_status=?,version=?,updated_at=? WHERE id=? AND version=?", replacement, legacy, item.Version, stamp, item.ID, ref.Version)
		if err == nil {
			err = addRevision(r.Context(), tx, "product", item.ID, item.Version, &user, "replace_status", before, productSnapshot(item))
		}
		if err != nil {
			writeError(w, err)
			return
		}
	}
	for _, ref := range usage.Recipes {
		item, e := loadRecipe(r.Context(), tx, ref.ID, true)
		if e != nil {
			writeError(w, e)
			return
		}
		before := recipeSnapshot(item)
		item.DevelopmentStatus, item.DevelopmentStatusLabel = replacement, label
		item.Version++
		item.UpdatedAt = stamp
		_, err = tx.ExecContext(r.Context(), "UPDATE recipes SET status_option_id=?,development_status=?,version=?,updated_at=? WHERE id=? AND version=?", replacement, legacy, item.Version, stamp, item.ID, ref.Version)
		if err == nil {
			err = addRevision(r.Context(), tx, "recipe", item.ID, item.Version, &user, "replace_status", before, recipeSnapshot(item))
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "UPDATE recipe_groups SET updated_at=? WHERE id=?", stamp, item.GroupID)
		}
		if err != nil {
			writeError(w, err)
			return
		}
	}
	_, err = tx.ExecContext(r.Context(), "UPDATE product_statuses SET deleted_at=?,version=version+1 WHERE id=?", stamp, usage.ID)
	if err == nil {
		err = a.publish(tx, "product_option.deleted", "status", usage.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"id": usage.ID, "label": usage.Label, "replacement": replacement, "replacement_label": label, "affected_items": usage.ItemCount, "affected_recipes": usage.RecipeCount})
}
