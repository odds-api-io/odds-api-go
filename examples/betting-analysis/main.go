// Value bets, arbitrage and dropping odds.
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

	valueBets, err := client.GetValueBets(ctx, oddsapi.ValueBetsParams{
		Bookmaker:           "Bet365",
		Sport:               "football",
		IncludeEventDetails: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, bet := range valueBets {
		if bet.Edge() >= 3 {
			fmt.Printf("value: %s %s %.2f%% edge\n", bet.Market.Name, bet.BetSide, bet.Edge())
		}
	}

	arbs, err := client.GetArbitrageBets(ctx, oddsapi.ArbitrageParams{
		Bookmakers: []string{"Bet365", "SingBet"},
		Sport:      "football",
		MinProfit:  1,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, arb := range arbs {
		fmt.Printf("arbitrage: %.2f%% on %s\n", arb.ProfitMargin, arb.Market.Name)
		for _, leg := range arb.Legs {
			fmt.Printf("  %s %s @ %s\n", leg.Bookmaker, leg.Side, leg.Odds)
		}
	}

	drops, err := client.GetDroppingOdds(ctx, oddsapi.DroppingOddsParams{
		Sport:   "football",
		Markets: []string{"ML"},
		MinDrop: 10,
		Sort:    "recent",
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range drops.Data {
		fmt.Printf("drop: %s %s %.2f -> %.2f (-%.1f%%)\n", d.Market.Name, d.BetSide, d.Odds.Opening, d.Odds.Current, d.Odds.Drop.SinceOpening)
	}
}
