package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"com-itemgather/internal/database"
	"com-itemgather/internal/filelock"
)

func TestBackupAndRestoreRejectMissingAttachmentFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO users(id,username,nickname,password_hash,created_at,updated_at) VALUES('u','user','User','unused','2026-01-01','2026-01-01')`,
		`INSERT INTO ideas(id,author_id,created_at,updated_at) VALUES('i','u','2026-01-01','2026-01-01')`,
		`INSERT INTO attachments(id,idea_id,uploader_id,original_name,mime_type,size,file_path,thumbnail_path,created_at) VALUES('a','i','u','photo.png','image/png',5,'photo.png','photo.thumb.jpg','2026-01-01')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	attachments := filepath.Join(dir, "attachments")
	manager := New(db, attachments, filepath.Join(dir, "backups"), 3)
	if _, err := manager.Create(ctx); err == nil {
		t.Fatal("backup accepted missing referenced originals and thumbnails")
	}
	os.MkdirAll(attachments, 0o700)
	os.WriteFile(filepath.Join(attachments, "photo.png"), []byte("bad"), 0o600)
	os.WriteFile(filepath.Join(attachments, "photo.thumb.jpg"), []byte("thumb"), 0o600)
	if _, err := manager.Create(ctx); err == nil {
		t.Fatal("backup accepted a truncated original")
	}
	os.WriteFile(filepath.Join(attachments, "photo.png"), []byte("photo"), 0o600)
	if _, err := manager.Create(ctx); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(dir, "incomplete")
	os.MkdirAll(stage, 0o700)
	if err := database.Backup(ctx, db, filepath.Join(stage, "database.db")); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "incomplete.tar.gz")
	if err := makeArchive(ctx, stage, archive); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "restored.db")
	os.WriteFile(target, []byte("keep-original-data"), 0o600)
	if _, err := Restore(ctx, archive, target, filepath.Join(dir, "restored-attachments")); err == nil {
		t.Fatal("restore accepted an archive missing referenced images")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep-original-data" {
		t.Fatal("failed restore changed the original data")
	}
}

func TestRestoreRefusesRunningServer(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	unlock, err := filelock.Acquire(ctx, path+".running.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := Restore(ctx, filepath.Join(dir, "unused.tar.gz"), path, filepath.Join(dir, "attachments")); err == nil {
		t.Fatal("restore must refuse a running server before touching its data")
	}
}
