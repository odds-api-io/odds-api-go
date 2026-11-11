# Odds-API.io Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/odds-api-io/odds-api-go.svg)](https://pkg.go.dev/github.com/odds-api-io/odds-api-go)
[![CI](https://github.com/odds-api-io/odds-api-go/actions/workflows/ci.yml/badge.svg)](https://github.com/odds-api-io/odds-api-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Official Go SDK for the [**Odds-API.io**](https://odds-api.io) sports betting odds API: real-time and pre-match odds from 450+ bookmakers across 34 sports, over REST and WebSocket.

Prefer another language? See the [Node.js SDK](https://github.com/odds-api-io/odds-api-node), the [Python SDK](https://github.com/odds-api-io/odds-api-python) or the [CLI](https://github.com/odds-api-io/odds-api-cli).

## Quickstart

1. Install (Go 1.22 or newer):

   ```bash
   go get github.com/odds-api-io/odds-api-go
   ```

2. Set your API key ([get one here](https://odds-api.io/pricing?ref=go)):

   ```bash
   export ODDS_API_KEY="your-api-key"
   ```

3. Make your first request:

   ```go
   package main

   import (
   	"context"
   	"fmt"
   	"log"
   	"os"

   	oddsapi "github.com/odds-api-io/odds-api-go"
   )

   func main() {
   	ctx := context.Background()
   	client := oddsapi.NewClient(os.Getenv("ODDS_API_KEY"))

   	events, err := client.GetEvents(ctx, oddsapi.EventsParams{Sport: "football", League: "england-premier-league"})
   	if err != nil {
   		log.Fatal(err)
   	}
   	odds, err := client.GetOdds(ctx, oddsapi.OddsParams{
   		EventID:    events[0].ID.Int64(),
   		Bookmakers: []string{"Bet365", "Unibet"},
   		Markets:    []string{"ML"},
   	})
   	if err != nil {
   		log.Fatal(err)
   	}
   	fmt.Println(odds.Home, "vs", odds.Away, odds.Market("Bet365", "ML"))
   }
   ```

## Get an API key

The odds API needs a key from a paid plan. Pick a plan at [**odds-api.io/pricing**](https://odds-api.io/pricing?ref=go). Rate limits, the number of bookmakers you can select and WebSocket access depend on the plan.

## Sports and bookmakers

| Sport | Example leagues |
|-------|-----------------|
| Football (soccer) | Premier League, La Liga, Champions League |
| Basketball | NBA, EuroLeague |
| American football | NFL, NCAAF |
| Baseball | MLB |
| Ice hockey | NHL |
| Tennis | ATP, WTA |
| Esports | CS2, League of Legends, Dota 2 |
| ...and 27 more | 34 sports in total |

Odds come from **450+ bookmakers**, including Bet365, DraftKings, FanDuel, Betfair, Unibet and SingBet. See the [full coverage list](https://odds-api.io/sportsbooks#coverage).

## Documentation

- [Quickstart: make your first request](https://docs.odds-api.io/quickstart)
- [SDK guide](https://docs.odds-api.io/guides/sdks)
- [Fetching pre-match and live odds](https://docs.odds-api.io/guides/fetching-odds)
- [Stream live odds over WebSocket](https://docs.odds-api.io/guides/websockets)
- [Find value bets](https://docs.odds-api.io/guides/value-bets) and [arbitrage opportunities](https://docs.odds-api.io/guides/arbitrage-bets)
- [Full API reference](https://docs.odds-api.io/api-reference/introduction)
- [Package reference on pkg.go.dev](https://pkg.go.dev/github.com/odds-api-io/odds-api-go)

## Features

- A method for every public v3 endpoint: events, odds, odds summary, movements, value bets, arbitrage, dropping odds, historical odds and closing lines, settlement and grading, golf outrights, participants, bookmaker mappings
- `context.Context` on every call, typed parameter structs and typed responses
- `oddsws`: a WebSocket client with permessage-deflate, reconnects that resume from the last `seq`, and replay handling
- Iterators for paginated endpoints (`IterateEvents`, `IterateClosingLines`, `IterateMappings`, `IterateDroppingOdds`, `IterateHistoricalEvents`) and `PollUpdatedOdds` for the `/odds/updated` delta
- `*APIError` with status, server message, raw body and `RetryAfter`, matched by `errors.Is` sentinels; optional retries with backoff
- Rate-limit headers on every response through `client.LastRateLimit()`
- The API key never appears in error messages
- No dependencies beyond the standard library for REST (`oddsws` uses [gorilla/websocket](https://github.com/gorilla/websocket))

## Usage

```go
ctx := context.Background()
client := oddsapi.NewClient(os.Getenv("ODDS_API_KEY"))

// Exact market names for a sport (no key needed)
markets, err := client.GetMarkets(ctx, "football")

// Upcoming and live Premier League matches
events, err := client.GetEvents(ctx, oddsapi.EventsParams{
	Sport:  "football",
	League: "england-premier-league",
	Status: []string{oddsapi.StatusPending, oddsapi.StatusLive},
})

// Odds for one match. Pass Markets to keep responses small.
odds, err := client.GetOdds(ctx, oddsapi.OddsParams{
	EventID:    events[0].ID.Int64(),
	Bookmakers: []string{"Bet365", "SingBet"},
	Markets:    []string{"ML", "Spread", "Totals"},
})
for bookmaker := range odds.Bookmakers {
	if ml := odds.Market(bookmaker, "ML"); ml != nil {
		fmt.Println(bookmaker, ml.Odds[0].Home, ml.Odds[0].Draw, ml.Odds[0].Away) // 2.10 3.40 3.20
	}
}

// Search, including in-play matches
live, err := client.SearchEvents(ctx, oddsapi.SearchEventsParams{Query: "Liverpool", Status: "live"})
```

Prices are `oddsapi.Decimal` values that keep the exact string the API sent (`"2.10"`); call `.Float()` for a `float64`. Every field of an odds line, including keys such as `1X`, `12` and `X2`, is also available through `line.Price("1X")`.

Market names must match in full: there is no `Match Winner` (use `ML`) and no bare `Over/Under`. Call `GetMarkets(ctx, sport)` to see the names a sport serves.

## Odds comparison and summary

```go
// Best, average, median, worst and fair price per line across bookmakers (Growth plans and above)
summary, err := client.GetOddsSummary(ctx, oddsapi.SummaryParams{
	EventID:          123456,
	Markets:          []string{"ML", "Totals"},
	IncludeExchanges: true,
})
home := summary.Markets[0].Lines[0].Outcomes["home"]
fmt.Println(home.Best, home.BestBookmakers, home.Fair)

// Price history for one market (prematch)
history, err := client.GetOddsMovements(ctx, oddsapi.MovementsParams{
	EventID: 123456, Bookmaker: "Bet365", Market: "Totals", MarketLine: "2.5",
})
fmt.Println(history.Opening.Home, history.Latest.Home)
```

## Polling for changes

`/odds/updated` returns only the market groups that changed. `PollUpdatedOdds` passes each `X-Next-Since` header back as the next `since`:

```go
err := client.PollUpdatedOdds(ctx, oddsapi.UpdatedOddsParams{
	Bookmaker: "Bet365",
	Sport:     "football",
	Markets:   []string{"ML", "Spread", "Totals"}, // max 40
}, 10*time.Second, func(batch *oddsapi.UpdatedOdds) error {
	for _, event := range batch.Data {
		// merge by event, bookmaker and market name; an absent market has not changed
	}
	return nil // or oddsapi.StopIteration
})
```

For live odds the WebSocket is cheaper than any poll interval.

## WebSocket

Package `oddsws` connects to `wss://api.odds-api.io/v3/ws`, tracks the highest `seq` and reconnects with `lastSeq`, so missed updates are replayed. The server requires permessage-deflate compression, which `oddsws` always negotiates. WebSocket access is a plan add-on.

```go
import "github.com/odds-api-io/odds-api-go/oddsws"

params := oddsapi.MultiOddsParams{EventIDs: []int64{123456, 123457}, Bookmakers: []string{"Bet365"}, Markets: []string{"ML", "Totals"}}

// 1. Snapshot with its sequence number
snapshot, err := client.GetOddsSnapshot(ctx, params)

// 2. Stream from that point
stream := oddsws.New(oddsws.Config{
	APIKey:   apiKey,
	Markets:  params.Markets,                       // required for the odds channel, max 40
	Channels: []string{"odds", "scores", "status"}, // optional, default odds only
	EventIDs: params.EventIDs,                      // or Sports / Leagues, Status: "live" | "prematch"
	LastSeq:  snapshot.Seq,
})

err = stream.Run(ctx, func(m *oddsws.Message) error {
	switch m.Type {
	case oddsws.TypeCreated, oddsws.TypeUpdated:
		store[key(m.ID, m.Bookie)] = m.Markets // replace, do not merge
	case oddsws.TypeDeleted, oddsws.TypeNoMarkets:
		delete(store, key(m.ID, m.Bookie))
	case oddsws.TypeScore:
		fmt.Println(m.ID, m.Scores.Home, m.Scores.Away, m.Clock)
	case oddsws.TypeResyncRequired:
		// replay could not be served: rebuild from REST, then continue
		snap, err := client.GetOddsSnapshot(ctx, params)
		if err != nil {
			return err
		}
		stream.SetLastSeq(snap.Seq)
	}
	return nil
})
```

`Run` returns when `ctx` ends, when your handler returns an error, when the handshake is refused (`*oddsws.HandshakeError`: 400, 401, 403, 426) or when a newer connection on the same key replaced this one (`*oddsws.ReplacedError`, close 1008; two connections per key). Close 1012 (server restart) reconnects after a second and 1013 (cooldown) waits at least 3 seconds. `stream.LastSeq()` is the resume point to persist. Options: `MaxReconnectAttempts`, `InitialReconnectDelay`, `MaxReconnectDelay`, `ReadTimeout`, `OnReconnect`, `DisableReplayComplete` and a custom `Dialer`. For a single connection without reconnects, use `oddsws.Dial` and `conn.Read()`.

## Historical odds and closing lines

```go
from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

// Finished events for one league (31 days per call)
past, err := client.GetHistoricalEvents(ctx, oddsapi.HistoricalEventsParams{
	Sport: "football", League: "england-premier-league", From: from, To: from.AddDate(0, 0, 30),
})

// Any range: fetched in 31-day windows
err = client.IterateHistoricalEvents(ctx, oddsapi.HistoricalEventsParams{
	Sport: "football", League: "england-premier-league", From: from.AddDate(0, -8, 0), To: from,
}, func(e oddsapi.Event) error {
	fmt.Println(e.Home, e.Away, e.Scores.Periods["ft"])
	return nil
})

// Odds for one finished event
closing, err := client.GetHistoricalOdds(ctx, oddsapi.OddsParams{
	EventID: past[0].ID.Int64(), Bookmakers: []string{"Bet365"}, Markets: []string{"ML"},
})

// Closing lines for every settled event across up to 10 leagues (paid plans; 366 days per call, 100 per page)
err = client.IterateClosingLines(ctx, oddsapi.ClosingLinesParams{
	Sport:      "football",
	Leagues:    []string{"england-premier-league", "spain-laliga"},
	From:       from,
	To:         from.AddDate(0, 2, 0),
	Markets:    []string{"ML", "Totals"},
	Bookmakers: []string{"Bet365", "SingBet"},
}, func(e oddsapi.EventOdds) error {
	fmt.Println(e.ID, e.Market("Bet365", "ML"))
	return nil
})

// Search finished events by team name
arsenal, err := client.SearchHistoricalEvents(ctx, oddsapi.SearchHistoricalEventsParams{Query: "Arsenal", Sport: "football"})
```

## Settlement and grading

Paid plans. `GetSettlements` returns status, a `Final` flag and final scores; `GradeBets` grades selections against the result. Batches over 50 are split for you.

```go
settled, err := client.GetSettlements(ctx, 74919888, 73101436)
fmt.Println(settled.Settlements, settled.NotFound)

hdp := -0.75
grades, err := client.GradeBets(ctx, []oddsapi.GradeSelection{
	{Ref: "bet-1", EventID: 74919888, Market: "ML", Side: "home"},
	{Ref: "bet-2", EventID: 74919888, Market: "Spread", Hdp: &hdp, Side: "home"},
	{Ref: "bet-3", EventID: 74919888, Market: "Correct Score", Label: "2-1"},
})
for _, g := range grades.Grades {
	fmt.Println(g.Ref, g.Result, g.Reason, g.Final) // won, lost, void, half_won, half_lost, pending, ungradeable
}
```

Re-send selections that are not `Final`, at most once a minute.

## Outrights (golf)

Outrights are a paid add-on: PGA Tour, DP World Tour, LIV Golf and the majors. A key without the add-on gets an `*APIError` matching `ErrForbidden`, whose `IsAddOnRequired()` is true and whose message links to the billing page.

```go
tournaments, err := client.GetOutrights(ctx, oddsapi.OutrightsParams{Sport: "golf", League: "pga-tour"})
odds, err := client.GetOutrightOdds(ctx, tournaments[0].ID.Int64(), "DraftKings", "Betfair Exchange")

for _, market := range odds.Bookmakers["Betfair Exchange"] {
	// Winner, Top 5, Top 5 (Incl. Ties), Top 10, Top 10 (Incl. Ties), Top 20, Top 20 (Incl. Ties), Make Cut
	for _, runner := range market.Odds {
		if runner.IsYesNo() {
			fmt.Println(market.Name, runner.Label, runner.Yes, runner.No) // Make Cut
		} else {
			fmt.Println(market.Name, runner.Label, runner.Odds, runner.LayOdds)
		}
	}
}

results, err := client.GetOutrightResults(ctx, tournaments[0].ID.Int64())
fmt.Println(results.Winners["Top 10 (Incl. Ties)"])
```

"Top N" settles a boundary tie under the bookmaker's dead-heat rules; "Top N (Incl. Ties)" pays it in full. They are separate markets.

## Value bets, arbitrage and dropping odds

```go
valueBets, err := client.GetValueBets(ctx, oddsapi.ValueBetsParams{Bookmaker: "Bet365", Sport: "football", IncludeEventDetails: true})
// ExpectedValue is bookmaker odds / fair odds x 100, so Edge() = ExpectedValue - 100 (103.42 = 3.42% edge)
for _, bet := range valueBets {
	if bet.Edge() >= 3 {
		fmt.Printf("%s %s %.2f%% edge\n", bet.Market.Name, bet.BetSide, bet.Edge())
	}
}

// Price gaps across your bookmakers. Prices move and bookmakers limit stakes, so check every leg before acting.
arbs, err := client.GetArbitrageBets(ctx, oddsapi.ArbitrageParams{
	Bookmakers: []string{"Bet365", "SingBet"}, Sport: "football", Markets: []string{"Totals"}, MinProfit: 1,
})
// ProfitMargin is already a percentage (2.48 = 2.48%)

// Dropping odds from sharp bookmakers (paid plans)
drops, err := client.GetDroppingOdds(ctx, oddsapi.DroppingOddsParams{
	Sport: "football", Markets: []string{"ML"}, MinDrop: 10, Sort: "recent", IncludeEventDetails: true,
})
for _, d := range drops.Data {
	fmt.Printf("%s %s: %.2f -> %.2f (-%.1f%%)\n", d.Market.Name, d.BetSide, d.Odds.Opening, d.Odds.Current, d.Odds.Drop.SinceOpening)
}
```

## Participants, logos and mappings

```go
teams, err := client.GetParticipants(ctx, oddsapi.ParticipantsParams{Sport: "football", Search: "Chelsea"})

// PNG crest. ParticipantLogoURL carries your key, so serve GetParticipantLogo bytes on public pages.
png, err := client.GetParticipantLogo(ctx, events[0].HomeID.Int64())

// A bookmaker's own market and outcome ids for our events (paid plans)
page, err := client.GetMappings(ctx, oddsapi.MappingsParams{Bookmaker: "Polymarket", EventID: 71515842})
err = client.IterateMappings(ctx, oddsapi.MappingsParams{Bookmaker: "Kalshi", Sport: "american-football"}, func(m oddsapi.Mapping) error {
	fmt.Println(m.EventID, m.Market, m.Side, m.MarketID, m.OutcomeID)
	return nil
})
```

## Bookmakers

```go
bookmakers, err := client.GetBookmakers(ctx) // Name, Active, CreatedAt, Status ("ok", "degraded", "down"), Reason
selected, err := client.SelectBookmakers(ctx, "Bet365", "SingBet")
selected, err = client.GetSelectedBookmakers(ctx)
selected, err = client.ClearSelectedBookmakers(ctx) // once every 12 hours
```

## Methods

| Area | Method | Endpoint |
|------|--------|----------|
| Catalogue | `GetSports` | `GET /sports` |
| | `GetMarkets`, `GetAllMarkets` | `GET /markets` |
| | `GetLeagues` | `GET /leagues` |
| | `GetBookmakers` | `GET /bookmakers` |
| | `GetSelectedBookmakers` | `GET /bookmakers/selected` |
| | `SelectBookmakers` | `PUT /bookmakers/selected/select` |
| | `ClearSelectedBookmakers` | `PUT /bookmakers/selected/clear` |
| Events | `GetEvents`, `IterateEvents` | `GET /events` |
| | `GetEvent` | `GET /events/{id}` |
| | `GetLiveEvents` | `GET /events/live` |
| | `SearchEvents` | `GET /events/search` |
| Odds | `GetOdds` | `GET /odds` |
| | `GetOddsMulti`, `GetOddsSnapshot` | `GET /odds/multi` |
| | `GetUpdatedOdds`, `PollUpdatedOdds` | `GET /odds/updated` |
| | `GetOddsMovements` | `GET /odds/movements` |
| | `GetOddsSummary` | `GET /odds/summary` |
| | `GetOddsSummaryMulti` | `GET /odds/summary/multi` |
| Analysis | `GetValueBets` | `GET /value-bets` |
| | `GetArbitrageBets` | `GET /arbitrage-bets` |
| | `GetDroppingOdds`, `IterateDroppingOdds` | `GET /dropping-odds` |
| Historical | `GetHistoricalEvents`, `IterateHistoricalEvents` | `GET /historical/events` |
| | `SearchHistoricalEvents` | `GET /historical/events/search` |
| | `GetHistoricalOdds` | `GET /historical/odds` |
| | `GetClosingLines`, `IterateClosingLines` | `GET /historical/closing-lines` |
| Settlement | `GetSettlements` | `GET /settlements` |
| | `GradeBets` | `POST /settlements/grade` |
| Outrights | `GetOutrights` | `GET /outrights` |
| | `GetOutrightOdds` | `GET /outrights/odds` |
| | `GetOutrightResults` | `GET /outrights/results` |
| Participants | `GetParticipants` | `GET /participants` |
| | `GetParticipant` | `GET /participants/{id}` |
| | `GetParticipantLogo`, `ParticipantLogoURL` | `GET /participants/{id}/logo` |
| Mappings | `GetMappings`, `IterateMappings` | `GET /mappings` |
| WebSocket | `oddsws.New(cfg).Run`, `oddsws.Dial` | `wss://api.odds-api.io/v3/ws` |

Full parameter and response reference: [docs.odds-api.io/api-reference](https://docs.odds-api.io/api-reference/introduction).

## Errors

Every non-2xx response is an `*oddsapi.APIError` with `StatusCode`, the server's own `Message`, the raw `Body`, `RetryAfter` and `RateLimit`. Match the status with `errors.Is`:

| Sentinel | When |
|----------|------|
| `ErrBadRequest` | 400: a parameter is missing or invalid (the message says which) |
| `ErrUnauthorized` | 401 |
| `ErrForbidden` | 403: the plan does not include the endpoint or bookmaker, or an add-on is missing (`IsAddOnRequired()`) |
| `ErrNotFound` | 404: the message often lists what does exist |
| `ErrRateLimited` | 429, with `RetryAfter` |
| `ErrServer` | 5xx |

Network failures and timeouts are returned as the underlying error (`context.DeadlineExceeded`, `*url.Error`), with the API key removed from the message.

```go
_, err := client.GetClosingLines(ctx, params)
var apiErr *oddsapi.APIError
switch {
case errors.Is(err, oddsapi.ErrRateLimited) && errors.As(err, &apiErr):
	fmt.Println("retry in", apiErr.RetryAfter)
case errors.Is(err, oddsapi.ErrForbidden):
	fmt.Println(err)
case errors.As(err, &apiErr):
	fmt.Println(apiErr.StatusCode, apiErr.Message)
}

if rl := client.LastRateLimit(); rl != nil {
	fmt.Println(rl.Limit, rl.Remaining, rl.Reset) // from the last response
}
```

## Configuration

```go
client := oddsapi.NewClient(apiKey,
	oddsapi.WithBaseURL("https://api.odds-api.io/v3"), // default
	oddsapi.WithTimeout(10*time.Second),              // default client timeout
	oddsapi.WithHTTPClient(&http.Client{}),           // bring your own client, transport or proxy
	oddsapi.WithMaxRetries(2),                        // retries on 5xx, network errors and short 429 waits; 0 turns them off
	oddsapi.WithMaxRetryDelay(10*time.Second),        // a 429 asking for a longer wait is returned, not waited out
	oddsapi.WithUserAgent("my-app/1.0"),
)
```

Bookmaker selection changes (PUT) are never retried. The client is safe for concurrent use.

## Examples

See [`examples/`](./examples):

- `basic`: upcoming matches and their odds
- `polling`: keep odds current with `/odds/updated`
- `betting-analysis`: value bets, arbitrage and dropping odds
- `websocket-feed`: REST snapshot plus the WebSocket stream
- `settlement`: bet grading and golf outrights

```bash
ODDS_API_KEY=your-api-key go run ./examples/basic
```

## Plans and Limits

An API key from a paid plan is required. Rate limits, bookmaker selection and WebSocket access depend on your plan; see [**odds-api.io/pricing**](https://odds-api.io/pricing?ref=go).

## Resources

- [API documentation](https://docs.odds-api.io/)
- [Odds-API.io website](https://odds-api.io)
- [Get your API key](https://odds-api.io/pricing?ref=go)
- [Report issues](https://github.com/odds-api-io/odds-api-go/issues)

## License

[MIT](LICENSE) © Odds-API.io

## Disclaimer

This is the official Go SDK for Odds-API.io. This tool is for informational purposes only and should not be used as the sole basis for betting decisions.
