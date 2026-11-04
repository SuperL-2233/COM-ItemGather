package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"com-itemgather/internal/database"
	"com-itemgather/internal/security"
)

func TestBackupCanRestoreDatabaseAndAttachments(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO product_statuses(id,name,created_at) VALUES(?,?,datetime('now'))", "status-1", "青苹果"); err != nil {
		t.Fatal(err)
	}
	attachments := filepath.Join(dir, "attachments")
	os.MkdirAll(attachments, 0o700)
	os.WriteFile(filepath.Join(attachments, "image.jpg"), []byte("original-image"), 0o600)
	manager := New(db, attachments, filepath.Join(dir, "backups"), 2, 30)
	archive, err := manager.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	restored := filepath.Join(dir, "restored")
	os.Mkdir(restored, 0o700)
	foundDB, foundImage := false, false
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.FileInfo().IsDir() {
			continue
		}
		cleanName := filepath.Clean(header.Name)
		if cleanName != header.Name || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, "../") {
			t.Fatalf("unsafe archive path %s", header.Name)
		}
		switch header.Name {
		case "database.db":
			foundDB = true
		case "attachments/image.jpg":
			foundImage = true
		default:
			continue
		}
		target := filepath.Join(restored, header.Name)
		os.MkdirAll(filepath.Dir(target), 0o700)
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.Copy(output, tarReader); err != nil {
			t.Fatal(err)
		}
		output.Close()
	}
	if !foundDB || !foundImage {
		t.Fatalf("backup missing database or attachment: %v %v", foundDB, foundImage)
	}
	restoredDB, err := database.Open(filepath.Join(restored, "database.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDB.Close()
	var name string
	if err = restoredDB.QueryRow("SELECT name FROM product_statuses WHERE id='status-1'").Scan(&name); err != nil || name != "青苹果" {
		t.Fatalf("restored data mismatch %q %v", name, err)
	}
	data, _ := os.ReadFile(filepath.Join(restored, "attachments/image.jpg"))
	if string(data) != "original-image" {
		t.Fatal("attachment restore mismatch")
	}
	_ = security.ID()
}
