// Package oddsws streams real-time odds, scores and status from the
// Odds-API.io WebSocket feed (wss://api.odds-api.io/v3/ws).
//
// The server requires permessage-deflate compression; this package always
// negotiates it. A Stream reconnects on its own and resumes from the highest
// seq it has seen, so missed updates are replayed.
//
//	s := oddsws.New(oddsws.Config{APIKey: key, Markets: []string{"ML", "Totals"}})
//	err := s.Run(ctx, func(m *oddsws.Message) error {
//		switch m.Type {
//		case oddsws.TypeCreated, oddsws.TypeUpdated:
//			// replace what you hold for (m.ID, m.Bookie) with m.Markets
//		}
//		return nil
//	})
package oddsws

import (
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

	"github.com/gorilla/websocket"
	oddsapi "github.com/odds-api-io/odds-api-go"
)

// DefaultURL is the production WebSocket endpoint.
const DefaultURL = "wss://api.odds-api.io/v3/ws"

// Message types.
const (
	TypeWelcome        = "welcome"
	TypeCreated        = "created"
	TypeUpdated        = "updated"
	TypeDeleted        = "deleted"
	TypeNoMarkets      = "no_markets"
	TypeScore          = "score"
	TypeStatus         = "status"
	TypeClock          = "clock"
	TypeResyncRequired = "resync_required"
	TypeReplayComplete = "replay_complete"
)

// Config configures a Stream or a single connection.
type Config struct {
	// APIKey is required. It is sent as a query parameter and never logged.
	APIKey string
	// URL overrides DefaultURL.
	URL string
	// Markets is required for the odds channel, max 40.
	Markets []string
	// Channels is any of odds, scores, status. Empty means odds only.
	Channels []string
	// Sports filters by sport slug, max 10.
	Sports []string
	// Leagues filters by league slug, max 20. Not with EventIDs.
	Leagues []string
	// EventIDs filters by event, max 50. Not with Leagues.
	EventIDs []int64
	// Status is "live" or "prematch".
	Status string
	// LastSeq resumes from this sequence number (from a previous stream or GetOddsSnapshot).
	LastSeq int64
	// DisableReplayComplete stops the server sending the replay_complete marker.
	DisableReplayComplete bool

	// Dialer overrides the default dialer. Compression is enabled on it regardless.
	Dialer *websocket.Dialer
	// Header is sent with the handshake.
	Header http.Header

	// MaxReconnectAttempts limits consecutive failed reconnects (0 means no limit).
	MaxReconnectAttempts int
	// InitialReconnectDelay defaults to 1s and MaxReconnectDelay to 30s.
	InitialReconnectDelay time.Duration
	MaxReconnectDelay     time.Duration
	// ReadTimeout closes a connection that sends nothing, not even a ping,
	// for this long (default 90s).
	ReadTimeout time.Duration
	// OnReconnect is called before each reconnect wait.
	OnReconnect func(attempt int, delay time.Duration, lastSeq int64, cause error)
}

// Message is one frame from the feed. Fields not used by a type are zero; Raw
// always holds the full frame.
type Message struct {
	Type string `json:"type"`
	// Seq is the global sequence number; zero when the frame carries none.
	Seq       int64        `json:"seq,omitempty"`
	Timestamp int64        `json:"timestamp,omitempty"`
	ID        oddsapi.ID   `json:"id,omitempty"`
	Bookie    string       `json:"bookie,omitempty"`
	URL       string       `json:"url,omitempty"`
	Date      oddsapi.Time `json:"date,omitempty"`
	// Markets is the complete set for (ID, Bookie): replace, do not merge.
	Markets []oddsapi.Market `json:"markets,omitempty"`
	Scores  *oddsapi.Score   `json:"scores,omitempty"`
	Clock   *oddsapi.Clock   `json:"clock,omitempty"`
	// Status is the event status on status messages.
	Status string `json:"status,omitempty"`

	// Reason is set on resync_required.
	Reason string `json:"reason,omitempty"`
	// Outcome is set on replay_complete: replayed, snapshot, resync_required or live.
	Outcome    string `json:"outcome,omitempty"`
	LastSeq    int64  `json:"last_seq,omitempty"`
	CurrentSeq int64  `json:"current_seq,omitempty"`
	ThroughSeq int64  `json:"through_seq,omitempty"`
	// Message is the welcome text.
	Message string `json:"message,omitempty"`
	Warning string `json:"warning,omitempty"`

	Raw json.RawMessage `json:"-"`
}

