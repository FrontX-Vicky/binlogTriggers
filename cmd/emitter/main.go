package main

// Mysql change broadcaster
import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"mysql_changelog_publisher/internal/event"

	"github.com/redis/go-redis/v9"

	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

const (
	defaultRedisAddr      = "127.0.0.1:6379"
	defaultRedisChannel   = "binlog:all"
	defaultDBPort         = "3306"
	defaultServerID       = uint32(100)
	defaultReconnectDelay = 5 * time.Second
)

var publisher *Publisher

type tableKey struct {
	schema string
	table  string
}

type schemaInfo struct {
	Columns  []string       // ordered
	ColIndex map[string]int // name -> index
	PKCols   []string       // primary key column names (ordered)
}

// Row identification strategy enum
type RowIDStrategy int

const (
	PrimaryKey RowIDStrategy = iota
	CompositeHash
)

// Row identifier structure
type RowIdentifier struct {
	Value    string
	Strategy RowIDStrategy
	Columns  []string
}

type Config struct {
	Addr           string
	DBUser         string
	DBPass         string
	DBHost         string
	DBPort         string
	DBName         string
	ServerID       uint32
	RedisAddr      string
	RedisPass      string
	RedisDB        int
	RedisChannel   string
	ReconnectDelay time.Duration
	LogFile        string
}

// Redis Publisher structure
type Publisher struct {
	r       *redis.Client
	ctx     context.Context
	channel string
	logger  *EventLogger
}

func NewPublisher(addr, pass string, db int, channel string, logger *EventLogger) *Publisher {
	return &Publisher{
		r: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: pass,
			DB:       db,
		}),
		ctx:     context.Background(),
		channel: channel,
		logger:  logger,
	}
}

type EventLogger struct {
	mu   sync.Mutex
	file *os.File
}

func NewEventLogger(path string) (*EventLogger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &EventLogger{file: f}, nil
}

func (l *EventLogger) Log(eventID string, payload []byte) {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	timestamp := time.Now().Format(time.RFC3339Nano)
	fmt.Fprintf(l.file, "%s event_id=%s payload=%s\n", timestamp, eventID, payload)
}

func (l *EventLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// PublishJSON publishes JSON bytes to Redis pub/sub channel
func (p *Publisher) PublishJSON(jsonBytes []byte, eventID string) error {
	if err := p.r.Publish(p.ctx, p.channel, string(jsonBytes)).Err(); err != nil {
		return err
	}
	if p.logger != nil {
		p.logger.Log(eventID, jsonBytes)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	var msgLogger *EventLogger
	if cfg.LogFile != "" {
		msgLogger, err = NewEventLogger(cfg.LogFile)
		if err != nil {
			return fmt.Errorf("init message logger: %w", err)
		}
		defer msgLogger.Close()
	}

	publisher = NewPublisher(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB, cfg.RedisChannel, msgLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		select {
		case sig := <-sigCh:
			log.Printf("signal %s received, shutting down...", sig)
			cancel()
		case <-ctx.Done():
		}
	}()

	for {
		if err := streamChanges(ctx, cfg); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			log.Printf("replication error: %v; retrying in %s", err, cfg.ReconnectDelay)
			select {
			case <-time.After(cfg.ReconnectDelay):
			case <-ctx.Done():
				return nil
			}
			continue
		}
		return nil
	}
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		DBUser:       os.Getenv("DB_USER"),
		DBPass:       os.Getenv("DB_PASS"),
		DBHost:       os.Getenv("DB_HOST"),
		DBPort:       os.Getenv("DB_PORT"),
		DBName:       os.Getenv("DB_NAME"),
		RedisAddr:    os.Getenv("REDIS_ADDR"),
		RedisPass:    os.Getenv("REDIS_PASS"),
		RedisChannel: os.Getenv("REDIS_CHANNEL"),
		LogFile:      os.Getenv("MESSAGE_LOG_FILE"),
	}

	if cfg.DBUser == "" {
		return nil, fmt.Errorf("DB_USER is required")
	}
	if cfg.DBHost == "" {
		return nil, fmt.Errorf("DB_HOST is required")
	}
	if cfg.DBName == "" {
		return nil, fmt.Errorf("DB_NAME is required")
	}

	if cfg.DBPort == "" {
		cfg.DBPort = defaultDBPort
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = fmt.Sprintf("%s:%s", cfg.DBHost, cfg.DBPort)
	}
	cfg.Addr = addr

	serverIDStr := os.Getenv("SERVER_ID")
	if serverIDStr == "" {
		cfg.ServerID = defaultServerID
	} else {
		id, err := strconv.ParseUint(serverIDStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid SERVER_ID: %w", err)
		}
		cfg.ServerID = uint32(id)
	}

	redisDBStr := os.Getenv("REDIS_DB")
	if redisDBStr != "" {
		val, err := strconv.Atoi(redisDBStr)
		if err != nil {
			return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
		}
		cfg.RedisDB = val
	}

	if cfg.RedisAddr == "" {
		cfg.RedisAddr = defaultRedisAddr
	}
	if cfg.RedisChannel == "" {
		cfg.RedisChannel = os.Getenv("REDIS_STREAM")
	}
	if cfg.RedisChannel == "" {
		cfg.RedisChannel = defaultRedisChannel
	}

	delayStr := os.Getenv("RECONNECT_DELAY")
	if delayStr == "" {
		cfg.ReconnectDelay = defaultReconnectDelay
	} else {
		delay, err := time.ParseDuration(delayStr)
		if err != nil {
			return nil, fmt.Errorf("invalid RECONNECT_DELAY: %w", err)
		}
		cfg.ReconnectDelay = delay
	}

	return cfg, nil
}

