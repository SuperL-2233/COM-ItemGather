package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"com-itemgather/internal/security"
)

type recipeFields struct {
	Version           int          `json:"version"`
	Name              string       `json:"name"`
	OutputQuantity    int          `json:"output_quantity"`
	Notes             string       `json:"notes"`
	Verification      string       `json:"verification"`
	DevelopmentStatus string       `json:"development_status"`
	Ingredients       []Ingredient `json:"ingredients"`
}

func recipeSnapshot(r Recipe) recipeFields {
	ingredients := []Ingredient{}
	for _, i := range r.Ingredients {
		ingredients = append(ingredients, Ingredient{ProductID: i.ProductID, Category: i.Category, Quantity: i.Quantity})
	}
	return recipeFields{Version: r.Version, Name: r.Name, OutputQuantity: r.OutputQuantity, Notes: r.Notes, Verification: r.Verification, DevelopmentStatus: r.DevelopmentStatus, Ingredients: ingredients}
}
func loadRecipe(ctx context.Context, q querier, id string, deleted bool) (Recipe, error) {
	var item Recipe
	where := ""
	if !deleted {
		where = " AND deleted_at IS NULL"
	}
	err := q.QueryRowContext(ctx, `SELECT id,group_id,name,output_quantity,notes,verification,COALESCE(status_option_id,development_status),COALESCE((SELECT name FROM product_statuses WHERE id=COALESCE(recipes.status_option_id,recipes.development_status)),development_status),COALESCE((SELECT color FROM product_statuses WHERE id=COALESCE(recipes.status_option_id,recipes.development_status)),'#7c3aed'),version,created_by,created_at,updated_at,COALESCE(deleted_at,'') FROM recipes WHERE id=?`+where, id).Scan(&item.ID, &item.GroupID, &item.Name, &item.OutputQuantity, &item.Notes, &item.Verification, &item.DevelopmentStatus, &item.DevelopmentStatusLabel, &item.DevelopmentStatusColor, &item.Version, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err != nil {
		return item, ensureNoRows(err)
	}
	rows, err := q.QueryContext(ctx, "SELECT COALESCE(product_id,''),category,quantity FROM recipe_ingredients WHERE recipe_id=? ORDER BY position,id", id)
	if err != nil {
		return item, err
	}
	item.Ingredients = []Ingredient{}
	for rows.Next() {
		var i Ingredient
		if err = rows.Scan(&i.ProductID, &i.Category, &i.Quantity); err != nil {
			rows.Close()
			return item, err
		}
		item.Ingredients = append(item.Ingredients, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return item, err
	}
	for j := range item.Ingredients {
		if item.Ingredients[j].ProductID != "" {
			p, e := loadProduct(ctx, q, item.Ingredients[j].ProductID, true, false)
			if e != nil {
				return item, e
			}
			item.Ingredients[j].Item = &p
		}
	}
	return item, nil
}
func loadRecipeGroup(ctx context.Context, q querier, id string, deleted bool) (RecipeGroup, error) {
	var g RecipeGroup
	where := ""
	if !deleted {
		where = " AND deleted_at IS NULL"
	}
	err := q.QueryRowContext(ctx, `SELECT id,output_id,version,created_by,created_at,updated_at,COALESCE(deleted_at,'') FROM recipe_groups WHERE id=?`+where, id).Scan(&g.ID, &g.OutputID, &g.Version, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt, &g.DeletedAt)
	if err != nil {
		return g, ensureNoRows(err)
	}
	g.Output, err = loadProduct(ctx, q, g.OutputID, true, false)
	if err != nil {
		return g, err
	}
	ids, err := queryIDs(ctx, q, "SELECT id FROM recipes WHERE group_id=? AND deleted_at IS NULL ORDER BY created_at,id", id)
	if err != nil {
		return g, err
	}
	g.Recipes = []Recipe{}
	for _, recipeID := range ids {
		item, e := loadRecipe(ctx, q, recipeID, false)
		if e != nil {
			return g, e
		}
		g.Recipes = append(g.Recipes, item)
	}
	return g, nil
}

// Search cards need an output and count, without hydrating every recipe and
// ingredient (which can include large, repeated item descriptions/attachments).
func loadRecipeGroupSummary(ctx context.Context, q querier, id string) (RecipeGroup, error) {
	var group RecipeGroup
	var count int
	err := q.QueryRowContext(ctx, `SELECT g.id,g.output_id,(SELECT COUNT(*) FROM recipes r WHERE r.group_id=g.id AND r.deleted_at IS NULL) FROM recipe_groups g WHERE g.id=? AND g.deleted_at IS NULL`, id).Scan(&group.ID, &group.OutputID, &count)
	if err != nil {
		return group, err
	}
	group.RecipeCount = &count
	group.Recipes = []Recipe{}
	group.Output, err = loadProduct(ctx, q, group.OutputID, true, false)
	return group, err
}
func listRecipeGroupsData(ctx context.Context, q querier, search, author, itemID string, deleted bool) ([]RecipeGroup, error) {
	return findRecipeGroups(ctx, q, search, productFilterValues([]string{author}), itemID, deleted, 0, false)
}
func findRecipeGroups(ctx context.Context, q querier, search string, authors []string, itemID string, deleted bool, limit int, summary bool) ([]RecipeGroup, error) {
	where := "g.deleted_at IS NULL"
	if deleted {
		where = "1=1"
	}
	like := "%" + escapeLike(search) + "%"
	query := `SELECT g.id FROM recipe_groups g JOIN products p ON p.id=g.output_id WHERE ` + where + ` AND (?='' OR g.output_id=? OR EXISTS(SELECT 1 FROM recipes r JOIN recipe_ingredients i ON i.recipe_id=r.id WHERE r.group_id=g.id AND r.deleted_at IS NULL AND (i.product_id=? OR (i.product_id IS NULL AND i.category=(SELECT category FROM products WHERE id=? AND item_kind='product'))))) AND (?='' OR p.chinese_name LIKE ? ESCAPE '\' OR p.english_name LIKE ? ESCAPE '\' OR p.variant_name LIKE ? ESCAPE '\' OR EXISTS(SELECT 1 FROM recipes r WHERE r.group_id=g.id AND r.deleted_at IS NULL AND (r.name LIKE ? ESCAPE '\' OR r.notes LIKE ? ESCAPE '\' OR EXISTS(SELECT 1 FROM recipe_ingredients i LEFT JOIN products m ON m.id=i.product_id WHERE i.recipe_id=r.id AND (i.category LIKE ? ESCAPE '\' OR m.chinese_name LIKE ? ESCAPE '\' OR m.english_name LIKE ? ESCAPE '\' OR m.variant_name LIKE ? ESCAPE '\')))))`
	args := []any{itemID, itemID, itemID, itemID, search, like, like, like, like, like, like, like, like, like}
	query, args = searchIn(query, args, "g.created_by", authors)
	query += " ORDER BY g.updated_at DESC,g.id"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	ids, err := queryIDs(ctx, q, query, args...)
	if err != nil {
		return nil, err
	}
	items := []RecipeGroup{}
	for _, id := range ids {
		var g RecipeGroup
		var e error
		if summary {
			g, e = loadRecipeGroupSummary(ctx, q, id)
		} else {
			g, e = loadRecipeGroup(ctx, q, id, true)
		}
		if e != nil {
			return nil, e
		}
		items = append(items, g)
	}
	return items, nil
}
func (a *App) listRecipeGroups(w http.ResponseWriter, r *http.Request) {
	items, err := listRecipeGroupsData(r.Context(), a.db, r.URL.Query().Get("q"), "", r.URL.Query().Get("item_id"), false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}
func (a *App) getRecipeGroup(w http.ResponseWriter, r *http.Request) {
	g, err := loadRecipeGroup(r.Context(), a.db, r.PathValue("id"), false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, g)
}
func (a *App) createRecipeGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OutputID string `json:"output_id"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = loadProduct(r.Context(), tx, body.OutputID, false, false); err != nil {
		writeError(w, err)
		return
	}
	var existing string
	err = tx.QueryRowContext(r.Context(), "SELECT id FROM recipe_groups WHERE output_id=?", body.OutputID).Scan(&existing)
	if err == nil {
		g, e := loadRecipeGroup(r.Context(), tx, existing, true)
		if e != nil {
			writeError(w, e)
			return
		}
		if g.DeletedAt != "" {
			writeError(w, fail(409, "group_deleted", "该产物的配方页面在回收站中，请先恢复"))
			return
		}
		writeJSON(w, 200, g)
		return
	}
	if err != sql.ErrNoRows {
		writeError(w, err)
		return
	}
	id := security.ID()
	stamp := now()
	user := currentUser(r).ID
	_, err = tx.ExecContext(r.Context(), "INSERT INTO recipe_groups(id,output_id,created_by,created_at,updated_at) VALUES(?,?,?,?,?)", id, body.OutputID, user, stamp, stamp)
	if err == nil {
		err = addRevision(r.Context(), tx, "recipe_group", id, 1, &user, "create", nil, map[string]string{"output_id": body.OutputID})
	}
	if err == nil {
		err = a.publish(tx, "recipe_group.created", "recipe_group", id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	g, err := loadRecipeGroup(r.Context(), a.db, id, false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, g)
}
func (a *App) saveRecipe(w http.ResponseWriter, r *http.Request) {
	var body recipeFields
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.OutputQuantity < 1 || body.OutputQuantity > 1000000000 {
		writeError(w, fail(422, "validation", "产出数量必须是正整数"))
		return
	}
	if body.Verification == "" {
		body.Verification = "untested"
	}
	if body.Verification != "untested" && body.Verification != "verified" && body.Verification != "invalid" {
		writeError(w, fail(422, "validation", "验证状态无效"))
		return
	}
	if len(body.Notes) > 100000 {
		writeError(w, fail(422, "validation", "配方备注过长"))
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	id := r.PathValue("id")
	groupID := r.PathValue("group")
	stamp := now()
	user := currentUser(r).ID
	item := Recipe{ID: id, GroupID: groupID, Version: 1, CreatedBy: user, CreatedAt: stamp, UpdatedAt: stamp}
	var before any
	existingIDs := map[string]bool{}
	action := "create"
	code := 201
	if r.Method == "PATCH" {
		previous, e := loadRecipe(r.Context(), tx, id, false)
		if e != nil {
			writeError(w, e)
			return
		}
		if previous.Version != body.Version {
			writeError(w, &APIError{Status: 409, Code: "version_conflict", Message: "这套配方已被其他人修改", Details: map[string]any{"current": previous, "submitted": body}})
			return
		}
		before = recipeSnapshot(previous)
		item = previous
		item.Version++
		item.UpdatedAt = stamp
		groupID = item.GroupID
		action = "update"
		code = 200
		for _, i := range previous.Ingredients {
			existingIDs[i.ProductID] = true
		}
	} else {
		item.ID = security.ID()
	}
	if _, err = loadRecipeGroup(r.Context(), tx, groupID, false); err != nil {
		writeError(w, err)
		return
	}
	if strings.Contains(r.URL.Path, "/revisions/") {
		action = "restore_revision"
	}
	statusID, legacyStatus, statusLabel, err := resolveProductStatus(r.Context(), tx, body.DevelopmentStatus)
	if err != nil {
		writeError(w, err)
		return
	}
	item.Name = clean(body.Name, 160)
	item.OutputQuantity = body.OutputQuantity
	item.Notes = body.Notes
	item.Verification = body.Verification
	item.DevelopmentStatus = statusID
	item.DevelopmentStatusLabel = statusLabel
	item.Ingredients = []Ingredient{}
	seen := map[string]int{}
	for _, i := range body.Ingredients {
		i.Category = clean(i.Category, 100)
		i.Item = nil
		if (i.ProductID == "") == (i.Category == "") || i.Quantity < 1 || i.Quantity > 1000000000 {
			writeError(w, fail(422, "validation", "每行请选择一种材料，并填写正整数数量"))
			return
		}
		if i.ProductID != "" {
			if _, e := loadProduct(r.Context(), tx, i.ProductID, existingIDs[i.ProductID], false); e != nil {
				writeError(w, e)
				return
			}
		}
		if i.Category != "" {
			i.Category, err = ensureProductCategory(r.Context(), tx, i.Category)
			if err != nil {
				writeError(w, err)
				return
			}
		}
		key := i.ProductID + "|" + i.Category
		if at, ok := seen[key]; ok {
			item.Ingredients[at].Quantity += i.Quantity
			if item.Ingredients[at].Quantity > 1000000000 {
				writeError(w, fail(422, "validation", "材料数量过大"))
				return
			}
		} else {
			seen[key] = len(item.Ingredients)
			item.Ingredients = append(item.Ingredients, i)
		}
	}
	if r.Method == "PATCH" {
		_, err = tx.ExecContext(r.Context(), "UPDATE recipes SET name=?,output_quantity=?,notes=?,verification=?,development_status=?,status_option_id=?,version=?,updated_at=? WHERE id=? AND version=?", item.Name, item.OutputQuantity, item.Notes, item.Verification, legacyStatus, statusID, item.Version, stamp, item.ID, body.Version)
	} else {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO recipes(id,group_id,name,output_quantity,notes,verification,development_status,status_option_id,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", item.ID, groupID, item.Name, item.OutputQuantity, item.Notes, item.Verification, legacyStatus, statusID, user, stamp, stamp)
	}
	if err == nil {
		err = replaceRecipeIngredients(r.Context(), tx, item.ID, item.Ingredients)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE recipe_groups SET updated_at=? WHERE id=?", stamp, groupID)
	}
	if err == nil {
		err = addRevision(r.Context(), tx, "recipe", item.ID, item.Version, &user, action, before, recipeSnapshot(item))
	}
	if err == nil {
		err = a.publish(tx, "recipe."+action, "recipe", item.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	item, err = loadRecipe(r.Context(), a.db, item.ID, false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, code, item)
}
func catalogTable(kind string) string {
	switch kind {
	case "company":
		return "companies"
	case "recipe":
		return "recipes"
	case "recipe_group":
		return "recipe_groups"
	}
	panic("invalid catalog kind")
}
func (a *App) catalogLifecycle(kind, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Version int `json:"version"`
		}
		if err := decodeBody(w, r, &body); err != nil {
			writeError(w, err)
			return
		}
		unlock, err := a.lockAttachments(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		defer unlock()
		tx, err := a.db.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, err)
			return
		}
		defer tx.Rollback()
		table := catalogTable(kind)
		id := r.PathValue("id")
		var version int
		var deleted string
		if err = tx.QueryRowContext(r.Context(), "SELECT version,COALESCE(deleted_at,'') FROM "+table+" WHERE id=?", id).Scan(&version, &deleted); err != nil {
			writeError(w, ensureNoRows(err))
			return
		}
		if version != body.Version {
			writeError(w, fail(409, "version_conflict", "记录已被其他人修改，请刷新后重试"))
			return
		}
		paths := []string{}
		var before any
		switch kind {
		case "company":
			before, err = loadCompany(r.Context(), tx, id, true)
		case "recipe":
			before, err = loadRecipe(r.Context(), tx, id, true)
		case "recipe_group":
			before, err = loadRecipeGroup(r.Context(), tx, id, true)
		}
		if err != nil {
			writeError(w, err)
			return
		}
		if kind == "company" && action != "restore" {
			var count int
			err = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM products WHERE company_id=?", id).Scan(&count)
			if err != nil {
				writeError(w, err)
				return
			}
			if count > 0 {
				writeError(w, fail(409, "company_in_use", "公司仍有关联商品或道具，请先更换所属公司或合并到其他公司"))
				return
			}
		}
		if action == "permanent" {
			if deleted == "" {
				writeError(w, fail(422, "not_deleted", "请先移入回收站"))
				return
			}
			if kind == "company" {
				paths = a.attachmentPaths(r.Context(), tx, "company", id)
			}
			if kind == "recipe_group" {
				_, err = tx.ExecContext(r.Context(), "DELETE FROM setting_idea_links WHERE entity_type='recipe' AND entity_id IN (SELECT id FROM recipes WHERE group_id=?)", id)
			} else {
				_, err = tx.ExecContext(r.Context(), "DELETE FROM setting_idea_links WHERE entity_type=? AND entity_id=?", kind, id)
			}
			if err == nil {
				_, err = tx.ExecContext(r.Context(), "DELETE FROM "+table+" WHERE id=? AND version=?", id, version)
			}
		} else {
			var value any
			if action == "delete" {
				value = now()
			}
			_, err = tx.ExecContext(r.Context(), "UPDATE "+table+" SET deleted_at=?,updated_at=?,version=version+1 WHERE id=? AND version=?", value, now(), id, version)
			user := currentUser(r).ID
			if err == nil {
				var after any
				switch kind {
				case "company":
					after, err = loadCompany(r.Context(), tx, id, true)
				case "recipe":
					var rec Recipe
					rec, err = loadRecipe(r.Context(), tx, id, true)
					after = recipeSnapshot(rec)
					if b, ok := before.(Recipe); ok {
						before = recipeSnapshot(b)
					}
				case "recipe_group":
					after, err = loadRecipeGroup(r.Context(), tx, id, true)
				}
				if err == nil {
					err = addRevision(r.Context(), tx, kind, id, version+1, &user, action, before, after)
				}
			}
		}
		if err == nil {
			err = a.publish(tx, kind+"."+action, kind, id)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			writeError(w, err)
			return
		}
		removeFiles(paths)
		a.afterCommit()
		writeJSON(w, 200, map[string]any{"ok": true, "version": version + 1})
	}
}
func (a *App) restoreCatalogRevision(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Version int `json:"version"`
		}
		if err := decodeBody(w, r, &body); err != nil {
			writeError(w, err)
			return
		}
		var raw string
		err := a.db.QueryRowContext(r.Context(), "SELECT after_data FROM revisions WHERE id=? AND entity_type=? AND entity_id=?", r.PathValue("revision"), kind, r.PathValue("id")).Scan(&raw)
		if err != nil {
			writeError(w, ensureNoRows(err))
			return
		}
		var fields map[string]any
		if err = json.Unmarshal([]byte(raw), &fields); err != nil {
			writeError(w, err)
			return
		}
		fields["version"] = body.Version
		if kind == "company" {
			for key := range fields {
				if key != "version" && key != "chinese_name" && key != "english_name" && key != "aliases" && key != "description" {
					delete(fields, key)
				}
			}
		}
		r.Body = io.NopCloser(strings.NewReader(encodeJSON(fields)))
		r.Method = "PATCH"
		if kind == "company" {
			a.saveCompany(w, r)
		} else {
			a.saveRecipe(w, r)
		}
	}
}

func (a *App) catalogExport(ctx context.Context, all bool) (map[string]any, error) {
	result := map[string]any{}
	where := " WHERE deleted_at IS NOT NULL"
	if all {
		where = ""
	}
	companies := []Company{}
	ids, err := queryIDs(ctx, a.db, "SELECT id FROM companies"+where)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		c, e := loadCompany(ctx, a.db, id, true)
		if e != nil {
			return nil, e
		}
		companies = append(companies, c)
	}
	result["companies"] = companies
	groups := []RecipeGroup{}
	ids, err = queryIDs(ctx, a.db, "SELECT id FROM recipe_groups"+where)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		g, e := loadRecipeGroup(ctx, a.db, id, true)
		if e != nil {
			return nil, e
		}
		groups = append(groups, g)
	}
	result["recipe_groups"] = groups
	recipes := []Recipe{}
	ids, err = queryIDs(ctx, a.db, "SELECT id FROM recipes"+where)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		rec, e := loadRecipe(ctx, a.db, id, true)
		if e != nil {
			return nil, e
		}
		recipes = append(recipes, rec)
	}
	result["recipes"] = recipes
	if all {
		options, e := loadProductOptions(ctx, a.db, "other-categories")
		if e != nil {
			return nil, e
		}
		result["other_item_categories"] = options
		for _, kind := range []string{"categories", "statuses"} {
			options, e := loadProductOptions(ctx, a.db, kind)
			if e != nil {
				return nil, e
			}
			result["product_"+kind] = options
		}
		rows, e := a.db.QueryContext(ctx, "SELECT id,idea_id,entity_type,entity_id,status FROM setting_idea_links")
		if e != nil {
			return nil, e
		}
		links := []settingLink{}
		for rows.Next() {
			var l settingLink
			if e = rows.Scan(&l.ID, &l.IdeaID, &l.EntityType, &l.EntityID, &l.Status); e != nil {
				rows.Close()
				return nil, e
			}
			links = append(links, l)
		}
		rows.Close()
		result["setting_links"] = links
		rows, e = a.db.QueryContext(ctx, "SELECT name FROM tool_use_categories ORDER BY created_at,name")
		if e != nil {
			return nil, e
		}
		names := []string{}
		for rows.Next() {
			var name string
			if e = rows.Scan(&name); e != nil {
				rows.Close()
				return nil, e
			}
			names = append(names, name)
		}
		rows.Close()
		result["tool_categories"] = names
	}
	return result, nil
}

func replaceRecipeIngredients(ctx context.Context, tx *sql.Tx, id string, ingredients []Ingredient) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM recipe_ingredients WHERE recipe_id=?", id); err != nil {
		return err
	}
	for at, i := range ingredients {
		if _, err := tx.ExecContext(ctx, "INSERT INTO recipe_ingredients(id,recipe_id,product_id,category,quantity,position) VALUES(?,?,?,?,?,?)", security.ID(), id, nullString(i.ProductID), i.Category, i.Quantity, at); err != nil {
			return err
		}
	}
	return nil
}
