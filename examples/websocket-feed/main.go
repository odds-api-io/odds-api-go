// REST snapshot plus the live WebSocket feed, resuming from the snapshot's seq.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	oddsapi "github.com/odds-api-io/odds-api-go"
	"github.com/odds-api-io/odds-api-go/oddsws"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	apiKey := os.Getenv("ODDS_API_KEY")
	client := oddsapi.NewClient(apiKey)

	live, err := client.GetLiveEvents(ctx, "football")
	if err != nil {
		log.Fatal(err)
	}
	if len(live) == 0 {
		fmt.Println("no live football right now")
		return
	}
	var ids []int64
	for _, e := range live[:min(len(live), 10)] {
		ids = append(ids, e.ID.Int64())
	}

	params := oddsapi.MultiOddsParams{EventIDs: ids, Bookmakers: []string{"Bet365"}, Markets: []string{"ML", "Totals"}}
	snapshot, err := client.GetOddsSnapshot(ctx, params)
	if err != nil {
		log.Fatal(err)
	}
	store := map[string][]oddsapi.Market{}
	for _, event := range snapshot.Data {
		for bookmaker, markets := range event.Bookmakers {
			store[fmt.Sprintf("%d/%s", event.ID, bookmaker)] = markets
		}
	}

	stream := oddsws.New(oddsws.Config{
		APIKey:   apiKey,
		Markets:  params.Markets,
		Channels: []string{"odds", "scores", "status"},
		EventIDs: ids,
		LastSeq:  snapshot.Seq,
		OnReconnect: func(attempt int, delay time.Duration, lastSeq int64, cause error) {
			log.Printf("reconnect #%d in %s from seq %d: %v", attempt, delay, lastSeq, cause)
		},
	})

	err = stream.Run(ctx, func(m *oddsws.Message) error {
		key := fmt.Sprintf("%d/%s", m.ID, m.Bookie)
		switch m.Type {
		case oddsws.TypeCreated, oddsws.TypeUpdated:
			store[key] = m.Markets // replace, do not merge
		case oddsws.TypeDeleted, oddsws.TypeNoMarkets:
			delete(store, key)
		case oddsws.TypeScore:
			fmt.Printf("%d score %v-%v\n", m.ID, m.Scores.Home, m.Scores.Away)
		case oddsws.TypeResyncRequired:
			snap, err := client.GetOddsSnapshot(ctx, params)
			if err != nil {
				return err
			}
			stream.SetLastSeq(snap.Seq)
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
