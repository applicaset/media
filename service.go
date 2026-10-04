package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
	"uuid"
)

const defaultContentType = "application/octet-stream"

type Service struct {
	files  FileRepository
	blobs  BlobStore
	logger *slog.Logger
	now    func() time.Time
	// blobLocks serializes the writes and removals of one blob, so a delete that sees no other
	// file cannot remove a blob an upload of the same bytes is about to record.
	// TODO: this holds only within one process. Several media instances over one disk need the
	// lock in the database.
	blobLocks keyedMutex
}

func NewService(files FileRepository, blobs BlobStore, logger *slog.Logger) *Service {
	return &Service{files: files, blobs: blobs, logger: logger, now: currentTime}
}

// currentTime is truncated to the precision every backend keeps, so a value held in memory and
// the same value read back compare equal.
func currentTime() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

// Upload stores everything r yields. A reader that ends in an error stores nothing.
func (svc *Service) Upload(ctx context.Context, r io.Reader, contentType string) (*File, error) {
	staged, err := svc.blobs.Stage(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("stage blob: %w", err)
	}

	if staged.Size() == 0 {
		return nil, errors.Join(ErrEmptyFile, staged.Discard())
	}

	file := &File{
		ID:          uuid.NewV7().String(),
		SHA256:      staged.SHA256(),
		Size:        staged.Size(),
		ContentType: normalizeContentType(contentType),
		CreatedAt:   svc.now(),
	}

	unlock := svc.blobLocks.lock(file.SHA256)
	defer unlock()

	if err := staged.Commit(); err != nil {
		return nil, fmt.Errorf("commit blob: %w", err)
	}

	if err := svc.files.Insert(ctx, file); err != nil {
		svc.removeIfUnused(ctx, file.SHA256)

		return nil, fmt.Errorf("insert file: %w", err)
	}

	return file, nil
}

func (svc *Service) Stat(ctx context.Context, id string) (*File, error) {
	file, err := svc.files.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}

	return file, nil
}

// Open returns the file's bytes. The caller closes them.
func (svc *Service) Open(ctx context.Context, id string) (*File, io.ReadSeekCloser, error) {
	file, err := svc.Stat(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	content, err := svc.blobs.Open(ctx, file.SHA256)
	if err != nil {
		return nil, nil, fmt.Errorf("open blob: %w", err)
	}

	return file, content, nil
}

// Delete removes the file, and its blob once no other file shares it.
func (svc *Service) Delete(ctx context.Context, id string) error {
	file, err := svc.Stat(ctx, id)
	if err != nil {
		return err
	}

	unlock := svc.blobLocks.lock(file.SHA256)
	defer unlock()

	if err := svc.files.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete file: %w", err)
	}

	svc.removeIfUnused(ctx, file.SHA256)

	return nil
}

// removeIfUnused only logs a failure. The file row is what callers see, and a blob left behind
// costs disk, not correctness.
func (svc *Service) removeIfUnused(ctx context.Context, sha256 string) {
	count, err := svc.files.CountBySHA256(ctx, sha256)
	if err != nil {
		svc.logger.WarnContext(ctx, "count blob users", slog.Any("error", err))

		return
	}

	if count > 0 {
		return
	}

	if err := svc.blobs.Remove(ctx, sha256); err != nil {
		svc.logger.WarnContext(ctx, "remove blob", slog.Any("error", err))
	}
}

func normalizeContentType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return defaultContentType
	}

	return contentType
}

// keyedMutex is one mutex per key, dropped when nobody holds or waits for it.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	sync.Mutex

	waiters int
}

func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()

	if k.locks == nil {
		k.locks = make(map[string]*keyedLock)
	}

	entry, ok := k.locks[key]
	if !ok {
		entry = &keyedLock{}
		k.locks[key] = entry
	}

	entry.waiters++
	k.mu.Unlock()

	entry.Lock()

	return func() {
		entry.Unlock()

		k.mu.Lock()
		defer k.mu.Unlock()

		entry.waiters--
		if entry.waiters == 0 {
			delete(k.locks, key)
		}
	}
}
