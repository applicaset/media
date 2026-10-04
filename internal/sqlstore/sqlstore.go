// Package sqlstore holds the media repository over database/sql. SQLite and Postgres differ only
// in placeholders and how a timestamp is stored, so each backend supplies a Dialect and shares the
// queries.
package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/applicaset/media"
)

const tableFiles = "media_files"

var errTimestampType = errors.New("cannot scan a timestamp from this type")

type Dialect struct {
	Placeholders squirrel.PlaceholderFormat
	// EncodeTime is how a timestamp is written. Reading accepts either a time or text.
	EncodeTime func(time.Time) any
	// TimeLayout parses a timestamp stored as text.
	TimeLayout string
}

type FileRepository struct {
	db      *sql.DB
	dialect Dialect
}

var _ media.FileRepository = (*FileRepository)(nil)

func NewFileRepository(db *sql.DB, dialect Dialect) *FileRepository {
	return &FileRepository{db: db, dialect: dialect}
}

func (r *FileRepository) builder() squirrel.StatementBuilderType {
	return squirrel.StatementBuilder.PlaceholderFormat(r.dialect.Placeholders).RunWith(r.db)
}

func (r *FileRepository) Insert(ctx context.Context, file *media.File) error {
	_, err := r.builder().
		Insert(tableFiles).
		Columns("id", "sha256", "size", "content_type", "created_at").
		Values(file.ID, file.SHA256, file.Size, file.ContentType,
			r.dialect.EncodeTime(file.CreatedAt.UTC())).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("insert file: %w", err)
	}

	return nil
}

func (r *FileRepository) Get(ctx context.Context, id string) (*media.File, error) {
	var (
		file      media.File
		createdAt = timestamp{layout: r.dialect.TimeLayout}
	)

	err := r.builder().
		Select("id", "sha256", "size", "content_type", "created_at").
		From(tableFiles).
		Where(squirrel.Eq{"id": id}).
		QueryRowContext(ctx).
		Scan(&file.ID, &file.SHA256, &file.Size, &file.ContentType, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, media.ErrFileNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("select file: %w", err)
	}

	file.CreatedAt = createdAt.value

	return &file, nil
}

func (r *FileRepository) Delete(ctx context.Context, id string) error {
	result, err := r.builder().
		Delete(tableFiles).
		Where(squirrel.Eq{"id": id}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete file: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted files: %w", err)
	}

	if affected == 0 {
		return media.ErrFileNotFound
	}

	return nil
}

func (r *FileRepository) CountBySHA256(ctx context.Context, sha256 string) (int, error) {
	var count int

	err := r.builder().
		Select("count(*)").
		From(tableFiles).
		Where(squirrel.Eq{"sha256": sha256}).
		QueryRowContext(ctx).
		Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count files: %w", err)
	}

	return count, nil
}

// timestamp scans a column stored as either a time or text in the dialect's layout.
type timestamp struct {
	layout string
	value  time.Time
}

func (t *timestamp) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		t.value = v.UTC()
	case string:
		return t.parse(v)
	case []byte:
		return t.parse(string(v))
	default:
		return fmt.Errorf("%w: %T", errTimestampType, src)
	}

	return nil
}

func (t *timestamp) parse(text string) error {
	parsed, err := time.Parse(t.layout, text)
	if err != nil {
		return fmt.Errorf("parse timestamp: %w", err)
	}

	t.value = parsed.UTC()

	return nil
}
