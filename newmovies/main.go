package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/igvaquero18/go-justwatch"
)

const timeFormat string = "2006-01-02"

func getOrElse(envVar, defaultValue string) string {
	value := os.Getenv(envVar)
	if value == "" {
		return defaultValue
	}
	return value
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, event events.CloudWatchEvent) error {
	providers := strings.Split(os.Getenv("JUSTWATCH_PROVIDERS"), ",")
	contentTypes := strings.Split(os.Getenv("JUSTWATCH_CONTENT_TYPES"), ",")
	fromDays, err := strconv.Atoi(getOrElse("JUSTWATCH_FROM_DAYS", "1"))

	if err != nil {
		return err
	}

	client, err := justwatch.NewClient()
	if err != nil {
		return err
	}

	response, err := client.SearchNew(&justwatch.SearchQuery{
		Providers:    providers,
		ContentTypes: contentTypes,
	})

	if err != nil {
		return err
	}

	for _, day := range response.Days {
		date, err := time.Parse(timeFormat, day.Date)
		if err != nil {
			return err
		}
		if date.After(time.Now().Add(-24 * time.Duration(fromDays) * time.Hour)) {
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
