package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"com-itemgather/internal/database"
	"com-itemgather/internal/security"
)

type Manager struct {
	db                     *sql.DB
	attachments, directory string
	keep                   int
	mu                     sync.Mutex
}

func New(db *sql.DB, attachments, directory string, keep int) *Manager {
	return &Manager{db: db, attachments: attachments, directory: directory, keep: keep}
}
func (m *Manager) Create(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.MkdirAll(m.directory, 0o700); err != nil {
		return "", err
	}
	temp := filepath.Join(m.directory, ".backup-"+security.ID())
	if err := os.Mkdir(temp, 0o700); err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	dbPath := filepath.Join(temp, "database.db")
	if err := database.Backup(ctx, m.db, dbPath); err != nil {
		return "", err
	}
	attachmentsPath := filepath.Join(temp, "attachments")
	if err := copyTree(m.attachments, attachmentsPath); err != nil {
		return "", err
	}
	archive := filepath.Join(m.directory, time.Now().UTC().Format("20060102T150405Z")+"-"+security.ID()+".tar.gz")
	if err := makeArchive(temp, archive); err != nil {
		os.Remove(archive)
		return "", err
	}
	os.Chmod(archive, 0o600)
	if err := m.clean(); err != nil {
		return archive, err
	}
	return archive, nil
}
func copyTree(source, destination string) error {
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if _, err = io.Copy(output, input); err != nil {
			output.Close()
			return err
		}
		return output.Close()
	})
}
func makeArchive(source, destination string) error {
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(output)
	tarWriter := tar.NewWriter(gzipWriter)
	walkErr := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(source, path)
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Uid = 0
		header.Gid = 0
		header.Uname = ""
		header.Gname = ""
		if err = tarWriter.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tarWriter, file)
		return err
	})
	if err := tarWriter.Close(); walkErr == nil {
		walkErr = err
	}
	if err := gzipWriter.Close(); walkErr == nil {
		walkErr = err
	}
	if err := output.Close(); walkErr == nil {
		walkErr = err
	}
	return walkErr
}
func (m *Manager) clean() error {
	entries, err := filepath.Glob(filepath.Join(m.directory, "*.tar.gz"))
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		left, _ := os.Stat(entries[i])
		right, _ := os.Stat(entries[j])
		return left.ModTime().After(right.ModTime())
	})
	if len(entries) <= m.keep {
		return nil
	}
	for _, path := range entries[m.keep:] {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}
func RestoreInstructions() string {
	return strings.TrimSpace(fmt.Sprintf("Stop the service, extract the backup into an empty directory, replace the configured SQLite database and attachment directory, then run the migrate command before restarting."))
}
