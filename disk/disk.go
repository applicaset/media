// Package disk keeps media blobs as files in one directory, named by their SHA-256.
package disk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/applicaset/media"
)

const (
	blobsDir = "blobs"
	tmpDir   = "tmp"
)

var (
	errInvalidHash = errors.New("invalid sha256")
	hashPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Store struct {
	root string
}

var _ media.BlobStore = (*Store)(nil)

// New creates root and its subdirectories if they are missing.
func New(root string) (*Store, error) {
	for _, dir := range []string{blobsDir, tmpDir} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			return nil, fmt.Errorf("create %s directory: %w", dir, err)
		}
	}

	return &Store{root: root}, nil
}

func (s *Store) Stage(ctx context.Context, r io.Reader) (media.StagedBlob, error) {
	temp, err := os.CreateTemp(filepath.Join(s.root, tmpDir), "upload-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary file: %w", err)
	}

	staged := &stagedBlob{store: s, tempPath: temp.Name()}

	hash := sha256.New()

	size, err := io.Copy(io.MultiWriter(temp, hash), contextReader{ctx: ctx, r: r})
	if err == nil {
		err = temp.Sync()
	}

	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		return nil, errors.Join(fmt.Errorf("write temporary file: %w", err), staged.Discard())
	}

	staged.sha256 = hex.EncodeToString(hash.Sum(nil))
	staged.size = size

	return staged, nil
}

func (s *Store) Open(_ context.Context, sha256 string) (io.ReadSeekCloser, error) {
	path, err := s.path(sha256)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: blob %s is missing", media.ErrFileNotFound, sha256)
	}

	if err != nil {
		return nil, fmt.Errorf("open blob: %w", err)
	}

	return file, nil
}

func (s *Store) Remove(_ context.Context, sha256 string) error {
	path, err := s.path(sha256)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove blob: %w", err)
	}

	return nil
}

// path shards by the first two bytes, so no directory holds more than a few thousand entries.
func (s *Store) path(sha256 string) (string, error) {
	if !hashPattern.MatchString(sha256) {
		return "", fmt.Errorf("%w: %q", errInvalidHash, sha256)
	}

	return filepath.Join(s.root, blobsDir, sha256[0:2], sha256[2:4], sha256), nil
}

type stagedBlob struct {
	store    *Store
	tempPath string
	sha256   string
	size     int64
}

func (b *stagedBlob) SHA256() string { return b.sha256 }

func (b *stagedBlob) Size() int64 { return b.size }

// Commit keeps an existing blob with the same hash: equal hashes mean equal bytes.
func (b *stagedBlob) Commit() error {
	path, err := b.store.path(b.sha256)
	if err != nil {
		return errors.Join(err, b.Discard())
	}

	if _, err := os.Stat(path); err == nil {
		return b.Discard()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return errors.Join(fmt.Errorf("create blob directory: %w", err), b.Discard())
	}

	if err := os.Rename(b.tempPath, path); err != nil {
		return errors.Join(fmt.Errorf("move blob into place: %w", err), b.Discard())
	}

	return nil
}

func (b *stagedBlob) Discard() error {
	if err := os.Remove(b.tempPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove temporary file: %w", err)
	}

	return nil
}

// contextReader stops a long copy once the caller has gone.
type contextReader struct {
	ctx context.Context //nolint:containedctx // Read has no context parameter to take it from.
	r   io.Reader
}

func (c contextReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, fmt.Errorf("read upload: %w", err)
	}

	// io.EOF must reach io.Copy unwrapped.
	return c.r.Read(p)
}
