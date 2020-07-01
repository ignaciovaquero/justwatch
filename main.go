package main

import (
	"fmt"

	"github.com/igvaquero18/go-justwatch"
)

// Handler is our lambda handler invoked by the `lambda.Start` function call
func main() {
	client, err := justwatch.NewClient()
	if err != nil {
		panic(err)
	}

	response, err := client.SearchNew(&justwatch.SearchQuery{
		Providers:    []string{"mvs"},
		ContentTypes: []string{"movies"},
	})

	if err != nil {
		panic(err)
	}

	for _, day := range response.Days {
		fmt.Println(day.Date)
	}
}
