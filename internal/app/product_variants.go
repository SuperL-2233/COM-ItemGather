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
	if item.ItemKind != "product" {
		if item.VariantName != "" || item.VariantDescription != "" || item.VariantGroupID != "" {
			return fail(422, "validation", "口味／样式变体仅用于商品")
		}
		return nil
	}
	if item.VariantGroupID == "" {
		return nil
	}
	rows, err := q.QueryContext(ctx, "SELECT variant_name FROM products WHERE variant_group_id=? AND id<>? AND deleted_at IS NULL", item.VariantGroupID, item.ID)
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
		return fail(422, "validation", "每个商品最多保留 100 款，请先整理已有款式")
	}
	return rows.Err()
}

func loadVariantMembers(ctx context.Context, q querier, groupID string) ([]Product, error) {
	ids, err := queryIDs(ctx, q, "SELECT id FROM products WHERE variant_group_id=? AND deleted_at IS NULL ORDER BY created_at,id", groupID)
	if err != nil {
		return nil, err
	}
	items := []Product{}
	for _, id := range ids {
		item, err := loadProduct(ctx, q, id, false, false)
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
// keys continue to work. Every shared edit updates the whole group atomically,
// including trash, with a version, revision and event for each affected member.
func (a *App) syncProductShared(ctx context.Context, tx *sql.Tx, before productFields, source Product, actor string, skip map[string]bool) error {
	if source.VariantGroupID == "" || sameProductShared(before, source) {
		return nil
	}
	ids, err := queryIDs(ctx, tx, "SELECT id FROM products WHERE variant_group_id=? AND id<>?", source.VariantGroupID, source.ID)
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
	_, err = tx.ExecContext(ctx, `INSERT INTO products(id,chinese_name,english_name,company_id,company,category,description,development_status,status_option_id,created_by,created_at,updated_at,variant_group_id,variant_name,variant_description) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.ChineseName, item.EnglishName, nullString(item.CompanyID), item.Company, item.Category, item.Description, legacyStatus, statusID, actor, item.CreatedAt, item.UpdatedAt, item.VariantGroupID, item.VariantName, item.VariantDescription)
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
	if err == nil && source.ItemKind != "product" {
		err = fail(422, "validation", "只能为商品添加口味／样式")
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

// Move only the selected item. Its ID, images, recipes and inspiration links
// stay intact. An empty target detaches it into its own independent product.
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
	if item.ItemKind != "product" || item.Version != body.Version {
		writeError(w, fail(409, "version_conflict", "商品已变化，请刷新后重试"))
		return
	}
	before := productSnapshot(item)
	item.VariantName = strings.TrimSpace(body.VariantName)
	if body.TargetID == "" {
		item.VariantGroupID = security.ID()
		_, err = tx.ExecContext(r.Context(), "INSERT INTO product_variant_groups(id) VALUES(?)", item.VariantGroupID)
	} else {
		var target Product
		target, err = loadProduct(r.Context(), tx, body.TargetID, false, false)
		if err == nil && (target.ItemKind != "product" || target.ID == item.ID || target.Version != body.TargetVersion) {
			err = fail(409, "version_conflict", "目标商品已变化，请重新选择")
		}
		if err == nil {
			if target.VariantGroupID == "" {
				target.VariantGroupID = security.ID()
				_, err = tx.ExecContext(r.Context(), "INSERT INTO product_variant_groups(id) VALUES(?)", target.VariantGroupID)
				if err == nil {
					_, err = tx.ExecContext(r.Context(), "UPDATE products SET variant_group_id=? WHERE id=?", target.VariantGroupID, target.ID)
				}
			}
			item.VariantGroupID = target.VariantGroupID
			copyProductShared(&item, target)
		}
	}
	if err == nil {
		err = validateVariant(r.Context(), tx, item)
	}
	actor := currentUser(r).ID
	item.Version++
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE products SET variant_group_id=?,variant_name=?,chinese_name=?,english_name=?,company_id=?,company=?,category=?,description=?,version=?,updated_at=? WHERE id=?`, item.VariantGroupID, item.VariantName, item.ChineseName, item.EnglishName, nullString(item.CompanyID), item.Company, item.Category, item.Description, item.Version, now(), item.ID)
	}
	if err == nil {
		err = addRevision(r.Context(), tx, "product", item.ID, item.Version, &actor, "update", before, productSnapshot(item))
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
