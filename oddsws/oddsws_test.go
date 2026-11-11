package oddsws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const testKey = "ws-secret-key"

type fakeServer struct {
	t        *testing.T
	mu       sync.Mutex
	queries  []string
	sessions [][]string
	closes   []int
}

func (f *fakeServer) handler(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(strings.ToLower(r.Header.Get("Sec-WebSocket-Extensions")), "permessage-deflate") {
		http.Error(w, "permessage-deflate required", http.StatusUpgradeRequired)
		return
	}
	if r.URL.Query().Get("apiKey") != testKey {
		http.Error(w, `{"error":"invalid apiKey"}`, http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	n := len(f.queries)
	f.queries = append(f.queries, r.URL.RawQuery)
	var msgs []string
	if n < len(f.sessions) {
		msgs = f.sessions[n]
	}
	code := websocket.CloseServiceRestart
	if n < len(f.closes) {
		code = f.closes[n]
	}
	f.mu.Unlock()

	up := websocket.Upgrader{EnableCompression: true}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		f.t.Errorf("upgrade: %v", err)
		return
	}
	defer conn.Close()
	for _, m := range msgs {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(m)); err != nil {
			return
		}
	}
	if code == 0 {
		conn.ReadMessage()
		return
	}
	reason := "server restart"
	if code == websocket.ClosePolicyViolation {
		reason = "connection limit reached, newer connection opened"
	}
	conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
	time.Sleep(20 * time.Millisecond)
}

func startServer(t *testing.T, f *fakeServer) string {
	f.t = t
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/v3/ws"
}

func TestStreamResumesFromLastSeq(t *testing.T) {
	f := &fakeServer{
		sessions: [][]string{
			{
				`{"type":"welcome","message":"Connected"}`,
				`{"type":"created","seq":10,"id":"63017989","bookie":"SingBet","date":"2026-01-15T21:00:00Z","markets":[{"name":"ML","updatedAt":"2026-01-15T10:30:00Z","odds":[{"home":"1.85","draw":"3.25","away":"2.10","max":500}]}]}`,
				`{"type":"updated","seq":12,"id":"63017989","bookie":"SingBet","markets":[]}`,
			},
			{
				`{"type":"replay_complete","outcome":"replayed","last_seq":12,"through_seq":15}`,
				`{"type":"score","id":"63017989","scores":{"home":1,"away":0},"timestamp":1}`,
			},
		},
		closes: []int{websocket.CloseServiceRestart, websocket.ClosePolicyViolation},
	}
	url := startServer(t, f)

	var got []*Message
	var reconnects int
	s := New(Config{
		APIKey: testKey, URL: url, Markets: []string{"ML", "Totals"}, Channels: []string{"odds", "scores"},
		EventIDs: []int64{63017989}, InitialReconnectDelay: time.Millisecond,
		OnReconnect: func(attempt int, delay time.Duration, lastSeq int64, cause error) {
			reconnects++
			if strings.Contains(cause.Error(), testKey) {
				t.Errorf("API key leaked: %v", cause)
			}
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.Run(ctx, func(m *Message) error {
		got = append(got, m)
		return nil
	})
	var re *ReplacedError
	if !errors.As(err, &re) {
		t.Fatalf("Run returned %v, want ReplacedError", err)
	}
	if len(got) != 5 || reconnects != 1 {
		t.Fatalf("messages %d reconnects %d", len(got), reconnects)
	}
	created := got[1]
	if created.ID != 63017989 || created.Bookie != "SingBet" || created.Markets[0].Odds[0].Home != "1.85" || created.Date.IsZero() {
		t.Fatalf("created %+v", created)
	}
	if got[3].Type != TypeReplayComplete || got[3].ThroughSeq != 15 || got[4].Scores.Home != 1 {
		t.Fatalf("second session %+v %+v", got[3], got[4])
	}
	if s.LastSeq() != 12 {
		t.Fatalf("LastSeq = %d", s.LastSeq())
	}
	if !strings.Contains(f.queries[0], "replayComplete=true") || strings.Contains(f.queries[0], "lastSeq") {
		t.Errorf("first query %s", f.queries[0])
	}
	if !strings.Contains(f.queries[1], "lastSeq=12") || !strings.Contains(f.queries[1], "markets=ML%2CTotals") ||
		!strings.Contains(f.queries[1], "eventIds=63017989") || !strings.Contains(f.queries[1], "channels=odds%2Cscores") {
		t.Errorf("second query %s", f.queries[1])
	}
}

func TestResyncClearsLastSeq(t *testing.T) {
	f := &fakeServer{
		sessions: [][]string{{
			`{"type":"updated","seq":50,"id":"1","bookie":"Bet365","markets":[]}`,
			`{"type":"resync_required","reason":"replay_window_expired","last_seq":50,"current_seq":900}`,
		}},
		closes: []int{0},
	}
	url := startServer(t, f)
	s := New(Config{APIKey: testKey, URL: url, Markets: []string{"ML"}, LastSeq: 40})
	stop := errors.New("stop")
	err := s.Run(context.Background(), func(m *Message) error {
		if m.Type == TypeResyncRequired {
			if s.LastSeq() != 0 || m.Reason != "replay_window_expired" || m.CurrentSeq != 900 {
				t.Errorf("resync %+v lastSeq %d", m, s.LastSeq())
			}
			s.SetLastSeq(777)
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Fatalf("Run returned %v", err)
	}
	if s.LastSeq() != 777 || !strings.Contains(f.queries[0], "lastSeq=40") {
		t.Fatalf("LastSeq %d query %s", s.LastSeq(), f.queries[0])
	}
}

func TestHandshakeRefusedIsNotRetried(t *testing.T) {
	f := &fakeServer{}
	url := startServer(t, f)
	s := New(Config{APIKey: "wrong-key", URL: url, Markets: []string{"ML"}, InitialReconnectDelay: time.Millisecond})
	err := s.Run(context.Background(), func(*Message) error { return nil })
	var he *HandshakeError
	if !errors.As(err, &he) || he.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "wrong-key") {
		t.Fatalf("API key leaked: %v", err)
	}
	if len(f.queries) != 0 {
		t.Fatalf("handshake should fail before upgrade, got %d sessions", len(f.queries))
	}
}

func TestContextCancelStopsRun(t *testing.T) {
	f := &fakeServer{sessions: [][]string{{`{"type":"welcome"}`}}, closes: []int{0}}
	url := startServer(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	s := New(Config{APIKey: testKey, URL: url, Markets: []string{"ML"}})
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, func(m *Message) error {
			if m.Type == TypeWelcome {
				cancel()
			}
			return nil
		})
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop on cancel")
	}
}

func TestBuildURL(t *testing.T) {
	u := buildURL(Config{APIKey: "k", Sports: []string{"football", "basketball"}, Status: "live", DisableReplayComplete: true})
	if !strings.HasPrefix(u, DefaultURL+"?") || !strings.Contains(u, "sport=football%2Cbasketball") ||
		!strings.Contains(u, "status=live") || strings.Contains(u, "replayComplete") {
		t.Fatalf("url %s", u)
	}
}
