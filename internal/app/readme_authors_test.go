package app

import "testing"

func TestReadmeSharedAuthorsAndIndependentSelections(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	admin := newAdminClient(t, environment)
	createTestUser(t, environment.db, "writer", "member")
	member := newTestClient(t, environment.server)
	member.login(t, "writer", "correct-horse-42")
	author := createTestReadmeAuthor(t, admin, "作者甲", "123+author@users.noreply.github.com")
	memberState := readReadme(t, member)
	if len(memberState.Authors) != 1 || memberState.Authors[0].ID != author.ID || memberState.SelectedAuthorID != "" {
		t.Fatal("author list should be shared without selecting an author for another member")
	}
	selectTestReadmeAuthor(t, admin, author)
	if readReadme(t, admin).SelectedAuthorID != author.ID || readReadme(t, member).SelectedAuthorID != "" {
		t.Fatal("author preference should belong to the signed-in user")
	}
	status, data := member.request(t, "POST", "/api/readme/authors", map[string]string{"name": author.Name, "email": "123+AUTHOR@users.noreply.github.com"})
	assertStatus(t, status, 409, data)
	if testError(t, data) != "readme_author_duplicate" {
		t.Fatal("duplicate identity was accepted")
	}
	status, data = member.request(t, "PATCH", "/api/readme/authors/"+author.ID, map[string]any{"name": "作者甲更新", "email": "updated@example.com", "version": author.Version})
	assertStatus(t, status, 200, data)
	updated := decodeTestJSON[readmeCommitAuthor](t, data)
	if updated.Version != author.Version+1 || readReadme(t, admin).Authors[0].Email != updated.Email {
		t.Fatal("author edit was not shared or versioned")
	}
	status, data = admin.request(t, "PATCH", "/api/readme/authors/"+author.ID, map[string]any{"name": "旧页面覆盖", "email": author.Email, "version": author.Version})
	assertStatus(t, status, 409, data)
	status, data = admin.request(t, "DELETE", "/api/readme/authors/"+author.ID, map[string]any{"version": author.Version})
	assertStatus(t, status, 409, data)
	status, data = member.request(t, "PUT", "/api/readme/author", map[string]any{"author_id": author.ID, "author_version": author.Version})
	assertStatus(t, status, 409, data)
	selectTestReadmeAuthor(t, member, updated)
	status, data = member.request(t, "DELETE", "/api/readme/authors/"+updated.ID, map[string]any{"version": updated.Version})
	assertStatus(t, status, 200, data)
	for _, client := range []*testClient{admin, member} {
		state := readReadme(t, client)
		if len(state.Authors) != 0 || state.SelectedAuthorID != "" {
			t.Fatal("deleting an author did not clear the selections")
		}
	}
	admin.csrf = "invalid"
	status, data = admin.request(t, "POST", "/api/readme/authors", map[string]string{"name": "CSRF", "email": "csrf@example.com"})
	assertStatus(t, status, 403, data)
}

func TestReadmePushRejectsChangedOrDeletedAuthor(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	mock := newReadmeMock(t, environment)
	admin := newAdminClient(t, environment)
	initial := readReadme(t, admin)
	author := createTestReadmeAuthor(t, admin, "原作者", "original@example.com")
	selectTestReadmeAuthor(t, admin, author)
	draft := saveTestReadme(t, admin, initial.Draft.Version, initial.Draft.Content+"\n保留这些修改。\n")
	// A stored preference cannot silently stand in for the author's explicit
	// identity and version shown in the confirmation dialog.
	status, data := admin.request(t, "POST", "/api/readme/push", map[string]any{"version": draft.Version, "message": "update"})
	assertStatus(t, status, 422, data)
	status, data = admin.request(t, "PATCH", "/api/readme/authors/"+author.ID, map[string]any{"name": "新作者", "email": "new@example.com", "version": author.Version})
	assertStatus(t, status, 200, data)
	updated := decodeTestJSON[readmeCommitAuthor](t, data)
	status, data = admin.request(t, "POST", "/api/readme/push", map[string]any{"version": draft.Version, "message": "update", "author_id": author.ID, "author_version": author.Version})
	assertStatus(t, status, 409, data)
	if testError(t, data) != "readme_author_conflict" {
		t.Fatal("stale author should require a new confirmation")
	}
	status, data = admin.request(t, "DELETE", "/api/readme/authors/"+updated.ID, map[string]any{"version": updated.Version})
	assertStatus(t, status, 200, data)
	status, data = admin.request(t, "POST", "/api/readme/push", map[string]any{"version": draft.Version, "message": "update", "author_id": updated.ID, "author_version": updated.Version})
	assertStatus(t, status, 422, data)
	if testError(t, data) != "readme_author_unavailable" {
		t.Fatal("deleted author was accepted")
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.pushes != 0 || readReadme(t, admin).Draft.Content != draft.Content {
		t.Fatal("invalid author pushed changes or discarded the draft")
	}
}
