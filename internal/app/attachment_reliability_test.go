package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"com-itemgather/internal/backup"
	"com-itemgather/internal/database"
	"com-itemgather/internal/filelock"
)

func TestAttachmentUploadRetryIsIdempotent(t *testing.T) {
	env := newTestEnvironment(t, nil)
	client := newAdminClient(t, env)
	idea := createTestIdea(t, client, "attachment-retry", "")
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 20, 20))); err != nil {
		t.Fatal(err)
	}
	env.app.cfg.MaxAttachments = 1
	status, data := client.multipart(t, "/api/attachments", idea.ID, imageData.Bytes(), "image.png", "attachment-request")
	assertStatus(t, status, 201, data)
	first := decodeTestJSON[Attachment](t, data)
	status, data = client.multipart(t, "/api/attachments", idea.ID, imageData.Bytes(), "image.png", "attachment-request")
	assertStatus(t, status, 200, data)
	if retry := decodeTestJSON[Attachment](t, data); retry.ID != first.ID {
		t.Fatal("upload retry created a different attachment")
	}
	other := createTestIdea(t, client, "attachment-other-parent", "")
	status, data = client.multipart(t, "/api/attachments", other.ID, imageData.Bytes(), "image.png", "attachment-request")
	assertStatus(t, status, 409, data)
	var count int
	env.db.QueryRow("SELECT COUNT(*) FROM attachments").Scan(&count)
	if count != 1 {
		t.Fatalf("upload retry created %d attachment rows", count)
	}
}

func TestBackupAndImageDeletionShareSnapshotLock(t *testing.T) {
	env := newTestEnvironment(t, nil)
	client := newAdminClient(t, env)
	idea := createTestIdea(t, client, "snapshot-deletion", "")
	var imageData bytes.Buffer
	png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 20, 20)))
	status, data := client.multipart(t, "/api/attachments", idea.ID, imageData.Bytes(), "image.png")
	assertStatus(t, status, 201, data)
	attachment := decodeTestJSON[Attachment](t, data)
	unlock, err := filelock.Acquire(context.Background(), env.app.cfg.AttachmentDir+".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	manager := backup.New(env.db, env.app.cfg.AttachmentDir, env.app.cfg.BackupDir, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	if _, err := manager.Create(ctx); err == nil {
		cancel()
		t.Fatal("backup ignored attachment mutation lock")
	}
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	request := httptest.NewRequest("DELETE", "/api/attachments/"+attachment.ID, nil).WithContext(ctx)
	for _, cookie := range cookieForURL(client.http, client.base) {
		request.AddCookie(cookie)
	}
	request.Header.Set("X-CSRF-Token", client.csrf)
	recorder := httptest.NewRecorder()
	env.app.ServeHTTP(recorder, request)
	cancel()
	if recorder.Code == 200 {
		t.Fatal("image deletion ignored backup snapshot lock")
	}
	unlock()
	// Either concurrent operation may win; every completed archive must restore
	// with a complete image or with no reference to the deleted image.
	for iteration := 0; iteration < 4; iteration++ {
		if iteration > 0 {
			status, data = client.multipart(t, "/api/attachments", idea.ID, imageData.Bytes(), "image.png")
			assertStatus(t, status, 201, data)
			attachment = decodeTestJSON[Attachment](t, data)
		}
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		type backupResult struct {
			path string
			err  error
		}
		archives := make(chan backupResult, 1)
		deletions := make(chan int, 1)
		go func() { path, err := manager.Create(ctx); archives <- backupResult{path, err} }()
		request = httptest.NewRequest(http.MethodDelete, "/api/attachments/"+attachment.ID, nil).WithContext(ctx)
		for _, cookie := range cookieForURL(client.http, client.base) {
			request.AddCookie(cookie)
		}
		request.Header.Set("X-CSRF-Token", client.csrf)
		go func(request *http.Request) {
			recorder := httptest.NewRecorder()
			env.app.ServeHTTP(recorder, request)
			deletions <- recorder.Code
		}(request)
		result := <-archives
		deleted := <-deletions
		cancel()
		if result.err != nil || deleted != 200 {
			t.Fatalf("concurrent backup/delete failed: %v HTTP %d", result.err, deleted)
		}
		destination := filepath.Join(t.TempDir(), fmt.Sprintf("restore-%d", iteration))
		if _, err := backup.Restore(context.Background(), result.path, filepath.Join(destination, "app.db"), filepath.Join(destination, "attachments")); err != nil {
			t.Fatalf("incomplete concurrent snapshot: %v", err)
		}
		restored, err := database.Open(filepath.Join(destination, "app.db"))
		if err != nil {
			t.Fatal(err)
		}
		var count int
		err = restored.QueryRow("SELECT COUNT(*) FROM attachments").Scan(&count)
		restored.Close()
		if err != nil || count > 1 {
			t.Fatalf("unexpected snapshot attachment count: %d %v", count, err)
		}
	}
}
