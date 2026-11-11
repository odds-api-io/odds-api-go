package oddsapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "secret-key-123"

func newTestClient(t *testing.T, h http.HandlerFunc, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	opts = append([]Option{WithBaseURL(srv.URL + "/v3"), WithMaxRetryDelay(50 * time.Millisecond)}, opts...)
	return NewClient(testKey, opts...)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestGetEventsBuildsQuery(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/events" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		want := map[string]string{
			"apiKey": testKey, "sport": "football", "league": "england-premier-league",
			"status": "pending,live", "from": "2026-09-01T00:00:00Z", "limit": "5", "participantId": "42",
		}
		for k, v := range want {
			if got := q.Get(k); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
		for _, k := range []string{"to", "skip", "bookmaker"} {
			if q.Has(k) {
				t.Errorf("unexpected param %s", k)
			}
		}
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "odds-api-go/") {
			t.Errorf("User-Agent = %q", ua)
		}
		w.Write([]byte(`[{"id":123,"home":"A","away":"B","homeId":"7","date":"2026-09-02T15:00:00Z","status":"pending",
			"sport":{"name":"Football","slug":"football"},"league":{"name":"EPL","slug":"england-premier-league"},
			"scores":{"home":1,"away":0,"periods":{"p1":{"home":1,"away":0}}}}]`))
	})
	events, err := c.GetEvents(context.Background(), EventsParams{
		Sport: "football", League: "england-premier-league", Status: []string{"pending", " live "},
		From: from, Limit: 5, ParticipantID: 42,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != 123 || events[0].HomeID != 7 || events[0].Scores.Periods["p1"].Home != 1 {
		t.Fatalf("decoded %+v", events)
	}
	if !events[0].Date.Equal(time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("date = %v", events[0].Date)
	}
}

func TestPublicEndpointsSendNoKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("apiKey") {
			t.Errorf("%s sent apiKey", r.URL.Path)
		}
		switch r.URL.Path {
		case "/v3/sports":
			w.Write([]byte(`[{"name":"Football","slug":"football"}]`))
		case "/v3/bookmakers":
			w.Write([]byte(`[{"name":"Bet365","active":true,"createdAt":"2025-10-11T11:38:05.549931Z","status":"ok","reason":null}]`))
		case "/v3/markets":
			if r.URL.Query().Get("sport") == "football" {
				w.Write([]byte(`{"sport":{"name":"Football","slug":"football"},"markets":[{"name":"ML","shape":"moneyline","period":"match","prematch":true,"live":true}]}`))
				return
			}
			w.Write([]byte(`{"football":[{"name":"ML"}]}`))
		}
	})
	ctx := context.Background()
	sports, err := c.GetSports(ctx)
	if err != nil || len(sports) != 1 || sports[0].Slug != "football" {
		t.Fatalf("sports %v %v", sports, err)
	}
	books, err := c.GetBookmakers(ctx)
	if err != nil || books[0].Name != "Bet365" || books[0].CreatedAt.IsZero() {
		t.Fatalf("bookmakers %v %v", books, err)
	}
	m, err := c.GetMarkets(ctx, "football")
	if err != nil || m.Markets[0].Shape != "moneyline" {
		t.Fatalf("markets %v %v", m, err)
	}
	all, err := c.GetAllMarkets(ctx)
	if err != nil || all["football"][0].Name != "ML" {
		t.Fatalf("all markets %v %v", all, err)
	}
}

func TestAPIErrorMapping(t *testing.T) {
	cases := []struct {
		status   int
		body     string
		sentinel error
		message  string
	}{
		{400, `{"error":"sport is required"}`, ErrBadRequest, "sport is required"},
		{401, `{"error":"You need to provide a valid apiKey"}`, ErrUnauthorized, "You need to provide a valid apiKey"},
		{403, `{"message":"Outrights are a paid add-on"}`, ErrForbidden, "Outrights are a paid add-on"},
		{404, `not here`, ErrNotFound, "not here"},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			})
			_, err := c.GetEvent(context.Background(), 1)
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tc.sentinel)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("not an APIError: %T", err)
			}
			if apiErr.StatusCode != tc.status || apiErr.Message != tc.message || string(apiErr.Body) != tc.body {
				t.Errorf("got %+v", apiErr)
			}
			if strings.Contains(err.Error(), testKey) || strings.Contains(apiErr.URL, testKey) {
				t.Errorf("API key leaked: %v", err)
			}
		})
	}
}

