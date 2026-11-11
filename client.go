// Package oddsapi is the official Go client for the Odds-API.io REST API v3:
// real-time and pre-match sports betting odds from 450+ bookmakers across 34 sports.
//
//	client := oddsapi.NewClient(os.Getenv("ODDS_API_KEY"))
//	events, err := client.GetEvents(ctx, oddsapi.EventsParams{Sport: "football", League: "england-premier-league"})
//
// Every method takes a context.Context first. Errors returned for HTTP
// responses are *APIError values; use errors.Is with ErrNotFound,
// ErrRateLimited and the other sentinels, or errors.As to read the status and body.
//
// Documentation: https://docs.odds-api.io/guides/sdks
package oddsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version is the SDK version, sent in the User-Agent header.
const Version = "0.1.0"

// DefaultBaseURL is the production REST endpoint.
const DefaultBaseURL = "https://api.odds-api.io/v3"

const (
	defaultTimeout       = 10 * time.Second
	defaultMaxRetries    = 2
	defaultMaxRetryDelay = 10 * time.Second
)

// Client calls the Odds-API.io REST API. It is safe for concurrent use.
type Client struct {
	apiKey        string
	baseURL       string
	httpClient    *http.Client
	userAgent     string
	maxRetries    int
	maxRetryDelay time.Duration
	timeout       time.Duration
	customHTTP    bool

	mu            sync.Mutex
	lastRateLimit *RateLimit
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the http.Client used for requests. Its Timeout is left as is;
// WithTimeout is applied only to the default client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
			c.customHTTP = true
		}
	}
}

// WithBaseURL overrides the API base URL (default https://api.odds-api.io/v3).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithTimeout sets the per-request timeout of the default http.Client (default 10s).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithUserAgent appends a product token to the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = "odds-api-go/" + Version + " " + ua }
}

// WithMaxRetries sets how often a GET or POST is retried after a 5xx, a network
// error, or a 429 that asks for a wait no longer than the max retry delay (default 2).
// Bookmaker selection changes (PUT) are never retried. Use 0 to turn retries off.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n < 0 {
			n = 0
		}
		c.maxRetries = n
	}
}

// WithMaxRetryDelay sets the longest wait between retries (default 10s).
func WithMaxRetryDelay(d time.Duration) Option {
	return func(c *Client) { c.maxRetryDelay = d }
}

// NewClient returns a Client for the given API key.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:        apiKey,
		baseURL:       DefaultBaseURL,
		timeout:       defaultTimeout,
		userAgent:     "odds-api-go/" + Version,
		maxRetries:    defaultMaxRetries,
		maxRetryDelay: defaultMaxRetryDelay,
	}
	for _, opt := range opts {
		opt(c)
	}
	if !c.customHTTP {
		c.httpClient = &http.Client{Timeout: c.timeout}
	}
	return c
}

// LastRateLimit returns the rate-limit headers of the most recent response
// that carried them, or nil.
func (c *Client) LastRateLimit() *RateLimit {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastRateLimit == nil {
		return nil
	}
	rl := *c.lastRateLimit
	return &rl
}

type request struct {
	method string
	path   string
	query  url.Values
	body   any
	noAuth bool
}

type response struct {
	body   []byte
	header http.Header
}

func (c *Client) buildURL(path string, query url.Values, auth bool) string {
	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	if auth {
		q.Set("apiKey", c.apiKey)
	}
	u := c.baseURL + "/" + strings.TrimLeft(path, "/")
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	return u
}

func (c *Client) do(ctx context.Context, r request) (*response, error) {
	if r.method == "" {
		r.method = http.MethodGet
	}
	fullURL := c.buildURL(r.path, r.query, !r.noAuth)
	safeURL := c.buildURL(r.path, r.query, false)

	var payload []byte
	if r.body != nil {
		var err error
		if payload, err = json.Marshal(r.body); err != nil {
			return nil, fmt.Errorf("oddsapi: encode request body: %w", err)
		}
	}

	retries := c.maxRetries
	if r.method == http.MethodPut {
		retries = 0
	}

	for attempt := 0; ; attempt++ {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, r.method, fullURL, body)
		if err != nil {
			return nil, fmt.Errorf("oddsapi: build request %s %s: %w", r.method, safeURL, redact(err, c.apiKey))
		}
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			err = sanitizeTransportError(err, safeURL, c.apiKey)
			if ctx.Err() == nil && attempt < retries {
				if sleepErr := sleepCtx(ctx, c.backoff(attempt)); sleepErr != nil {
					return nil, sleepErr
				}
				continue
			}
			return nil, err
		}

		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		rl := parseRateLimit(resp.Header)
		if rl != nil {
			c.mu.Lock()
			c.lastRateLimit = rl
			c.mu.Unlock()
		}

		if readErr != nil {
			err := fmt.Errorf("oddsapi: read response %s %s: %w", r.method, safeURL, redact(readErr, c.apiKey))
			if ctx.Err() == nil && attempt < retries {
				if sleepErr := sleepCtx(ctx, c.backoff(attempt)); sleepErr != nil {
					return nil, sleepErr
				}
				continue
			}
			return nil, err
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return &response{body: data, header: resp.Header}, nil
		}

		apiErr := newAPIError(resp, data, rl, r.method, safeURL)
		if wait, ok := c.retryDelay(apiErr, attempt); ok && attempt < retries {
			if sleepErr := sleepCtx(ctx, wait); sleepErr != nil {
				return nil, sleepErr
			}
			continue
		}
		return nil, apiErr
	}
}

