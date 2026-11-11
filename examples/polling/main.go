// Keeps a local copy of Bet365 football odds current by polling /odds/updated.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	oddsapi "github.com/odds-api-io/odds-api-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := oddsapi.NewClient(os.Getenv("ODDS_API_KEY"))
	store := map[string]oddsapi.Market{}

	err := client.PollUpdatedOdds(ctx, oddsapi.UpdatedOddsParams{
		Bookmaker: "Bet365",
		Sport:     "football",
		Markets:   []string{"ML", "Spread", "Totals"},
	}, 10*time.Second, func(batch *oddsapi.UpdatedOdds) error {
		for _, event := range batch.Data {
			for bookmaker, markets := range event.Bookmakers {
				for _, m := range markets {
					store[fmt.Sprintf("%d/%s/%s", event.ID, bookmaker, m.Name)] = m
				}
			}
		}
		fmt.Printf("%d events changed, %d markets held\n", len(batch.Data), len(store))
		return nil
	})
	if err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
