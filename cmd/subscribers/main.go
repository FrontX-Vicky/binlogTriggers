package main

// Multi-subscriber runner with API calling support
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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"mysql_changelog_publisher/internal/event"
	"mysql_changelog_publisher/internal/subscriber"

	"github.com/redis/go-redis/v9"
)

type debouncer struct {
	mu       sync.Mutex
	pending  map[string]*pendingEvent
	duration time.Duration
}

type pendingEvent struct {
	event     *event.RowEvent
	timer     *time.Timer
	apiURL    string
	apiLogger *log.Logger
	logPrefix string
}

func main() {
	files := envFilesList()
	if len(files) == 0 {
		log.Fatalf("ENV_FILES (or ENV_FILE) is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for _, f := range files {
		cfg, apiURL, apiLogger, logFile, debounceSeconds, err := loadSubscriberConfig(f)
		if err != nil {
			log.Fatalf("load config %s: %v", f, err)
		}
		if strings.TrimSpace(cfg.Name) == "" {
			cfg.Name = strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		}

		wg.Add(1)
		go func(c *subscriber.Config, url string, logger *log.Logger, lf *os.File, debounce int) {
			defer wg.Done()
			if lf != nil {
				defer lf.Close()
			}
			if url != "" {
				if err := runWithAPI(ctx, c, url, logger, debounce); err != nil && err != context.Canceled {
					log.Printf("subscriber %s exited: %v", c.Name, err)
				}
			} else {
				if err := subscriber.Run(ctx, c); err != nil && err != context.Canceled {
					log.Printf("subscriber %s exited: %v", c.Name, err)
				}
			}
		}(cfg, apiURL, apiLogger, logFile, debounceSeconds)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		log.Println("shutting down…")
		cancel()
	case <-ctx.Done():
	}

	wg.Wait()
}

func envFilesList() []string {
	files := os.Getenv("ENV_FILES")
	if strings.TrimSpace(files) == "" {
		files = os.Getenv("ENV_FILE")
	}
	if strings.TrimSpace(files) != "" {
		return subscriber.ParseEnvFilesList(files)
	}

	// Auto-discover .env.* files
	discovered, err := discoverEnvFiles()
	if err != nil {
		log.Printf("auto-discover env files: %v", err)
		return nil
	}
	if len(discovered) > 0 {
		log.Printf("auto-discovered %d subscriber config(s): %v", len(discovered), discovered)
	}
	return discovered
}

func discoverEnvFiles() ([]string, error) {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var envFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Match .env.* but exclude base .env file
		if strings.HasPrefix(name, ".env.") && name != ".env" {
			envFiles = append(envFiles, name)
		}
	}
	return envFiles, nil
}

func loadConfigFromFile(path string) (*subscriber.Config, error) {
	vals, err := subscriber.LoadEnvFiles([]string{path})
	if err != nil {
		return nil, err
	}
	return subscriber.LoadConfigFromMap(vals), nil
}

func loadSubscriberConfig(path string) (*subscriber.Config, string, *log.Logger, *os.File, int, error) {
	vals, err := subscriber.LoadEnvFiles([]string{path})
	if err != nil {
		return nil, "", nil, nil, 0, err
	}

	cfg := subscriber.LoadConfigFromMap(vals)
	apiURL := vals["API_URL"]

	debounceSeconds := 0
	if d := vals["DEBOUNCE_SECONDS"]; d != "" {
		debounceSeconds, _ = strconv.Atoi(d)
	}

	var apiLogger *log.Logger
	var logFileHandle *os.File
	if strings.TrimSpace(apiURL) != "" {
		// Setup API logger for this subscriber
		logDir := filepath.Join("logs", cfg.Name)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, "", nil, nil, 0, err
		}

		logFilePath := filepath.Join(logDir, "api_calls.log")
		logFileHandle, err = os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, "", nil, nil, 0, err
		}

		apiLogger = log.New(io.MultiWriter(os.Stdout, logFileHandle), fmt.Sprintf("[%s-API] ", cfg.Name), log.LstdFlags)
		log.Printf("API logs for %s: %s", cfg.Name, logFilePath)
		if debounceSeconds > 0 {
			log.Printf("Debouncing enabled for %s: %d seconds", cfg.Name, debounceSeconds)
		}
	}

	return cfg, apiURL, apiLogger, logFileHandle, debounceSeconds, nil
}

