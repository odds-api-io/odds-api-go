package oddsapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors matched by *APIError through errors.Is.
var (
	ErrBadRequest   = errors.New("oddsapi: bad request")
	ErrUnauthorized = errors.New("oddsapi: invalid API key")
	ErrForbidden    = errors.New("oddsapi: forbidden")
	ErrNotFound     = errors.New("oddsapi: not found")
	ErrRateLimited  = errors.New("oddsapi: rate limit exceeded")
	ErrServer       = errors.New("oddsapi: server error")
)

// APIError is returned for every non-2xx response.
type APIError struct {
	// StatusCode is the HTTP status.
	StatusCode int
	// Message is the server's own message (the "error" or "message" field), or the raw body.
	Message string
	// Body is the raw response body.
	Body []byte
	// RetryAfter is how long the server asked to wait (Retry-After, or
	// X-RateLimit-Reset on a 429). Zero when not given.
	RetryAfter time.Duration
	// RateLimit holds the rate-limit headers of the response, if any.
	RateLimit *RateLimit
	// Method and URL identify the request. The URL never contains the API key.
	Method string
	URL    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("oddsapi: %s %s: %d %s", e.Method, e.URL, e.StatusCode, e.Message)
}

// Is matches the sentinel for the error's status code.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrBadRequest:
		return e.StatusCode == http.StatusBadRequest
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized
	case ErrForbidden:
		return e.StatusCode == http.StatusForbidden
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrRateLimited:
		return e.StatusCode == http.StatusTooManyRequests
	case ErrServer:
		return e.StatusCode >= 500
	}
	return false
}

// IsAddOnRequired reports whether a 403 is about a missing plan add-on
// (outrights, for example); the message then links to the billing page.
func (e *APIError) IsAddOnRequired() bool {
	if e.StatusCode != http.StatusForbidden {
		return false
	}
	m := strings.ToLower(e.Message)
	return strings.Contains(m, "add-on") || strings.Contains(m, "addon")
}

func newAPIError(resp *http.Response, body []byte, rl *RateLimit, method, safeURL string) *APIError {
	msg := strings.TrimSpace(string(body))
	var parsed struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		if s, ok := parsed.Error.(string); ok && s != "" {
			msg = s
		} else if parsed.Message != "" {
			msg = parsed.Message
		}
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	e := &APIError{
		StatusCode: resp.StatusCode,
		Message:    msg,
		Body:       body,
		RateLimit:  rl,
		Method:     method,
		URL:        safeURL,
	}
	e.RetryAfter = retryAfter(resp.Header, rl, resp.StatusCode == http.StatusTooManyRequests)
	return e
}

func retryAfter(h http.Header, rl *RateLimit, useReset bool) time.Duration {
	if raw := strings.TrimSpace(h.Get("Retry-After")); raw != "" {
		if secs, err := strconv.ParseFloat(raw, 64); err == nil {
			return time.Duration(max(0, secs) * float64(time.Second))
		}
		if t, err := http.ParseTime(raw); err == nil {
			return max(0, time.Until(t))
		}
	}
	if useReset && rl != nil && !rl.Reset.IsZero() {
		return max(0, time.Until(rl.Reset))
	}
	return 0
}

// RateLimit is the hourly request budget reported by the X-RateLimit-* headers.
type RateLimit struct {
	// Limit is X-RateLimit-Limit, or -1 when the header was missing.
	Limit int
	// Remaining is X-RateLimit-Remaining, or -1 when the header was missing.
	Remaining int
	// Reset is when the window resets (X-RateLimit-Reset), zero when unknown.
	Reset time.Time
}

func parseRateLimit(h http.Header) *RateLimit {
	limitRaw := h.Get("X-RateLimit-Limit")
	remainingRaw := h.Get("X-RateLimit-Remaining")
	resetRaw := strings.TrimSpace(h.Get("X-RateLimit-Reset"))
	if limitRaw == "" && remainingRaw == "" && resetRaw == "" {
		return nil
	}
	rl := &RateLimit{Limit: -1, Remaining: -1}
	if n, ok := headerInt(h, "X-RateLimit-Limit"); ok {
		rl.Limit = int(n)
	}
	if n, ok := headerInt(h, "X-RateLimit-Remaining"); ok {
		rl.Remaining = int(n)
	}
	if resetRaw != "" {
		if t, err := time.Parse(time.RFC3339, resetRaw); err == nil {
			rl.Reset = t
		} else if secs, err := strconv.ParseInt(resetRaw, 10, 64); err == nil {
			rl.Reset = time.Unix(secs, 0)
		}
	}
	return rl
}
