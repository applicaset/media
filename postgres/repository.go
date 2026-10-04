// Package postgres stores media file records in Postgres. It owns its schema, applied by Migrate.
package postgres

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

func Migrate(ctx context.Context, db *sql.DB) error {
	runner := sqlmigrate.Runner{
		FileSystem: migrations,
		Directory:  "migrations",
		TableName:  "media_schema_migrations",
		Dialect:    sqlmigrate.Postgres{},
	}

	if err := runner.Up(ctx, db); err != nil {
		return fmt.Errorf("migrate media schema: %w", err)
	}

	return nil
}

var dialect = sqlstore.Dialect{
	Placeholders: squirrel.Dollar,
	EncodeTime:   func(t time.Time) any { return t.UTC() },
	TimeLayout:   time.RFC3339Nano,
}

func NewFileRepository(db *sql.DB) *sqlstore.FileRepository {
	return sqlstore.NewFileRepository(db, dialect)
}
