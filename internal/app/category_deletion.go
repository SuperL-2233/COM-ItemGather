package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strings"
)

type categoryReference struct {
	ID      string
	Version int
}
type categoryUsage struct {
	Name        string              `json:"name"`
	ItemCount   int                 `json:"item_count"`
	RecipeCount int                 `json:"recipe_count"`
	Token       string              `json:"token"`
	Version     int                 `json:"-"`
	Items       []categoryReference `json:"-"`
	Recipes     []categoryReference `json:"-"`
}

// Match SQLite's NOCASE semantics: only ASCII letter case is folded.
func sameCategoryName(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		left, right := a[i], b[i]
		if left >= 'A' && left <= 'Z' {
			left += 'a' - 'A'
		}
		if right >= 'A' && right <= 'Z' {
			right += 'a' - 'A'
		}
		if left != right {
			return false
		}
	}
	return true
}
func categoryReferences(ctx context.Context, q querier, query, name string) ([]categoryReference, error) {
	rows, err := q.QueryContext(ctx, query, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []categoryReference{}
	for rows.Next() {
		var item categoryReference
		if err = rows.Scan(&item.ID, &item.Version); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func loadCategoryUsage(ctx context.Context, q querier, name string) (categoryUsage, error) {
	var usage categoryUsage
	err := q.QueryRowContext(ctx, "SELECT name,version FROM product_categories WHERE name=? COLLATE NOCASE AND deleted_at IS NULL", name).Scan(&usage.Name, &usage.Version)
	if err != nil {
		if err == sql.ErrNoRows {
			return usage, fail(404, "category_deleted", "该分类不存在或已被删除")
		}
		return usage, err
	}
	usage.Items, err = categoryReferences(ctx, q, "SELECT id,version FROM products WHERE category=? COLLATE NOCASE ORDER BY id", usage.Name)
	if err != nil {
		return usage, err
	}
	usage.Recipes, err = categoryReferences(ctx, q, "SELECT r.id,r.version FROM recipes r WHERE EXISTS(SELECT 1 FROM recipe_ingredients i WHERE i.recipe_id=r.id AND i.category=? COLLATE NOCASE) ORDER BY r.id", usage.Name)
	if err != nil {
		return usage, err
	}
	usage.ItemCount = len(usage.Items)
	usage.RecipeCount = len(usage.Recipes)
	digest := sha256.Sum256([]byte(encodeJSON(struct {
		Name           string
		Version        int
		Items, Recipes []categoryReference
	}{usage.Name, usage.Version, usage.Items, usage.Recipes})))
	usage.Token = hex.EncodeToString(digest[:])
	return usage, nil
}
func (a *App) productCategoryUsage(w http.ResponseWriter, r *http.Request) {
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	usage, err := loadCategoryUsage(r.Context(), tx, r.URL.Query().Get("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, usage)
}
func (a *App) deleteProductCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Replacement string `json:"replacement"`
		Token       string `json:"token"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Replacement = strings.TrimSpace(body.Replacement)
	if body.Token == "" {
		writeError(w, fail(422, "validation", "请先查看分类使用情况，再确认删除"))
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	usage, err := loadCategoryUsage(r.Context(), tx, body.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	if usage.Token != body.Token {
		writeError(w, &APIError{Status: 409, Code: "category_changed", Message: "分类使用情况已变化，请确认最新使用情况后再次删除", Details: map[string]any{"usage": usage}})
		return
	}
	replacement := ""
	if body.Replacement != "" {
		err = tx.QueryRowContext(r.Context(), "SELECT name FROM product_categories WHERE name=? COLLATE NOCASE AND deleted_at IS NULL", body.Replacement).Scan(&replacement)
		if err != nil {
			if err == sql.ErrNoRows {
				err = fail(422, "validation", "替代分类不存在或已被删除，请重新选择")
			}
			writeError(w, err)
			return
		}
		if sameCategoryName(replacement, usage.Name) {
			writeError(w, fail(422, "validation", "替代分类不能是当前分类"))
			return
		}
	}
	if usage.RecipeCount > 0 && replacement == "" {
		writeError(w, fail(422, "category_in_use", "有配方使用该分类作为材料，请选择替代分类"))
		return
	}
	user := currentUser(r).ID
	stamp := now()
	action := "remove_category"
	if replacement != "" {
		action = "replace_category"
	}
	for _, ref := range usage.Items {
		item, e := loadProduct(r.Context(), tx, ref.ID, true, false)
		if e != nil {
			writeError(w, e)
			return
		}
		before := productSnapshot(item)
		item.Category = replacement
		item.Version++
		item.UpdatedAt = stamp
		_, err = tx.ExecContext(r.Context(), "UPDATE products SET category=?,version=?,updated_at=? WHERE id=? AND version=?", replacement, item.Version, stamp, item.ID, ref.Version)
		if err == nil {
			err = addRevision(r.Context(), tx, "product", item.ID, item.Version, &user, action, before, productSnapshot(item))
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
		merged := []Ingredient{}
		seen := map[string]int{}
		for _, ingredient := range item.Ingredients {
			if sameCategoryName(ingredient.Category, usage.Name) || sameCategoryName(ingredient.Category, replacement) {
				ingredient.Category = replacement
			}
			key := ingredient.ProductID + "|" + ingredient.Category
			if at, ok := seen[key]; ok {
				merged[at].Quantity += ingredient.Quantity
				if merged[at].Quantity > 1000000000 {
					writeError(w, fail(422, "quantity_overflow", "替换后配方材料数量超过上限，请先调整配方"))
					return
				}
			} else {
				seen[key] = len(merged)
				merged = append(merged, ingredient)
			}
		}
		item.Ingredients = merged
		item.Version++
		item.UpdatedAt = stamp
		_, err = tx.ExecContext(r.Context(), "UPDATE recipes SET version=?,updated_at=? WHERE id=? AND version=?", item.Version, stamp, item.ID, ref.Version)
		if err == nil {
			err = replaceRecipeIngredients(r.Context(), tx, item.ID, item.Ingredients)
		}
		if err == nil {
			err = addRevision(r.Context(), tx, "recipe", item.ID, item.Version, &user, action, before, recipeSnapshot(item))
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "UPDATE recipe_groups SET updated_at=? WHERE id=?", stamp, item.GroupID)
		}
		if err != nil {
			writeError(w, err)
			return
		}
	}
	_, err = tx.ExecContext(r.Context(), "UPDATE product_categories SET deleted_at=?,version=version+1 WHERE name=? COLLATE NOCASE", stamp, usage.Name)
	if err == nil {
		err = a.publish(tx, "product_option.deleted", "category", usage.Name)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"ok": true, "name": usage.Name, "replacement": replacement, "affected_items": usage.ItemCount, "affected_recipes": usage.RecipeCount})
}
