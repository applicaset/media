package media_test

import (
	"database/sql"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/applicaset/media"
	"github.com/applicaset/media/disk"
	"github.com/applicaset/media/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newService(t *testing.T) *media.Service {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/test.db")
	require.NoError(t, err)

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, sqlite.Migrate(t.Context(), db))

	blobs, err := disk.New(t.TempDir())
	require.NoError(t, err)

	return media.NewService(sqlite.NewFileRepository(db), blobs, slog.New(slog.DiscardHandler))
}

func read(t *testing.T, svc *media.Service, id string) string {
	t.Helper()

	_, content, err := svc.Open(t.Context(), id)
	require.NoError(t, err)

	defer func() { _ = content.Close() }()

	body, err := io.ReadAll(content)
	require.NoError(t, err)

	return string(body)
}

func TestUploadStoresBytesUnderTheirHash(t *testing.T) {
	svc := newService(t)

	file, err := svc.Upload(t.Context(), strings.NewReader("hello"), "")
	require.NoError(t, err)

	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", file.SHA256)
	assert.Equal(t, int64(5), file.Size)
	assert.Equal(t, "application/octet-stream", file.ContentType)
	assert.Equal(t, "hello", read(t, svc, file.ID))
}

func TestUploadRefusesAnEmptyFile(t *testing.T) {
	svc := newService(t)

	_, err := svc.Upload(t.Context(), strings.NewReader(""), "text/plain")
	require.ErrorIs(t, err, media.ErrEmptyFile)
}

func TestDeleteKeepsABlobAnotherFileShares(t *testing.T) {
	svc := newService(t)

	first, err := svc.Upload(t.Context(), strings.NewReader("same"), "text/plain")
	require.NoError(t, err)

	second, err := svc.Upload(t.Context(), strings.NewReader("same"), "text/plain")
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID)

	require.NoError(t, svc.Delete(t.Context(), first.ID))

	_, err = svc.Stat(t.Context(), first.ID)
	require.ErrorIs(t, err, media.ErrFileNotFound)
	assert.Equal(t, "same", read(t, svc, second.ID))

	require.NoError(t, svc.Delete(t.Context(), second.ID))

	_, _, err = svc.Open(t.Context(), second.ID)
	require.ErrorIs(t, err, media.ErrFileNotFound)
}
