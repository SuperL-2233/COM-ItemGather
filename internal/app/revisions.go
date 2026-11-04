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
	item.DisplayTitle = data.DisplayTitle
	item.Summary = data.Summary
	item.Notes = data.Notes
	item.IdeaTypes = validStringList(data.IdeaTypes, 30)
	item.ProductDirections = validStringList(data.ProductDirections, 30)
	item.ManualFields = append([]string{}, data.ManualFields...)
	if item.Content != data.Content {
		item.Content = data.Content
		item.ContentVersion++
	}
	item.AIStatus = "idle"
	item.AIError = ""
	if _, err = tx.ExecContext(r.Context(), "UPDATE ai_tasks SET status='cancelled',updated_at=? WHERE idea_id=? AND status IN ('pending','running')", now(), id); err != nil {
		writeError(w, err)
		return
	}
	item.Version++
	item.UpdatedAt = now()
	result, err := tx.ExecContext(r.Context(), `UPDATE ideas SET content=?,display_title=?,summary=?,notes=?,idea_types_json=?,product_directions_json=?,manual_fields_json=?,content_version=?,ai_status=?,ai_error=?,version=?,updated_at=? WHERE id=? AND version=?`, item.Content, item.DisplayTitle, item.Summary, item.Notes, encodeJSON(item.IdeaTypes), encodeJSON(item.ProductDirections), encodeJSON(item.ManualFields), item.ContentVersion, item.AIStatus, item.AIError, item.Version, item.UpdatedAt, id, version)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(409, "version_conflict", "记录版本已变化"))
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
	statusID, legacyStatus, _, err := resolveProductStatus(r.Context(), tx, data.DevelopmentStatus)
	if err != nil {
		writeError(w, err)
		return
	}
	item.ChineseName = clean(data.ChineseName, 160)
	item.EnglishName = clean(data.EnglishName, 160)
	item.CompanyID, item.Company, err = resolveCompany(r.Context(), tx, data.CompanyID, data.Company, user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	if err = validateToolSettings(data.ToolSettings); err != nil {
		writeError(w, err)
		return
	}
	if err = ensureUseCategory(r.Context(), tx, data.UseCategory); err != nil {
		writeError(w, err)
		return
	}
	item.UseCategory = clean(data.UseCategory, 80)
	item.ToolSettings = data.ToolSettings
	item.Category, err = ensureProductCategory(r.Context(), tx, data.Category)
	if err != nil {
		writeError(w, err)
		return
	}
	item.Description = data.Description
	item.DevelopmentStatus = data.DevelopmentStatus
	item.Attributes = attrs
	item.Version++
	item.UpdatedAt = now()
	result, err := tx.ExecContext(r.Context(), "UPDATE products SET chinese_name=?,english_name=?,company=?,category=?,description=?,development_status=?,company_id=?,use_category=?,tool_settings_json=?,status_option_id=?,version=?,updated_at=? WHERE id=? AND version=?", item.ChineseName, item.EnglishName, item.Company, item.Category, item.Description, legacyStatus, nullString(item.CompanyID), item.UseCategory, encodeJSON(item.ToolSettings), statusID, item.Version, item.UpdatedAt, id, version)
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
