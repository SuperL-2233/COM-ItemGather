package app

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"com-itemgather/internal/security"
)

type variantInput struct {
	Name        string `json:"variant_name"`
	Description string `json:"variant_description"`
	Status      string `json:"development_status"`
}

func validateVariant(ctx context.Context, q querier, item Product) error {
	if len([]rune(item.VariantName)) > 160 || len(item.VariantDescription) > 100000 {
		return fail(422, "validation", "口味／样式名称最多 160 字，差异说明最多 100000 字节")
	}
	if item.ParentProductID == "" {
		if item.VariantName != "" || item.VariantDescription != "" {
			return fail(422, "validation", "商品本身不填写口味，请在商品口味／样式列表中添加")
		}
		return nil
	}
	if item.ItemKind != "product" {
		return fail(422, "validation", "口味／样式仅用于商品")
	}
	var parentKind string
	err := q.QueryRowContext(ctx, "SELECT item_kind FROM products WHERE id=? AND parent_product_id IS NULL AND deleted_at IS NULL", item.ParentProductID).Scan(&parentKind)
	if err != nil || parentKind != "product" {
		return fail(422, "parent_unavailable", "所属商品不存在或已在回收站，请先恢复商品")
	}
	rows, err := q.QueryContext(ctx, "SELECT variant_name FROM products WHERE parent_product_id=? AND id<>? AND deleted_at IS NULL", item.ParentProductID, item.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if strings.EqualFold(name, item.VariantName) {
			return fail(422, "variant_exists", "这个商品已有相同口味／样式，请填写不同名称")
		}
		count++
	}
	if count >= 100 {
		return fail(422, "validation", "每个商品最多保留 100 个口味／样式")
	}
	return rows.Err()
}