func TestAddOnRequired(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":"Outrights need the Outrights add-on: https://odds-api.io/dashboard/billing"}`))
	})
	_, err := c.GetOutrights(context.Background(), OutrightsParams{Sport: "golf"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !apiErr.IsAddOnRequired() {
		t.Fatalf("want add-on error, got %v", err)
	}
}

func TestRetriesServerErrorsThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`[]`))
	})
	if _, err := c.GetLiveEvents(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
}

func TestRateLimitRetryAndHeaders(t *testing.T) {
	var calls atomic.Int32
	reset := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Reset", reset.Format(time.RFC3339))
		if calls.Add(1) == 1 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", "0.01")
			w.WriteHeader(429)
			w.Write([]byte(`{"error":"Rate limit exceeded"}`))
			return
		}
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Write([]byte(`[]`))
	})
	if _, err := c.GetLiveEvents(context.Background(), "football"); err != nil {
		t.Fatal(err)
	}
	rl := c.LastRateLimit()
	if rl == nil || rl.Limit != 5000 || rl.Remaining != 4999 || !rl.Reset.Equal(reset) {
		t.Fatalf("rate limit = %+v", rl)
	}
}

func TestRateLimitLongWaitIsReturned(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	})
	_, err := c.GetSports(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, ErrRateLimited) || apiErr.RetryAfter != 120*time.Second {
		t.Fatalf("got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestPutIsNeverRetried(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPut || r.URL.Query().Get("bookmakers") != "Bet365,SingBet" {
			t.Errorf("%s %s", r.Method, r.URL)
		}
		w.WriteHeader(500)
	})
	_, err := c.SelectBookmakers(context.Background(), "Bet365", "SingBet")
	if !errors.Is(err, ErrServer) || calls.Load() != 1 {
		t.Fatalf("err %v, calls %d", err, calls.Load())
	}
}

func TestTransportErrorDoesNotLeakKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := NewClient(testKey, WithBaseURL(url), WithMaxRetries(0))
	_, err := c.GetLiveEvents(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Fatalf("API key leaked: %v", err)
	}
}

func TestContextCancel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.GetLiveEvents(ctx, ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestValidationSendsNothing(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	})
	ctx := context.Background()
	checks := []error{
		func() error { _, err := c.GetEvents(ctx, EventsParams{}); return err }(),
		func() error { _, err := c.GetOdds(ctx, OddsParams{EventID: 1}); return err }(),
		func() error { _, err := c.GetValueBets(ctx, ValueBetsParams{}); return err }(),
		func() error {
			_, err := c.GetHistoricalEvents(ctx, HistoricalEventsParams{Sport: "a", League: "b"})
			return err
		}(),
		func() error { _, err := c.GetSettlements(ctx); return err }(),
	}
	for i, err := range checks {
		if err == nil {
			t.Errorf("check %d: expected validation error", i)
		}
	}
}

func TestGetOddsDecodesLines(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("eventId") != "62924717" || q.Get("bookmakers") != "Bet365,SingBet" || q.Get("markets") != "ML,Double Chance" {
			t.Errorf("query %v", q)
		}
		w.Write([]byte(`{"id":62924717,"home":"A","away":"B","date":"2026-09-02T15:00:00Z","status":"pending",
			"sport":{"name":"Football","slug":"football"},"league":{"name":"EPL","slug":"epl"},
			"urls":{"Bet365":"https://example.com"},
			"bookmakers":{"Bet365":[
				{"name":"ML","updatedAt":"2026-09-01T10:00:00Z","odds":[{"home":"2.10","draw":"3.40","away":3.2,"max":500}]},
				{"name":"Double Chance","updatedAt":"","odds":[{"1X":"1.30","12":"1.25","X2":"1.60"}]}]}}`))
	})
	odds, err := c.GetOdds(context.Background(), OddsParams{EventID: 62924717, Bookmakers: []string{"Bet365", "SingBet"}, Markets: []string{"ML", "Double Chance"}})
	if err != nil {
		t.Fatal(err)
	}
	ml := odds.Market("Bet365", "ML")
	if ml == nil || ml.Odds[0].Home != "2.10" || ml.Odds[0].Away != "3.2" || ml.Odds[0].Away.Float() != 3.2 || *ml.Odds[0].Max != 500 {
		t.Fatalf("ML = %+v", ml)
	}
	dc := odds.Market("Bet365", "Double Chance")
	if dc == nil || dc.Odds[0].Price("1X") != "1.30" || !dc.UpdatedAt.IsZero() {
		t.Fatalf("Double Chance = %+v", dc)
	}
	if odds.Market("SingBet", "ML") != nil {
		t.Fatal("expected nil market")
	}
}

