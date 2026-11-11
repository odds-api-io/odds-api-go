# Changelog

## 0.1.0 (2026-11-11)

First release.

- REST client for every public v3 endpoint: sports, leagues, markets, bookmakers and selection, events (live, search), odds (multi, snapshot, updated, movements, summary), value bets, arbitrage, dropping odds, historical events, odds and closing lines, settlements and grading, golf outrights, participants and logos, bookmaker mappings
- Iterators for paginated endpoints and `PollUpdatedOdds`
- `*APIError` with `errors.Is` sentinels, retries with backoff, rate-limit headers
- `oddsws` WebSocket client with permessage-deflate, `lastSeq` resume and replay handling
