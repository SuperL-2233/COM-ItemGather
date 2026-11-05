-- Only a Markdown working copy is stored; no game repository or Git objects.
CREATE TABLE readme_drafts (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repository TEXT NOT NULL,
    branch TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    base_content TEXT NOT NULL DEFAULT '',
    base_sha TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL,
    last_commit_url TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, repository, branch)
);

CREATE TABLE readme_authors (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    email TEXT NOT NULL
);