func TestHeaderBackedResponses(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/odds/multi":
			if r.URL.Query().Get("includeSeq") != "true" || r.URL.Query().Get("eventIds") != "1,2" {
				t.Errorf("query %v", r.URL.Query())
			}
			w.Header().Set("X-OddsAPI-Seq", "482917")
			w.Write([]byte(`[]`))
		case "/v3/odds/updated":
			w.Header().Set("X-Next-Since", "1700000100")
			w.Header().Set("X-Server-Time", "1700000110")
			w.Header().Set("X-Unknown-Markets", "Foo, Bar")
			w.Write([]byte(`[]`))
		case "/v3/dropping-odds":
			w.Header().Set("X-Total-Count", "321")
			w.Header().Set("X-Page", "2")
			w.Header().Set("X-Per-Page", "50")
			w.Header().Set("X-Updated-At", "1700000000000")
			w.Write([]byte(`[{"eventId":1,"market":{"name":"ML"},"betSide":"home","odds":{"opening":2.0,"current":1.8,"drop":{"sinceOpening":10,"12h":null,"24h":5,"48h":null}},"lastMovedAt":null,"updatedAt":1}]`))
		}
	})
	ctx := context.Background()
	snap, err := c.GetOddsSnapshot(ctx, MultiOddsParams{EventIDs: []int64{1, 2}, Bookmakers: []string{"Bet365"}})
	if err != nil || snap.Seq != 482917 {
		t.Fatalf("snapshot %+v %v", snap, err)
	}
	upd, err := c.GetUpdatedOdds(ctx, UpdatedOddsParams{Since: 1700000000, Bookmaker: "Bet365", Sport: "football", Markets: []string{"ML"}})
	if err != nil || upd.NextSince != 1700000100 || upd.ServerTime != 1700000110 || strings.Join(upd.UnknownMarkets, "|") != "Foo|Bar" {
		t.Fatalf("updated %+v %v", upd, err)
	}
	page, err := c.GetDroppingOdds(ctx, DroppingOddsParams{Sport: "football", Page: 2})
	if err != nil || page.Total != 321 || page.Page != 2 || page.PerPage != 50 || page.UpdatedAt != 1700000000000 {
		t.Fatalf("dropping %+v %v", page, err)
	}
	if d := page.Data[0].Odds.Drop; d.SinceOpening != 10 || d.H12 != nil || *d.H24 != 5 {
		t.Fatalf("drop %+v", d)
	}
}

