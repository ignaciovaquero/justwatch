package main

import (
	"context"
	"fmt"
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
	providersEnv       string = "JUSTWATCH_PROVIDERS"
	contentTypesEnv    string = "JUSTWATCH_CONTENT_TYPES"
	verboseEnv         string = "JUSTWATCH_VERBOSE"
	fromDaysEnv        string = "JUSTWATCH_FROM_DAYS"
	telegramTokenEnv   string = "JUSTWATCH_TELEGRAM_TOKEN"
	chatIDEnv          string = "JUSTWATCH_CHAT_ID"
	releaseYearEnv     string = "JUSTWATCH_MINIMUM_RELEASE_YEAR"
	minIMDBScoreEnv    string = "JUSTWATCH_MINIMUM_IMDB_SCORE"
	minTMDBScoreEnv    string = "JUSTWATCH_MINIMUM_TMDB_SCORE"
	timeFormat         string = "2006-01-02"
	defaultReleaseYear string = "2010"
	defaultIMDBScore   string = "6.5"
	defaultTMDBScore   string = "6.5"
	imdbScore          string = "imdb:score"
	tmdbScore          string = "tmdb:score"
)

var (
	providers      []string = strings.Split(os.Getenv(providersEnv), ",")
	contentTypes   []string = strings.Split(os.Getenv(contentTypesEnv), ",")
	telegramToken  string   = os.Getenv(telegramTokenEnv)
	fromDays       int
	chatID         int64
	releaseYear    int
	minIMDBScore   float32
	minTMDBScore   float32
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

	verbose, err = strconv.ParseBool(getOrElse(verboseEnv, "false"))

	if err != nil {
		log.Printf("Error when parsing '%s': %s is not a valid boolean value", verboseEnv, getOrElse(verboseEnv, "false"))
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
		sugar.Fatalf("Error when converting days to integer: %s", err.Error())
	}

	chatID, err = strconv.ParseInt(os.Getenv(chatIDEnv), 10, 64)
	if err != nil {
		sugar.Fatalf("Error when converting chat ID to integer: %s", err.Error())
	}

	releaseYear, err = strconv.Atoi(getOrElse(releaseYearEnv, defaultReleaseYear))
	if err != nil {
		sugar.Warnw(
			"Error when parsing release year. Fallback to default value",
			"year",
			os.Getenv(releaseYearEnv),
			"default",
			defaultReleaseYear,
		)
		releaseYear, _ = strconv.Atoi(defaultReleaseYear)
	}

	minScore, err := strconv.ParseFloat(getOrElse(minIMDBScoreEnv, defaultIMDBScore), 32)
	if err != nil {
		sugar.Warnw(
			"Error when parsing minimum IMDB score. Fallback to default value",
			"score",
			os.Getenv(minIMDBScoreEnv),
			"default",
			defaultIMDBScore,
		)
		minScore, _ = strconv.ParseFloat(defaultIMDBScore, 32)
	}
	minIMDBScore = float32(minScore)

	minScore, err = strconv.ParseFloat(getOrElse(minTMDBScoreEnv, defaultTMDBScore), 32)
	if err != nil {
		sugar.Warnw(
			"Error when parsing minimum TMDB score. Fallback to default value",
			"score",
			os.Getenv(minTMDBScoreEnv),
			"default",
			defaultTMDBScore,
		)
		minScore, _ = strconv.ParseFloat(defaultTMDBScore, 32)
	}
	minTMDBScore = float32(minScore)

	jwClient, err = justwatch.NewClient(justwatch.SetLogger(sugar))
	if err != nil {
		sugar.Fatalf("Error when creating new JustWatch client: %s", err.Error())
	}

	telegramClient, err = telegram.NewClient(telegramToken, sugar)
	if err != nil {
		sugar.Fatalf("Error when creating the Telegram client: %s", err.Error())
	}
}

func filterContent(contentType string, id int) (*justwatch.Content, error) {
	sugar.Debugw("Getting content", "type", contentType, "id", id)
	content, err := jwClient.GetContentByTypeAndID(contentType, id)
	if err != nil {
		return nil, err
	}
	if content.OriginalReleaseYear < releaseYear {
		return nil, nil
	}
	for _, scoring := range content.Scoring {
		if scoring.ProviderType == imdbScore && scoring.Value < minIMDBScore {
			return nil, nil
		}
		if scoring.ProviderType == tmdbScore && scoring.Value < minTMDBScore {
			return nil, nil
		}
	}
	return content, nil
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
				providerName := "unknown"
				providerData, _ := jwClient.GetProviderByID(provider.ProviderID)
				providerName = providerData.ClearName
				for _, item := range provider.Items {
					content, err := filterContent(item.ObjectType, item.ID)
					if err != nil {
						sugar.Errorw("Error when getting content", "type", item.ObjectType, "id", item.ID, "msg", err.Error())
						continue
					}
					if content == nil {
						continue
					}
					genres := []string{}
					for genreID := range content.GenreIDs {
						genre, err := jwClient.GetGenreByID(genreID)
						if err != nil {
							continue
						}
						genres = append(genres, genre.TechnicalName)
					}
					if len(genres) == 0 {
						genres = []string{"N/A"}
					}
					sugar.Debugw("Sending telegram notification", "content", content.Title, "chat", chatID)
					body := fmt.Sprintf(
						"Título: %s\nDescripción: %s\nAño: %d\nGéneros: %s\nDisponible en: %s\n",
						content.Title,
						content.ShortDescription,
						content.OriginalReleaseYear,
						strings.Join(genres, ","),
						providerName,
					)
					if err := telegramClient.SendNotification("Nuevo contenido disponible", body, []int64{chatID}); err != nil {
						sugar.Errorf("Error when sending Telegram notification: %s", err.Error())
					}
				}
			}
		}
	}
	return nil
}

func main() {
	lambda.Start(Handler)
}