func (c Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBName)
}

func streamChanges(ctx context.Context, cfg *Config) error {
	sqlDB, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}

	file, pos, err := readMasterFilePos(ctx, sqlDB)
	if err != nil {
		return fmt.Errorf("read master status: %w", err)
	}

	host, port := splitHostPort(cfg.Addr)
	syncerCfg := replication.BinlogSyncerConfig{
		ServerID:        cfg.ServerID,
		Flavor:          "mysql",
		Host:            host,
		Port:            port,
		User:            cfg.DBUser,
		Password:        cfg.DBPass,
		UseDecimal:      true,
		ParseTime:       true,
		HeartbeatPeriod: 30 * time.Second,
		ReadTimeout:     90 * time.Second,
	}

	syncer := replication.NewBinlogSyncer(syncerCfg)
	defer syncer.Close()

	startPos := mysql.Position{Name: file, Pos: uint32(pos)}
	streamer, err := syncer.StartSync(startPos)
	if err != nil {
		return fmt.Errorf("start binlog sync: %w", err)
	}
	log.Printf("Streaming from master tip: %s:%d (realtime)", startPos.Name, startPos.Pos)

	var (
		tableMap sync.Map // map[uint64]*replication.TableMapEvent
		schemaMu sync.Mutex
		schema   = make(map[tableKey]*schemaInfo)
	)

	for {
		ev, err := streamer.GetEvent(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("get event: %w", err)
		}

		switch e := ev.Event.(type) {
		case *replication.RotateEvent:
			log.Printf("Rotate to %s:%d", string(e.NextLogName), e.Position)

		case *replication.TableMapEvent:
			tableMap.Store(e.TableID, e)

			key := tableKey{schema: string(e.Schema), table: string(e.Table)}
			schemaMu.Lock()
			if _, ok := schema[key]; !ok {
				info, err := loadSchemaInfo(ctx, sqlDB, key.schema, key.table)
				if err != nil {
					log.Printf("warn: load schema for %s.%s: %v", key.schema, key.table, err)
				} else {
					schema[key] = info
				}
			}
			schemaMu.Unlock()

		case *replication.RowsEvent:
			v, ok := tableMap.Load(e.TableID)
			if !ok {
				log.Printf("warn: missing table map for table_id=%d", e.TableID)
				continue
			}
			tm := v.(*replication.TableMapEvent)
			dbName := string(tm.Schema)
			tblName := string(tm.Table)
			key := tableKey{schema: dbName, table: tblName}

			schemaMu.Lock()
			ti := schema[key]
			schemaMu.Unlock()

			switch ev.Header.EventType {
			case replication.WRITE_ROWS_EVENTv1, replication.WRITE_ROWS_EVENTv2:
				for _, row := range e.Rows {
					printInsert(dbName, tblName, ti, row)
				}
			case replication.DELETE_ROWS_EVENTv1, replication.DELETE_ROWS_EVENTv2:
				for _, row := range e.Rows {
					printDelete(dbName, tblName, ti, row)
				}
			case replication.UPDATE_ROWS_EVENTv1, replication.UPDATE_ROWS_EVENTv2:
				for i := 0; i < len(e.Rows); i += 2 {
					before := e.Rows[i]
					after := e.Rows[i+1]
					printUpdate(dbName, tblName, ti, before, after)
				}
			default:
				// ignore other row event types
			}

		default:
			// ignore other event types
		}
	}
}