// HandshakeError is returned when the server refuses the connection (bad
// parameters, invalid key, no WebSocket access). It is not retried.
type HandshakeError struct {
	StatusCode int
	Body       string
}

func (e *HandshakeError) Error() string {
	return fmt.Sprintf("oddsws: handshake refused: %d %s", e.StatusCode, strings.TrimSpace(e.Body))
}

// ReplacedError is returned when the server closed the connection because a
// newer connection on the same key replaced it. Run stops instead of fighting it.
type ReplacedError struct{ Reason string }

func (e *ReplacedError) Error() string { return "oddsws: connection replaced: " + e.Reason }

// Conn is a single connection without reconnects.
type Conn struct {
	ws          *websocket.Conn
	readTimeout time.Duration
}

// Dial opens one connection.
func Dial(ctx context.Context, cfg Config) (*Conn, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("oddsws: APIKey is required")
	}
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 30 * time.Second}
	if cfg.Dialer != nil {
		dialer = *cfg.Dialer
	}
	dialer.EnableCompression = true

	ws, resp, err := dialer.DialContext(ctx, buildURL(cfg), cfg.Header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return nil, &HandshakeError{StatusCode: resp.StatusCode, Body: redact(string(body), cfg.APIKey)}
		}
		return nil, fmt.Errorf("oddsws: dial: %s", redact(err.Error(), cfg.APIKey))
	}
	c := &Conn{ws: ws, readTimeout: cfg.ReadTimeout}
	if c.readTimeout <= 0 {
		c.readTimeout = 90 * time.Second
	}
	ws.SetPingHandler(func(data string) error {
		_ = ws.SetReadDeadline(time.Now().Add(c.readTimeout))
		err := ws.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(5*time.Second))
		if errors.Is(err, websocket.ErrCloseSent) {
			return nil
		}
		return err
	})
	return c, nil
}

// Read returns the next message. A close from the server is returned as a
// *websocket.CloseError (or *ReplacedError for code 1008 after a newer connection).
func (c *Conn) Read() (*Message, error) {
	_ = c.ws.SetReadDeadline(time.Now().Add(c.readTimeout))
	_, data, err := c.ws.ReadMessage()
	if err != nil {
		var ce *websocket.CloseError
		if errors.As(err, &ce) && ce.Code == websocket.ClosePolicyViolation && strings.Contains(ce.Text, "newer connection") {
			return nil, &ReplacedError{Reason: ce.Text}
		}
		return nil, err
	}
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("oddsws: decode message: %w", err)
	}
	m.Raw = data
	return &m, nil
}

// Close sends a normal close frame and closes the connection.
func (c *Conn) Close() error {
	_ = c.ws.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	return c.ws.Close()
}

// Stream is a reconnecting feed that tracks the highest seq it has seen.
type Stream struct {
	cfg     Config
	mu      sync.Mutex
	lastSeq int64
}

// New returns a Stream. Call Run to start it.
func New(cfg Config) *Stream {
	return &Stream{cfg: cfg, lastSeq: cfg.LastSeq}
}

// LastSeq is the resume point: persist it to continue after a restart.
func (s *Stream) LastSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSeq
}

// SetLastSeq sets the resume point, for example to the Seq of a fresh REST
// snapshot after a resync_required message.
func (s *Stream) SetLastSeq(seq int64) {
	s.mu.Lock()
	s.lastSeq = seq
	s.mu.Unlock()
}

func (s *Stream) observe(seq int64) {
	s.mu.Lock()
	if seq > s.lastSeq {
		s.lastSeq = seq
	}
	s.mu.Unlock()
}

