package oddsapi

import (
	"context"
	"time"
)

// OddsParams is GET /odds.
type OddsParams struct {
	EventID int64
	// Bookmakers by name, required, max 30.
	Bookmakers []string
	// Markets by exact name, max 40, as listed at https://odds-api.io/markets.
	// Strongly recommended: a full response can be very large.
	Markets []string
}

// GetOdds returns the odds of one event.
func (c *Client) GetOdds(ctx context.Context, p OddsParams) (*EventOdds, error) {
	if p.EventID == 0 {
		return nil, errRequired("EventID")
	}
	if len(p.Bookmakers) == 0 {
		return nil, errRequired("Bookmakers")
	}
	q := newParams().int64("eventId", p.EventID).list("bookmakers", p.Bookmakers).list("markets", p.Markets)
	var out EventOdds
	if _, err := c.getJSON(ctx, "odds", q.Values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MultiOddsParams is GET /odds/multi.
type MultiOddsParams struct {
	// EventIDs, up to 10.
	EventIDs   []int64
	Bookmakers []string
	Markets    []string
}

func (p MultiOddsParams) query() (params, error) {
	if len(p.EventIDs) == 0 {
		return params{}, errRequired("EventIDs")
	}
	if len(p.Bookmakers) == 0 {
		return params{}, errRequired("Bookmakers")
	}
	return newParams().ids("eventIds", p.EventIDs).list("bookmakers", p.Bookmakers).list("markets", p.Markets), nil
}

// GetOddsMulti returns the odds of up to 10 events in one request.
func (c *Client) GetOddsMulti(ctx context.Context, p MultiOddsParams) ([]EventOdds, error) {
	q, err := p.query()
	if err != nil {
		return nil, err
	}
	var out []EventOdds
	_, err = c.getJSON(ctx, "odds/multi", q.Values, &out)
	return out, err
}

// OddsSnapshot is a /odds/multi response with the WebSocket sequence at that moment.
type OddsSnapshot struct {
	Data []EventOdds
	// Seq is X-OddsAPI-Seq: pass it as LastSeq when you connect the WebSocket.
	// Zero when the server did not send it.
	Seq int64
}

// GetOddsSnapshot is GetOddsMulti plus the WebSocket sequence number, so a
// stream can start exactly where the snapshot ends.
func (c *Client) GetOddsSnapshot(ctx context.Context, p MultiOddsParams) (*OddsSnapshot, error) {
	q, err := p.query()
	if err != nil {
		return nil, err
	}
	q.Set("includeSeq", "true")
	var data []EventOdds
	resp, err := c.getJSON(ctx, "odds/multi", q.Values, &data)
	if err != nil {
		return nil, err
	}
	seq, _ := headerInt(resp.header, "X-OddsAPI-Seq")
	return &OddsSnapshot{Data: data, Seq: seq}, nil
}

// UpdatedOddsParams is GET /odds/updated.
type UpdatedOddsParams struct {
	// Since is unix seconds, no more than 90 seconds old (or the previous NextSince).
	Since     int64
	Bookmaker string
	Sport     string
	// Markets by exact name, max 40.
	Markets []string
	// Limit is 1 to 10000.
	Limit int
}

// UpdatedOdds is a /odds/updated response with its headers.
type UpdatedOdds struct {
	// Data holds only the market groups that changed: merge by event, bookmaker and market.
	Data []EventOdds
	// NextSince (X-Next-Since) is the Since of the next call.
	NextSince int64
	// ServerTime is X-Server-Time, unix seconds.
	ServerTime int64
	// UnknownMarkets lists requested market names that do not exist (X-Unknown-Markets).
	UnknownMarkets []string
}

// GetUpdatedOdds returns the market groups changed since p.Since for one bookmaker and sport.
func (c *Client) GetUpdatedOdds(ctx context.Context, p UpdatedOddsParams) (*UpdatedOdds, error) {
	if p.Since == 0 {
		return nil, errRequired("Since")
	}
	if p.Bookmaker == "" {
		return nil, errRequired("Bookmaker")
	}
	if p.Sport == "" {
		return nil, errRequired("Sport")
	}
	q := newParams().int64("since", p.Since).str("bookmaker", p.Bookmaker).str("sport", p.Sport).
		list("markets", p.Markets).int("limit", p.Limit)
	var data []EventOdds
	resp, err := c.getJSON(ctx, "odds/updated", q.Values, &data)
	if err != nil {
		return nil, err
	}
	out := &UpdatedOdds{Data: data, UnknownMarkets: splitComma(resp.header.Get("X-Unknown-Markets"))}
	out.NextSince, _ = headerInt(resp.header, "X-Next-Since")
	out.ServerTime, _ = headerInt(resp.header, "X-Server-Time")
	return out, nil
}

// PollUpdatedOdds polls /odds/updated every interval (10s when 0), passing
// each X-Next-Since back as the next Since, until ctx is done or fn returns
// an error. p.Since defaults to now minus 20 seconds. A page cut short by
// Limit is followed up at once. Returning StopIteration from fn ends it with a nil error.
func (c *Client) PollUpdatedOdds(ctx context.Context, p UpdatedOddsParams, interval time.Duration, fn func(*UpdatedOdds) error) error {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if p.Since == 0 {
		p.Since = time.Now().Unix() - 20
	}
	for {
		batch, err := c.GetUpdatedOdds(ctx, p)
		if err != nil {
			return err
		}
		if err := fn(batch); err != nil {
			return stopOK(err)
		}
		switch {
		case batch.NextSince > 0:
			p.Since = batch.NextSince
		case batch.ServerTime > 0:
			p.Since = batch.ServerTime - 10
		default:
			p.Since = time.Now().Unix() - 10
		}
		if p.Limit > 0 && len(batch.Data) >= p.Limit {
			continue
		}
		if err := sleepCtx(ctx, interval); err != nil {
			return err
		}
	}
}

// MovementsParams is GET /odds/movements.
type MovementsParams struct {
	EventID   int64
	Bookmaker string
	Market    string
	// MarketLine selects a line on Spread/Totals; leave empty for the main line.
	MarketLine string
}

// GetOddsMovements returns the price history of one market at one bookmaker (prematch).
func (c *Client) GetOddsMovements(ctx context.Context, p MovementsParams) (*OddsMovement, error) {
	if p.EventID == 0 || p.Bookmaker == "" || p.Market == "" {
		return nil, errRequired("EventID, Bookmaker and Market")
	}
	q := newParams().int64("eventId", p.EventID).str("bookmaker", p.Bookmaker).str("market", p.Market).
		str("marketLine", p.MarketLine)
	var out OddsMovement
	if _, err := c.getJSON(ctx, "odds/movements", q.Values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SummaryParams is GET /odds/summary and /odds/summary/multi.
type SummaryParams struct {
	// EventID for GetOddsSummary.
	EventID int64
	// EventIDs (up to 10) for GetOddsSummaryMulti.
	EventIDs []int64
	// Markets, max 20. Server default ML,Spread,Totals.
	Markets []string
	// Lines is "main" (default) or "all".
	Lines            string
	Bookmakers       []string
	IncludeExchanges bool
}

func (p SummaryParams) query() params {
	return newParams().list("markets", p.Markets).str("lines", p.Lines).list("bookmakers", p.Bookmakers).
		flag("includeExchanges", p.IncludeExchanges)
}

// GetOddsSummary returns the best, average, median, worst and fair price per
// line across bookmakers (Growth plans and above).
func (c *Client) GetOddsSummary(ctx context.Context, p SummaryParams) (*EventSummary, error) {
	if p.EventID == 0 {
		return nil, errRequired("EventID")
	}
	var out EventSummary
	if _, err := c.getJSON(ctx, "odds/summary", p.query().int64("eventId", p.EventID).Values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOddsSummaryMulti is GetOddsSummary for up to 10 events.
func (c *Client) GetOddsSummaryMulti(ctx context.Context, p SummaryParams) ([]EventSummary, error) {
	if len(p.EventIDs) == 0 {
		return nil, errRequired("EventIDs")
	}
	var out []EventSummary
	_, err := c.getJSON(ctx, "odds/summary/multi", p.query().ids("eventIds", p.EventIDs).Values, &out)
	return out, err
}
