package app

import (
	"net/http"
)

func (a *App) restoreIdeaRevision(w http.ResponseWriter, r *http.Request, version int, data ideaFields) {
	user := currentUser(r)
	id := r.PathValue("id")
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	item, err := loadIdea(r.Context(), tx, id, user.ID, false, true)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.Version != version {
		writeError(w, fail(409, "version_conflict", "恢复前记录又有修改，请刷新后重试"))
		return
	}
	before := ideaSnapshot(item)
	attrs, err := validAttributes(data.Attributes)
	if err != nil {
		writeError(w, err)
		return
	}
	item.DisplayTitle = data.DisplayTitle
	item.Summary = data.Summary
	item.Notes = data.Notes
	item.NameCandidates = validStringList(data.NameCandidates, 100)
	item.IdeaTypes = validStringList(data.IdeaTypes, 30)
	item.ProductDirections = validStringList(data.ProductDirections, 30)
	item.Tags = validStringList(data.Tags, 50)
	item.Attributes = attrs
	if item.Content != data.Content {
		item.Content = data.Content
		item.ContentVersion++
		item.AIStatus = "idle"
		item.AIError = ""
		if _, err = tx.ExecContext(r.Context(), "UPDATE ai_tasks SET status='cancelled',updated_at=? WHERE idea_id=? AND status IN ('pending','running')", now(), id); err != nil {
			writeError(w, err)
			return
		}
	}
	item.Version++
	item.UpdatedAt = now()
	result, err := tx.ExecContext(r.Context(), `UPDATE ideas SET content=?,display_title=?,summary=?,notes=?,name_candidates_json=?,idea_types_json=?,product_directions_json=?,tags_json=?,manual_fields_json=?,content_version=?,version=?,updated_at=? WHERE id=? AND version=?`, item.Content, item.DisplayTitle, item.Summary, item.Notes, encodeJSON(item.NameCandidates), encodeJSON(item.IdeaTypes), encodeJSON(item.ProductDirections), encodeJSON(item.Tags), encodeJSON(data.ManualFields), item.ContentVersion, item.Version, item.UpdatedAt, id, version)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(409, "version_conflict", "记录版本已变化"))
		return
	}
	if err = replaceIdeaAttributes(r.Context(), tx, id, attrs); err != nil {
		writeError(w, err)
		return
	}
	if err = replaceIdeaTags(r.Context(), tx, id, item.Tags); err != nil {
		writeError(w, err)
		return
	}
	if err = addRevision(r.Context(), tx, "idea", id, item.Version, &user.ID, "restore_revision", before, ideaSnapshot(item)); err != nil {
		writeError(w, err)
		return
	}
	a.publish(tx, "idea.updated", "idea", id)
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"ok": true, "version": item.Version})
}

func (a *App) restoreProductRevision(w http.ResponseWriter, r *http.Request, version int, data productFields) {
	user := currentUser(r)
	id := r.PathValue("id")
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	item, err := loadProduct(r.Context(), tx, id, false, true)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.Version != version {
		writeError(w, fail(409, "version_conflict", "恢复前记录又有修改，请刷新后重试"))
		return
	}
	before := productSnapshot(item)
	attrs, err := validAttributes(data.Attributes)
	if err != nil {
		writeError(w, err)
		return
	}
	if clean(data.InternalName, 160) == "" || !validProductStatus(data.DevelopmentStatus) {
		writeError(w, fail(422, "invalid_revision", "历史版本内容无效"))
		return
	}
	item.InternalName = clean(data.InternalName, 160)
	item.FinalBrandName = clean(data.FinalBrandName, 160)
	item.Category = clean(data.Category, 100)
	item.Description = data.Description
	item.DevelopmentStatus = data.DevelopmentStatus
	item.Attributes = attrs
	item.Version++
	item.UpdatedAt = now()
	result, err := tx.ExecContext(r.Context(), "UPDATE products SET internal_name=?,final_brand_name=?,category=?,description=?,development_status=?,version=?,updated_at=? WHERE id=? AND version=?", item.InternalName, item.FinalBrandName, item.Category, item.Description, item.DevelopmentStatus, item.Version, item.UpdatedAt, id, version)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(409, "version_conflict", "记录版本已变化"))
		return
	}
	if err = replaceProductAttributes(r.Context(), tx, id, attrs); err != nil {
		writeError(w, err)
		return
	}
	if err = addRevision(r.Context(), tx, "product", id, item.Version, &user.ID, "restore_revision", before, productSnapshot(item)); err != nil {
		writeError(w, err)
		return
	}
	a.publish(tx, "product.updated", "product", id)
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"ok": true, "version": item.Version})
}