func loadVariantMembers(ctx context.Context, q querier, parentID string) ([]Product, error) {
	ids, err := queryIDs(ctx, q, "SELECT id FROM products WHERE parent_product_id=? AND deleted_at IS NULL ORDER BY created_at,id", parentID)
	if err != nil {
		return nil, err
	}
	items := []Product{}
	for _, id := range ids {
		item, err := loadProduct(ctx, q, id, false, true)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func sameProductShared(before productFields, item Product) bool {
	return before.ChineseName == item.ChineseName && before.EnglishName == item.EnglishName &&
		before.CompanyID == item.CompanyID && before.Company == item.Company &&
		before.Category == item.Category && before.Description == item.Description
}

func copyProductShared(item *Product, source Product) {
	item.ChineseName, item.EnglishName = source.ChineseName, source.EnglishName
	item.CompanyID, item.Company = source.CompanyID, source.Company
	item.Category, item.Description = source.Category, source.Description
}

// Keep shared fields on the existing item rows so all legacy readers and foreign
// keys continue to work. Every product edit updates its own children atomically,
// including trash, with a version, revision and event for each affected member.
func (a *App) syncProductShared(ctx context.Context, tx *sql.Tx, before productFields, source Product, actor string, skip map[string]bool) error {
	if source.ParentProductID != "" || sameProductShared(before, source) {
		return nil
	}
	ids, err := queryIDs(ctx, tx, "SELECT id FROM products WHERE parent_product_id=?", source.ID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if skip[id] {
			continue
		}
		item, err := loadProduct(ctx, tx, id, true, false)
		if err != nil {
			return err
		}
		old := productSnapshot(item)
		if sameProductShared(old, source) {
			continue
		}
		copyProductShared(&item, source)
		item.Version++
		if _, err = tx.ExecContext(ctx, `UPDATE products SET chinese_name=?,english_name=?,company_id=?,company=?,category=?,description=?,version=?,updated_at=? WHERE id=?`, item.ChineseName, item.EnglishName, nullString(item.CompanyID), item.Company, item.Category, item.Description, item.Version, now(), id); err != nil {
			return err
		}
		if err = addRevision(ctx, tx, "product", id, item.Version, &actor, "update", old, productSnapshot(item)); err != nil {
			return err
		}
		if err = a.publish(tx, "product.updated", "product", id); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) insertProductVariant(ctx context.Context, tx *sql.Tx, source Product, input variantInput, actor string) (Product, error) {
	item := source
	item.ID, item.VariantName, item.VariantDescription = security.ID(), strings.TrimSpace(input.Name), input.Description
	item.ParentProductID = source.ID
	item.ParentDeletedAt = ""
	item.Variants, item.Attachments, item.Attributes, item.Links = nil, nil, []Attribute{}, nil
	item.UseCategory, item.ToolSettings = "", ToolSettings{}
	stamp := now()
	item.CreatedBy, item.CreatedAt, item.UpdatedAt, item.Version, item.DeletedAt = actor, stamp, stamp, 1, ""
	if item.VariantName == "" {
		return item, fail(422, "validation", "添加新款时请填写口味／样式名称")
	}
	if err := validateVariant(ctx, tx, item); err != nil {
		return item, err
	}
	item.DevelopmentStatus = input.Status
	statusID, legacyStatus, _, err := resolveProductStatus(ctx, tx, item.DevelopmentStatus)
	if err != nil {
		return item, err
	}
	item.DevelopmentStatus = statusID
	_, err = tx.ExecContext(ctx, `INSERT INTO products(id,chinese_name,english_name,company_id,company,category,description,development_status,status_option_id,created_by,created_at,updated_at,variant_group_id,variant_name,variant_description,parent_product_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.ChineseName, item.EnglishName, nullString(item.CompanyID), item.Company, item.Category, item.Description, legacyStatus, statusID, actor, item.CreatedAt, item.UpdatedAt, item.VariantGroupID, item.VariantName, item.VariantDescription, item.ParentProductID)
	if err == nil {
		err = addRevision(ctx, tx, "product", item.ID, 1, &actor, "create", nil, productSnapshot(item))
	}
	if err == nil {
		err = a.publish(tx, "product.created", "product", item.ID)
	}
	return item, err
}

func (a *App) createProductVariant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version int `json:"version"`
		variantInput
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
	source, err := loadProduct(r.Context(), tx, r.PathValue("id"), false, false)
	if err == nil && (source.ItemKind != "product" || source.ParentProductID != "") {
		err = fail(422, "validation", "请在所属商品下添加口味／样式")
	}
	if err == nil && source.Version != body.Version {
		err = fail(409, "version_conflict", "商品已有新修改，请刷新后再添加")
	}
	if err == nil && source.VariantGroupID == "" {
		source.VariantGroupID = security.ID()
		_, err = tx.ExecContext(r.Context(), "INSERT INTO product_variant_groups(id) VALUES(?)", source.VariantGroupID)
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "UPDATE products SET variant_group_id=? WHERE id=?", source.VariantGroupID, source.ID)
		}
	}
	var item Product
	if err == nil {
		item, err = a.insertProductVariant(r.Context(), tx, source, body.variantInput, currentUser(r).ID)
	}
	if err == nil {
		err = a.touchVariantParent(r.Context(), tx, source.ID, currentUser(r).ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	item, err = loadProduct(r.Context(), a.db, item.ID, false, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, item)
}

// Adopt an existing leaf item as a flavor of another product, retaining its ID.
// Products which already own flavors cannot be nested or moved as a flavor.
func (a *App) moveProductVariant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version       int    `json:"version"`
		TargetID      string `json:"target_id"`
		TargetVersion int    `json:"target_version"`
		VariantName   string `json:"variant_name"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.TargetID == "" || strings.TrimSpace(body.VariantName) == "" {
		writeError(w, fail(422, "validation", "请选择所属商品并填写口味／样式名称"))
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	item, err := loadProduct(r.Context(), tx, r.PathValue("id"), false, false)
	if err != nil {
		writeError(w, err)
		return
	}
	target, err := loadProduct(r.Context(), tx, body.TargetID, false, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.Version != body.Version || target.Version != body.TargetVersion {
		writeError(w, fail(409, "version_conflict", "商品已变化，请刷新后重试"))
		return
	}
	if item.ItemKind != "product" || target.ItemKind != "product" || target.ParentProductID != "" || item.VariantCount > 0 || item.ID == target.ID {
		writeError(w, fail(422, "validation", "只能把没有下级口味的物品加入另一个商品"))
		return
	}
	before := productSnapshot(item)
	oldParent := item.ParentProductID
	copyProductShared(&item, target)
	item.ParentProductID, item.VariantGroupID, item.VariantName = target.ID, target.VariantGroupID, strings.TrimSpace(body.VariantName)
	if err = validateVariant(r.Context(), tx, item); err != nil {
		writeError(w, err)
		return
	}
	actor := currentUser(r).ID
	item.Version++
	_, err = tx.ExecContext(r.Context(), `UPDATE products SET parent_product_id=?,variant_group_id=?,variant_name=?,chinese_name=?,english_name=?,company_id=?,company=?,category=?,description=?,version=?,updated_at=? WHERE id=?`, item.ParentProductID, nullString(item.VariantGroupID), item.VariantName, item.ChineseName, item.EnglishName, nullString(item.CompanyID), item.Company, item.Category, item.Description, item.Version, now(), item.ID)
	if err == nil {
		err = addRevision(r.Context(), tx, "product", item.ID, item.Version, &actor, "update", before, productSnapshot(item))
	}
	if err == nil {
		err = a.touchVariantParent(r.Context(), tx, target.ID, actor)
	}
	if err == nil && oldParent != "" && oldParent != target.ID {
		err = a.touchVariantParent(r.Context(), tx, oldParent, actor)
	}
	if err == nil {
		err = a.publish(tx, "product.updated", "product", item.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	item, err = loadProduct(r.Context(), a.db, item.ID, false, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, item)
}

func (a *App) touchVariantParent(ctx context.Context, tx *sql.Tx, id, actor string) error {
	parent, err := loadProduct(ctx, tx, id, true, false)
	if err != nil {
		return err
	}
	if parent.DeletedAt != "" {
		return nil
	}
	before := productSnapshot(parent)
	parent.Version++
	if _, err = tx.ExecContext(ctx, "UPDATE products SET version=?,updated_at=? WHERE id=?", parent.Version, now(), id); err != nil {
		return err
	}
	if err = addRevision(ctx, tx, "product", id, parent.Version, &actor, "update", before, productSnapshot(parent)); err != nil {
		return err
	}
	return a.publish(tx, "product.updated", "product", id)
}

func (a *App) variantHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var parentID string
		err := a.db.QueryRowContext(r.Context(), "SELECT parent_product_id FROM products WHERE id=? AND item_kind='product'", r.PathValue("variant")).Scan(&parentID)
		if err != nil || parentID != r.PathValue("id") {
			writeError(w, fail(404, "not_found", "商品口味／样式不存在"))
			return
		}
		r.SetPathValue("id", r.PathValue("variant"))
		next(w, r)
	}
}

// Restoring a product restores only children deleted together with that product.
// Previously removed individual flavors remain in trash.
func (a *App) changeVariantChildren(ctx context.Context, tx *sql.Tx, parent Product, stamp, actor string, remove bool) error {
	condition := "deleted_at IS NULL"
	args := []any{parent.ID}
	if !remove {
		condition = "parent_deleted_at=? AND deleted_at IS NOT NULL"
		args = append(args, stamp)
	}
	ids, err := queryIDs(ctx, tx, "SELECT id FROM products WHERE parent_product_id=? AND "+condition, args...)
	if err != nil {
		return err
	}
	for _, id := range ids {
		child, err := loadProduct(ctx, tx, id, true, false)
		if err != nil {
			return err
		}
		before := productSnapshot(child)
		child.Version++
		var removed any
		marker := ""
		action := "restore"
		if remove {
			removed = stamp
			marker = stamp
			action = "delete"
		} else if err = validateVariant(ctx, tx, child); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE products SET deleted_at=?,parent_deleted_at=?,version=?,updated_at=? WHERE id=?", removed, marker, child.Version, now(), id); err != nil {
			return err
		}
		if err = addRevision(ctx, tx, "product", id, child.Version, &actor, action, before, productSnapshot(child)); err != nil {
			return err
		}
		if err = a.publish(tx, "product."+action, "product", id); err != nil {
			return err
		}
	}
	return nil
}
