package oddsapi

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// ID is a numeric id that decodes from a JSON number or a numeric string.
type ID int64

// UnmarshalJSON accepts 123, "123" and null.
func (id *ID) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" || string(b) == `""` {
		*id = 0
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		b = []byte(strings.TrimSpace(s))
	}
	n, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(string(b), 64)
		if ferr != nil {
			return err
		}
		n = int64(f)
	}
	*id = ID(n)
	return nil
}

// Int64 returns the id as an int64.
func (id ID) Int64() int64 { return int64(id) }

// String returns the id in base 10.
func (id ID) String() string { return strconv.FormatInt(int64(id), 10) }

// Decimal is a decimal price kept exactly as the API sent it ("1.85").
// It decodes from a JSON string or number.
type Decimal string

// UnmarshalJSON accepts "1.85", 1.85 and null.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*d = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*d = Decimal(s)
		return nil
	}
	*d = Decimal(b)
	return nil
}

// Float returns the price as a float64, or 0 when it is empty or not a number.
func (d Decimal) Float() float64 {
	f, _ := strconv.ParseFloat(string(d), 64)
	return f
}

// Time is a time.Time that decodes RFC 3339 strings and treats "" and null as zero.
type Time struct{ time.Time }

// UnmarshalJSON accepts RFC 3339 strings, "" and null.
func (t *Time) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" || string(b) == `""` {
		t.Time = time.Time{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// MarshalJSON writes RFC 3339, or null for the zero time.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time.Format(time.RFC3339Nano))
}

// NamedSlug is a {name, slug} pair, used for sports and leagues.
type NamedSlug struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Sport is one entry of GET /sports.
type Sport = NamedSlug

// League is one entry of GET /leagues.
type League struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	EventsCount int    `json:"eventsCount,omitempty"`
}

// Bookmaker is one entry of GET /bookmakers.
type Bookmaker struct {
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	CreatedAt Time   `json:"createdAt"`
	// Status is "ok", "degraded" (no update for 60+ minutes, still served) or "down".
	Status string `json:"status"`
	// Reason is "maintenance" or "unavailable", empty when ok.
	Reason string `json:"reason,omitempty"`
}

// SelectedBookmakers is the response of the /bookmakers/selected endpoints.
type SelectedBookmakers struct {
	Bookmakers []string `json:"bookmakers"`
	Count      int      `json:"count"`
	Message    string   `json:"message,omitempty"`
	Added      int      `json:"added,omitempty"`
}

// MarketInfo describes one market a sport serves.
type MarketInfo struct {
	Name string `json:"name"`
	// Shape is moneyline, line, outcomes, yesno or labelled.
	Shape string `json:"shape"`
	// Period is match, first-half, second-half, quarter, set, game or inning.
	Period   string `json:"period"`
	Prematch bool   `json:"prematch"`
	Live     bool   `json:"live"`
}

// MarketsResponse is GET /markets?sport=...
type MarketsResponse struct {
	Sport   NamedSlug    `json:"sport"`
	Markets []MarketInfo `json:"markets"`
}

// Event statuses.
const (
	StatusPending   = "pending"
	StatusLive      = "live"
	StatusSettled   = "settled"
	StatusCancelled = "cancelled"
)

// PeriodScore is the score of one period. Wickets and overs are cricket only.
type PeriodScore struct {
	Home    float64     `json:"home"`
	Away    float64     `json:"away"`
	Wickets *SideFloats `json:"wickets,omitempty"`
	Overs   *SideFloats `json:"overs,omitempty"`
}

// SideFloats holds a home and away value.
type SideFloats struct {
	Home float64 `json:"home"`
	Away float64 `json:"away"`
}

// Score is an event score. Periods keys are p1..pN, ft (end of regulation),
// ot (after overtime), ap (after penalties), currentgame (live tennis),
// map1.. (esports) and inning1.. (cricket); which appear depends on sport and source.
type Score struct {
	Home    float64                `json:"home"`
	Away    float64                `json:"away"`
	Periods map[string]PeriodScore `json:"periods,omitempty"`
}

