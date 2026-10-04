// Package httpapi serves the media service over HTTP, for callers running as separate processes.
// It is not published through the gateway.
package httpapi

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/applicaset/media"
	"github.com/applicaset/pkg/api/mediaapi"
	"github.com/applicaset/pkg/httpx"
)

var errMissingDependency = errors.New("missing dependency")

type Handler struct {
	service  *media.Service
	logger   *slog.Logger
	maxBytes int64
}

// NewHandler refuses an upload larger than maxBytes.
func NewHandler(service *media.Service, logger *slog.Logger, maxBytes int64) (*Handler, error) {
	if service == nil || logger == nil {
		return nil, fmt.Errorf("%w: service and logger must be set", errMissingDependency)
	}

	return &Handler{service: service, logger: logger, maxBytes: maxBytes}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST "+mediaapi.PathUpload, h.upload)
	mux.HandleFunc("GET "+mediaapi.PathDownload+"{id}", h.download)
	mux.HandleFunc("POST "+mediaapi.PathStat, h.stat)
	mux.HandleFunc("POST "+mediaapi.PathDelete, h.delete)
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	// The server's deadlines are meant for small calls. A large file takes as long as it takes.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Time{})
	_ = controller.SetWriteDeadline(time.Time{})

	body := http.MaxBytesReader(w, r.Body, h.maxBytes)

	file, err := h.service.Upload(r.Context(), body, r.Header.Get("Content-Type"))
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		err = media.ErrFileTooLarge
	}

	if err != nil {
		h.fail(w, r, "upload file", err)

		return
	}

	httpx.WriteJSON(w, mediaapi.FileResponse{File: WireFile(file)})
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	file, content, err := h.service.Open(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, "open file", err)

		return
	}
	defer func() { _ = content.Close() }()

	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Time{})

	Serve(w, r, file, content)
}

// Serve answers r with the file's bytes, honouring Range and If-None-Match. Its bytes never change,
// so a cache may keep them for good.
func Serve(w http.ResponseWriter, r *http.Request, file *media.File, content io.ReadSeeker) {
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("ETag", `"`+file.SHA256+`"`)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")

	if strings.HasPrefix(file.ContentType, "text/") {
		w.Header().Set("X-Content-Type-Options", "nosniff")
	}

	http.ServeContent(w, r, "", file.CreatedAt, content)
}

func (h *Handler) stat(w http.ResponseWriter, r *http.Request) {
	var request mediaapi.FileRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	file, err := h.service.Stat(r.Context(), request.FileID)
	if err != nil {
		h.fail(w, r, "stat file", err)

		return
	}

	httpx.WriteJSON(w, mediaapi.FileResponse{File: WireFile(file)})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	var request mediaapi.FileRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	if err := h.service.Delete(r.Context(), request.FileID); err != nil {
		h.fail(w, r, "delete file", err)

		return
	}

	httpx.WriteJSON(w, struct{}{})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if code, message, ok := Classify(err); ok {
		httpx.WriteError(w, code, message)

		return
	}

	httpx.WriteInternal(r.Context(), w, h.logger, operation, err)
}

// Classify is the only place a media error becomes a wire code. False means the system failed,
// not the request.
func Classify(err error) (httpx.Code, string, bool) {
	switch {
	case errors.Is(err, media.ErrFileNotFound):
		return httpx.CodeNotFound, "That file could not be found.", true
	case errors.Is(err, media.ErrFileTooLarge):
		return httpx.CodeInvalidInput, "That file is too large.", true
	case errors.Is(err, media.ErrEmptyFile):
		return httpx.CodeInvalidInput, "That file is empty.", true
	default:
		return "", "", false
	}
}

func WireFile(file *media.File) mediaapi.File {
	return mediaapi.File{
		ID:          file.ID,
		SHA256:      file.SHA256,
		Size:        file.Size,
		ContentType: file.ContentType,
		CreatedAt:   file.CreatedAt,
	}
}
