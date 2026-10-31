package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestAIWorkerRecoversPersistedTaskAfterRestart(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	environment.cancel()
	idea := createTestIdea(t, client, "restart-recovery", "重启后仍需整理")
	if _, err := environment.db.Exec("UPDATE ai_tasks SET status='running' WHERE idea_id=?", idea.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.db.Exec("UPDATE ideas SET ai_status='running' WHERE id=?", idea.ID); err != nil {
		t.Fatal(err)
	}
	cfg := environment.app.cfg
	environment.server.Close()
	restarted := New(environment.db, cfg, environment.app.log)
	server := httptest.NewServer(restarted)
	restarted.cfg.PublicURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	if err := restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cancel)
	t.Cleanup(server.Close)
	environment.app = restarted
	environment.server = server
	environment.cancel = cancel
	client.base = server.URL
	recovered := waitAIStatus(t, client, idea.ID, "succeeded")
	if recovered.OriginalContent != "重启后仍需整理" || recovered.DisplayTitle != "绿色风暴" {
		t.Fatalf("persisted task did not recover correctly: %+v", recovered)
	}
	var tasks int
	if err := environment.db.QueryRow("SELECT COUNT(*) FROM ai_tasks WHERE idea_id=?", idea.ID).Scan(&tasks); err != nil || tasks != 1 {
		t.Fatalf("restart duplicated AI tasks: %d %v", tasks, err)
	}
}

func TestRestartHonorsPersistedAttemptBudget(t *testing.T) {
	for _, attempts := range []int{3, 4} {
		t.Run(strconv.Itoa(attempts), func(t *testing.T) {
			var calls atomic.Int32
			env := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
			env.cancel()
			client := newAdminClient(t, env)
			idea := createTestIdea(t, client, "persisted-budget", "必须保留的原文")
			if _, err := env.db.Exec("UPDATE ai_tasks SET status='running',attempts=?,max_attempts=4 WHERE idea_id=?", attempts, idea.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := env.db.Exec("UPDATE ideas SET ai_status='running' WHERE id=?", idea.ID); err != nil {
				t.Fatal(err)
			}
			restarted := New(env.db, env.app.cfg, env.app.log)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := restarted.Start(ctx); err != nil {
				t.Fatal(err)
			}
			result := waitAIStatus(t, client, idea.ID, "failed")
			if calls.Load() != int32(4-attempts) || result.OriginalContent != "必须保留的原文" {
				t.Fatalf("restart exceeded budget: calls=%d previous_attempts=%d", calls.Load(), attempts)
			}
			var persisted int
			env.db.QueryRow("SELECT attempts FROM ai_tasks WHERE idea_id=?", idea.ID).Scan(&persisted)
			if persisted != 4 {
				t.Fatalf("attempt count was not persisted: %d", persisted)
			}
		})
	}
}

func TestRestartCancelsStaleExhaustedTaskWithoutFailingNewContent(t *testing.T) {
	var calls atomic.Int32
	env := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	env.cancel()
	client := newAdminClient(t, env)
	idea := createTestIdea(t, client, "stale-budget", "旧内容")
	if _, err := env.db.Exec("UPDATE ai_tasks SET status='running',attempts=4,max_attempts=4 WHERE idea_id=?", idea.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.db.Exec("UPDATE ideas SET content='新内容',content_version=2,ai_status='running' WHERE id=?", idea.ID); err != nil {
		t.Fatal(err)
	}
	restarted := New(env.db, env.app.cfg, env.app.log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	result := waitAIStatus(t, client, idea.ID, "idle")
	if result.Content != "新内容" || result.AIError != "" || calls.Load() != 0 {
		t.Fatal("stale exhausted task changed new content or status")
	}
	var status string
	if err := env.db.QueryRow("SELECT status FROM ai_tasks WHERE idea_id=?", idea.ID).Scan(&status); err != nil || status != "cancelled" {
		t.Fatalf("stale task was not cancelled: %s %v", status, err)
	}
}
