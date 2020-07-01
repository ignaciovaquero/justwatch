package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/igvaquero18/go-justwatch"
)

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, event events.CloudWatchEvent) error {
	providers := strings.Split(os.Getenv("JUSTWATCH_PROVIDERS"), ",")

	client, err := justwatch.NewClient()
	if err != nil {
		return err
	}

	response, err := client.SearchNew(&justwatch.SearchQuery{
		Providers: providers,
		ContentTypes: []string{"movies"},
	})

	if err != nil {
		return err
	}

	for _, day := range response.Days {
		if day.Date
	}
}

func main() {
	lambda.Start(Handler)
}
