package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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
		Providers:    providers,
		ContentTypes: []string{"movies"},
	})

	if err != nil {
		return err
	}

	for _, day := range response.Days {
		date, err := time.Parse("2006-01-02", day.Date)
		if err != nil {
			return err
		}
		if date.After(time.Now().Add(-24 * time.Hour)) {
			for _, provider := range day.Providers {
				for _, item := range provider.Items {
					fmt.Println(item)
				}
			}
		}
	}
	return nil
}

func main() {
	lambda.Start(Handler)
}
