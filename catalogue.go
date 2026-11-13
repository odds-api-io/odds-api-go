package oddsapi

import (
	"context"
	"net/http"
	"strings"
)

// GetSports lists all sports. No API key needed.
func (c *Client) GetSports(ctx context.Context) ([]Sport, error) {
	var out []Sport
	_, err := c.sendJSON(ctx, request{path: "sports", noAuth: true}, &out)
	return out, err
}

// LeaguesParams filters GET /leagues.
type LeaguesParams struct {
	// Sport slug, required.
	Sport string
	// All includes leagues without current events.
	All bool
}

// GetLeagues lists the leagues of a sport.
func (c *Client) GetLeagues(ctx context.Context, p LeaguesParams) ([]League, error) {
	if p.Sport == "" {
		return nil, errRequired("Sport")
	}
	var out []League
	_, err := c.getJSON(ctx, "leagues", newParams().str("sport", p.Sport).flag("all", p.All).Values, &out)
	return out, err
}

// GetMarkets returns the exact market names a sport serves, with shape,
// period and whether each is served prematch and live. No API key needed.
// The same catalogue is browsable per sport at https://odds-api.io/markets
// (for example https://odds-api.io/markets/football).
func (c *Client) GetMarkets(ctx context.Context, sport string) (*MarketsResponse, error) {
	if sport == "" {
		return nil, errRequired("sport")
	}
	var out MarketsResponse
	_, err := c.sendJSON(ctx, request{path: "markets", query: newParams().str("sport", sport).Values, noAuth: true}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAllMarkets returns the market catalogue of every sport, keyed by sport slug.
func (c *Client) GetAllMarkets(ctx context.Context) (map[string][]MarketInfo, error) {
	var out map[string][]MarketInfo
	_, err := c.sendJSON(ctx, request{path: "markets", noAuth: true}, &out)
	return out, err
}

// GetBookmakers lists all bookmakers with their current status. No API key needed.
// Coverage by sport: https://odds-api.io/sportsbooks#coverage
func (c *Client) GetBookmakers(ctx context.Context) ([]Bookmaker, error) {
	var out []Bookmaker
	_, err := c.sendJSON(ctx, request{path: "bookmakers", noAuth: true}, &out)
	return out, err
}

// GetSelectedBookmakers returns the bookmakers selected on your key.
func (c *Client) GetSelectedBookmakers(ctx context.Context) (*SelectedBookmakers, error) {
	var out SelectedBookmakers
	if _, err := c.getJSON(ctx, "bookmakers/selected", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SelectBookmakers adds bookmakers to your selection. It is never retried.
func (c *Client) SelectBookmakers(ctx context.Context, bookmakers ...string) (*SelectedBookmakers, error) {
	q := newParams().list("bookmakers", bookmakers)
	if q.Get("bookmakers") == "" {
		return nil, errRequired("bookmakers")
	}
	var out SelectedBookmakers
	if _, err := c.sendJSON(ctx, request{method: http.MethodPut, path: "bookmakers/selected/select", query: q.Values}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ClearSelectedBookmakers clears your selection. Allowed once every 12 hours.
func (c *Client) ClearSelectedBookmakers(ctx context.Context) (*SelectedBookmakers, error) {
	var out SelectedBookmakers
	if _, err := c.sendJSON(ctx, request{method: http.MethodPut, path: "bookmakers/selected/clear"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func splitComma(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
