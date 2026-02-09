package subscriber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"mysql_changelog_publisher/internal/event"

	"github.com/redis/go-redis/v9"
)

func Run(ctx context.Context, cfg *Config) error {
	logPrefix := ""
	if strings.TrimSpace(cfg.Name) != "" {
		logPrefix = "[" + cfg.Name + "] "
	}
	log.Printf("%ssubscriber start | redis=%s db=%d channel=%s", logPrefix, cfg.RedisAddr, cfg.RedisDB, cfg.RedisChannel)

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
			fmt.Printf("%s%s\n", logPrefix, string(b))
		} else {
			fmt.Printf("%s%s\n", logPrefix, raw)
		}
		return nil
	}

	msgs := pubsub.Channel(redis.WithChannelHealthCheckInterval(10 * time.Second))
	for {
		select {
		case <-ctx.Done():
			if err := pubsub.Close(); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("%spubsub close error: %v", logPrefix, err)
			}
			_ = client.Close()
			return ctx.Err()
		case msg := <-msgs:
			if msg == nil || msg.Payload == "" {
				continue
			}
			if err := handle(msg.Payload); err != nil {
				log.Printf("%shandler error: %v", logPrefix, err)
			}
		}
	}
}