// Clock is the live clock of an event.
type Clock struct {
	Minute        int    `json:"minute,omitempty"`
	PlayedSeconds int    `json:"playedSeconds,omitempty"`
	Period        int    `json:"period,omitempty"`
	Running       bool   `json:"running"`
	StatusDetail  string `json:"statusDetail,omitempty"`
	Serve         string `json:"serve,omitempty"`
	InjuryTime    int    `json:"injuryTime,omitempty"`
	// PeriodStartedAt is unix seconds, only on feeds without a played clock.
	PeriodStartedAt int64 `json:"periodStartedAt,omitempty"`
}

// EsportsDetails is attached to esports events.
type EsportsDetails struct {
	Game   NamedSlug `json:"game"`
	League NamedSlug `json:"league"`
	Region string    `json:"region,omitempty"`
	Format *struct {
		Type  string `json:"type"`
		Games int    `json:"games"`
	} `json:"format,omitempty"`
}

// Event is one entry of /events, /events/live, /events/search and /historical/events.
type Event struct {
	ID             ID              `json:"id"`
	Home           string          `json:"home"`
	Away           string          `json:"away"`
	HomeID         ID              `json:"homeId,omitempty"`
	AwayID         ID              `json:"awayId,omitempty"`
	Date           Time            `json:"date"`
	Status         string          `json:"status"`
	Sport          NamedSlug       `json:"sport"`
	League         NamedSlug       `json:"league"`
	Scores         *Score          `json:"scores,omitempty"`
	BookmakerCount int             `json:"bookmakerCount,omitempty"`
	Clock          *Clock          `json:"clock,omitempty"`
	Esports        *EsportsDetails `json:"esports,omitempty"`
}

// Participant is a team or player.
type Participant struct {
	ID        ID     `json:"id"`
	Name      string `json:"name"`
	Sport     string `json:"sport"`
	Game      string `json:"game,omitempty"`
	ShortCode string `json:"shortCode,omitempty"`
}

