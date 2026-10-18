package bosk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prdoyle/terraform-provider-bosk/internal/bosk/pathutil"
)

const (
	defaultRequestTimeout = 10 * time.Second
	defaultUserAgent      = "terraform-provider-bosk/dev"
	maxResponseBody       = 64 << 20
	maxErrorBody          = 4 << 10
)

// GetOptions modifies a GET request.
type GetOptions struct {
	// Fresh asks bosk to flush before reading. The provider uses this for every
	// read, so Terraform observes the latest state.
	Fresh bool
}

// PutOptions modifies a PUT request.
type PutOptions struct {
	IfMatch         string
	IfNoneMatchStar bool
}

// DeleteOptions modifies a DELETE request.
type DeleteOptions struct {
	IfMatch string
}

// Response is a successful maintenance API response.
type Response struct {
	Body       []byte
	ETag       string
	StatusCode int
}

// Client talks to the bosk maintenance API. It has no Terraform dependencies.
type Client struct {
	endpointBase string
	httpClient   *http.Client
	config       Config
	retry        RetryPolicy
}

// NewClient validates cfg and constructs a client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = defaultRequestTimeout
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUserAgent
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	httpClient, err := cfg.httpClient()
	if err != nil {
		return nil, err
	}

	return &Client{
		endpointBase: strings.TrimRight(cfg.Endpoint.String(), "/"),
		httpClient:   httpClient,
		config:       cfg,
		retry:        cfg.Retry.normalized(),
	}, nil
}

// Get reads the node at path.
func (c *Client) Get(ctx context.Context, path string, opts GetOptions) (*Response, error) {
	if err := pathutil.Validate(path); err != nil {
		return nil, err
	}

	headers := http.Header{}
	if opts.Fresh {
		headers.Set("Cache-Control", "no-cache")
	}

	return c.do(ctx, http.MethodGet, path, nil, headers)
}

// Put replaces the node at path.
func (c *Client) Put(ctx context.Context, path string, value []byte, opts PutOptions) error {
	if err := pathutil.ValidateResourcePath(path); err != nil {
		return err
	}

	headers := http.Header{}
	if opts.IfMatch != "" {
		headers.Set("If-Match", opts.IfMatch)
	}
	if opts.IfNoneMatchStar {
		headers.Set("If-None-Match", "*")
	}

	_, err := c.do(ctx, http.MethodPut, path, value, headers)
	return err
}

// Delete deletes the node at path.
func (c *Client) Delete(ctx context.Context, path string, opts DeleteOptions) error {
	if err := pathutil.ValidateResourcePath(path); err != nil {
		return err
	}

	headers := http.Header{}
	if opts.IfMatch != "" {
		headers.Set("If-Match", opts.IfMatch)
	}

	_, err := c.do(ctx, http.MethodDelete, path, nil, headers)
	return err
}

type rawResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, headers http.Header) (*Response, error) {
	for attempt := 1; ; attempt++ {
		raw, err := c.doOnce(ctx, method, path, body, headers)
		if err != nil {
			if attempt < c.retry.MaxAttempts && retryableNetworkError(ctx, err) {
				if err := sleepContext(ctx, c.backoff(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}

		if raw.StatusCode >= 200 && raw.StatusCode < 300 {
			return &Response{
				Body:       raw.Body,
				ETag:       raw.Header.Get("ETag"),
				StatusCode: raw.StatusCode,
			}, nil
		}

		if raw.StatusCode == http.StatusPreconditionFailed {
			return nil, &PreconditionError{Method: method, Path: path}
		}

		apiErr := &APIError{
			Method:     method,
			URL:        c.endpointBase + path,
			StatusCode: raw.StatusCode,
			Status:     http.StatusText(raw.StatusCode),
			Body:       truncate(string(raw.Body), maxErrorBody),
		}

		if raw.StatusCode == http.StatusNotFound && method != http.MethodPut {
			return nil, &NotFoundError{Path: path}
		}

		if attempt < c.retry.MaxAttempts && retryableStatus(raw.StatusCode) {
			delay := c.retryDelay(attempt, raw.Header)
			if err := sleepContext(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}

		return nil, apiErr
	}
}

func (c *Client) doOnce(ctx context.Context, method, path string, body []byte, headers http.Header) (*rawResponse, error) {
	reqCtx := ctx
	if c.config.RequestTimeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.config.RequestTimeout)
		defer cancel()
	}

	rawURL := c.endpointBase + path
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building bosk request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.config.UserAgent)
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.config.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.Token)
	} else if c.config.Username != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}
	for name, value := range c.config.Headers {
		req.Header.Set(name, value)
	}
	for name, values := range headers {
		for _, value := range values {
			req.Header.Set(name, value)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling bosk: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBody+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading bosk response: %w", err)
	}
	if len(responseBody) > maxResponseBody {
		return nil, fmt.Errorf("bosk response exceeds %d bytes", maxResponseBody)
	}

	return &rawResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       responseBody,
	}, nil
}

func (c *Client) backoff(attempt int) time.Duration {
	delay := c.retry.BaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= c.retry.MaxDelay {
			return c.retry.MaxDelay
		}
	}
	return delay
}

func (c *Client) retryDelay(attempt int, headers http.Header) time.Duration {
	if value := headers.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
			return time.Duration(seconds) * time.Second
		}
		if when, err := http.ParseTime(value); err == nil {
			if delay := time.Until(when); delay > 0 {
				return delay
			}
			return 0
		}
	}
	return c.backoff(attempt)
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryableNetworkError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
