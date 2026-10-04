package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/applicaset/media"
	"github.com/applicaset/media/repotest"
	"github.com/applicaset/media/sqlite"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestRepository(t *testing.T) {
	repotest.Run(t, func(t *testing.T) media.FileRepository {
		t.Helper()

		db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/test.db?_pragma=foreign_keys(ON)")
		require.NoError(t, err)

		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })

		require.NoError(t, sqlite.Migrate(t.Context(), db))

		return sqlite.NewFileRepository(db)
	})
}
