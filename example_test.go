package restapiclient_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/tekkamanendless/restapiclient"
)

// ExampleClient demonstrates how to use the Client to make a simple request.
func ExampleClient() {
	ctx := context.Background()

	client := restapiclient.New()

	var output restapiclient.RawBytes
	err := client.Do(ctx, http.MethodGet, "https://www.google.com", nil, &output)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Println(string(output))
}

// ExampleAPI demonstrates how to use the client to make a REST API request.
//
// This example uses the Free Currency Exchange Rates API from this GitHub project:
// https://github.com/fawazahmed0/exchange-api
func ExampleClient_restAPI() {
	ctx := context.Background()

	client := restapiclient.New(
		restapiclient.OptionBaseURL("https://cdn.jsdelivr.net/npm/@fawazahmed0"),
	)

	date := "2024-03-06"

	var output struct {
		Date string             `json:"date"`
		Euro map[string]float64 `json:"eur"`
	}
	err := client.Do(ctx, http.MethodGet, "/currency-api@"+url.PathEscape(date)+"/v1/currencies/eur.json", nil, &output)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("On %s, 1 EUR was worth %f USD.\n", output.Date, output.Euro["usd"])
	// Output: On 2024-03-06, 1 EUR was worth 1.084987 USD.
}
