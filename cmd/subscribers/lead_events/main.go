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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mysql_changelog_publisher/internal/event"
	"mysql_changelog_publisher/internal/subscriber"

	"github.com/redis/go-redis/v9"
)

var apiLogger *log.Logger

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	apiURL := os.Getenv("API_URL")
	if strings.TrimSpace(apiURL) == "" {
		log.Fatalf("API_URL is required")
	}

	// Setup API log file
	if err := setupAPILogger(); err != nil {
		log.Fatalf("setup api logger: %v", err)
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

func setupAPILogger() error {
	logDir := filepath.Join("cmd", "subscribers", "lead_events")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logFile := filepath.Join(logDir, "api_calls.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	apiLogger = log.New(io.MultiWriter(os.Stdout, f), "[API] ", log.LstdFlags)
	log.Printf("API call logs will be written to: %s", logFile)
	return nil
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
		apiLogger.Printf("FAILED | URL=%s | Error=%v | Event: op=%s table=%s key=%v", apiURL, err, ev.Op, ev.Table, ev.RowKey)
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

	startTime := time.Now()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		apiLogger.Printf("ERROR | Status=%d | Duration=%v | URL=%s | Response=%s | Payload=%s",
			resp.StatusCode, duration, url, string(body), string(payload))
		return fmt.Errorf("api returned %d: %s", resp.StatusCode, string(body))
	}

	apiLogger.Printf("SUCCESS | Status=%d | Duration=%v | URL=%s | Response=%s | Event: op=%s table=%s key=%v",
		resp.StatusCode, duration, url, string(body), ev.Op, ev.Table, ev.RowKey)

	return nil
}
