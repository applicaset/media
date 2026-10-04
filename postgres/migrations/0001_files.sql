CREATE TABLE media_files (
    id           text COLLATE "C" NOT NULL PRIMARY KEY,
    sha256       text COLLATE "C" NOT NULL,
    size         bigint NOT NULL,
    content_type text NOT NULL,
    created_at   timestamptz NOT NULL,
    CHECK (size > 0)
);

CREATE INDEX media_files_sha256_idx ON media_files (sha256);
