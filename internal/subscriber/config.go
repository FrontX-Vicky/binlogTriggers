package subscriber

import (
	"strconv"
	"strings"
)

const (
	DefaultRedisAddr    = "127.0.0.1:6379"
	DefaultRedisChannel = "binlog:all"
)

type Config struct {
	Name         string
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

	// Exclude filters (blacklist)
	ExcludeDBs    []string
	ExcludeTables []string
}

type EnvLookup func(string) (string, bool)

func LoadConfigFromLookup(lookup EnvLookup) *Config {
	get := func(key string) string {
		if v, ok := lookup(key); ok {
			return v
		}
		return ""
	}

	cfg := &Config{
		Name:            get("SUBSCRIBER_NAME"),
		RedisAddr:       envDefault(get("REDIS_ADDR"), DefaultRedisAddr),
		RedisPass:       get("REDIS_PASS"),
		RedisChannel:    envDefault(get("REDIS_CHANNEL"), ""),
		PrettyPrint:     envBool(get("PRETTY_PRINT"), false),
		FilterDBs:       parseCSV(get("FILTER_DBS")),
		FilterTables:    parseCSV(get("FILTER_TABLES")),
		FilterIDs:       parseCSV(get("FILTER_IDS")),
		FilterOps:       parseCSV(get("FILTER_OPS")),
		FilterChangeAny: parseCSV(get("FILTER_CHANGE_ANY")),
		FilterChangeAll: parseCSV(get("FILTER_CHANGE_ALL")),
		
		// Exclude filters (blacklist)
		ExcludeDBs:    parseCSV(get("EXCLUDE_DBS")),
		ExcludeTables: parseCSV(get("EXCLUDE_TABLES")),
	}

	if cfg.RedisChannel == "" {
		cfg.RedisChannel = envDefault(get("REDIS_STREAM"), "")
	}
	if cfg.RedisChannel == "" {
		cfg.RedisChannel = DefaultRedisChannel
	}

	if dbStr := strings.TrimSpace(get("REDIS_DB")); dbStr != "" {
		if v, err := strconv.Atoi(dbStr); err == nil {
			cfg.RedisDB = v
		}
	}

	return cfg
}

func LoadConfigFromMap(env map[string]string) *Config {
	return LoadConfigFromLookup(func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	})
}

func envDefault(val, def string) string {
	if strings.TrimSpace(val) != "" {
		return val
	}
	return def
}

func envBool(val string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(val))
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
