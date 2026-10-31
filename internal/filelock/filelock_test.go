package filelock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCrossProcessExclusionAndCancellation(t *testing.T) {
	if path := os.Getenv("ITEMGATHER_TEST_LOCK"); path != "" {
		if unlock, err := TryAcquire(context.Background(), path); !errors.Is(err, ErrLocked) {
			if unlock != nil {
				unlock()
			}
			t.Fatalf("another process entered the locked resource: %v", err)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "resource.lock")
	unlock, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	child := exec.Command(os.Args[0], "-test.run=^TestCrossProcessExclusionAndCancellation$")
	child.Env = append(os.Environ(), "ITEMGATHER_TEST_LOCK="+path)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("cross-process exclusion: %v\n%s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if release, err := Acquire(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		if release != nil {
			release()
		}
		t.Fatalf("lock acquisition ignored cancellation: %v", err)
	}
	unlock()
	release, err := TryAcquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