// Run connects and calls fn for every message until ctx is done, fn returns
// an error, the handshake is refused, or the connection is replaced by a newer
// one. It reconnects with backoff and resumes from LastSeq. On resync_required
// the stored LastSeq is cleared before fn is called: rebuild from REST and
// call SetLastSeq with the snapshot's Seq. Run returns ctx.Err() when ctx ends.
func (s *Stream) Run(ctx context.Context, fn func(*Message) error) error {
	initial := s.cfg.InitialReconnectDelay
	if initial <= 0 {
		initial = time.Second
	}
	maxDelay := s.cfg.MaxReconnectDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}

	failures := 0
	for {
		cfg := s.cfg
		cfg.LastSeq = s.LastSeq()
		conn, err := Dial(ctx, cfg)
		if err == nil {
			failures = 0
			err = s.consume(ctx, conn, fn)
			conn.Close()
			if err == nil || errors.Is(err, errHandler) {
				return unwrapHandler(err)
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var he *HandshakeError
		var re *ReplacedError
		if errors.As(err, &he) || errors.As(err, &re) {
			return err
		}

		failures++
		if s.cfg.MaxReconnectAttempts > 0 && failures > s.cfg.MaxReconnectAttempts {
			return fmt.Errorf("oddsws: gave up after %d reconnect attempts: %w", failures-1, err)
		}
		delay := backoff(initial, maxDelay, failures)
		var ce *websocket.CloseError
		if errors.As(err, &ce) {
			switch ce.Code {
			case websocket.CloseServiceRestart:
				delay = time.Second
			case websocket.CloseTryAgainLater:
				delay = max(delay, 3*time.Second)
			}
		}
		if s.cfg.OnReconnect != nil {
			s.cfg.OnReconnect(failures, delay, s.LastSeq(), err)
		}
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

var errHandler = errors.New("oddsws: handler")

type handlerError struct{ err error }

func (e *handlerError) Error() string        { return e.err.Error() }
func (e *handlerError) Unwrap() error        { return e.err }
func (e *handlerError) Is(target error) bool { return target == errHandler }

func unwrapHandler(err error) error {
	var he *handlerError
	if errors.As(err, &he) {
		return he.err
	}
	return err
}

func (s *Stream) consume(ctx context.Context, conn *Conn, fn func(*Message) error) error {
	stop := context.AfterFunc(ctx, func() { conn.ws.Close() })
	defer stop()
	for {
		m, err := conn.Read()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if m.Type == TypeResyncRequired {
			s.SetLastSeq(0)
		} else if m.Seq > 0 {
			s.observe(m.Seq)
		}
		if err := fn(m); err != nil {
			return &handlerError{err}
		}
	}
}

func backoff(initial, maxDelay time.Duration, attempt int) time.Duration {
	d := initial << min(attempt-1, 16)
	if d > maxDelay || d <= 0 {
		d = maxDelay
	}
	return time.Duration(float64(d) * (0.75 + rand.Float64()*0.25))
}

func buildURL(cfg Config) string {
	base := cfg.URL
	if base == "" {
		base = DefaultURL
	}
	q := url.Values{}
	q.Set("apiKey", cfg.APIKey)
	setList(q, "markets", cfg.Markets)
	setList(q, "channels", cfg.Channels)
	setList(q, "sport", cfg.Sports)
	setList(q, "leagues", cfg.Leagues)
	if len(cfg.EventIDs) > 0 {
		ids := make([]string, len(cfg.EventIDs))
		for i, id := range cfg.EventIDs {
			ids[i] = strconv.FormatInt(id, 10)
		}
		q.Set("eventIds", strings.Join(ids, ","))
	}
	if cfg.Status != "" {
		q.Set("status", cfg.Status)
	}
	if cfg.LastSeq > 0 {
		q.Set("lastSeq", strconv.FormatInt(cfg.LastSeq, 10))
	}
	if !cfg.DisableReplayComplete {
		q.Set("replayComplete", "true")
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + q.Encode()
}

func setList(q url.Values, key string, v []string) {
	var parts []string
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) > 0 {
		q.Set(key, strings.Join(parts, ","))
	}
}

func redact(s, apiKey string) string {
	if apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, apiKey, "REDACTED")
}
