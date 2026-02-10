package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mysql_changelog_publisher/internal/event"
	"mysql_changelog_publisher/internal/subscriber"

	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	apiURL := os.Getenv("API_URL")
	if strings.TrimSpace(apiURL) == "" {
		log.Fatalf("API_URL is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := runWithAPIHandler(ctx, cfg, apiURL); err != nil && err != context.Canceled {
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

func loadConfig() (*subscriber.Config, error) {
	customEnv := os.Getenv("ENV_FILE")
	if strings.TrimSpace(customEnv) == "" {
		customEnv = ".env.lead_events"
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

func runWithAPIHandler(ctx context.Context, cfg *subscriber.Config, apiURL string) error {
	logPrefix := "[lead_events] "
	log.Printf("%ssubscriber start | redis=%s db=%d channel=%s | api=%s", logPrefix, cfg.RedisAddr, cfg.RedisDB, cfg.RedisChannel, apiURL)

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDB,
	})
	defer client.Close()

	pubsub := client.Subscribe(ctx, cfg.RedisChannel)
	defer pubsub.Close()

	filter := subscriber.NewFilter(cfg)

	msgs := pubsub.Channel(redis.WithChannelHealthCheckInterval(10 * time.Second))
	for {
		select {
		case <-ctx.Done():
			_ = pubsub.Close()
			_ = client.Close()
			return ctx.Err()
		case msg := <-msgs:
			if msg == nil || msg.Payload == "" {
				continue
			}
			if err := handleEvent(msg.Payload, filter, apiURL, logPrefix); err != nil {
				log.Printf("%shandler error: %v", logPrefix, err)
			}
		}
	}
}

func handleEvent(raw string, filter *subscriber.Filter, apiURL string, logPrefix string) error {
	var ev event.RowEvent
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&ev); err != nil {
		return fmt.Errorf("json decode: %w", err)
	}

	if !filter.Matches(&ev) {
		return nil
	}

	log.Printf("%sevent matched: op=%s table=%s key=%v", logPrefix, ev.Op, ev.Table, ev.RowKey)

	// Call API
	if err := callAPI(apiURL, &ev); err != nil {
		return fmt.Errorf("api call: %w", err)
	}

	log.Printf("%sAPI called successfully", logPrefix)
	return nil
}

func callAPI(url string, ev *event.RowEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
