package oddsapi

import "context"

// ValueBetsParams is GET /value-bets.
type ValueBetsParams struct {
	// Bookmaker, required.
	Bookmaker string
	Sport     string
	// League requires Sport.
	League              string
	IncludeEventDetails bool
}

// GetValueBets returns prices above a fair-odds estimate at one bookmaker.
func (c *Client) GetValueBets(ctx context.Context, p ValueBetsParams) ([]ValueBet, error) {
	if p.Bookmaker == "" {
		return nil, errRequired("Bookmaker")
	}
	q := newParams().str("bookmaker", p.Bookmaker).str("sport", p.Sport).str("league", p.League).
		flag("includeEventDetails", p.IncludeEventDetails)
	var out []ValueBet
	_, err := c.getJSON(ctx, "value-bets", q.Values, &out)
	return out, err
}

// ArbitrageParams is GET /arbitrage-bets.
type ArbitrageParams struct {
	// Bookmakers, required.
	Bookmakers []string
	Sport      string
	// League requires Sport.
	League  string
	Markets []string
	// MinProfit is the minimum profit margin in percent, 0 to 100.
	MinProfit float64
	// AllCombinations returns every bookmaker combination per event, market and line.
	AllCombinations bool
	// Limit defaults to 50 on the server, max 500.
	Limit               int
	IncludeEventDetails bool
}

// GetArbitrageBets returns price gaps across your bookmakers where the best
// prices imply under 100% (prematch). Prices move and limits apply, so check
// every leg before acting.
func (c *Client) GetArbitrageBets(ctx context.Context, p ArbitrageParams) ([]ArbitrageBet, error) {
	if len(p.Bookmakers) == 0 {
		return nil, errRequired("Bookmakers")
	}
	q := newParams().list("bookmakers", p.Bookmakers).str("sport", p.Sport).str("league", p.League).
		list("markets", p.Markets).float("minProfit", p.MinProfit).flag("allCombinations", p.AllCombinations).
		int("limit", p.Limit).flag("includeEventDetails", p.IncludeEventDetails)
	var out []ArbitrageBet
	_, err := c.getJSON(ctx, "arbitrage-bets", q.Values, &out)
	return out, err
}

// DroppingOddsParams is GET /dropping-odds.
type DroppingOddsParams struct {
	Sport string
	// League requires Sport; not with Leagues.
	League  string
	Leagues []string
	Markets []string
	// TimeWindow is opening (default), 12h, 24h or 48h.
	TimeWindow string
	// Sort is drop (default), recent or kickoff.
	Sort    string
	MinDrop float64
	// Limit is max 200 (server default 50).
	Limit int
	// Page is 1-indexed.
	Page                int
	IncludeEventDetails bool
}

// DroppingOddsPage is a /dropping-odds response with its pagination headers.
type DroppingOddsPage struct {
	Data []DroppingOddsEntry
	// Total is X-Total-Count, -1 when missing.
	Total int64
	// Page is X-Page and PerPage is X-Per-Page.
	Page    int64
	PerPage int64
	// UpdatedAt is X-Updated-At, unix milliseconds.
	UpdatedAt int64
}

// GetDroppingOdds returns one page of dropping odds from sharp bookmakers (paid plans).
func (c *Client) GetDroppingOdds(ctx context.Context, p DroppingOddsParams) (*DroppingOddsPage, error) {
	q := newParams().str("sport", p.Sport).str("league", p.League).list("leagues", p.Leagues).
		list("markets", p.Markets).str("timeWindow", p.TimeWindow).str("sort", p.Sort).
		float("minDrop", p.MinDrop).int("limit", p.Limit).int("page", p.Page).
		flag("includeEventDetails", p.IncludeEventDetails)
	var data []DroppingOddsEntry
	resp, err := c.getJSON(ctx, "dropping-odds", q.Values, &data)
	if err != nil {
		return nil, err
	}
	page := &DroppingOddsPage{Data: data, Total: -1}
	if n, ok := headerInt(resp.header, "X-Total-Count"); ok {
		page.Total = n
	}
	page.Page, _ = headerInt(resp.header, "X-Page")
	page.PerPage, _ = headerInt(resp.header, "X-Per-Page")
	page.UpdatedAt, _ = headerInt(resp.header, "X-Updated-At")
	return page, nil
}

// IterateDroppingOdds calls fn for every dropping-odds entry matching p, page
// by page (200 per page when Limit is 0). Return StopIteration from fn to stop.
func (c *Client) IterateDroppingOdds(ctx context.Context, p DroppingOddsParams, fn func(DroppingOddsEntry) error) error {
	if p.Limit <= 0 {
		p.Limit = 200
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	var seen int64
	for {
		page, err := c.GetDroppingOdds(ctx, p)
		if err != nil {
			return err
		}
		for _, e := range page.Data {
			if err := fn(e); err != nil {
				return stopOK(err)
			}
		}
		seen += int64(len(page.Data))
		if len(page.Data) < p.Limit || (page.Total >= 0 && seen >= page.Total) {
			return nil
		}
		p.Page++
	}
}
