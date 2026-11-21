// Package convoy is a small, dependency-free HTTP client for the subset of the
// Convoy REST API (https://www.getconvoy.io/docs/api-reference) that the
// Terraform provider manages.
package convoy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the Convoy Cloud US region API.
const DefaultBaseURL = "https://us.getconvoy.cloud/api"

// Client talks to the Convoy API.
type Client struct {
	baseURL    string
	apiKey     string
	userAgent  string
	httpClient *http.Client
	maxRetries int
	// backoff returns how long to wait before retry attempt n (1-based).
	backoff func(attempt int) time.Duration
}

// Option customises a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithMaxRetries sets how many times a retryable request is retried.
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = n } }

// WithBackoff overrides the retry backoff (mainly for tests).
func WithBackoff(f func(attempt int) time.Duration) Option {
	return func(c *Client) { c.backoff = f }
}

// NewClient builds a client. baseURL must point at the API root, e.g.
// "https://us.getconvoy.cloud/api" or "http://localhost:5005/api".
func NewClient(baseURL, apiKey string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid Convoy base URL %q", baseURL)
	}
	if apiKey == "" {
		return nil, errors.New("a Convoy API key is required")
	}
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		userAgent:  "terraform-provider-convoy",
		httpClient: &http.Client{Timeout: 60 * time.Second},
		maxRetries: 3,
		backoff: func(attempt int) time.Duration {
			return time.Duration(math.Pow(2, float64(attempt-1))*500) * time.Millisecond
		},
	}
	for _, o := range opts {
		o(c)
	}
	// Never follow redirects: Go would replay a POST as a GET, and the page at
	// the end (a login page, the Convoy dashboard) would pass for a response.
	h := *c.httpClient
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.httpClient = &h
	return c, nil
}

// APIError is returned for any non-2xx response.
type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Message    string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("convoy: %s %s returned %d: %s", e.Method, e.Path, e.StatusCode, msg)
}

// IsNotFound reports whether err means the requested object does not exist.
// Convoy answers 404 for most missing objects but some handlers answer 400
// with a "... not found" message, so both are treated as not found.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode == http.StatusNotFound {
		return true
	}
	return apiErr.StatusCode == http.StatusBadRequest &&
		strings.Contains(strings.ToLower(apiErr.Message), "not found")
}

// envelope is Convoy's standard response wrapper.
type envelope struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("convoy: encoding request body: %w", err)
		}
	}
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, full, bytesReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < c.maxRetries && ctx.Err() == nil && idempotent(method) {
				if werr := sleep(ctx, c.backoff(attempt+1)); werr != nil {
					return werr
				}
				continue
			}
			return fmt.Errorf("convoy: %s %s: %w", method, path, err)
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("convoy: reading response: %w", readErr)
		}

		if attempt < c.maxRetries && retryable(method, resp.StatusCode) {
			wait := c.backoff(attempt + 1)
			if ra := retryAfter(resp.Header.Get("Retry-After")); ra > 0 {
				wait = ra
			}
			if werr := sleep(ctx, wait); werr != nil {
				return werr
			}
			continue
		}

		if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
			return &APIError{StatusCode: resp.StatusCode, Method: method, Path: path, Message: fmt.Sprintf(
				"redirected to %q. base_url must be the Convoy API root (ending in /api), reachable without "+
					"an authentication proxy such as Cloudflare Access in front of it", resp.Header.Get("Location"))}
		}

		var env envelope
		envErr := json.Unmarshal(raw, &env)
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			msg := env.Message
			if msg == "" {
				msg = snippet(raw)
			}
			return &APIError{StatusCode: resp.StatusCode, Method: method, Path: path, Message: msg}
		}
		if envErr != nil && len(bytes.TrimSpace(raw)) > 0 {
			return fmt.Errorf("convoy: %s %s returned %d with a %q body that isn't a Convoy API response (%s). "+
				"base_url must be the Convoy API root, ending in /api", method, path, resp.StatusCode,
				resp.Header.Get("Content-Type"), snippet(raw))
		}
		if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
			return nil
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("convoy: decoding %s %s response: %w", method, path, err)
		}
		return nil
	}
}

// snippet shortens a response body for error messages.
func snippet(raw []byte) string {
	const max = 200
	s := strings.Join(strings.Fields(string(raw)), " ")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

func bytesReader(b []byte) io.Reader {
	if b == nil {
		return nil
	}
	return bytes.NewReader(b)
}

func idempotent(method string) bool {
	return method == http.MethodGet || method == http.MethodPut || method == http.MethodDelete
}

// retryable: 429 is always safe to retry (the request was rejected before
// processing); 5xx gateway errors are only retried for idempotent methods.
func retryable(method string, status int) bool {
	switch status {
	case http.StatusTooManyRequests:
		return true
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return idempotent(method)
	}
	return false
}

func retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func projectPath(projectID string, parts ...string) string {
	p := "/v1/projects/" + url.PathEscape(projectID)
	for _, s := range parts {
		p += "/" + url.PathEscape(s)
	}
	return p
}
