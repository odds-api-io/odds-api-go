package oddsapi

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// StopIteration can be returned from an iteration callback to stop early
// without an error.
var StopIteration = errors.New("oddsapi: stop iteration")

// EventsParams filters GET /events.
type EventsParams struct {
	// Sport slug, required.
	Sport  string
	League string
	// ParticipantID limits to events of one team or player.
	ParticipantID int64
	// Status is any of pending, live, settled, cancelled.
	Status []string
	From   time.Time
	// To defaults to 14 days from now on the server.
	To time.Time
	// Bookmaker limits to events with odds from this bookmaker.
	Bookmaker string
	// Limit caps the response (server max 5000).
	Limit int
	Skip  int
}

func (p EventsParams) query() params {
	return newParams().
		str("sport", p.Sport).
		str("league", p.League).
		int64("participantId", p.ParticipantID).
		list("status", p.Status).
		time("from", p.From).
		time("to", p.To).
		str("bookmaker", p.Bookmaker).
		int("limit", p.Limit).
		int("skip", p.Skip)
}

// GetEvents lists events for a sport.
func (c *Client) GetEvents(ctx context.Context, p EventsParams) ([]Event, error) {
	if p.Sport == "" {
		return nil, errRequired("Sport")
	}
	var out []Event
	_, err := c.getJSON(ctx, "events", p.query().Values, &out)
	return out, err
}

// IterateEvents calls fn for every event matching p, paging with limit and
// skip (pageSize per request, 1000 when 0). Return StopIteration from fn to stop.
func (c *Client) IterateEvents(ctx context.Context, p EventsParams, pageSize int, fn func(Event) error) error {
	if pageSize <= 0 {
		pageSize = 1000
	}
	p.Limit = pageSize
	for {
		events, err := c.GetEvents(ctx, p)
		if err != nil {
			return err
		}
		for _, e := range events {
			if err := fn(e); err != nil {
				return stopOK(err)
			}
		}
		if len(events) < pageSize {
			return nil
		}
		p.Skip += len(events)
	}
}

// GetEvent returns one event by id.
func (c *Client) GetEvent(ctx context.Context, id int64) (*Event, error) {
	var out Event
	if _, err := c.getJSON(ctx, "events/"+pathID(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLiveEvents lists live events, optionally for one sport ("" for all).
func (c *Client) GetLiveEvents(ctx context.Context, sport string) ([]Event, error) {
	var out []Event
	_, err := c.getJSON(ctx, "events/live", newParams().str("sport", sport).Values, &out)
	return out, err
}

// SearchEventsParams is GET /events/search.
type SearchEventsParams struct {
	// Query is a team or league name, at least 3 characters.
	Query string
	// Status is "upcoming" (default) or "live".
	Status string
}

// SearchEvents searches events by team or league name (up to 10 results).
func (c *Client) SearchEvents(ctx context.Context, p SearchEventsParams) ([]Event, error) {
	if p.Query == "" {
		return nil, errRequired("Query")
	}
	var out []Event
	_, err := c.getJSON(ctx, "events/search", newParams().str("query", p.Query).str("status", p.Status).Values, &out)
	return out, err
}

// ParticipantsParams filters GET /participants.
type ParticipantsParams struct {
	// Sport slug, required.
	Sport  string
	Search string
}

// GetParticipants lists teams and players for a sport.
func (c *Client) GetParticipants(ctx context.Context, p ParticipantsParams) ([]Participant, error) {
	if p.Sport == "" {
		return nil, errRequired("Sport")
	}
	var out []Participant
	_, err := c.getJSON(ctx, "participants", newParams().str("sport", p.Sport).str("search", p.Search).Values, &out)
	return out, err
}

// GetParticipant returns one participant by id.
func (c *Client) GetParticipant(ctx context.Context, id int64) (*Participant, error) {
	var out Participant
	if _, err := c.getJSON(ctx, "participants/"+pathID(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetParticipantLogo returns a participant's logo as PNG bytes. A participant
// without a logo gives an error matching ErrNotFound.
func (c *Client) GetParticipantLogo(ctx context.Context, id int64) ([]byte, error) {
	resp, err := c.do(ctx, request{path: "participants/" + pathID(id) + "/logo"})
	if err != nil {
		return nil, err
	}
	return resp.body, nil
}

// ParticipantLogoURL returns the URL of a participant's PNG logo. The URL
// carries your API key: do not hand it to untrusted clients; proxy the bytes
// from GetParticipantLogo instead.
func (c *Client) ParticipantLogoURL(id int64) string {
	return c.buildURL("participants/"+pathID(id)+"/logo", nil, true)
}

func stopOK(err error) error {
	if errors.Is(err, StopIteration) {
		return nil
	}
	return err
}

func requireTimes(from, to time.Time) error {
	if from.IsZero() || to.IsZero() {
		return fmt.Errorf("oddsapi: From and To are required")
	}
	if !to.After(from) {
		return fmt.Errorf("oddsapi: To must be after From")
	}
	return nil
}
