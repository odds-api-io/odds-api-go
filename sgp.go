package oddsapi

import (
	"context"
	"net/http"
	"strings"
)

// SgpBookmakerStatus values.
const (
	SgpBookmakerLive     = "live"
	SgpBookmakerPaused   = "paused"
	SgpBookmakerDegraded = "degraded"
)

// SgpQuoteStatus values.
const (
	SgpQuoteOK                     = "ok"
	SgpQuoteNotCombinable          = "not_combinable"
	SgpQuoteLegUnavailable         = "leg_unavailable"
	SgpQuoteSuspended              = "suspended"
	SgpQuoteUnsupported            = "unsupported"
	SgpQuoteTimeout                = "timeout"
	SgpQuoteTemporarilyUnavailable = "temporarily_unavailable"
)

// SgpBookmaker is a bookmaker that can quote same game parlays.
type SgpBookmaker struct {
	Name         string   `json:"name"`
	Sports       []string `json:"sports"`
	MaxLegs      int      `json:"maxLegs"`
	Prebuilt     bool     `json:"prebuilt"`
	Jurisdiction string   `json:"jurisdiction"`
	// Status is live, paused or degraded.
	Status string `json:"status"`
}

// SgpLegsParams is GET /sgp/legs.
type SgpLegsParams struct {
	// EventID of a pre-match event, required.
	EventID int64
	// Bookmakers, required, max 8.
	Bookmakers []string
	// Markets keeps only these market names, max 40.
	Markets []string
	// Limit defaults to 200 on the server, max 1000.
	Limit int
	Skip  int
}

// SgpLegBook is one bookmaker's view of a leg.
type SgpLegBook struct {
	// Eligible reports whether the bookmaker can combine the leg into a parlay.
	Eligible bool `json:"eligible"`
	// Price is the bookmaker's current single price; empty when it does not price the leg.
	Price     Decimal `json:"price,omitempty"`
	UpdatedAt Time    `json:"updatedAt"`
}

// SgpLeg is one leg that can go into a parlay.
type SgpLeg struct {
	// LegID goes into SgpQuoteLeg.LegID. Copy it; do not build it by hand.
	LegID  string   `json:"legId"`
	Market string   `json:"market"`
	Hdp    *float64 `json:"hdp,omitempty"`
	Label  string   `json:"label,omitempty"`
	Side   string   `json:"side"`
	// Bookmakers is keyed by bookmaker name.
	Bookmakers map[string]SgpLegBook `json:"bookmakers"`
}

// SgpLegsResponse is GET /sgp/legs.
type SgpLegsResponse struct {
	EventID ID       `json:"eventId"`
	Date    Time     `json:"date"`
	Status  string   `json:"status"`
	Legs    []SgpLeg `json:"legs"`
	// HasMore is true when another call with a higher Skip returns more legs.
	HasMore bool `json:"hasMore"`
}

// SgpQuoteLeg is one leg to quote. Give either LegID, or Market, Hdp, Label
// and Side exactly as served on /odds.
type SgpQuoteLeg struct {
	LegID  string   `json:"legId,omitempty"`
	Market string   `json:"market,omitempty"`
	Hdp    *float64 `json:"hdp,omitempty"`
	Label  string   `json:"label,omitempty"`
	Side   string   `json:"side,omitempty"`
	// ExpectedPrice is the single price you saw; the leg comes back with
	// PriceMoved set when the current price differs.
	ExpectedPrice string `json:"expectedPrice,omitempty"`
}

// SgpQuoteParams is POST /sgp/quote.
type SgpQuoteParams struct {
	// EventID of a pre-match event, required.
	EventID int64
	// Bookmakers, required, max 8.
	Bookmakers []string
	// Legs, 2 to 10.
	Legs []SgpQuoteLeg
}

// SgpQuoteLegResult is a leg as priced by one bookmaker.
type SgpQuoteLegResult struct {
	LegID string `json:"legId"`
	// Price is the current single price at this bookmaker.
	Price      Decimal  `json:"price,omitempty"`
	Hdp        *float64 `json:"hdp,omitempty"`
	PriceMoved bool     `json:"priceMoved,omitempty"`
}

// SgpQuoteLinks are betslip links.
type SgpQuoteLinks struct {
	Desktop string `json:"desktop,omitempty"`
	Mobile  string `json:"mobile,omitempty"`
}

// SgpBookQuote is one bookmaker's answer: a parlay Price when Status is
// "ok", otherwise Status, NextAction and Reason say what to do.
type SgpBookQuote struct {
	Bookmaker          string              `json:"bookmaker"`
	Status             string              `json:"status"`
	Price              Decimal             `json:"price,omitempty"`
	LegsMultiplied     Decimal             `json:"legsMultiplied,omitempty"`
	PriceAdjustmentPct string              `json:"priceAdjustmentPct,omitempty"`
	Legs               []SgpQuoteLegResult `json:"legs,omitempty"`
	// LegIDs are the legs that made the quote fail (leg_unavailable).
	LegIDs         []string       `json:"legIds,omitempty"`
	Links          *SgpQuoteLinks `json:"links,omitempty"`
	Jurisdiction   string         `json:"jurisdiction,omitempty"`
	DurationMs     int            `json:"durationMs,omitempty"`
	QuotedAt       Time           `json:"quotedAt"`
	Cached         bool           `json:"cached,omitempty"`
	CacheExpiresAt Time           `json:"cacheExpiresAt"`
	Retryable      bool           `json:"retryable,omitempty"`
	// RetryAfter is in seconds.
	RetryAfter int    `json:"retryAfter,omitempty"`
	NextAction string `json:"nextAction,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// SgpQuoteResponse is POST /sgp/quote, one quote per requested bookmaker in request order.
type SgpQuoteResponse struct {
	EventID ID             `json:"eventId"`
	Quotes  []SgpBookQuote `json:"quotes"`
}

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
	}{p.EventID, strings.Join(p.Bookmakers, ","), p.Legs}
	var out SgpQuoteResponse
	if _, err := c.sendJSON(ctx, request{method: http.MethodPost, path: "sgp/quote", body: body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
