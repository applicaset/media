// Package client calls the media service over HTTP.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/applicaset/pkg/api/mediaapi"
	"github.com/applicaset/pkg/httpx"
	"github.com/applicaset/pkg/reqid"
)

const maxErrorBytes = 8 << 10

var errUnexpectedAnswer = errors.New("unexpected answer")

// forwardedHeaders are the request headers a download passes through, so a browser's range and
// cache checks reach media.
var forwardedHeaders = []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"}

// copiedHeaders are the response headers a download passes back.
var copiedHeaders = []string{
	"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified",
	"Cache-Control", "X-Content-Type-Options",
}

type Client struct {
	call    *httpx.Client
	baseURL string
	// transfer has no timeout: a large file takes as long as it takes. The caller's context bounds it.
	transfer *http.Client
}

func New(baseURL string, opts httpx.ClientOptions) (*Client, error) {
	call, err := httpx.NewClient("media", baseURL, opts)
	if err != nil {
		return nil, err
	}

	return &Client{
		call:     call,
		baseURL:  strings.TrimRight(baseURL, "/"),
		transfer: &http.Client{},
	}, nil
}

func (c *Client) Upload(
	ctx context.Context,
	r io.Reader,
	contentType string,
) (mediaapi.File, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+mediaapi.PathUpload, r)
	if err != nil {
		return mediaapi.File{}, fmt.Errorf("build media upload: %w", err)
	}

	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", "application/json")
	setRequestID(ctx, request)

	response, err := c.transfer.Do(request)
	if err != nil {
		return mediaapi.File{}, fmt.Errorf("media upload: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return mediaapi.File{}, fmt.Errorf("media upload: %w", decodeError(response))
	}

	var body mediaapi.FileResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return mediaapi.File{}, fmt.Errorf("decode media upload: %w", err)
	}

	return body.File, nil
}

func (c *Client) Stat(ctx context.Context, fileID string) (mediaapi.File, error) {
	var response mediaapi.FileResponse

	err := c.call.Call(ctx, mediaapi.PathStat, mediaapi.FileRequest{FileID: fileID}, &response)

	return response.File, err
}

func (c *Client) Delete(ctx context.Context, fileID string) error {
	return c.call.Call(ctx, mediaapi.PathDelete, mediaapi.FileRequest{FileID: fileID}, nil)
}

// Open returns the file's whole content. The caller closes it.
func (c *Client) Open(ctx context.Context, fileID string) (io.ReadCloser, error) {
	response, err := c.get(ctx, fileID, nil)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		defer func() { _ = response.Body.Close() }()

		return nil, fmt.Errorf("media download: %w", decodeError(response))
	}

	return response.Body, nil
}

// Serve answers a browser's request with the file, passing its range and cache checks through.
func (c *Client) Serve(w http.ResponseWriter, r *http.Request, fileID string) error {
	response, err := c.get(r.Context(), fileID, r.Header)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusNotModified,
		http.StatusRequestedRangeNotSatisfiable:
	default:
		return fmt.Errorf("media download: %w", decodeError(response))
	}

	for _, name := range copiedHeaders {
		for _, value := range response.Header.Values(name) {
			w.Header().Add(name, value)
		}
	}

	w.WriteHeader(response.StatusCode)

	// The status is sent, so a failed copy can only be logged by the caller, not answered.
	if _, err := io.Copy(w, response.Body); err != nil {
		return fmt.Errorf("copy media download: %w", err)
	}

	return nil
}

func (c *Client) get(
	ctx context.Context,
	fileID string,
	header http.Header,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+mediaapi.PathDownload+url.PathEscape(fileID), nil)
	if err != nil {
		return nil, fmt.Errorf("build media download: %w", err)
	}

	for _, name := range forwardedHeaders {
		for _, value := range header.Values(name) {
			request.Header.Add(name, value)
		}
	}

	setRequestID(ctx, request)

	response, err := c.transfer.Do(request)
	if err != nil {
		return nil, fmt.Errorf("media download: %w", err)
	}

	return response, nil
}

func setRequestID(ctx context.Context, request *http.Request) {
	if id, ok := reqid.FromContext(ctx); ok {
		request.Header.Set(reqid.Header, id)
	}
}

// decodeError mirrors httpx: only a well-formed domain failure becomes *httpx.Error.
func decodeError(response *http.Response) error {
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || response.StatusCode < 400 {
		return fmt.Errorf("%w: status %d", errUnexpectedAnswer, response.StatusCode)
	}

	var envelope httpx.Envelope
	if err := json.NewDecoder(io.LimitReader(response.Body, maxErrorBytes)).
		Decode(&envelope); err != nil {
		return fmt.Errorf("status %d with undecodable body: %w", response.StatusCode, err)
	}

	if !envelope.Code.Known() {
		return fmt.Errorf("%w: status %d with code %q", errUnexpectedAnswer,
			response.StatusCode, envelope.Code)
	}

	return &httpx.Error{Code: envelope.Code, Message: envelope.Message}
}
