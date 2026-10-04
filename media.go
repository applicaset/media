// Package media stores files and hands their bytes back, whole or by range. It knows nothing about
// what a file means or who may read it: the service that holds the file's ref decides that.
package media

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/applicaset/pkg/ref"
)

const (
	ServiceName      = "media"
	FileResourceType = "file"
)

var (
	ErrFileNotFound = errors.New("file not found")
	ErrFileTooLarge = errors.New("file too large")
	ErrEmptyFile    = errors.New("file is empty")
)

// File is one stored upload. Two files with the same bytes share one blob, so deleting one leaves
// the other readable.
type File struct {
	ID          string
	SHA256      string
	Size        int64
	ContentType string
	CreatedAt   time.Time
}

func FileRef(id string) string {
	return ref.MustNew(ServiceName, FileResourceType, id).String()
}

type FileRepository interface {
	Insert(ctx context.Context, file *File) error
	Get(ctx context.Context, id string) (*File, error)
	Delete(ctx context.Context, id string) error
	CountBySHA256(ctx context.Context, sha256 string) (int, error)
}

// BlobStore keeps bytes under their SHA-256. A blob is written in two steps, so the caller can
// record the file between learning the hash and making the blob visible.
type BlobStore interface {
	Stage(ctx context.Context, r io.Reader) (StagedBlob, error)
	Open(ctx context.Context, sha256 string) (io.ReadSeekCloser, error)
	Remove(ctx context.Context, sha256 string) error
}

// StagedBlob is written but not yet addressable. Exactly one of Commit or Discard must follow.
type StagedBlob interface {
	SHA256() string
	Size() int64
	Commit() error
	Discard() error
}
