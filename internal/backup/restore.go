package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"com-itemgather/internal/security"
)

type movedPath struct {
	from string
	to   string
}

func Restore(ctx context.Context, archivePath, databasePath, attachmentsPath string) ([]string, error) {
	if databasePath == "" || databasePath == ":memory:" || attachmentsPath == "" {
		return nil, fmt.Errorf("database and attachment paths must be persistent files and directories")
	}
	databaseDir := filepath.Dir(databasePath)
	if err := os.MkdirAll(databaseDir, 0o700); err != nil {
		return nil, err
	}
	id := security.ID()
	stage := filepath.Join(databaseDir, ".restore-"+id)
	if err := os.Mkdir(stage, 0o700); err != nil {
		return nil, fmt.Errorf("create restore staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := extractArchive(archivePath, stage); err != nil {
		return nil, err
	}
	if err := validateSnapshot(ctx, filepath.Join(stage, "database.db")); err != nil {
		return nil, err
	}
	attachmentsParent := filepath.Dir(attachmentsPath)
	if err := os.MkdirAll(attachmentsParent, 0o700); err != nil {
		return nil, err
	}
	newAttachments := filepath.Join(attachmentsParent, ".restore-"+id)
	if err := copyTree(filepath.Join(stage, "attachments"), newAttachments); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(newAttachments, 0o700); err != nil {
		return nil, err
	}
	defer os.RemoveAll(newAttachments)

	moved := []movedPath{}
	preserved := []string{}
	rollback := func() {
		for index := len(moved) - 1; index >= 0; index-- {
			_ = os.Rename(moved[index].to, moved[index].from)
		}
	}
	move := func(from, to string, preserve bool) error {
		if err := os.Rename(from, to); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			rollback()
			return err
		}
		moved = append(moved, movedPath{from: from, to: to})
		if preserve {
			preserved = append(preserved, to)
		}
		return nil
	}
	for _, target := range []string{databasePath, databasePath + "-wal", databasePath + "-shm", attachmentsPath} {
		if err := move(target, target+".pre-restore-"+id, true); err != nil {
			return nil, fmt.Errorf("preserve existing data: %w", err)
		}
	}
	if err := move(filepath.Join(stage, "database.db"), databasePath, false); err != nil {
		return nil, fmt.Errorf("install restored database: %w", err)
	}
	if err := move(newAttachments, attachmentsPath, false); err != nil {
		return nil, fmt.Errorf("install restored attachments: %w", err)
	}
	return preserved, nil
}

func extractArchive(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open backup archive: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	foundDatabase := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read backup archive: %w", err)
		}
		name := path.Clean(header.Name)
		if name == "." && header.FileInfo().IsDir() {
			continue
		}
		if name == "." || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || name != header.Name && !header.FileInfo().IsDir() {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		if name != "database.db" && name != "attachments" && !strings.HasPrefix(name, "attachments/") {
			return fmt.Errorf("unexpected backup entry %q", header.Name)
		}
		if header.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(destination, filepath.FromSlash(name)), 0o700); err != nil {
				return err
			}
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA || header.Size < 0 {
			return fmt.Errorf("backup entry is not a regular file: %q", header.Name)
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, reader)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if name == "database.db" {
			foundDatabase = true
		}
	}
	if !foundDatabase {
		return fmt.Errorf("backup does not contain database.db")
	}
	return nil
}

func validateSnapshot(ctx context.Context, filename string) error {
	db, err := sql.Open("sqlite", "file:"+filename+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return fmt.Errorf("validate restored database: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("restored database failed integrity check: %s", integrity)
	}
	var migrations int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migrations); err != nil || migrations < 1 {
		return fmt.Errorf("backup is not a migrated COM-ItemGather database")
	}
	return nil
}
