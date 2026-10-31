package app

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"com-itemgather/internal/security"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

func (a *App) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUploadBytes+1024*1024)
	if err := r.ParseMultipartForm(1024 * 1024); err != nil {
		writeError(w, fail(413, "upload_too_large", "图片超过大小限制或表单无效"))
		return
	}
	ideaID := r.FormValue("idea_id")
	productID := r.FormValue("product_id")
	if (ideaID == "") == (productID == "") {
		writeError(w, fail(422, "validation", "必须且只能指定一条灵感或一个商品"))
		return
	}
	kind := "idea"
	parentID := ideaID
	if productID != "" {
		kind = "product"
		parentID = productID
	}
	var count int
	a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM attachments WHERE "+kind+"_id=?", parentID).Scan(&count)
	if count >= a.cfg.MaxAttachments {
		writeError(w, fail(422, "attachment_limit", "附件数量已达到上限"))
		return
	}
	var parentCount int
	a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM "+kind+"s WHERE id=? AND deleted_at IS NULL", parentID).Scan(&parentCount)
	if parentCount != 1 {
		writeError(w, fail(404, "not_found", "要添加附件的记录不存在"))
		return
	}
	var used int64
	a.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(size),0) FROM attachments").Scan(&used)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, fail(422, "validation", "请选择要上传的图片"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, a.cfg.MaxUploadBytes+1))
	if err != nil {
		writeError(w, err)
		return
	}
	if int64(len(data)) > a.cfg.MaxUploadBytes {
		writeError(w, fail(413, "upload_too_large", "图片超过大小限制"))
		return
	}
	if len(data) == 0 || used+int64(len(data)) > a.cfg.MaxStorageBytes {
		writeError(w, fail(413, "storage_limit", "存储空间不足或文件为空"))
		return
	}
	mimeType := http.DetectContentType(data)
	extension := ""
	switch mimeType {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/webp":
		extension = ".webp"
	default:
		writeError(w, fail(415, "unsupported_image", "只支持 JPEG、PNG 和 WebP 图片"))
		return
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 30000 || config.Height > 30000 || int64(config.Width)*int64(config.Height) > 40000000 {
		writeError(w, fail(415, "invalid_image", "图片内容损坏或尺寸无效"))
		return
	}
	if err = os.MkdirAll(a.cfg.AttachmentDir, 0o700); err != nil {
		writeError(w, err)
		return
	}
	id := security.ID()
	filename := id + extension
	path := filepath.Join(a.cfg.AttachmentDir, filename)
	temporary := filepath.Join(a.cfg.AttachmentDir, "."+id+".tmp")
	if err = os.WriteFile(temporary, data, 0o600); err != nil {
		writeError(w, err)
		return
	}
	if err = os.Rename(temporary, path); err != nil {
		os.Remove(temporary)
		writeError(w, err)
		return
	}
	thumbnailPath, thumbErr := makeThumbnail(data, a.cfg.AttachmentDir, id)
	if thumbErr != nil {
		removeFiles([]string{path, thumbnailPath})
		writeError(w, fail(415, "invalid_image", "图片内容损坏，无法读取"))
		return
	}
	thumbnail := filepath.Base(thumbnailPath)
	timestamp := now()
	originalName := clean(filepath.Base(header.Filename), 200)
	if originalName == "" {
		originalName = "image" + extension
	}
	var ideaValue, productValue any
	if ideaID != "" {
		ideaValue = ideaID
	} else {
		productValue = productID
	}
	_, err = a.db.ExecContext(r.Context(), "INSERT INTO attachments(id,idea_id,product_id,uploader_id,original_name,mime_type,size,file_path,thumbnail_path,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)", id, ideaValue, productValue, currentUser(r).ID, originalName, mimeType, len(data), filename, nullString(thumbnail), timestamp)
	if err != nil {
		removeFiles([]string{path, thumbnailPath})
		writeError(w, err)
		return
	}
	a.emit("attachment.changed", "attachment", "")
	attachment := Attachment{ID: id, IdeaID: ideaID, ProductID: productID, OriginalName: originalName, MimeType: mimeType, Size: int64(len(data)), URL: "/api/attachments/" + id, CreatedAt: timestamp}
	if thumbnail != "" {
		attachment.ThumbnailURL = attachment.URL + "?thumbnail=1"
	}
	writeJSON(w, 201, attachment)
}

func makeThumbnail(data []byte, directory, id string) (string, error) {
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	scale := 1.0
	if width > 480 || height > 480 {
		scaleX := float64(480) / float64(width)
		scaleY := float64(480) / float64(height)
		scale = scaleX
		if scaleY < scale {
			scale = scaleY
		}
	}
	target := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale))))
	draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)
	path := filepath.Join(directory, id+".thumb.jpg")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if err = jpeg.Encode(file, target, &jpeg.Options{Quality: 82}); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		os.Remove(path)
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	return path, nil
}

func (a *App) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	var id, name, mimeType, path string
	var thumbnail sql.NullString
	err := a.db.QueryRowContext(r.Context(), "SELECT id,original_name,mime_type,file_path,thumbnail_path FROM attachments WHERE id=?", r.PathValue("id")).Scan(&id, &name, &mimeType, &path, &thumbnail)
	if err != nil {
		writeError(w, ensureNoRows(err))
		return
	}
	if r.URL.Query().Get("thumbnail") == "1" && thumbnail.Valid {
		path = thumbnail.String
		mimeType = "image/jpeg"
	}
	if filepath.Base(path) != path {
		writeError(w, fail(500, "invalid_path", "附件路径无效"))
		return
	}
	fullPath := filepath.Join(a.cfg.AttachmentDir, path)
	file, err := os.Open(fullPath)
	if err != nil {
		writeError(w, fail(404, "not_found", "附件文件不存在"))
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeContent(w, r, name, stat.ModTime(), file)
}
func (a *App) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	var path string
	var thumbnail sql.NullString
	err := a.db.QueryRowContext(r.Context(), "SELECT file_path,thumbnail_path FROM attachments WHERE id=?", r.PathValue("id")).Scan(&path, &thumbnail)
	if err != nil {
		writeError(w, ensureNoRows(err))
		return
	}
	result, err := a.db.ExecContext(r.Context(), "DELETE FROM attachments WHERE id=?", r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(404, "not_found", "附件不存在"))
		return
	}
	paths := []string{filepath.Join(a.cfg.AttachmentDir, path)}
	if thumbnail.Valid {
		paths = append(paths, filepath.Join(a.cfg.AttachmentDir, thumbnail.String))
	}
	removeFiles(paths)
	a.emit("attachment.changed", "attachment", "")
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) attachmentPaths(ctx context.Context, q querier, kind, id string) []string {
	rows, err := q.QueryContext(ctx, "SELECT file_path,COALESCE(thumbnail_path,'') FROM attachments WHERE "+kind+"_id=?", id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	paths := []string{}
	for rows.Next() {
		var path, thumbnail string
		rows.Scan(&path, &thumbnail)
		if filepath.Base(path) == path {
			paths = append(paths, filepath.Join(a.cfg.AttachmentDir, path))
		}
		if thumbnail != "" && filepath.Base(thumbnail) == thumbnail {
			paths = append(paths, filepath.Join(a.cfg.AttachmentDir, thumbnail))
		}
	}
	return paths
}
func removeFiles(paths []string) {
	for _, path := range paths {
		if path != "" {
			_ = os.Remove(path)
		}
	}
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
