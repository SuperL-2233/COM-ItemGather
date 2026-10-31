package app

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestAIRetriesAndManualRecovery(t *testing.T) {
	var calls atomic.Int32
	environment := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			writeJSON(w, 200, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "  "}, "finish_reason": "stop"}}})
		case 2:
			writeJSON(w, 200, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "not-json"}, "finish_reason": "stop"}}})
		case 3:
			writeError(w, fail(500, "upstream", "error"))
		default:
			validAIHandler(w, r)
		}
	})
	client := newAdminClient(t, environment)
	idea := createTestIdea(t, client, "retry", "绿色风暴")
	waitAIStatus(t, client, idea.ID, "succeeded")
	if calls.Load() != 4 {
		t.Fatalf("expected four AI calls, got %d", calls.Load())
	}
}
func TestAIFailurePreservesOriginalAndRetries(t *testing.T) {
	var calls atomic.Int32
	var healthy atomic.Bool
	environment := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if healthy.Load() {
			validAIHandler(w, r)
			return
		}
		writeError(w, fail(500, "upstream", "temporary"))
	})
	environment.app.cfg.AIMaxRetries = 1
	client := newAdminClient(t, environment)
	idea := createTestIdea(t, client, "failed", "必须保留的原文")
	failed := waitAIStatus(t, client, idea.ID, "failed")
	if failed.OriginalContent != "必须保留的原文" || failed.AIError == "" {
		t.Fatal("AI failure lost original or error")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected two calls, got %d", calls.Load())
	}
	healthy.Store(true)
	status, data := client.request(t, "POST", "/api/ideas/"+idea.ID+"/ai", map[string]string{"mode": "normal"})
	assertStatus(t, status, 202, data)
	waitAIStatus(t, client, idea.ID, "succeeded")
	if calls.Load() != 3 {
		t.Fatal("manual retry did not call AI")
	}
}
func TestAITimeoutAndInvalidCredentials(t *testing.T) {
	environment := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(100 * time.Millisecond); validAIHandler(w, r) })
	environment.app.cfg.AITimeout = 20 * time.Millisecond
	environment.app.cfg.AIMaxRetries = 0
	client := newAdminClient(t, environment)
	idea := createTestIdea(t, client, "timeout", "timeout test")
	failed := waitAIStatus(t, client, idea.ID, "failed")
	if failed.OriginalContent != "timeout test" {
		t.Fatal("timeout lost content")
	}
	var calls atomic.Int32
	unauthorized := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeError(w, fail(401, "unauthorized", "bad key"))
	})
	admin := newAdminClient(t, unauthorized)
	second := createTestIdea(t, admin, "unauthorized", "test")
	waitAIStatus(t, admin, second.ID, "failed")
	if calls.Load() != 1 {
		t.Fatalf("invalid API key should not retry, got %d calls", calls.Load())
	}
}
func TestAIProtectsManualFieldsAndStaleContent(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{}, 2)
	environment := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) { started <- struct{}{}; <-release; validAIHandler(w, r) })
	client := newAdminClient(t, environment)
	idea := createTestIdea(t, client, "manual", "绿色风暴")
	<-started
	status, data := client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, "display_title": "人工标题"})
	assertStatus(t, status, 200, data)
	release <- struct{}{}
	idea = waitAIStatus(t, client, idea.ID, "succeeded")
	if idea.DisplayTitle != "人工标题" {
		t.Fatalf("AI overwrote manual title: %s", idea.DisplayTitle)
	}
	status, data = client.request(t, "POST", "/api/ideas/"+idea.ID+"/ai", map[string]string{"mode": "normal"})
	assertStatus(t, status, 202, data)
	<-started
	status, data = client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, "content": "更新后的正文"})
	assertStatus(t, status, 200, data)
	release <- struct{}{}
	idea = waitAIStatus(t, client, idea.ID, "idle")
	if idea.Content != "更新后的正文" || idea.DisplayTitle != "人工标题" {
		t.Fatal("stale AI result overwrote new content")
	}
	time.Sleep(100 * time.Millisecond)
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID, nil)
	assertStatus(t, status, 200, data)
	if decodeTestJSON[Idea](t, data).AIStatus == "failed" {
		t.Fatal("cancelled old task marked idea as failed")
	}
}
