// Grades bets against final results and lists golf outrights.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	oddsapi "github.com/odds-api-io/odds-api-go"
)

func main() {
	ctx := context.Background()
	client := oddsapi.NewClient(os.Getenv("ODDS_API_KEY"))

	hdp := -0.75
	grades, err := client.GradeBets(ctx, []oddsapi.GradeSelection{
		{Ref: "bet-1", EventID: 74919888, Market: "ML", Side: "home"},
		{Ref: "bet-2", EventID: 74919888, Market: "Spread", Hdp: &hdp, Side: "home"},
		{Ref: "bet-3", EventID: 74919888, Market: "Correct Score", Label: "2-1"},
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, g := range grades.Grades {
		fmt.Println(g.Ref, g.Result, g.Reason, g.Final)
	}

	tournaments, err := client.GetOutrights(ctx, oddsapi.OutrightsParams{Sport: "golf", League: "pga-tour"})
	var apiErr *oddsapi.APIError
	if errors.As(err, &apiErr) && apiErr.IsAddOnRequired() {
		fmt.Println(apiErr.Message)
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	if len(tournaments) == 0 {
		return
	}
	odds, err := client.GetOutrightOdds(ctx, tournaments[0].ID.Int64(), "DraftKings", "Betfair Exchange")
	if err != nil {
		log.Fatal(err)
	}
	for bookmaker, markets := range odds.Bookmakers {
		for _, market := range markets {
			for _, runner := range market.Odds[:min(len(market.Odds), 3)] {
				if runner.IsYesNo() {
					fmt.Println(bookmaker, market.Name, runner.Label, runner.Yes, runner.No)
				} else {
					fmt.Println(bookmaker, market.Name, runner.Label, runner.Odds)
				}
			}
		}
	}
}
