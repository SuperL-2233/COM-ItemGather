package app

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func openEventStream(t *testing.T, client *testClient, lastID string) (*bufio.Reader, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	req, _ := http.NewRequestWithContext(ctx, "GET", client.base+"/api/events", nil)
	for _, cookie := range cookieForURL(client.http, client.base) {
		req.AddCookie(cookie)
	}
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	response, err := client.http.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		cancel()
		response.Body.Close()
		t.Fatalf("SSE status %d", response.StatusCode)
	}
	t.Cleanup(func() { cancel(); response.Body.Close() })
	return bufio.NewReader(response.Body), cancel
}
func waitEvent(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var event strings.Builder
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		event.WriteString(line)
		if line == "\n" && strings.Contains(event.String(), "event: change") {
			return event.String()
		}
		if event.Len() > 100000 {
			t.Fatal("event too large")
		}
	}
	t.Fatal("SSE event timeout")
	return ""
}
func TestSSEUpdateAndReplay(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	reader, _ := openEventStream(t, client, "")
	idea := createTestIdea(t, client, "sse", "绿色风暴")
	event := waitEvent(t, reader)
	if !strings.Contains(event, "idea.created") || !strings.Contains(event, idea.ID) {
		t.Fatalf("unexpected SSE event %s", event)
	}
	replayed, _ := openEventStream(t, client, "0")
	event = waitEvent(t, replayed)
	if !strings.Contains(event, "idea.created") {
		t.Fatal("SSE reconnect did not replay missed events")
	}
}
