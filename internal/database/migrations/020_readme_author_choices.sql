ALTER TABLE readme_authors RENAME TO readme_legacy_authors;

CREATE TABLE readme_authors (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL COLLATE NOCASE,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (name, email)
);

-- Preserve existing identities, combining identical name/email pairs.
INSERT INTO readme_authors (id, name, email, created_at, updated_at)
SELECT MIN(user_id), name, MIN(email),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
FROM readme_legacy_authors
GROUP BY name, email COLLATE NOCASE;

CREATE TABLE readme_author_choices (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    author_id TEXT REFERENCES readme_authors(id) ON DELETE SET NULL
);

INSERT INTO readme_author_choices (user_id, author_id)
SELECT previous.user_id, author.id
FROM readme_legacy_authors previous
JOIN readme_authors author ON author.name = previous.name AND author.email = previous.email;

DROP TABLE readme_legacy_authors;