func TestPollUpdatedOddsFollowsNextSince(t *testing.T) {
	var sinces []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sinces = append(sinces, r.URL.Query().Get("since"))
		w.Header().Set("X-Next-Since", strconv.Itoa(1000+len(sinces)))
		w.Write([]byte(`[]`))
	})
	n := 0
	err := c.PollUpdatedOdds(context.Background(), UpdatedOddsParams{Since: 999, Bookmaker: "Bet365", Sport: "football"}, time.Millisecond, func(*UpdatedOdds) error {
		n++
		if n == 3 {
			return StopIteration
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(sinces, ",") != "999,1001,1002" {
		t.Fatalf("sinces = %v", sinces)
	}
}

func TestIterateEventsPages(t *testing.T) {
	var skips []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		skip := r.URL.Query().Get("skip")
		skips = append(skips, skip)
		if r.URL.Query().Get("limit") != "2" {
			t.Errorf("limit = %s", r.URL.Query().Get("limit"))
		}
		switch skip {
		case "":
			w.Write([]byte(`[{"id":1},{"id":2}]`))
		case "2":
			w.Write([]byte(`[{"id":3},{"id":4}]`))
		default:
			w.Write([]byte(`[{"id":5}]`))
		}
	})
	var ids []int64
	err := c.IterateEvents(context.Background(), EventsParams{Sport: "football"}, 2, func(e Event) error {
		ids = append(ids, e.ID.Int64())
		return nil
	})
	if err != nil || len(ids) != 5 || strings.Join(skips, ",") != ",2,4" {
		t.Fatalf("ids %v skips %v err %v", ids, skips, err)
	}

	ids = nil
	err = c.IterateEvents(context.Background(), EventsParams{Sport: "football"}, 2, func(e Event) error {
		ids = append(ids, e.ID.Int64())
		if len(ids) == 3 {
			return StopIteration
		}
		return nil
	})
	if err != nil || len(ids) != 3 {
		t.Fatalf("stop: ids %v err %v", ids, err)
	}
}

func TestIterateHistoricalEventsWindows(t *testing.T) {
	var windows []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		windows = append(windows, q.Get("from")+"/"+q.Get("to"))
		w.Write([]byte(`[{"id":1},{"id":2}]`))
	})
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(70 * 24 * time.Hour)
	var n int
	err := c.IterateHistoricalEvents(context.Background(), HistoricalEventsParams{Sport: "football", League: "epl", From: from, To: to}, func(Event) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"2026-01-01T00:00:00Z/2026-02-01T00:00:00Z",
		"2026-02-01T00:00:00Z/2026-03-04T00:00:00Z",
		"2026-03-04T00:00:00Z/2026-03-12T00:00:00Z",
	}
	if strings.Join(windows, " ") != strings.Join(want, " ") {
		t.Fatalf("windows %v", windows)
	}
	if n != 2 {
		t.Fatalf("deduplicated events = %d, want 2", n)
	}
}

func TestSettlementsBatching(t *testing.T) {
	var batches []int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ids := strings.Split(r.URL.Query().Get("eventIds"), ",")
		batches = append(batches, len(ids))
		writeJSON(w, map[string]any{
			"settlements": []map[string]any{{"eventId": ids[0], "sport": "football", "status": "settled", "final": true}},
			"notFound":    []string{},
		})
	})
	ids := make([]int64, 120)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	res, err := c.GetSettlements(context.Background(), ids...)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 3 || batches[0] != 50 || batches[2] != 20 || len(res.Settlements) != 3 || res.Settlements[1].EventID != 51 {
		t.Fatalf("batches %v res %+v", batches, res)
	}
}

func TestGradeBetsPostsJSON(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/settlements/grade" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		if r.URL.Query().Get("apiKey") != testKey {
			t.Error("missing apiKey")
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Selections []map[string]any `json:"selections"`
		}
		if err := json.Unmarshal(body, &req); err != nil || len(req.Selections) != 2 {
			t.Fatalf("body %s", body)
		}
		if req.Selections[1]["hdp"] != -0.75 || req.Selections[0]["hdp"] != nil {
			t.Errorf("selections %v", req.Selections)
		}
		w.Write([]byte(`{"grades":[{"ref":"a","eventId":1,"result":"won","reason":null,"final":true},
			{"ref":"b","eventId":1,"result":"half_won","reason":null,"final":true,"basis":{"scope":"ft","home":2,"away":1}}]}`))
	})
	hdp := -0.75
	res, err := c.GradeBets(context.Background(), []GradeSelection{
		{Ref: "a", EventID: 1, Market: "ML", Side: "home"},
		{Ref: "b", EventID: 1, Market: "Spread", Side: "home", Hdp: &hdp},
	})
	if err != nil || len(res.Grades) != 2 || res.Grades[1].Result != GradeHalfWon || res.Grades[1].Basis.Home != 2 {
		t.Fatalf("grades %+v %v", res, err)
	}
}

