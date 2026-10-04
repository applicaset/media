// Package sqlite stores media file records in SQLite. It owns its schema, applied by Migrate.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/applicaset/media/internal/sqlstore"
	"github.com/applicaset/pkg/sqlmigrate"
)

//go:embed migrations/*.sql
var migrations embed.FS

// timeFormat sorts lexicographically, so ordering and range queries work on the stored text.
const timeFormat = "2006-01-02T15:04:05.000Z"

func Migrate(ctx context.Context, db *sql.DB) error {
	runner := sqlmigrate.Runner{
		FileSystem: migrations,
		Directory:  "migrations",
		TableName:  "media_schema_migrations",
	}

	if err := runner.Up(ctx, db); err != nil {
		return fmt.Errorf("migrate media schema: %w", err)
	}

	return nil
}

var dialect = sqlstore.Dialect{
	Placeholders: squirrel.Question,
	EncodeTime:   func(t time.Time) any { return t.UTC().Format(timeFormat) },
	TimeLayout:   timeFormat,
}

func NewFileRepository(db *sql.DB) *sqlstore.FileRepository {
	return sqlstore.NewFileRepository(db, dialect)
}