func (c *Client) backoff(attempt int) time.Duration {
	base := 500 * time.Millisecond << attempt
	if base > c.maxRetryDelay || base <= 0 {
		base = c.maxRetryDelay
	}
	return time.Duration(float64(base) * (0.75 + rand.Float64()*0.25))
}

func (c *Client) retryDelay(e *APIError, attempt int) (time.Duration, bool) {
	switch {
	case e.StatusCode == http.StatusTooManyRequests:
		if e.RetryAfter <= 0 || e.RetryAfter > c.maxRetryDelay {
			return 0, false
		}
		return e.RetryAfter, true
	case e.StatusCode == 500 || e.StatusCode == 502 || e.StatusCode == 503 || e.StatusCode == 504:
		if e.RetryAfter > 0 {
			return min(e.RetryAfter, c.maxRetryDelay), true
		}
		return c.backoff(attempt), true
	}
	return 0, false
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) (*response, error) {
	return c.sendJSON(ctx, request{path: path, query: query}, out)
}

func (c *Client) sendJSON(ctx context.Context, r request, out any) (*response, error) {
	resp, err := c.do(ctx, r)
	if err != nil {
		return nil, err
	}
	if out != nil {
		if err := json.Unmarshal(resp.body, out); err != nil {
			return nil, fmt.Errorf("oddsapi: decode %s response: %w", r.path, err)
		}
	}
	return resp, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// sanitizeTransportError keeps the API key out of *url.Error messages.
func sanitizeTransportError(err error, safeURL, apiKey string) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return &url.Error{Op: ue.Op, URL: safeURL, Err: redact(ue.Err, apiKey)}
	}
	return redact(err, apiKey)
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

func redact(err error, apiKey string) error {
	if err == nil || apiKey == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, apiKey) {
		return err
	}
	return &redactedError{msg: strings.ReplaceAll(msg, apiKey, "REDACTED"), err: err}
}

// query helpers

type params struct{ url.Values }

func newParams() params { return params{url.Values{}} }

func (p params) str(key, v string) params {
	if v != "" {
		p.Set(key, v)
	}
	return p
}

func (p params) list(key string, v []string) params {
	var parts []string
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) > 0 {
		p.Set(key, strings.Join(parts, ","))
	}
	return p
}

func (p params) ids(key string, v []int64) params {
	if len(v) > 0 {
		parts := make([]string, len(v))
		for i, id := range v {
			parts[i] = strconv.FormatInt(id, 10)
		}
		p.Set(key, strings.Join(parts, ","))
	}
	return p
}

func (p params) int(key string, v int) params {
	if v != 0 {
		p.Set(key, strconv.Itoa(v))
	}
	return p
}

func (p params) int64(key string, v int64) params {
	if v != 0 {
		p.Set(key, strconv.FormatInt(v, 10))
	}
	return p
}

func (p params) float(key string, v float64) params {
	if v != 0 {
		p.Set(key, strconv.FormatFloat(v, 'f', -1, 64))
	}
	return p
}

func (p params) flag(key string, v bool) params {
	if v {
		p.Set(key, "true")
	}
	return p
}

func (p params) optBool(key string, v *bool) params {
	if v != nil {
		p.Set(key, strconv.FormatBool(*v))
	}
	return p
}

func (p params) time(key string, t time.Time) params {
	if !t.IsZero() {
		p.Set(key, t.UTC().Format(time.RFC3339))
	}
	return p
}

func pathID(id int64) string { return url.PathEscape(strconv.FormatInt(id, 10)) }

func headerInt(h http.Header, name string) (int64, bool) {
	raw := strings.TrimSpace(h.Get(name))
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(raw, 64)
		if ferr != nil {
			return 0, false
		}
		n = int64(f)
	}
	return n, true
}

func errRequired(name string) error {
	return fmt.Errorf("oddsapi: %s is required", name)
}
