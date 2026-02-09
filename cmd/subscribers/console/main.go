package main

// Redis Pub/Sub subscriber with filters
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mysql_changelog_publisher/internal/event"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisAddr    = "127.0.0.1:6379"
	defaultRedisChannel = "binlog:all"
)

// ===================== CONFIG =====================

type Config struct {
	RedisAddr    string
	RedisPass    string
	RedisDB      int
	RedisChannel string
	PrettyPrint  bool

	FilterDBs       []string
	FilterTables    []string
	FilterIDs       []string
	FilterOps       []string
	FilterChangeAny []string
	FilterChangeAll []string
}

func loadConfig() *Config {
	cfg := &Config{
		RedisAddr:       envDefault("REDIS_ADDR", defaultRedisAddr),
		RedisPass:       os.Getenv("REDIS_PASS"),
		RedisChannel:    envDefault("REDIS_CHANNEL", ""),
		PrettyPrint:     envBool("PRETTY_PRINT", false),
		FilterDBs:       parseCSV(os.Getenv("FILTER_DBS")),
		FilterTables:    parseCSV(os.Getenv("FILTER_TABLES")),
		FilterIDs:       parseCSV(os.Getenv("FILTER_IDS")),
		FilterOps:       parseCSV(os.Getenv("FILTER_OPS")),
		FilterChangeAny: parseCSV(os.Getenv("FILTER_CHANGE_ANY")),
		FilterChangeAll: parseCSV(os.Getenv("FILTER_CHANGE_ALL")),
	}

	if cfg.RedisChannel == "" {
		cfg.RedisChannel = envDefault("REDIS_STREAM", "")
	}
	if cfg.RedisChannel == "" {
		cfg.RedisChannel = defaultRedisChannel
	}

	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		if v, err := strconv.Atoi(dbStr); err == nil {
			cfg.RedisDB = v
		}
	}
	return cfg
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func parseCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return []string{}
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ===================== FILTER UTILS =====================

type strset map[string]struct{}

func toSet(list []string, normalizeLower bool) strset {
	if len(list) == 0 {
		return nil
	}
	out := make(strset, len(list))
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if normalizeLower {
			v = strings.ToLower(v)
		}
		out[v] = struct{}{}
	}
	return out
}

func inSet(s strset, v string) bool {
	if s == nil {
		return true // no filter
	}
	_, ok := s[v]
	return ok
}

func rowKeyToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

func hasAnyColumns(changes []event.ColumnChange, cols strset) bool {
	if cols == nil {
		return true
	}
	for _, c := range changes {
		if _, ok := cols[c.Column]; ok {
			return true
		}
	}
	return false
}

func hasAllColumns(changes []event.ColumnChange, cols strset) bool {
	if cols == nil || len(cols) == 0 {
		return true
	}
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Column] = true
	}
	for col := range cols {
		if !seen[col] {
			return false
		}
	}
	return true
}

// ===================== MAIN =====================

func main() {
	loadEnv()

	cfg := loadConfig()
	log.Printf("subscriber start | redis=%s db=%d channel=%s", cfg.RedisAddr, cfg.RedisDB, cfg.RedisChannel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDB,
	})
	defer client.Close()

	pubsub := client.Subscribe(ctx, cfg.RedisChannel)
	defer pubsub.Close()

	// Build filter sets
	dbSet := toSet(cfg.FilterDBs, false)
	tableSet := toSet(cfg.FilterTables, false)
	idSet := toSet(cfg.FilterIDs, false)
	opSet := toSet(cfg.FilterOps, true)
	changeAny := toSet(cfg.FilterChangeAny, false)
	changeAll := toSet(cfg.FilterChangeAll, false)

	handle := func(raw string) error {
		var ev event.RowEvent
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&ev); err != nil {
			return fmt.Errorf("json decode: %w", err)
		}

		if !inSet(dbSet, ev.DB) {
			return nil
		}
		if !inSet(tableSet, ev.Table) {
			return nil
		}
		if !inSet(idSet, rowKeyToString(ev.RowKey)) {
			return nil
		}
		if !inSet(opSet, strings.ToLower(ev.Op)) {
			return nil
		}
		if !hasAnyColumns(ev.Changes, changeAny) {
			return nil
		}
		if !hasAllColumns(ev.Changes, changeAll) {
			return nil
		}

		if cfg.PrettyPrint {
			var obj map[string]interface{}
			_ = json.Unmarshal([]byte(raw), &obj)
			b, _ := json.MarshalIndent(obj, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println(raw)
		}
		return nil
	}

	msgs := pubsub.Channel(redis.WithChannelHealthCheckInterval(10 * time.Second))

	go func() {
		for msg := range msgs {
			if msg == nil || msg.Payload == "" {
				continue
			}
			if err := handle(msg.Payload); err != nil {
				log.Printf("handler error: %v", err)
			}
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

	if err := pubsub.Close(); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("pubsub close error: %v", err)
	}
	_ = client.Close()
	time.Sleep(250 * time.Millisecond)
}

func loadEnv() {
	customEnv := strings.TrimSpace(os.Getenv("ENV_FILE=.env.console"))
	if customEnv == "" {
		if err := godotenv.Load(); err != nil {
			log.Printf("No .env file found: %v", err)
		}
		return
	}
	files := make([]string, 0)
	for _, f := range strings.Split(customEnv, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		if err := godotenv.Load(); err != nil {
			log.Printf("No .env file found: %v", err)
		}
		return
	}
	if err := godotenv.Overload(files...); err != nil {
		log.Printf("Failed to load ENV_FILE (%s): %v", customEnv, err)
	}
}
