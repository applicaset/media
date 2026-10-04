// Package app is the composition root of the media service running on its own. It is a package
// rather than a main so a test can build the routes without a listener.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/applicaset/media"
	"github.com/applicaset/media/backend"
	"github.com/applicaset/media/disk"
	"github.com/applicaset/media/httpapi"
	"github.com/applicaset/pkg/config"
	"github.com/applicaset/pkg/serve"
	"github.com/applicaset/pkg/storage"
	"github.com/nasermirzaei89/env"
)

// schema is the Postgres schema this service owns. SQLite ignores it.
const schema = "media"

var errInvalidConfig = errors.New("invalid configuration")

type Config struct {
	config.Server

	Log      config.Log
	Database storage.Config
	Files    FilesConfig
}

// FilesConfig says where blobs live and how large one upload may be. Every binary embedding media
// reads the same variables.
type FilesConfig struct {
	Dir      string
	MaxBytes int64
}

func LoadFilesConfig() FilesConfig {
	return FilesConfig{
		Dir:      env.GetString("MEDIA_DIR", "buildset-media"),
		MaxBytes: env.GetInt64("MEDIA_MAX_BYTES", 2<<30),
	}
}

func (c FilesConfig) Validate() error {
	if c.Dir == "" {
		return fmt.Errorf("%w: MEDIA_DIR must not be empty", errInvalidConfig)
	}

	if c.MaxBytes < 1 {
		return fmt.Errorf("%w: MEDIA_MAX_BYTES must be positive, got %d", errInvalidConfig,
			c.MaxBytes)
	}

	return nil
}

func LoadConfig(ctx context.Context) (*Config, error) {
	log, err := config.LoadLog()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server:   config.LoadServer(),
		Log:      log,
		Database: storage.Load(),
		Files:    LoadFilesConfig(),
	}

	if err := cfg.Validate(ctx); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate(ctx context.Context) error {
	if err := c.Server.Validate(ctx); err != nil {
		return err
	}

	if err := c.Database.Validate(); err != nil {
		return err
	}

	return c.Files.Validate()
}

// Build wires the media service over an open database. A single binary embedding media calls it
// with its own handle.
func Build(
	ctx context.Context,
	driver string,
	handle *storage.Handle,
	files FilesConfig,
	logger *slog.Logger,
) (*media.Service, error) {
	repo, err := backend.New(ctx, driver, handle)
	if err != nil {
		return nil, fmt.Errorf("build media repository: %w", err)
	}

	blobs, err := disk.New(files.Dir)
	if err != nil {
		return nil, fmt.Errorf("open media directory: %w", err)
	}

	return media.NewService(repo, blobs, logger), nil
}

type Service struct {
	handle *storage.Handle
	routes http.Handler
}

func New(ctx context.Context, cfg *Config, logger *slog.Logger) (*Service, error) {
	handle, err := storage.OpenHandle(ctx, cfg.Database, schema)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	service, err := Build(ctx, cfg.Database.Driver, handle, cfg.Files, logger)
	if err != nil {
		_ = handle.Close()

		return nil, err
	}

	handler, err := httpapi.NewHandler(service, logger, cfg.Files.MaxBytes)
	if err != nil {
		_ = handle.Close()

		return nil, fmt.Errorf("build media handler: %w", err)
	}

	mux := http.NewServeMux()
	handler.Register(mux)

	return &Service{handle: handle, routes: mux}, nil
}

func (svc *Service) Routes() http.Handler { return svc.routes }

func (svc *Service) Ping(ctx context.Context) error { return svc.handle.Ping(ctx) }

func (svc *Service) Close() error { return svc.handle.Close() }

func Run(ctx context.Context) error {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := serve.NewLogger(cfg.Log)
	slog.SetDefault(logger)

	service, err := New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := service.Close(); err != nil {
			logger.ErrorContext(ctx, "close service", slog.Any("error", err))
		}
	}()

	return serve.Run(ctx, serve.Options{
		Name:            "media",
		Address:         cfg.Address(),
		ShutdownTimeout: cfg.ShutdownTimeout,
		Logger:          logger,
		Routes:          service.Routes(),
		Ready:           service.Ping,
		// No browser reaches this service. Callers are sibling services, so their request
		// identifier is kept.
		CrossOrigin:    false,
		TrustRequestID: true,
	})
}
