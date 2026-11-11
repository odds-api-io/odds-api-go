package oddsapi

import (
	"context"
	"net/http"
)

const settlementBatch = 50

// GetSettlements returns settlement records (status, final flag, final scores)
// for events (paid plans). More than 50 ids are split into several requests.
func (c *Client) GetSettlements(ctx context.Context, eventIDs ...int64) (*SettlementsResponse, error) {
	if len(eventIDs) == 0 {
		return nil, errRequired("eventIDs")
	}
	out := &SettlementsResponse{Settlements: []Settlement{}, NotFound: []string{}}
	for i := 0; i < len(eventIDs); i += settlementBatch {
		batch := eventIDs[i:min(i+settlementBatch, len(eventIDs))]
		var page SettlementsResponse
		if _, err := c.getJSON(ctx, "settlements", newParams().ids("eventIds", batch).Values, &page); err != nil {
			return nil, err
		}
		out.Settlements = append(out.Settlements, page.Settlements...)
		out.NotFound = append(out.NotFound, page.NotFound...)
	}
	return out, nil
}

// GradeBets grades bet selections against final results (paid plans) and
// returns one grade per selection, in order. More than 50 selections are sent
// in batches. Re-send selections that are not Final, at most once a minute.
func (c *Client) GradeBets(ctx context.Context, selections []GradeSelection) (*GradesResponse, error) {
	if len(selections) == 0 {
		return nil, errRequired("selections")
	}
	out := &GradesResponse{Grades: []Grade{}}
	for i := 0; i < len(selections); i += settlementBatch {
		body := struct {
			Selections []GradeSelection `json:"selections"`
		}{selections[i:min(i+settlementBatch, len(selections))]}
		var page GradesResponse
		if _, err := c.sendJSON(ctx, request{method: http.MethodPost, path: "settlements/grade", body: body}, &page); err != nil {
			return nil, err
		}
		out.Grades = append(out.Grades, page.Grades...)
	}
	return out, nil
}

// OutrightsParams is GET /outrights.
type OutrightsParams struct {
	// Sport is golf today.
	Sport string
	// League is pga-tour, dp-world-tour, liv-golf or majors.
	League string
	// Status defaults to pending,live on the server.
	Status []string
}

// GetOutrights lists tournaments (paid add-on; without it the error matches
// ErrForbidden and IsAddOnRequired is true).
func (c *Client) GetOutrights(ctx context.Context, p OutrightsParams) ([]Outright, error) {
	if p.Sport == "" {
		return nil, errRequired("Sport")
	}
	var out []Outright
	_, err := c.getJSON(ctx, "outrights", newParams().str("sport", p.Sport).str("league", p.League).list("status", p.Status).Values, &out)
	return out, err
}

// GetOutrightOdds returns the prices of one tournament per bookmaker: Winner,
// Top 5/10/20, the "(Incl. Ties)" variants and Make Cut.
func (c *Client) GetOutrightOdds(ctx context.Context, id int64, bookmakers ...string) (*OutrightOdds, error) {
	if id == 0 {
		return nil, errRequired("id")
	}
	if len(bookmakers) == 0 {
		return nil, errRequired("bookmakers")
	}
	var out OutrightOdds
	if _, err := c.getJSON(ctx, "outrights/odds", newParams().int64("id", id).list("bookmakers", bookmakers).Values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOutrightResults returns final positions and winners per market for one tournament.
func (c *Client) GetOutrightResults(ctx context.Context, id int64) (*OutrightResults, error) {
	if id == 0 {
		return nil, errRequired("id")
	}
	var out OutrightResults
	if _, err := c.getJSON(ctx, "outrights/results", newParams().int64("id", id).Values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MappingsParams is GET /mappings. Without EventID(s), MarketID(s) or
// OutcomeID it is a bulk sync that pages with Cursor.
type MappingsParams struct {
	// Bookmaker, required.
	Bookmaker string
	EventID   int64
	// EventIDs, up to 10.
	EventIDs []int64
	MarketID string
	// MarketIDs, up to 10.
	MarketIDs []string
	OutcomeID string
	// Market is the exact market name as in /odds, case-sensitive.
	Market string
	Active *bool
	// Sport, League, Limit (1 to 5000) and Cursor apply to bulk sync only.
	Sport  string
	League string
	Limit  int
	Cursor string
}

// GetMappings returns a bookmaker's own event, market and outcome ids for our events (paid plans).
func (c *Client) GetMappings(ctx context.Context, p MappingsParams) (*MappingsPage, error) {
	if p.Bookmaker == "" {
		return nil, errRequired("Bookmaker")
	}
	q := newParams().str("bookmaker", p.Bookmaker).int64("eventId", p.EventID).ids("eventIds", p.EventIDs).
		str("marketId", p.MarketID).list("marketIds", p.MarketIDs).str("outcomeId", p.OutcomeID).
		str("market", p.Market).optBool("active", p.Active).str("sport", p.Sport).str("league", p.League).
		int("limit", p.Limit).str("cursor", p.Cursor)
	var out MappingsPage
	if _, err := c.getJSON(ctx, "mappings", q.Values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IterateMappings calls fn for every mapping row, following NextCursor until
// it is empty. Return StopIteration from fn to stop.
func (c *Client) IterateMappings(ctx context.Context, p MappingsParams, fn func(Mapping) error) error {
	for {
		page, err := c.GetMappings(ctx, p)
		if err != nil {
			return err
		}
		for _, m := range page.Data {
			if err := fn(m); err != nil {
				return stopOK(err)
			}
		}
		if page.NextCursor == "" || page.NextCursor == p.Cursor {
			return nil
		}
		p.Cursor = page.NextCursor
	}
}
