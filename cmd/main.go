package main
// Mysql change broadcaster
import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	// "flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"
	"path/filepath"
    "github.com/redis/go-redis/v9"


	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
)

var publisher *Publisher


type tableKey struct {
	schema string
	table  string
}
type schemaInfo struct {
	Columns  []string          // ordered
	ColIndex map[string]int    // name -> index
	PKCols   []string          // primary key column names (ordered)
}

// Add these new types to help with event structuring
type TransactionInfo struct {
    GTID     string `json:"gtid,omitempty"`
    Binlog   struct {
        File string `json:"file"`
        Pos  uint32 `json:"pos"`
    } `json:"binlog"`
    ServerID uint32 `json:"server_id"`
    XID      uint64 `json:"xid,omitempty"`
    Seq      int64  `json:"seq"`
}

// Updated RowEvent struct - remove TransactionInfo
type RowEvent struct {
    Op        string                 `json:"op"`
    Timestamp string                 `json:"timestamp"`  // IST timestamp
    DB        string                 `json:"db"`
    Table     string                 `json:"table"`
    RowKey    interface{}            `json:"row_key"`
    After     map[string]interface{} `json:"after,omitempty"`
    Before    map[string]interface{} `json:"before,omitempty"`
    Changes   []ColumnChange         `json:"changes,omitempty"`
    Tombstone bool                   `json:"tombstone,omitempty"`
    Seq       int64                  `json:"seq,omitempty"`    // Add sequence number directly to RowEvent
}