// --- helpers ---

// Helper function to convert UTC to IST
func toIST(utcTime string) string {
	t, err := time.Parse(time.RFC3339, utcTime)
	if err != nil {
		return utcTime
	}
	ist, _ := time.LoadLocation("Asia/Kolkata")
	return t.In(ist).Format("2006-01-02 15:04:05")
}

func splitHostPort(addr string) (string, uint16) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return addr, 3306
	}
	var p uint16 = 3306
	fmt.Sscanf(parts[1], "%d", &p)
	return parts[0], p
}

func readMasterFilePos(ctx context.Context, db *sql.DB) (file string, pos uint64, err error) {
	row := db.QueryRowContext(ctx, "SHOW MASTER STATUS")
	var binDo, binIgnore, execGTID sql.NullString
	if err = row.Scan(&file, &pos, &binDo, &binIgnore, &execGTID); err != nil {
		return "", 0, err
	}
	if file == "" {
		return "", 0, fmt.Errorf("binary logging not enabled (empty file from SHOW MASTER STATUS)")
	}
	return file, pos, nil
}

func loadSchemaInfo(ctx context.Context, db *sql.DB, schema, table string) (*schemaInfo, error) {
	cols := []string{}
	rows, err := db.QueryContext(ctx, `
		SELECT COLUMN_NAME
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pk := []string{}
	rows2, err := db.QueryContext(ctx, `
		SELECT COLUMN_NAME
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY'
		ORDER BY ORDINAL_POSITION`, schema, table)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var c string
			if err := rows2.Scan(&c); err == nil {
				pk = append(pk, c)
			}
		}
	}

	idx := map[string]int{}
	for i, c := range cols {
		idx[c] = i
	}
	return &schemaInfo{Columns: cols, ColIndex: idx, PKCols: pk}, nil
}

// Updated printUpdate function
func printUpdate(db, table string, ti *schemaInfo, before, after []interface{}) {
	// Generate row identifier
	rowID := generateRowIdentifier(ti, after)

	// Collect changes
	var changes []event.ColumnChange
	if ti != nil {
		for i, col := range ti.Columns {
			if !valueEqual(before[i], after[i]) {
				changes = append(changes, event.ColumnChange{
					Column: col,
					From:   sanitize(before[i]),
					To:     sanitize(after[i]),
				})
			}
		}
	}

	// Create the event
	e := &event.RowEvent{
		Op:        "update",
		Timestamp: toIST(time.Now().UTC().Format(time.RFC3339)),
		DB:        db,
		Table:     table,
		RowKey:    rowID.Value,
		Changes:   changes,
	}

	// Marshal the event
	data, err := json.Marshal(e)
	if err != nil {
		log.Printf("error marshaling JSON: %v", err)
		return
	}

	// Publish to Redis
	err = publisher.PublishJSON(data, fmt.Sprintf("%s.%s:update:%v", db, table, e.RowKey))
	if err != nil {
		log.Printf("error publishing to Redis: %v", err)
		return
	}
}

// Updated printInsert function
func printInsert(db, table string, ti *schemaInfo, row []interface{}) {
	pkVal := pkValue(ti, row)
	if pkVal == nil {
		log.Printf("warning: no primary key found for %s.%s", db, table)
		return
	}

	e := &event.RowEvent{
		Op:        "create",
		Timestamp: toIST(time.Now().UTC().Format(time.RFC3339)),
		DB:        db,
		Table:     table,
		RowKey:    pkVal,
		After:     rowAsNamedMap(ti, row),
	}

	// Marshal the event
	data, err := json.Marshal(e)
	if err != nil {
		log.Printf("error marshaling JSON: %v", err)
		return
	}

	// Publish to Redis
	err = publisher.PublishJSON(data, fmt.Sprintf("%s.%s:insert:%v", db, table, e.RowKey))
	if err != nil {
		log.Printf("error publishing to Redis: %v", err)
		return
	}
}

// Updated printDelete function
func printDelete(db, table string, ti *schemaInfo, row []interface{}) {
	pkVal := pkValue(ti, row)
	if pkVal == nil {
		log.Printf("warning: no primary key found for %s.%s", db, table)
		return
	}

	e := &event.RowEvent{
		Op:        "delete",
		Timestamp: toIST(time.Now().UTC().Format(time.RFC3339)),
		DB:        db,
		Table:     table,
		RowKey:    pkVal,
		Before:    rowAsNamedMap(ti, row),
		Tombstone: true,
	}

	// Marshal the event
	data, err := json.Marshal(e)
	if err != nil {
		log.Printf("error marshaling JSON: %v", err)
		return
	}

	// Publish to Redis
	err = publisher.PublishJSON(data, fmt.Sprintf("%s.%s:delete:%v", db, table, e.RowKey))
	if err != nil {
		log.Printf("error publishing to Redis: %v", err)
		return
	}
}

func rowAsNamedMap(ti *schemaInfo, row []interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(row))
	if ti != nil && len(ti.Columns) == len(row) {
		for i, col := range ti.Columns {
			m[col] = sanitize(row[i])
		}
	} else {
		for i := range row {
			m[fmt.Sprintf("col_%d", i+1)] = sanitize(row[i])
		}
	}
	return m
}

func pkValue(ti *schemaInfo, row []interface{}) interface{} {
	if ti == nil || len(ti.PKCols) == 0 || len(ti.Columns) != len(row) {
		return nil
	}
	if len(ti.PKCols) == 1 {
		return sanitize(row[ti.ColIndex[ti.PKCols[0]]])
	}
	// composite key
	out := make(map[string]interface{}, len(ti.PKCols))
	for _, c := range ti.PKCols {
		out[c] = sanitize(row[ti.ColIndex[c]])
	}
	return out
}

func sanitize(v interface{}) interface{} {
	// Convert []byte to string for nicer JSON
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

func valueEqual(a, b interface{}) bool {
	// Treat []byte vs string equalities sanely in console
	if ab, ok := a.([]byte); ok {
		a = string(ab)
	}
	if bb, ok := b.([]byte); ok {
		b = string(bb)
	}
	return reflect.DeepEqual(a, b)
}

// Generate a unique identifier for a row
func generateRowIdentifier(ti *schemaInfo, row []interface{}) RowIdentifier {
	// Try primary key first
	if ti != nil && len(ti.PKCols) > 0 && len(ti.Columns) == len(row) {
		pkVal := pkValue(ti, row)
		if pkVal != nil {
			return RowIdentifier{
				Value:    fmt.Sprintf("%v", pkVal),
				Strategy: PrimaryKey,
				Columns:  ti.PKCols,
			}
		}
	}

	// Fall back to hash of all values
	values := make([]string, 0, len(row))
	columns := make([]string, 0, len(row))

	if ti != nil && len(ti.Columns) == len(row) {
		for i, col := range ti.Columns {
			values = append(values, fmt.Sprintf("%v", sanitize(row[i])))
			columns = append(columns, col)
		}
	} else {
		for i, v := range row {
			values = append(values, fmt.Sprintf("%v", sanitize(v)))
			columns = append(columns, fmt.Sprintf("col_%d", i+1))
		}
	}

	// Create hash of concatenated values
	h := sha256.New()
	h.Write([]byte(strings.Join(values, "|")))
	hash := fmt.Sprintf("%x", h.Sum(nil)[:8]) // Use first 8 bytes for readability

	return RowIdentifier{
		Value:    hash,
		Strategy: CompositeHash,
		Columns:  columns,
	}
}
