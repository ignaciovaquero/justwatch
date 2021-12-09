package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
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
		log.Printf("error when parsing '%s': %s is not a valid boolean value", verboseEnv, getOrElse(verboseEnv, "false"))
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
		log.Fatalf("error when initializing logger: %s", err.Error())
	}

	sugar = zl.Sugar()
	sugar.Debug("logger initialization successful")

	fromDays, err = strconv.Atoi(getOrElse(fromDaysEnv, "1"))
	if err != nil {
		sugar.Fatalf("error when converting days to integer: %s", err.Error())
	}

	chatID, err = strconv.ParseInt(os.Getenv(chatIDEnv), 10, 64)
	if err != nil {
		sugar.Fatalf("error when converting chat ID to integer: %s", err.Error())
	}

	releaseYear, err = strconv.Atoi(getOrElse(releaseYearEnv, defaultReleaseYear))
	if err != nil {
		sugar.Warnw(
			"error when parsing release year. Fallback to default value",
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
			"error when parsing minimum IMDB score. Fallback to default value",
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
			"error when parsing minimum TMDB score. Fallback to default value",
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
		sugar.Fatalf("error when creating new JustWatch client: %s", err.Error())
	}

	telegramClient, err = telegram.NewClient(telegramToken, sugar)
	if err != nil {
		sugar.Fatalf("error when creating the Telegram client: %s", err.Error())
	}
}

func filterContent(contentType string, id int) (*justwatch.Content, error) {
	sugar.Debugw("Getting content", "type", contentType, "id", id)
	content, err := jwClient.GetContentByTypeAndID(contentType, id)
	if err != nil {
		return nil, fmt.Errorf("error getting content for content ID %d: %w", id, err)
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

func getNameAndContentsForProviders(providers []*justwatch.SearchProvider) (map[string][]*justwatch.Content, error) {
	providerIDs := map[int]struct{}{}
	wg := sync.WaitGroup{}
	doneCh := make(chan struct{})
	providerCh := make(chan struct {
		providerName string
		content      *justwatch.Content
	})
	errCh := make(chan error)
	for _, provider := range providers {
		sugar.Debugw("getting name for provider with ID %d", provider.ProviderID)
		if _, ok := providerIDs[provider.ProviderID]; ok {
			continue
		}
		wg.Add(1)
		providerIDs[provider.ProviderID] = struct{}{}
		go func(id int, p chan<- *justwatch.Provider, e chan<- error) {
			providerData, err := jwClient.GetProviderByID(id)
			if err != nil {
				e <- fmt.Errorf("error getting provider with id %d: %w", id, err)
			} else {
				p <- providerData
			}
			wg.Done()
		}(provider.ProviderID, providerCh, errCh)
	}

	go func(d chan<- struct{}) {
		wg.Wait()
		close(d)
	}(doneCh)

	providerNames := map[int]string{}
	done := false
	for !done {
		select {
		case provider := <-providerCh:
			providerNames[provider.ID] = provider.ClearName
		case err := <-errCh:
			return map[int]string{}, err
		case <-doneCh:
			done = true
		}
	}
	return providerNames, nil
}

func getContentForProviders(providers []*justwatch.SearchProvider) (map[int][]*justwatch.Content, error) {
	contents := map[int][]*justwatch.Content{}
	for _, provider := range providers {
		contentCh := make(chan struct {
			providerID int
			content    *justwatch.Content
		})
		errCh := make(chan error)
		wg := sync.WaitGroup{}
		wg.Add(len(provider.Items))
		for _, item := range provider.Items {
			go func(i *justwatch.Item, providerID int, c chan<- struct {
				providerID int
				content    *justwatch.Content
			}, e chan<- error) {
				content, err := filterContent(i.ObjectType, i.ID)
				if err != nil {
					errCh <- err
				} else {
					contentCh <- struct {
						providerID int
						content    *justwatch.Content
					}{providerID, content}
				}
				wg.Done()
			}(item, provider.ProviderID, contentCh, errCh)
		}

		doneCh := make(chan struct{})
		go func(d chan<- struct{}) {
			wg.Wait()
			close(d)
		}(doneCh)

		done := false

		for !done {
			select {
			case content := <-contentCh:
				contents[content.providerID] = append(contents[content.providerID], content.content)
			case err := <-errCh:
				return map[int][]*justwatch.Content{}, err
			case <-doneCh:
				done = true
			}
		}
	}
	return contents, nil
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, event events.CloudWatchEvent) error {
	sugar.Debugw("executing search query", "providers", providers, "content types", contentTypes)
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
			sugar.Debug("getting provider names")
			providerNames, err := getProviderNames(day.Providers)
			if err != nil {
				return fmt.Errorf("error getting provider names: %w", err)
			}
			providerContents, err := getContentForProviders(day.Providers)
			if err != nil {
				return fmt.Errorf("error getting content for providers: %w", err)
			}
			for id, contents := range providerContents {
				for _, content := range contents {
					sugar.Debugw("sending telegram notification", "content", content.Title, "chat", chatID)
					// TODO: get genres
					body := fmt.Sprintf("Título: %s\nDescripción: %s\nAño: %d\nDisponible en: %s\n",
						content.Title,
						content.ShortDescription,
						content.OriginalReleaseYear,
						providerNames[id],
					)
					if err := telegramClient.SendNotification("Nuevo contenido disponible", body, []int64{chatID}); err != nil {
						sugar.Errorf("error when sending Telegram notification: %s", err.Error())
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
