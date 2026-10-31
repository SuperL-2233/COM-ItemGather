package app

import (
	"context"
	"net/http/httptest"
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
	restarted.Start(ctx)
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
