# Changelog

## 0.2.2 (2026-11-21)

- `oddsws`: `Stream.Run` no longer stops when a reconnect handshake is refused with HTTP 429 or 5xx, such as the 503 the server sends while it restarts. These are retried with backoff and wait at least as long as `Retry-After` asks (`HandshakeError.RetryAfter`, capped at one hour). A 403 right after the server dropped a live connection (close 1001, 1006, 1012 or a broken transport) is retried up to 3 times. 400, 401, 426 and a 403 on the first connect stay fatal. After `MaxReconnectAttempts` the returned error still wraps the last `*HandshakeError`.

## 0.2.1 (2026-11-20)

- Docs: coverage is now 500+ bookmakers in the package doc and README.

## 0.2.0 (2026-11-19)

- Same game parlays: `GetSgpBookmakers`, `GetSgpLegs` and `QuoteSgp` for `GET /sgp/bookmakers`, `GET /sgp/legs` and `POST /sgp/quote`, with types
- `SgpLegsResponse.Bookmakers`: per requested bookmaker, whether its legs could be checked (`Status`, `Retryable`, `NextAction`, `Reason`, `RetryAfter`)
- `SgpNext*` constants for `NextAction`. `Reason` is a sentence for display; branch on `Status` and `NextAction`

## 0.1.1 (2026-11-13)

- Docs: `GetMarkets`, `GetBookmakers`, `OddsParams.Markets` and the README link to the market reference at https://odds-api.io/markets and bookmaker coverage at https://odds-api.io/sportsbooks#coverage.

## 0.1.0 (2026-11-11)

First release.

- REST client for every public v3 endpoint: sports, leagues, markets, bookmakers and selection, events (live, search), odds (multi, snapshot, updated, movements, summary), value bets, arbitrage, dropping odds, historical events, odds and closing lines, settlements and grading, golf outrights, participants and logos, bookmaker mappings
- Iterators for paginated endpoints and `PollUpdatedOdds`
- `*APIError` with `errors.Is` sentinels, retries with backoff, rate-limit headers
- `oddsws` WebSocket client with permessage-deflate, `lastSeq` resume and replay handling
