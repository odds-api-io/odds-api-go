package oddsapi

import (
	"context"
	"net/http"
)

// GetSgpBookmakers lists the bookmakers that can quote same game parlays.
// sport (a slug from GetSports) is optional. Same game parlays are enabled per
// key; without it the error matches ErrForbidden.
func (c *Client) GetSgpBookmakers(ctx context.Context, sport string) ([]SgpBookmaker, error) {
	var out []SgpBookmaker
	if _, err := c.getJSON(ctx, "sgp/bookmakers", newParams().str("sport", sport).Values, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetSgpLegs lists the legs of a pre-match event that can go into a same game
// parlay, with a LegID for each and whether each bookmaker can combine it.
func (c *Client) GetSgpLegs(ctx context.Context, p SgpLegsParams) (*SgpLegsResponse, error) {
	if p.EventID == 0 {
		return nil, errRequired("EventID")
	}
	if len(p.Bookmakers) == 0 {
		return nil, errRequired("Bookmakers")
	}
	q := newParams().int64("eventId", p.EventID).list("bookmakers", p.Bookmakers).list("markets", p.Markets).
		int("limit", p.Limit).int("skip", p.Skip)
	var out SgpLegsResponse
	if _, err := c.getJSON(ctx, "sgp/legs", q.Values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// QuoteSgp prices 2 to 10 legs as a same game parlay at each bookmaker. Every
// requested bookmaker gets one entry in Quotes; check Status and NextAction.
func (c *Client) QuoteSgp(ctx context.Context, p SgpQuoteParams) (*SgpQuoteResponse, error) {
	if p.EventID == 0 {
		return nil, errRequired("EventID")
	}
	if len(p.Bookmakers) == 0 {
		return nil, errRequired("Bookmakers")
	}
	if len(p.Legs) == 0 {
		return nil, errRequired("Legs")
	}
	body := struct {
		EventID    int64         `json:"eventId"`
		Bookmakers string        `json:"bookmakers"`
		Legs       []SgpQuoteLeg `json:"legs"`
	}{p.EventID, joinList(p.Bookmakers), p.Legs}
	var out SgpQuoteResponse
	if _, err := c.sendJSON(ctx, request{method: http.MethodPost, path: "sgp/quote", body: body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
