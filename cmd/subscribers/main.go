package main

// Multi-subscriber runner (one binary)
import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"mysql_changelog_publisher/internal/subscriber"
)

func main() {
	files := envFilesList()
	if len(files) == 0 {
		log.Fatalf("ENV_FILES (or ENV_FILE) is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for _, f := range files {
		cfg, err := loadConfigFromFile(f)
		if err != nil {
			log.Fatalf("load config %s: %v", f, err)
		}
		if strings.TrimSpace(cfg.Name) == "" {
			cfg.Name = strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		}

		wg.Add(1)
		go func(c *subscriber.Config) {
			defer wg.Done()
			if err := subscriber.Run(ctx, c); err != nil && err != context.Canceled {
				log.Printf("subscriber %s exited: %v", c.Name, err)
			}
		}(cfg)
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
	return subscriber.ParseEnvFilesList(files)
}

func loadConfigFromFile(path string) (*subscriber.Config, error) {
	vals, err := subscriber.LoadEnvFiles([]string{path})
	if err != nil {
		return nil, err
	}
	return subscriber.LoadConfigFromMap(vals), nil
}
