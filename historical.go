package oddsapi

import (
	"context"
	"time"
)

// HistoricalWindow is the longest span GET /historical/events accepts.
const HistoricalWindow = 31 * 24 * time.Hour

// HistoricalEventsParams is GET /historical/events.
type HistoricalEventsParams struct {
	Sport  string
	League string
	// From and To span at most 31 days (see IterateHistoricalEvents for longer ranges).
	From time.Time
	To   time.Time
}

// GetHistoricalEvents lists finished events of one league.
func (c *Client) GetHistoricalEvents(ctx context.Context, p HistoricalEventsParams) ([]Event, error) {
	if p.Sport == "" || p.League == "" {
		return nil, errRequired("Sport and League")
	}
	if err := requireTimes(p.From, p.To); err != nil {
		return nil, err
	}
	q := newParams().str("sport", p.Sport).str("league", p.League).time("from", p.From).time("to", p.To)
	var out []Event
	_, err := c.getJSON(ctx, "historical/events", q.Values, &out)
	return out, err
}

// IterateHistoricalEvents calls fn for every finished event of one league over
// any range, fetched in 31-day windows. Return StopIteration from fn to stop.
func (c *Client) IterateHistoricalEvents(ctx context.Context, p HistoricalEventsParams, fn func(Event) error) error {
	if err := requireTimes(p.From, p.To); err != nil {
		return err
	}
	seen := map[ID]bool{}
	end := p.To
	for start := p.From; start.Before(end); start = start.Add(HistoricalWindow) {
		window := p
		window.From = start
		window.To = start.Add(HistoricalWindow)
		if window.To.After(end) {
			window.To = end
		}
		events, err := c.GetHistoricalEvents(ctx, window)
		if err != nil {
			return err
		}
		for _, e := range events {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			if err := fn(e); err != nil {
				return stopOK(err)
			}
		}
	}
	return nil
}

// SearchHistoricalEventsParams is GET /historical/events/search.
type SearchHistoricalEventsParams struct {
	// Query is a team name, at least 3 characters.
	Query string
	Sport string
	// From defaults to 31 days before To; To defaults to now. Span at most 31 days.
	From time.Time
	To   time.Time
}

// SearchHistoricalEvents searches finished events by team name across leagues.
func (c *Client) SearchHistoricalEvents(ctx context.Context, p SearchHistoricalEventsParams) ([]Event, error) {
	if p.Query == "" {
		return nil, errRequired("Query")
	}
	q := newParams().str("query", p.Query).str("sport", p.Sport).time("from", p.From).time("to", p.To)
	var out []Event
	_, err := c.getJSON(ctx, "historical/events/search", q.Values, &out)
	return out, err
}

// GetHistoricalOdds returns the odds of a finished event, in the /odds shape.
func (c *Client) GetHistoricalOdds(ctx context.Context, p OddsParams) (*EventOdds, error) {
	if p.EventID == 0 {
		return nil, errRequired("EventID")
	}
	if len(p.Bookmakers) == 0 {
		return nil, errRequired("Bookmakers")
	}
	q := newParams().int64("eventId", p.EventID).list("bookmakers", p.Bookmakers).list("markets", p.Markets)
	var out EventOdds
	if _, err := c.getJSON(ctx, "historical/odds", q.Values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ClosingLinesParams is GET /historical/closing-lines.
type ClosingLinesParams struct {
	Sport string
	// Leagues, up to 10 slugs.
	Leagues []string
	// From and To span at most 366 days.
	From       time.Time
	To         time.Time
	Markets    []string
	Bookmakers []string
	// Limit is capped at 100.
	Limit int
	Skip  int
}

// GetClosingLines returns closing odds for every settled event across up to
// 10 leagues (paid plans), 100 events per page at most.
func (c *Client) GetClosingLines(ctx context.Context, p ClosingLinesParams) ([]EventOdds, error) {
	if p.Sport == "" || len(p.Leagues) == 0 || len(p.Markets) == 0 || len(p.Bookmakers) == 0 {
		return nil, errRequired("Sport, Leagues, Markets and Bookmakers")
	}
	if err := requireTimes(p.From, p.To); err != nil {
		return nil, err
	}
	q := newParams().str("sport", p.Sport).list("leagues", p.Leagues).time("from", p.From).time("to", p.To).
		list("markets", p.Markets).list("bookmakers", p.Bookmakers).int("limit", p.Limit).int("skip", p.Skip)
	var out []EventOdds
	_, err := c.getJSON(ctx, "historical/closing-lines", q.Values, &out)
	return out, err
}

// IterateClosingLines calls fn for every closing line matching p, paging with
// limit and skip (100 per page). Return StopIteration from fn to stop.
func (c *Client) IterateClosingLines(ctx context.Context, p ClosingLinesParams, fn func(EventOdds) error) error {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 100
	}
	for {
		page, err := c.GetClosingLines(ctx, p)
		if err != nil {
			return err
		}
		for _, e := range page {
			if err := fn(e); err != nil {
				return stopOK(err)
			}
		}
		if len(page) < p.Limit {
			return nil
		}
		p.Skip += len(page)
	}
}
