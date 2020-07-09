package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/igvaquero18/go-justwatch"
	"github.com/igvaquero18/telegram-notifier/telegram"
	"go.uber.org/zap"
)

const (
	providersEnv     string = "JUSTWATCH_PROVIDERS"
	contentTypesEnv  string = "JUSTWATCH_CONTENT_TYPES"
	verboseEnv       string = "JUSTWATCH_VERBOSE"
	fromDaysEnv      string = "JUSTWATCH_FROM_DAYS"
	telegramTokenEnv string = "JUSTWATCH_TELEGRAM_TOKEN"
	timeFormat       string = "2006-01-02"
)

var (
	providers      []string = strings.Split(os.Getenv(providersEnv), ",")
	contentTypes   []string = strings.Split(os.Getenv(contentTypesEnv), ",")
	telegramToken  string   = os.Getenv(telegramTokenEnv)
	fromDays       int
	chatID         int64
	jwClient       *justwatch.Client
	telegramClient *telegram.Client
	sugar          *zap.SugaredLogger
	verbose        bool
)

func getOrElse(envVar, defaultValue string) string {
	value := os.Getenv(envVar)
	if value == "" {
		return defaultValue
	}
	return value
}

func init() {
	var err error

	verbose, err = strconv.ParseBool(getOrElse(os.Getenv(verboseEnv), "false"))

	if err != nil {
		verbose = false
	}

	var zl *zap.Logger
	cfg := zap.Config{
		Development: false,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
	if verbose {
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	} else {
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	zl, err = cfg.Build()

	if err != nil {
		log.Fatalf("Error when initializing logger: %s", err.Error())
	}

	sugar = zl.Sugar()
	sugar.Debug("Logger initialization successful")

	fromDays, err = strconv.Atoi(getOrElse(fromDaysEnv, "1"))
	if err != nil {
		log.Fatalf("Error when converting string to integer: %s", err.Error())
	}

	jwClient, err = justwatch.NewClient()
	if err != nil {
		log.Fatalf("Error when creating new JustWatch client: %s", err.Error())
	}

	telegramClient, err = telegram.NewClient(telegramToken, sugar)
	if err != nil {
		sugar.Fatalf("Error when creating the Telegram client: %s", err.Error())
	}
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, event events.CloudWatchEvent) error {
	sugar.Debugw("Executing search query", "providers", providers, "content types", contentTypes)
	response, err := jwClient.SearchNew(&justwatch.SearchQuery{
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
					telegramClient.SendNotification("Nueva peli en Movistar", item.String(), []int64{chatID})
				}
			}
		}
	}
	return nil
}

func main() {
	lambda.Start(Handler)
}
