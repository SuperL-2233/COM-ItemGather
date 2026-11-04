package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"com-itemgather/internal/database"
)

func TestRestoreReplacesDatabaseAndAttachments(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "data", "app.db")
	attachmentsPath := filepath.Join(dir, "files", "attachments")
	db, err := database.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO product_statuses(id,name,created_at) VALUES('restored-status','原始状态',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(attachmentsPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(attachmentsPath, "original.png"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive, err := New(db, attachmentsPath, filepath.Join(dir, "backups"), 3, 30).Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(databasePath, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(attachmentsPath, "new.png"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	preserved, err := Restore(context.Background(), archive, databasePath, attachmentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(preserved) < 2 {
		t.Fatalf("existing data was not preserved: %v", preserved)
	}
	for _, filename := range preserved {
		if _, err = os.Stat(filename); err != nil {
			t.Fatalf("preserved data is missing: %v", err)
		}
	}
	restored, err := database.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var name string
	if err = restored.QueryRow("SELECT name FROM product_statuses WHERE id='restored-status'").Scan(&name); err != nil || name != "原始状态" {
		t.Fatalf("restored database mismatch: %q %v", name, err)
	}
	data, err := os.ReadFile(filepath.Join(attachmentsPath, "original.png"))
	if err != nil || string(data) != "original" {
		t.Fatalf("restored attachment mismatch: %q %v", data, err)
	}
	if _, err = os.Stat(filepath.Join(attachmentsPath, "new.png")); !os.IsNotExist(err) {
		t.Fatal("attachment from the newer database was not replaced")
	}
}

func TestRestoreRejectsUnsafeArchive(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "unsafe.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	writer := tar.NewWriter(compressed)
	content := []byte("not a database")
	if err = writer.WriteHeader(&tar.Header{Name: "../escaped", Mode: 0o600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = Restore(context.Background(), archivePath, filepath.Join(dir, "data", "app.db"), filepath.Join(dir, "attachments")); err == nil {
		t.Fatal("unsafe archive was accepted")
	}
	if _, err = os.Stat(filepath.Join(dir, "escaped")); !os.IsNotExist(err) {
		t.Fatal("archive wrote outside the staging directory")
	}
}
