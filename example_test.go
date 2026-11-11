package oddsapi_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	oddsapi "github.com/odds-api-io/odds-api-go"
)

func ExampleNewClient() {
	client := oddsapi.NewClient(os.Getenv("ODDS_API_KEY"),
		oddsapi.WithTimeout(15*time.Second),
		oddsapi.WithMaxRetries(3),
	)
	_ = client
}

func ExampleClient_GetOdds() {
	ctx := context.Background()
	client := oddsapi.NewClient(os.Getenv("ODDS_API_KEY"))

	odds, err := client.GetOdds(ctx, oddsapi.OddsParams{
		EventID:    62924717,
		Bookmakers: []string{"Bet365", "SingBet"},
		Markets:    []string{"ML", "Totals"},
	})
	if err != nil {
		log.Fatal(err)
	}
	if ml := odds.Market("Bet365", "ML"); ml != nil {
		fmt.Println(ml.Odds[0].Home, ml.Odds[0].Draw, ml.Odds[0].Away)
	}
}

func ExampleAPIError() {
	client := oddsapi.NewClient(os.Getenv("ODDS_API_KEY"))
	_, err := client.GetEvent(context.Background(), 1)

	var apiErr *oddsapi.APIError
	switch {
	case errors.Is(err, oddsapi.ErrRateLimited) && errors.As(err, &apiErr):
		fmt.Println("retry in", apiErr.RetryAfter)
	case errors.Is(err, oddsapi.ErrNotFound):
		fmt.Println("no such event")
	case errors.As(err, &apiErr):
		fmt.Println(apiErr.StatusCode, apiErr.Message)
	}
}
