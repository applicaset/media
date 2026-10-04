package httpapi_test

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/applicaset/media"
	"github.com/applicaset/media/client"
	"github.com/applicaset/media/disk"
	"github.com/applicaset/media/httpapi"
	"github.com/applicaset/media/sqlite"
	"github.com/applicaset/pkg/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newClient(t *testing.T, maxBytes int64) *client.Client {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/test.db")
	require.NoError(t, err)

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, sqlite.Migrate(t.Context(), db))

	blobs, err := disk.New(t.TempDir())
	require.NoError(t, err)

	logger := slog.New(slog.DiscardHandler)

	handler, err := httpapi.NewHandler(
		media.NewService(sqlite.NewFileRepository(db), blobs, logger), logger, maxBytes)
	require.NoError(t, err)

	mux := http.NewServeMux()
	handler.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mediaClient, err := client.New(server.URL, httpx.ClientOptions{})
	require.NoError(t, err)

	return mediaClient
}

func TestUploadThenServeARange(t *testing.T) {
	mediaClient := newClient(t, 1<<20)

	file, err := mediaClient.Upload(t.Context(), strings.NewReader("0123456789"), "audio/mpeg")
	require.NoError(t, err)
	assert.Equal(t, int64(10), file.Size)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	request.Header.Set("Range", "bytes=2-4")

	recorder := httptest.NewRecorder()
	require.NoError(t, mediaClient.Serve(recorder, request, file.ID))

	assert.Equal(t, http.StatusPartialContent, recorder.Code)
	assert.Equal(t, "234", recorder.Body.String())
	assert.Equal(t, "audio/mpeg", recorder.Header().Get("Content-Type"))
	assert.Equal(t, `"`+file.SHA256+`"`, recorder.Header().Get("ETag"))
}

func TestOpenReadsTheWholeFile(t *testing.T) {
	mediaClient := newClient(t, 1<<20)

	file, err := mediaClient.Upload(t.Context(), strings.NewReader("whole"), "text/plain")
	require.NoError(t, err)

	content, err := mediaClient.Open(t.Context(), file.ID)
	require.NoError(t, err)

	defer func() { _ = content.Close() }()

	body, err := io.ReadAll(content)
	require.NoError(t, err)
	assert.Equal(t, "whole", string(body))
}

func TestUploadOverTheLimitIsRefused(t *testing.T) {
	mediaClient := newClient(t, 4)

	_, err := mediaClient.Upload(t.Context(), strings.NewReader("too long"), "text/plain")

	httpError, ok := errors.AsType[*httpx.Error](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, httpx.CodeInvalidInput, httpError.Code)
}

func TestDeleteThenStatIsNotFound(t *testing.T) {
	mediaClient := newClient(t, 1<<20)

	file, err := mediaClient.Upload(t.Context(), strings.NewReader("gone"), "text/plain")
	require.NoError(t, err)

	require.NoError(t, mediaClient.Delete(t.Context(), file.ID))

	_, err = mediaClient.Stat(t.Context(), file.ID)

	httpError, ok := errors.AsType[*httpx.Error](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, httpx.CodeNotFound, httpError.Code)
}