func TestIterateMappingsFollowsCursor(t *testing.T) {
	var cursors []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		cursors = append(cursors, q.Get("cursor"))
		if q.Get("active") != "true" {
			t.Errorf("active = %q", q.Get("active"))
		}
		if q.Get("cursor") == "" {
			w.Write([]byte(`{"data":[{"eventId":1,"marketId":"m1","outcomeId":"o1","side":"home","extra":{"tick":0.01}}],"nextCursor":"abc"}`))
			return
		}
		w.Write([]byte(`{"data":[{"eventId":2,"marketId":"m2","outcomeId":"o2","side":"away"}],"nextCursor":null}`))
	})
	active := true
	var rows []Mapping
	err := c.IterateMappings(context.Background(), MappingsParams{Bookmaker: "Kalshi", Sport: "football", Active: &active}, func(m Mapping) error {
		rows = append(rows, m)
		return nil
	})
	if err != nil || len(rows) != 2 || strings.Join(cursors, ",") != ",abc" || string(rows[0].Extra["tick"]) != "0.01" {
		t.Fatalf("rows %+v cursors %v err %v", rows, cursors, err)
	}
}

func TestOutrights(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/outrights/odds":
			if r.URL.Query().Get("id") != "1001" || r.URL.Query().Get("bookmakers") != "DraftKings" {
				t.Errorf("query %v", r.URL.Query())
			}
			w.Write([]byte(`{"id":1001,"name":"Open","bookmakers":{"DraftKings":[
				{"name":"Winner","odds":[{"label":"A","participantId":5,"odds":"8.00"}]},
				{"name":"Make Cut","odds":[{"label":"A","participantId":5,"yes":"1.20","no":"4.00"}]}]}}`))
		case "/v3/outrights/results":
			w.Write([]byte(`{"id":1001,"name":"Open","status":"settled","results":[{"label":"A","participantId":5,"position":1,"tied":false,"madeCut":true,"status":"finished"}],"winners":{"Winner":[5]}}`))
		}
	})
	ctx := context.Background()
	odds, err := c.GetOutrightOdds(ctx, 1001, "DraftKings")
	if err != nil {
		t.Fatal(err)
	}
	mk := odds.Bookmakers["DraftKings"]
	if mk[0].Odds[0].IsYesNo() || !mk[1].Odds[0].IsYesNo() || mk[0].Odds[0].Odds != "8.00" {
		t.Fatalf("odds %+v", mk)
	}
	res, err := c.GetOutrightResults(ctx, 1001)
	if err != nil || *res.Results[0].Position != 1 || res.Winners["Winner"][0] != 5 {
		t.Fatalf("results %+v %v", res, err)
	}
}

func TestParticipantLogo(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/participants/77/logo" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("\x89PNG"))
	})
	png, err := c.GetParticipantLogo(context.Background(), 77)
	if err != nil || string(png) != "\x89PNG" {
		t.Fatalf("%q %v", png, err)
	}
	if u := c.ParticipantLogoURL(77); !strings.Contains(u, "/participants/77/logo?apiKey="+testKey) {
		t.Fatalf("url %s", u)
	}
}

func TestCustomHTTPClientAndTimeoutOrder(t *testing.T) {
	hc := &http.Client{Timeout: 3 * time.Second}
	c := NewClient("k", WithHTTPClient(hc), WithTimeout(time.Second))
	if c.httpClient != hc || hc.Timeout != 3*time.Second {
		t.Fatal("WithTimeout must not change a custom client")
	}
	c = NewClient("k", WithTimeout(time.Second))
	if c.httpClient.Timeout != time.Second {
		t.Fatalf("timeout = %v", c.httpClient.Timeout)
	}
}

func TestTypeDecoding(t *testing.T) {
	var v struct {
		A ID      `json:"a"`
		B ID      `json:"b"`
		C Decimal `json:"c"`
		D Decimal `json:"d"`
		E Time    `json:"e"`
		F Time    `json:"f"`
	}
	err := json.Unmarshal([]byte(`{"a":"63017989","b":12,"c":1.85,"d":"2.10","e":null,"f":"2026-01-15T21:00:00Z"}`), &v)
	if err != nil {
		t.Fatal(err)
	}
	if v.A != 63017989 || v.B != 12 || v.C != "1.85" || v.D.Float() != 2.1 || !v.E.IsZero() || v.F.Year() != 2026 {
		t.Fatalf("%+v", v)
	}
}
