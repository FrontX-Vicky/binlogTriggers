package main

// Redis Pub/Sub subscriber with filters
import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"mysql_changelog_publisher/internal/subscriber"
)

func main() {
	cfg, err := loadConfigFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := subscriber.Run(ctx, cfg); err != nil && err != context.Canceled {
			log.Printf("subscriber exited: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		log.Println("shutting down…")
		cancel()
	case <-ctx.Done():
	}
}

func loadConfigFromEnv() (*subscriber.Config, error) {
	customEnv := os.Getenv("ENV_FILE")
	if strings.TrimSpace(customEnv) == "" {
		return subscriber.LoadConfigFromLookup(os.LookupEnv), nil
	}
	files := subscriber.ParseEnvFilesList(customEnv)
	if len(files) == 0 {
		return subscriber.LoadConfigFromLookup(os.LookupEnv), nil
	}
	vals, err := subscriber.LoadEnvFiles(files)
	if err != nil {
		return nil, err
	}
	return subscriber.LoadConfigFromMap(vals), nil
}
