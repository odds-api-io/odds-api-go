// Upcoming Premier League matches and their odds.
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

	events, err := client.GetEvents(ctx, oddsapi.EventsParams{
		Sport:  "football",
		League: "england-premier-league",
		Status: []string{oddsapi.StatusPending},
		Limit:  5,
	})
	if err != nil {
		log.Fatal(err)
	}

	for _, event := range events {
		odds, err := client.GetOdds(ctx, oddsapi.OddsParams{
			EventID:    event.ID.Int64(),
			Bookmakers: []string{"Bet365", "SingBet"},
			Markets:    []string{"ML", "Totals"},
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s vs %s, %s\n", event.Home, event.Away, event.Date.Format("Mon 2 Jan 15:04"))
		for bookmaker := range odds.Bookmakers {
			if ml := odds.Market(bookmaker, "ML"); ml != nil && len(ml.Odds) > 0 {
				line := ml.Odds[0]
				fmt.Printf("  %-8s home %s  draw %s  away %s\n", bookmaker, line.Home, line.Draw, line.Away)
			}
		}
	}

	if rl := client.LastRateLimit(); rl != nil {
		fmt.Printf("%d of %d requests left this hour\n", rl.Remaining, rl.Limit)
	}
}