type ColumnChange struct {
    Column string      `json:"column"`
    From   interface{} `json:"from"`
    To     interface{} `json:"to"`
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

// Change tracking structure
type ChangeTracking struct {
    TableName      string
    HasPrimaryKey  bool
    PrimaryKeys    []string
    ChangeType     string
    RowID          RowIdentifier
    ChangedColumns []string
}

// Redis Publisher structure
type Publisher struct {
    r   *redis.Client
    ctx context.Context
}

func NewPublisher(addr, pass string, db int) *Publisher {
    return &Publisher{
        r: redis.NewClient(&redis.Options{
            Addr:     addr,
            Password: pass,
            DB:       db,
        }),
        ctx: context.Background(),
    }
}

// PublishJSON publishes JSON bytes to Redis stream
func (p *Publisher) PublishJSON(jsonBytes []byte, eventID string) error {
    args := &redis.XAddArgs{
        Stream: "binlog:all",
        Values: map[string]any{
            "payload":  string(jsonBytes),
            "event_id": eventID,
            "ts":       time.Now().Unix(),
        },
    }
    _, err := p.r.XAdd(p.ctx, args).Result()
    return err
}

func main() {
		// Load .env file
    err := godotenv.Load()
    if err != nil {
        log.Fatalf("Error loading .env file: %v", err)
    }

    // Read variables from .env
    addr := os.Getenv("ADDR") // e.g. "localhost:3306"
	user := os.Getenv("DB_USER")
    pass := os.Getenv("DB_PASS")
    host := os.Getenv("DB_HOST")
    dbPort := os.Getenv("DB_PORT")
    dbName := os.Getenv("DB_NAME")
    serverID := os.Getenv("SERVER_ID")
    useGTID := os.Getenv("USE_GTID")

    if user == "" {
        log.Fatal("please provide DB_USER in .env")
    }
    if host == "" {
        log.Fatal("please provide DB_HOST in .env")
    }
    if dbPort == "" {
        dbPort = "3306"
    }
    if dbName == "" {
        log.Fatal("please provide DB_NAME in .env")
    }
    if addr == "" {
        addr = fmt.Sprintf("%s:%s", host, dbPort)
    }
    if serverID == "" {
        serverID = "100"
    }

    // Parse serverID to uint32
    var sid uint32
    fmt.Sscanf(serverID, "%d", &sid)

    // Plain SQL connection (for SHOW MASTER STATUS & information_schema).
    sqlDB, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, pass, host, dbPort, dbName))
    if err != nil { log.Fatal(err) }
    defer sqlDB.Close()

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle Ctrl+C cleanly
    go func() {
        ch := make(chan os.Signal, 1)
        signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
        <-ch
        log.Println("signal received, shutting down...")
        cancel()
    }()

    // Build binlog syncer
    host, port := splitHostPort(addr)
    syncerCfg := replication.BinlogSyncerConfig{
        ServerID:  sid,
        Flavor:    "mysql",
        Host:      host,
        Port:      port,
        User:      user,
        Password:  pass,
        UseDecimal: true,
        ParseTime:  true,
    }
    syncer := replication.NewBinlogSyncer(syncerCfg)
    defer syncer.Close()

    var streamer *replication.BinlogStreamer

    if strings.ToLower(useGTID) == "true" {
        // Read current executed GTID set and start from it
        gtidStr, err := readExecutedGTIDSet(ctx, sqlDB)
        if err != nil {
            log.Fatalf("read GTID set: %v", err)
        }
        gs, err := mysql.ParseGTIDSet("mysql", gtidStr)
        if err != nil {
            log.Fatalf("parse GTID set: %v", err)
        }
        log.Printf("Starting from GTID: %s\n", gs.String())
        streamer, err = syncer.StartSyncGTID(gs)
        if err != nil { log.Fatal(err) }
    } else {
        // Start from current master file/pos
        file, pos, err := readMasterFilePos(ctx, sqlDB)
        if err != nil { log.Fatalf("SHOW MASTER STATUS: %v", err) }
        startPos := mysql.Position{Name: file, Pos: uint32(pos)}
        log.Printf("Starting from position: %s:%d\n", startPos.Name, startPos.Pos)
        streamer, err = syncer.StartSync(startPos)
        if err != nil { log.Fatal(err) }
    }


	// Cache table mappings (TableID -> TableMapEvent) and schema (columns + PKs)
	var (
		tableMap sync.Map // map[uint64]*replication.TableMapEvent
		schemaMu sync.Mutex
		schema   = make(map[tableKey]*schemaInfo)
	)

	// Add periodic cleanup of tableMap cache
	cleanupTableMapCache(&tableMap)

    publisher = NewPublisher("127.0.0.1:6379", "", 0)  // localhost Redis with no password

	for {
		ev, err := streamer.GetEvent(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // graceful shutdown
			}
			log.Fatalf("GetEvent error: %v", err)
		}

		switch e := ev.Event.(type) {
		case *replication.RotateEvent:
			log.Printf("Rotate to %s:%d\n", string(e.NextLogName), e.Position)

		case *replication.TableMapEvent:
			tableMap.Store(e.TableID, e) // cache table map event

			// Ensure schema cache for this (schema, table)
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
			// Find schema/table via last TableMap
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
			// other events (XID/QUERY/GTID) can be handled if you need them
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

func readExecutedGTIDSet(ctx context.Context, db *sql.DB) (string, error) {
	var gtid string
	err := db.QueryRowContext(ctx, "SELECT @@GLOBAL.gtid_executed").Scan(&gtid)
	return gtid, err
}

func loadSchemaInfo(ctx context.Context, db *sql.DB, schema, table string) (*schemaInfo, error) {
	cols := []string{}
	rows, err := db.QueryContext(ctx, `
		SELECT COLUMN_NAME
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION`, schema, table)
	if err != nil { return nil, err }
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil { return nil, err }
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil { return nil, err }

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

// Helper function to get next sequence number
func getNextSeq(filename string) (int64, error) {
    if _, err := os.Stat(filename); os.IsNotExist(err) {
        return 1, nil
    }

    // Read file and count existing entries
    data, err := os.ReadFile(filename)
    if err != nil {
        return 0, err
    }
    
    count := int64(strings.Count(string(data), `"seq":`))
    return count + 1, nil
}

// Add error handling for file operations
func appendToJSONFile(event *RowEvent, filename string) error {
    maxRetries := 3
    retryInterval := time.Second * 2
    
    var err error
    for i := 0; i < maxRetries; i++ {
        err = tryAppendToFile(event, filename)
        if err == nil {
            return nil
        }
        
        if os.IsNotExist(err) {
            if err = os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
                continue
            }
        }
        
        log.Printf("Retry %d: Failed to write to file %s: %v", i+1, filename, err)
        time.Sleep(retryInterval)
    }
    return fmt.Errorf("failed to write after %d attempts: %v", maxRetries, err)
}

// Update appendToJSONFile function
func tryAppendToFile(event *RowEvent, filename string) error {
    seq, err := getNextSeq(filename)
    if err != nil {
        return fmt.Errorf("failed to get sequence number: %v", err)
    }
    event.Seq = seq  // Update this line to use event.Seq instead of event.Tx.Seq

    var file *os.File
    if _, err := os.Stat(filename); os.IsNotExist(err) {
        file, err = os.Create(filename)
        if err != nil {
            return fmt.Errorf("failed to create file: %v", err)
        }
        if _, err := file.WriteString("[\n"); err != nil {
            file.Close()
            return err
        }
    } else {
        file, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
        if err != nil {
            return fmt.Errorf("failed to open file: %v", err)
        }
        if _, err := file.WriteString(",\n"); err != nil {
            file.Close()
            return err
        }
    }
    defer file.Close()

    data, err := json.MarshalIndent(event, "  ", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal JSON: %v", err)
    }

    if _, err := file.Write(data); err != nil {
        return fmt.Errorf("failed to write event: %v", err)
    }

    return nil
}

// Updated printUpdate function
func printUpdate(db, table string, ti *schemaInfo, before, after []interface{}) {
    // Generate row identifier
    rowID := generateRowIdentifier(ti, after)
    
    // Collect changes
    var changes []ColumnChange
    if ti != nil {
        for i, col := range ti.Columns {
            if !valueEqual(before[i], after[i]) {
                changes = append(changes, ColumnChange{
                    Column: col,
                    From:   sanitize(before[i]),
                    To:     sanitize(after[i]),
                })
            }
        }
    }

    // Create the event
    event := &RowEvent{
        Op:        "update",
        Timestamp: toIST(time.Now().UTC().Format(time.RFC3339)),
        DB:        db,
        Table:     table,
        RowKey:    rowID.Value,
        Changes:   changes,
    }

    // Marshal the event
    data, err := json.Marshal(event)
    if err != nil {
        log.Printf("error marshaling JSON: %v", err)
        return
    }

    // Publish to Redis
    // id, err := publisher.PublishJSON(data, fmt.Sprintf("%s.%s:update:%v", db, table, event.RowKey))
    err = publisher.PublishJSON(data, fmt.Sprintf("%s.%s:update:%v", db, table, event.RowKey))
    if err != nil {
        log.Printf("error publishing to Redis: %v", err)
        return
    }
    // log.Printf("Published update event as %s", id)
}

// Updated printInsert function
func printInsert(db, table string, ti *schemaInfo, row []interface{}) {
    pkVal := pkValue(ti, row)
    if pkVal == nil {
        log.Printf("warning: no primary key found for %s.%s", db, table)
        return
    }

    event := &RowEvent{
        Op:        "create",
        Timestamp: toIST(time.Now().UTC().Format(time.RFC3339)),
        DB:        db,
        Table:     table,
        RowKey:    pkVal,
        After:     rowAsNamedMap(ti, row),
    }

    // Marshal the event
    data, err := json.Marshal(event)
    if err != nil {
        log.Printf("error marshaling JSON: %v", err)
        return
    }

    // Publish to Redis
    // id, err := publisher.PublishJSON(data, fmt.Sprintf("%s.%s:insert:%v", db, table, event.RowKey))
    err = publisher.PublishJSON(data, fmt.Sprintf("%s.%s:insert:%v", db, table, event.RowKey))
    if err != nil {
        log.Printf("error publishing to Redis: %v", err)
        return
    }
    // log.Printf("Published insert event as %s", id)
}

// Updated printDelete function
func printDelete(db, table string, ti *schemaInfo, row []interface{}) {
    pkVal := pkValue(ti, row)
    if pkVal == nil {
        log.Printf("warning: no primary key found for %s.%s", db, table)
        return
    }

    event := &RowEvent{
        Op:        "delete",
        Timestamp: toIST(time.Now().UTC().Format(time.RFC3339)),
        DB:        db,
        Table:     table,
        RowKey:    pkVal,
        Before:    rowAsNamedMap(ti, row),
        Tombstone: true,
    }

    // Marshal the event
    data, err := json.Marshal(event)
    if err != nil {
        log.Printf("error marshaling JSON: %v", err)
        return
    }

    // Publish to Redis
    // id, err := publisher.PublishJSON(data, fmt.Sprintf("%s.%s:delete:%v", db, table, event.RowKey))
    err = publisher.PublishJSON(data, fmt.Sprintf("%s.%s:delete:%v", db, table, event.RowKey))
    if err != nil {
        log.Printf("error publishing to Redis: %v", err)
        return
    }
    // log.Printf("Published delete event as %s", id)
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

func emit(doc map[string]interface{}) {
	j, err := json.Marshal(doc)
	if err != nil {
		log.Printf("json error: %v", err)
		return
	}
	fmt.Println(string(j))
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

// Log change tracking information
func logChangeTracking(tracking ChangeTracking) {
    var idType string
    if tracking.HasPrimaryKey {
        idType = fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(tracking.PrimaryKeys, ", "))
    } else {
        idType = fmt.Sprintf("COMPOSITE HASH of (%s)", strings.Join(tracking.RowID.Columns, ", "))
    }

    // Create changed columns string
    var changedColsStr string
    if tracking.ChangedColumns != nil && len(tracking.ChangedColumns) > 0 {
        changedColsStr = fmt.Sprintf("Changed Cols: %s", strings.Join(tracking.ChangedColumns, ", "))
    }

    log.Printf(`
Table Change Details:
-------------------
Table:        %s
ID Type:      %s
Row ID:       %s
Operation:    %s
%s`,
        tracking.TableName,
        idType,
        tracking.RowID.Value,
        tracking.ChangeType,
        changedColsStr)
}

// Add reconnection logic
func connectDB(maxRetries int, retryInterval time.Duration) (*sql.DB, error) {
    var db *sql.DB
    var err error
    
    for i := 0; i < maxRetries; i++ {
        db, err = sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", 
            os.Getenv("DB_USER"), 
            os.Getenv("DB_PASS"), 
            os.Getenv("DB_HOST"), 
            os.Getenv("DB_PORT"), 
            os.Getenv("DB_NAME")))
            
        if err == nil {
            if err = db.Ping(); err == nil {
                return db, nil
            }
        }
        
        log.Printf("Database connection attempt %d failed: %v. Retrying in %v...", 
            i+1, err, retryInterval)
        time.Sleep(retryInterval)
    }
    return nil, fmt.Errorf("failed to connect after %d attempts: %v", maxRetries, err)
}

// Add periodic cleanup of tableMap cache
func cleanupTableMapCache(tableMap *sync.Map) {
    ticker := time.NewTicker(1 * time.Hour)
    go func() {
        for range ticker.C {
            var keysToDelete []uint64
            tableMap.Range(func(k, v interface{}) bool {
                // Add logic to determine old entries
                keysToDelete = append(keysToDelete, k.(uint64))
                return true
            })
            for _, k := range keysToDelete {
                tableMap.Delete(k)
            }
            log.Printf("Cleaned up %d old table map entries", len(keysToDelete))
        }
    }()
}



