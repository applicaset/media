package postgres_test

import (
	"testing"

	"github.com/applicaset/media"
	"github.com/applicaset/media/postgres"
	"github.com/applicaset/media/repotest"
	"github.com/applicaset/pkg/pgtest"
	"github.com/stretchr/testify/require"
)

func TestRepository(t *testing.T) {
	dsn := pgtest.DSN(t)

	repotest.Run(t, func(t *testing.T) media.FileRepository {
		t.Helper()

		db := pgtest.Open(t, dsn)
		require.NoError(t, postgres.Migrate(t.Context(), db))

		return postgres.NewFileRepository(db)
	})
}