// OddsLine is one line of a market. Which fields are set depends on the
// market shape. Every field the API sent, including ones without a named
// field here, is in Fields.
type OddsLine struct {
	Hdp   *float64 `json:"hdp,omitempty"`
	Label string   `json:"label,omitempty"`
	Home  Decimal  `json:"home,omitempty"`
	Draw  Decimal  `json:"draw,omitempty"`
	Away  Decimal  `json:"away,omitempty"`
	Over  Decimal  `json:"over,omitempty"`
	Under Decimal  `json:"under,omitempty"`
	// Exactly is the push price on three-way lines.
	Exactly Decimal `json:"exactly,omitempty"`
	Yes     Decimal `json:"yes,omitempty"`
	No      Decimal `json:"no,omitempty"`
	// Odds is the price of a labelled outcome (Correct Score, Player Props, ...).
	Odds Decimal `json:"odds,omitempty"`
	// Max is the stake limit, when the bookmaker publishes one.
	Max      *float64 `json:"max,omitempty"`
	LayHome  Decimal  `json:"layHome,omitempty"`
	LayDraw  Decimal  `json:"layDraw,omitempty"`
	LayAway  Decimal  `json:"layAway,omitempty"`
	LayOver  Decimal  `json:"layOver,omitempty"`
	LayUnder Decimal  `json:"layUnder,omitempty"`
	LayYes   Decimal  `json:"layYes,omitempty"`
	LayNo    Decimal  `json:"layNo,omitempty"`
	HomeLink string   `json:"homeLink,omitempty"`
	DrawLink string   `json:"drawLink,omitempty"`
	AwayLink string   `json:"awayLink,omitempty"`

	// Fields holds every field of the line as sent, for keys such as 1X, 12, X2, odd and even.
	Fields map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the named fields and keeps every field in Fields.
func (l *OddsLine) UnmarshalJSON(b []byte) error {
	type plain OddsLine
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	if err := json.Unmarshal(b, &p.Fields); err != nil {
		return err
	}
	*l = OddsLine(p)
	return nil
}

// Price returns the price of any outcome key ("home", "1X", "over", ...) as a Decimal.
func (l OddsLine) Price(key string) Decimal {
	raw, ok := l.Fields[key]
	if !ok {
		return ""
	}
	var d Decimal
	if err := d.UnmarshalJSON(raw); err != nil {
		return ""
	}
	return d
}

// Market is one market at one bookmaker.
type Market struct {
	Name      string     `json:"name"`
	UpdatedAt Time       `json:"updatedAt"`
	Odds      []OddsLine `json:"odds"`
}

// EventOdds is the response of /odds, /odds/multi, /odds/updated,
// /historical/odds and /historical/closing-lines.
type EventOdds struct {
	ID     ID        `json:"id"`
	Home   string    `json:"home"`
	Away   string    `json:"away"`
	HomeID ID        `json:"homeId,omitempty"`
	AwayID ID        `json:"awayId,omitempty"`
	Date   Time      `json:"date"`
	Status string    `json:"status"`
	Sport  NamedSlug `json:"sport"`
	League NamedSlug `json:"league"`
	Scores *Score    `json:"scores,omitempty"`
	// URLs are bookmaker links for the event, keyed by bookmaker.
	URLs map[string]string `json:"urls,omitempty"`
	// BookmakerIDs are the bookmakers' own event ids, where known.
	BookmakerIDs map[string]string `json:"bookmakerIds,omitempty"`
	// Bookmakers maps bookmaker name to its markets.
	Bookmakers map[string][]Market `json:"bookmakers"`
}

// Market returns the named market at a bookmaker, or nil.
func (o *EventOdds) Market(bookmaker, name string) *Market {
	for i := range o.Bookmakers[bookmaker] {
		if o.Bookmakers[bookmaker][i].Name == name {
			return &o.Bookmakers[bookmaker][i]
		}
	}
	return nil
}

// OddsMovementPoint is one price in a movement history. On Totals the over
// price is in Home and the under price in Away.
type OddsMovementPoint struct {
	Home float64  `json:"home"`
	Draw *float64 `json:"draw,omitempty"`
	Away float64  `json:"away"`
	Hdp  Decimal  `json:"hdp,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	// Timestamp is unix milliseconds.
	Timestamp int64 `json:"timestamp"`
}

// OddsMovement is GET /odds/movements.
type OddsMovement struct {
	EventID   ID                  `json:"eventid"`
	Bookmaker string              `json:"bookmaker"`
	Opening   OddsMovementPoint   `json:"opening"`
	Latest    OddsMovementPoint   `json:"latest"`
	Movements []OddsMovementPoint `json:"movements"`
}

// SummaryOutcome aggregates one outcome across bookmakers.
type SummaryOutcome struct {
	Best           float64  `json:"best"`
	BestBookmakers []string `json:"bestBookmakers"`
	Average        float64  `json:"average"`
	Median         float64  `json:"median"`
	Worst          float64  `json:"worst"`
	Fair           *float64 `json:"fair,omitempty"`
	SharpFair      *float64 `json:"sharpFair,omitempty"`
	Excluded       int      `json:"excluded,omitempty"`
}

// SummaryExchangeQuote is the best exchange price for an outcome.
type SummaryExchangeQuote struct {
	Best      float64  `json:"best"`
	Venue     string   `json:"venue"`
	Liquidity *float64 `json:"liquidity,omitempty"`
	Currency  string   `json:"currency,omitempty"`
}

// SummaryLine is one line of a summary market.
type SummaryLine struct {
	Hdp       *float64 `json:"hdp"`
	Main      bool     `json:"main"`
	Books     int      `json:"books"`
	FairBooks int      `json:"fairBooks,omitempty"`
	// Outcomes is keyed home/draw/away, over/under or yes/no.
	Outcomes map[string]SummaryOutcome `json:"outcomes"`
	Payout   *struct {
		Best    float64 `json:"best"`
		Average float64 `json:"average"`
	} `json:"payout,omitempty"`
	Exchanges map[string]SummaryExchangeQuote `json:"exchanges,omitempty"`
}

// SummaryMarket is one market of an odds summary.
type SummaryMarket struct {
	Name  string        `json:"name"`
	Lines []SummaryLine `json:"lines"`
}

// EventSummary is GET /odds/summary. Plans below Growth get Demo=true and a sample.
type EventSummary struct {
	Demo       bool            `json:"demo,omitempty"`
	Message    string          `json:"message,omitempty"`
	EventID    ID              `json:"eventId"`
	Home       string          `json:"home"`
	Away       string          `json:"away"`
	Date       Time            `json:"date"`
	Status     string          `json:"status"`
	ComputedAt Time            `json:"computedAt"`
	Markets    []SummaryMarket `json:"markets"`
}

// EventInfo is attached with IncludeEventDetails.
type EventInfo struct {
	Home   string `json:"home"`
	Away   string `json:"away"`
	Date   Time   `json:"date"`
	Sport  string `json:"sport"`
	League string `json:"league"`
}

// ArbitrageLeg is one leg of an arbitrage opportunity.
type ArbitrageLeg struct {
	Side               string   `json:"side"`
	Bookmaker          string   `json:"bookmaker"`
	Odds               Decimal  `json:"odds"`
	NetOdds            Decimal  `json:"netOdds,omitempty"`
	MaxStake           *float64 `json:"maxStake,omitempty"`
	OddsUpdatedAt      Time     `json:"oddsUpdatedAt,omitempty"`
	Href               string   `json:"href,omitempty"`
	DirectLink         string   `json:"directLink,omitempty"`
	Label              string   `json:"label,omitempty"`
	BookmakerFixtureID string   `json:"bookmakerFixtureId,omitempty"`
}

// OptimalStake is the stake split for one leg.
type OptimalStake struct {
	Side            string  `json:"side"`
	Bookmaker       string  `json:"bookmaker"`
	Stake           float64 `json:"stake"`
	PotentialReturn float64 `json:"potentialReturn"`
}

// ArbitrageBet is one entry of GET /arbitrage-bets.
type ArbitrageBet struct {
	ID      string `json:"id"`
	EventID ID     `json:"eventId"`
	Market  struct {
		Name  string   `json:"name"`
		Hdp   *float64 `json:"hdp,omitempty"`
		Label string   `json:"label,omitempty"`
	} `json:"market"`
	// ProfitMargin is a percentage of TotalStake (2.48 = 2.48%).
	ProfitMargin       float64        `json:"profitMargin"`
	ImpliedProbability float64        `json:"impliedProbability"`
	TotalStake         float64        `json:"totalStake"`
	Legs               []ArbitrageLeg `json:"legs"`
	OptimalStakes      []OptimalStake `json:"optimalStakes"`
	UpdatedAt          Time           `json:"updatedAt"`
	FirstSeenAt        Time           `json:"firstSeenAt,omitempty"`
	Event              *EventInfo     `json:"event,omitempty"`
}

// ValueBet is one entry of GET /value-bets.
type ValueBet struct {
	ID        string `json:"id"`
	EventID   ID     `json:"eventId"`
	Bookmaker string `json:"bookmaker"`
	Market    struct {
		Name string   `json:"name"`
		Hdp  *float64 `json:"hdp,omitempty"`
		Home Decimal  `json:"home"`
		Draw Decimal  `json:"draw,omitempty"`
		Away Decimal  `json:"away"`
		Max  *float64 `json:"max"`
	} `json:"market"`
	BetSide string `json:"betSide,omitempty"`
	// ExpectedValue is bookmaker odds / fair odds x 100: 103.42 is a 3.42% edge.
	ExpectedValue          float64 `json:"expectedValue"`
	ExpectedValueUpdatedAt Time    `json:"expectedValueUpdatedAt"`
	FirstSeenAt            Time    `json:"firstSeenAt,omitempty"`
	BookmakerOddsUpdatedAt Time    `json:"bookmakerOddsUpdatedAt,omitempty"`
	BookmakerOdds          struct {
		Home           Decimal `json:"home"`
		Away           Decimal `json:"away"`
		Draw           Decimal `json:"draw,omitempty"`
		Hdp            Decimal `json:"hdp,omitempty"`
		Href           string  `json:"href,omitempty"`
		HomeDirectLink string  `json:"homeDirectLink,omitempty"`
		DrawDirectLink string  `json:"drawDirectLink,omitempty"`
		AwayDirectLink string  `json:"awayDirectLink,omitempty"`
	} `json:"bookmakerOdds"`
	Event *EventInfo `json:"event,omitempty"`
}

// Edge returns the edge in percent (ExpectedValue - 100).
func (v ValueBet) Edge() float64 { return v.ExpectedValue - 100 }

// DroppingOddsEntry is one entry of GET /dropping-odds.
type DroppingOddsEntry struct {
	EventID ID `json:"eventId"`
	Event   *struct {
		Home   string    `json:"home"`
		Away   string    `json:"away"`
		Date   Time      `json:"date"`
		Sport  NamedSlug `json:"sport"`
		League NamedSlug `json:"league"`
	} `json:"event,omitempty"`
	Market struct {
		Name  string   `json:"name"`
		Hdp   *float64 `json:"hdp,omitempty"`
		Label string   `json:"label,omitempty"`
	} `json:"market"`
	BetSide string `json:"betSide"`
	Odds    struct {
		Opening float64 `json:"opening"`
		Current float64 `json:"current"`
		Drop    struct {
			SinceOpening float64  `json:"sinceOpening"`
			H12          *float64 `json:"12h"`
			H24          *float64 `json:"24h"`
			H48          *float64 `json:"48h"`
		} `json:"drop"`
	} `json:"odds"`
	// LastMovedAt and UpdatedAt are unix milliseconds.
	LastMovedAt *int64 `json:"lastMovedAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// Settlement is the settlement record of one event.
type Settlement struct {
	EventID ID     `json:"eventId"`
	Sport   string `json:"sport"`
	// Status is pending, live, awaiting_result, settled or cancelled.
	Status string `json:"status"`
	// Final is true when you can stop polling.
	Final     bool   `json:"final"`
	SettledAt Time   `json:"settledAt,omitempty"`
	Scores    *Score `json:"scores,omitempty"`
}

// SettlementsResponse is GET /settlements.
type SettlementsResponse struct {
	Settlements []Settlement `json:"settlements"`
	NotFound    []string     `json:"notFound"`
}

// GradeSelection is one selection to grade.
type GradeSelection struct {
	// Ref is echoed back, up to 64 characters.
	Ref     string `json:"ref,omitempty"`
	EventID int64  `json:"eventId"`
	// Market is the market name exactly as served on /odds (case-insensitive).
	Market string `json:"market"`
	// Side is home, away, draw, over, under, yes, no, odd, even, 1x, 12, x2,
	// none, neither, or an HT/FT pair such as draw/home.
	Side string `json:"side,omitempty"`
	// Hdp is the line as served on /odds, stated for the home side.
	Hdp *float64 `json:"hdp,omitempty"`
	// Label is required for Player Props and labelled markets.
	Label string `json:"label,omitempty"`
	// TieRule is "lose" or "void".
	TieRule string `json:"tieRule,omitempty"`
}

// Grade results.
const (
	GradeWon         = "won"
	GradeLost        = "lost"
	GradeVoid        = "void"
	GradeHalfWon     = "half_won"
	GradeHalfLost    = "half_lost"
	GradePending     = "pending"
	GradeUngradeable = "ungradeable"
)

// Grade is the verdict for one selection.
type Grade struct {
	Ref     string `json:"ref,omitempty"`
	EventID ID     `json:"eventId"`
	Result  string `json:"result"`
	Reason  string `json:"reason,omitempty"`
	Final   bool   `json:"final"`
	Basis   *struct {
		Scope string  `json:"scope"`
		Home  float64 `json:"home"`
		Away  float64 `json:"away"`
	} `json:"basis,omitempty"`
}

// GradesResponse is POST /settlements/grade.
type GradesResponse struct {
	Grades []Grade `json:"grades"`
}

// Outright is a tournament from GET /outrights.
type Outright struct {
	ID             ID        `json:"id"`
	Name           string    `json:"name"`
	Date           Time      `json:"date"`
	Status         string    `json:"status"`
	Sport          NamedSlug `json:"sport"`
	League         NamedSlug `json:"league"`
	BookmakerCount int       `json:"bookmakerCount"`
}

// OutrightRunner is one runner of an outright market. Winner and Top N
// runners have Odds (and LayOdds on exchanges); Make Cut runners have Yes/No.
type OutrightRunner struct {
	Label         string  `json:"label"`
	ParticipantID ID      `json:"participantId"`
	Odds          Decimal `json:"odds,omitempty"`
	LayOdds       Decimal `json:"layOdds,omitempty"`
	Yes           Decimal `json:"yes,omitempty"`
	No            Decimal `json:"no,omitempty"`
	LayYes        Decimal `json:"layYes,omitempty"`
	LayNo         Decimal `json:"layNo,omitempty"`
}

// IsYesNo reports whether the runner carries yes/no prices (Make Cut).
func (r OutrightRunner) IsYesNo() bool { return r.Odds == "" && (r.Yes != "" || r.No != "") }

// OutrightMarket is one outright market at one bookmaker.
type OutrightMarket struct {
	Name      string           `json:"name"`
	UpdatedAt Time             `json:"updatedAt"`
	Odds      []OutrightRunner `json:"odds"`
}

// OutrightOdds is GET /outrights/odds.
type OutrightOdds struct {
	ID         ID                          `json:"id"`
	Name       string                      `json:"name"`
	Date       Time                        `json:"date"`
	Status     string                      `json:"status"`
	Sport      NamedSlug                   `json:"sport"`
	League     NamedSlug                   `json:"league"`
	Bookmakers map[string][]OutrightMarket `json:"bookmakers"`
}

// OutrightResultRow is the final position of one participant.
type OutrightResultRow struct {
	Label         string `json:"label"`
	ParticipantID ID     `json:"participantId"`
	// Position is nil for cut, withdrawn and disqualified.
	Position *int `json:"position"`
	Tied     bool `json:"tied"`
	// MadeCut is nil for events without a cut.
	MadeCut *bool `json:"madeCut"`
	// Status is finished, cut, withdrawn or disqualified.
	Status string `json:"status"`
}

// OutrightResults is GET /outrights/results.
type OutrightResults struct {
	ID      ID                  `json:"id"`
	Name    string              `json:"name"`
	Status  string              `json:"status"`
	Results []OutrightResultRow `json:"results"`
	// Winners lists participant ids per market name.
	Winners map[string][]ID `json:"winners"`
}

// Mapping is a bookmaker's own market and outcome ids for one of our outcomes.
type Mapping struct {
	EventID       ID                         `json:"eventId"`
	Home          string                     `json:"home"`
	Away          string                     `json:"away"`
	Date          Time                       `json:"date"`
	Sport         NamedSlug                  `json:"sport"`
	League        NamedSlug                  `json:"league"`
	Bookmaker     string                     `json:"bookmaker"`
	Market        string                     `json:"market"`
	Label         string                     `json:"label,omitempty"`
	Hdp           *float64                   `json:"hdp,omitempty"`
	Side          string                     `json:"side"`
	NativeEventID string                     `json:"nativeEventId,omitempty"`
	MarketID      string                     `json:"marketId"`
	OutcomeID     string                     `json:"outcomeId"`
	OutcomeLabel  string                     `json:"outcomeLabel,omitempty"`
	Extra         map[string]json.RawMessage `json:"extra,omitempty"`
	Active        bool                       `json:"active"`
	FirstSeenAt   Time                       `json:"firstSeenAt"`
	LastCheckedAt Time                       `json:"lastCheckedAt"`
}

// MappingsPage is GET /mappings. NextCursor is set in bulk sync while more pages remain.
type MappingsPage struct {
	Data       []Mapping `json:"data"`
	NextCursor string    `json:"nextCursor,omitempty"`
}
