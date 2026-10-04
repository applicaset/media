// Package repotest is the contract every media repository must satisfy. Both backends run it, so a
// behaviour that differs between SQLite and Postgres fails here rather than in production.
package repotest

import (
	"testing"
	"time"

	"github.com/applicaset/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// New builds a repository over empty storage.
type New func(t *testing.T) media.FileRepository

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func Run(t *testing.T, newRepository New) {
	t.Helper()

	createdAt := time.Date(2026, 3, 1, 12, 0, 0, 123_000_000, time.UTC)

	newFile := func(id, hash string) *media.File {
		return &media.File{
			ID: id, SHA256: hash, Size: 42, ContentType: "audio/flac", CreatedAt: createdAt,
		}
	}

	t.Run("Get returns what Insert stored", func(t *testing.T) {
		repo := newRepository(t)

		require.NoError(t, repo.Insert(t.Context(), newFile("f1", hashA)))

		got, err := repo.Get(t.Context(), "f1")
		require.NoError(t, err)
		assert.Equal(t, newFile("f1", hashA), got)
	})

	t.Run("Get of an unknown id is ErrFileNotFound", func(t *testing.T) {
		repo := newRepository(t)

		_, err := repo.Get(t.Context(), "missing")
		require.ErrorIs(t, err, media.ErrFileNotFound)
	})

	t.Run("CountBySHA256 counts files sharing a blob", func(t *testing.T) {
		repo := newRepository(t)

		require.NoError(t, repo.Insert(t.Context(), newFile("f1", hashA)))
		require.NoError(t, repo.Insert(t.Context(), newFile("f2", hashA)))
		require.NoError(t, repo.Insert(t.Context(), newFile("f3", hashB)))

		count, err := repo.CountBySHA256(t.Context(), hashA)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("Delete removes one file", func(t *testing.T) {
		repo := newRepository(t)

		require.NoError(t, repo.Insert(t.Context(), newFile("f1", hashA)))
		require.NoError(t, repo.Insert(t.Context(), newFile("f2", hashA)))
		require.NoError(t, repo.Delete(t.Context(), "f1"))

		_, err := repo.Get(t.Context(), "f1")
		require.ErrorIs(t, err, media.ErrFileNotFound)

		count, err := repo.CountBySHA256(t.Context(), hashA)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("Delete of an unknown id is ErrFileNotFound", func(t *testing.T) {
		repo := newRepository(t)

		require.ErrorIs(t, repo.Delete(t.Context(), "missing"), media.ErrFileNotFound)
	})
}
