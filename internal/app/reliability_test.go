package app

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestReliabilityFullRegenerationPreservesEditsAfterStart(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{}, 1)
	var calls atomic.Int32
	env := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) > 1 {
			started <- struct{}{}
			<-release
		}
		validAIHandler(w, r)
	})
	env.app.cfg.AITimeout = 5 * time.Second
	client := newAdminClient(t, env)
	idea := createTestIdea(t, client, "review-full", "绿色风暴")
	idea = waitAIStatus(t, client, idea.ID, "succeeded")
	status, data := client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, "display_title": "生成前的人工标题", "summary": "生成前的人工摘要"})
	assertStatus(t, status, 200, data)
	idea = decodeTestJSON[Idea](t, data)
	status, data = client.request(t, "POST", "/api/ideas/"+idea.ID+"/ai", map[string]string{"mode": "full"})
	assertStatus(t, status, 202, data)
	<-started
	status, data = client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, "display_title": "任务启动之后才写的人工标题"})
	assertStatus(t, status, 200, data)
	release <- struct{}{}
	result := waitAIStatus(t, client, idea.ID, "succeeded")
	if result.DisplayTitle != "任务启动之后才写的人工标题" {
		t.Fatalf("新人工编辑被覆盖: got=%q, want=%q", result.DisplayTitle, "任务启动之后才写的人工标题")
	}
	if result.Summary != "青苹果饮料" {
		t.Fatalf("已确认覆盖且未再编辑的字段应重新生成: summary=%q", result.Summary)
	}
}

func TestReliabilityNormalRegenerationUpdatesUntouchedFields(t *testing.T) {
	var calls atomic.Int32
	env := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			validAIHandler(w, r)
			return
		}
		result := AIResult{DisplayTitle: "更新后的整理标题", Summary: "第二轮根据新正文整理的摘要", NameCandidates: []string{}, IdeaTypes: []string{}, ProductDirections: []string{}, Tags: []string{}, Attributes: []Attribute{}}
		writeJSON(w, 200, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": encodeJSON(result)}, "finish_reason": "stop"}}})
	})
	client := newAdminClient(t, env)
	idea := createTestIdea(t, client, "review-normal", "绿色风暴")
	idea = waitAIStatus(t, client, idea.ID, "succeeded")
	// Match IdeaDetailView.payload: changing only content still submits every field.
	body := map[string]any{"version": idea.Version, "content": "后来补充了完全不同的正文", "display_title": idea.DisplayTitle, "summary": idea.Summary, "notes": idea.Notes, "name_candidates": idea.NameCandidates, "idea_types": idea.IdeaTypes, "product_directions": idea.ProductDirections, "tags": idea.Tags, "attributes": idea.Attributes}
	status, data := client.request(t, "PATCH", "/api/ideas/"+idea.ID, body)
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "POST", "/api/ideas/"+idea.ID+"/ai", map[string]string{"mode": "normal"})
	assertStatus(t, status, 202, data)
	result := waitAIStatus(t, client, idea.ID, "succeeded")
	if result.Summary != "第二轮根据新正文整理的摘要" {
		t.Fatalf("用户只改正文却把其余字段全部锁成人工字段: summary=%q, manual_fields=%v", result.Summary, result.ManualFields)
	}
}

func TestReliabilityHistoryRestoreResetsCancelledAIState(t *testing.T) {
	env := newTestEnvironment(t, nil)
	client := newAdminClient(t, env)
	idea := createTestIdea(t, client, "review-history", "最初的正文")
	idea = waitAIStatus(t, client, idea.ID, "succeeded")
	env.cancel()
	status, data := client.request(t, "GET", "/api/ideas/"+idea.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[map[string][]Revision](t, data)["items"]
	status, data = client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, "content": "第二版的正文"})
	assertStatus(t, status, 200, data)
	idea = decodeTestJSON[Idea](t, data)
	status, data = client.request(t, "POST", "/api/ideas/"+idea.ID+"/ai", map[string]string{"mode": "normal"})
	assertStatus(t, status, 202, data)
	status, data = client.request(t, "POST", "/api/ideas/"+idea.ID+"/revisions/"+revisions[len(revisions)-1].ID+"/restore", map[string]int{"version": idea.Version})
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID, nil)
	assertStatus(t, status, 200, data)
	result := decodeTestJSON[Idea](t, data)
	var active int
	env.db.QueryRow("SELECT COUNT(*) FROM ai_tasks WHERE idea_id=? AND status IN ('pending','running')", idea.ID).Scan(&active)
	if result.AIStatus != "idle" || active != 0 {
		t.Fatalf("历史恢复已取消任务，界面仍显示旧状态: ai_status=%s active_tasks=%d", result.AIStatus, active)
	}
}

func TestReliabilityTagRenameAdvancesVersionAndPublishesEvent(t *testing.T) {
	env := newTestEnvironment(t, nil)
	client := newAdminClient(t, env)
	idea := createTestIdea(t, client, "review-tags", "绿色风暴")
	idea = waitAIStatus(t, client, idea.ID, "succeeded")
	var tagID string
	var beforeEvent, afterEvent int
	env.db.QueryRow("SELECT id FROM tags WHERE name='青苹果'").Scan(&tagID)
	env.db.QueryRow("SELECT COALESCE(MAX(id),0) FROM events").Scan(&beforeEvent)
	status, data := client.request(t, "PATCH", "/api/tags/"+tagID, map[string]string{"name": "重命名后的标签"})
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID, nil)
	assertStatus(t, status, 200, data)
	updated := decodeTestJSON[Idea](t, data)
	env.db.QueryRow("SELECT COALESCE(MAX(id),0) FROM events").Scan(&afterEvent)
	if updated.Version <= idea.Version || afterEvent <= beforeEvent {
		t.Fatalf("标签变更绕过版本与事件: version=%d->%d, event=%d->%d, tags=%v", idea.Version, updated.Version, beforeEvent, afterEvent, updated.Tags)
	}
	status, data = client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, "tags": idea.Tags})
	assertStatus(t, status, 409, data)
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[map[string][]Revision](t, data)["items"]
	if revisions[0].Action != "update_tags" || revisions[0].Version != updated.Version {
		t.Fatal("标签修改未记录历史")
	}
	status, data = client.request(t, "DELETE", "/api/tags/"+tagID, nil)
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID, nil)
	assertStatus(t, status, 200, data)
	deleted := decodeTestJSON[Idea](t, data)
	if deleted.Version != updated.Version+1 || len(deleted.Tags) != 0 {
		t.Fatal("删除标签未更新灵感版本或内容")
	}
}
