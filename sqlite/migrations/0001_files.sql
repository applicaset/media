CREATE TABLE media_files (
    id           TEXT NOT NULL PRIMARY KEY,
    sha256       TEXT NOT NULL,
    size         INTEGER NOT NULL,
    content_type TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    CHECK (size > 0)
) STRICT;

CREATE INDEX media_files_sha256_idx ON media_files (sha256);