func runWithAPI(ctx context.Context, cfg *subscriber.Config, apiURL string, apiLogger *log.Logger, debounceSeconds int) error {
	logPrefix := fmt.Sprintf("[%s] ", cfg.Name)
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

	var deb *debouncer
	if debounceSeconds > 0 {
		deb = &debouncer{
			pending:  make(map[string]*pendingEvent),
			duration: time.Duration(debounceSeconds) * time.Second,
		}
	}

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
			if deb != nil {
				if err := handleEventWithDebounce(msg.Payload, filter, apiURL, apiLogger, logPrefix, deb); err != nil {
					log.Printf("%shandler error: %v", logPrefix, err)
				}
			} else {
				if err := handleEventWithAPI(msg.Payload, filter, apiURL, apiLogger, logPrefix); err != nil {
					log.Printf("%shandler error: %v", logPrefix, err)
				}
			}
		}
	}
}

func handleEventWithDebounce(raw string, filter *subscriber.Filter, apiURL string, apiLogger *log.Logger, logPrefix string, deb *debouncer) error {
	var ev event.RowEvent
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&ev); err != nil {
		return fmt.Errorf("json decode: %w", err)
	}

	if !filter.Matches(&ev) {
		return nil
	}

	log.Printf("%sevent matched (debouncing): op=%s table=%s key=%v", logPrefix, ev.Op, ev.Table, ev.RowKey)

	// Create unique key for this row
	rowID := fmt.Sprintf("%s:%s:%v", ev.DB, ev.Table, ev.RowKey)

	deb.mu.Lock()
	if existing, ok := deb.pending[rowID]; ok {
		// Cancel existing timer and update event
		existing.timer.Stop()
		existing.event = &ev
		existing.timer = time.AfterFunc(deb.duration, func() {
			callDebouncedAPI(rowID, deb)
		})
		deb.mu.Unlock()
		return nil
	}

	// New event - schedule API call
	deb.pending[rowID] = &pendingEvent{
		event:     &ev,
		apiURL:    apiURL,
		apiLogger: apiLogger,
		logPrefix: logPrefix,
		timer: time.AfterFunc(deb.duration, func() {
			callDebouncedAPI(rowID, deb)
		}),
	}
	deb.mu.Unlock()
	return nil
}

func callDebouncedAPI(rowID string, deb *debouncer) {
	deb.mu.Lock()
	pe, ok := deb.pending[rowID]
	if !ok {
		deb.mu.Unlock()
		return
	}
	delete(deb.pending, rowID)
	deb.mu.Unlock()

	if err := callAPI(pe.apiURL, pe.event, pe.apiLogger); err != nil {
		pe.apiLogger.Printf("FAILED | URL=%s | Error=%v | Event: op=%s table=%s key=%v", pe.apiURL, err, pe.event.Op, pe.event.Table, pe.event.RowKey)
		log.Printf("%sAPI call failed: %v", pe.logPrefix, err)
	} else {
		log.Printf("%sAPI called successfully (debounced)", pe.logPrefix)
	}
}

func handleEventWithAPI(raw string, filter *subscriber.Filter, apiURL string, apiLogger *log.Logger, logPrefix string) error {
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
	if err := callAPI(apiURL, &ev, apiLogger); err != nil {
		apiLogger.Printf("FAILED | URL=%s | Error=%v | Event: op=%s table=%s key=%v", apiURL, err, ev.Op, ev.Table, ev.RowKey)
		return fmt.Errorf("api call: %w", err)
	}

	log.Printf("%sAPI called successfully", logPrefix)
	return nil
}

func callAPI(url string, ev *event.RowEvent, apiLogger *log.Logger) error {
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
