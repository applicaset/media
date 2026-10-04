// Package backend is the one place that maps a storage driver to the media repository.
package backend

import (
	"context"

	"github.com/applicaset/media"
	mediapostgres "github.com/applicaset/media/postgres"
	mediasqlite "github.com/applicaset/media/sqlite"
	"github.com/applicaset/pkg/storage"
)

// New migrates the schema before returning a repository over it.
func New(ctx context.Context, driver string, handle *storage.Handle) (media.FileRepository, error) {
	db := handle.SQL

	switch driver {
	case storage.DriverPostgres:
		if err := mediapostgres.Migrate(ctx, db); err != nil {
			return nil, err
		}

		return mediapostgres.NewFileRepository(db), nil
	default:
		if err := mediasqlite.Migrate(ctx, db); err != nil {
			return nil, err
		}

		return mediasqlite.NewFileRepository(db), nil
	}
}
