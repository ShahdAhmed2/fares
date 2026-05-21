package query

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Engine wraps SQLite with concurrent-safe query execution
type Engine struct {
	db   *sql.DB
	mu   sync.RWMutex
	path string
}

// QueryResult holds the result of a SQL query
type QueryResult struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Affected int64          `json:"affected,omitempty"`
	Message  string         `json:"message,omitempty"`
	Duration string         `json:"duration"`
	Shard    string         `json:"shard,omitempty"`
}

// ShardInfo describes the data distribution
type ShardInfo struct {
	NodeID    string `json:"node_id"`
	City      string `json:"city"`
	RowCount  int    `json:"row_count"`
	Status    string `json:"status"`
}

// ShardMap defines which cities belong to which worker nodes
// ID shard
var ShardMap = map[string]string{
	"Cairo":      "worker-1",
	"Alexandria": "worker-2",
	"Assiut":     "worker-3",
	"Luxor":      "worker-3", // Luxor co-located with Assiut on node 3
}

func NewEngine(dbPath string) (*Engine, error) {
	if err := os.MkdirAll("./data", 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal=WAL&_timeout=5000&cache=shared")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite WAL mode

	e := &Engine{db: db, path: dbPath}
	if err := e.initSchema(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Engine) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS client (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		name         TEXT NOT NULL,
		national_id  TEXT NOT NULL UNIQUE,
		phone        TEXT NOT NULL,
		email        TEXT NOT NULL,
		gender       TEXT NOT NULL CHECK(gender IN ('Male','Female')),
		birth_date   TEXT NOT NULL,
		city         TEXT NOT NULL CHECK(city IN ('Cairo','Assiut','Luxor','Alexandria')),
		address      TEXT NOT NULL,
		account_type TEXT NOT NULL CHECK(account_type IN ('Savings','Current','Fixed Deposit','Business')),
		balance      REAL NOT NULL DEFAULT 0,
		created_at   TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_city ON client(city);
	CREATE INDEX IF NOT EXISTS idx_account_type ON client(account_type);

	CREATE TABLE IF NOT EXISTS replication_log (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		op         TEXT NOT NULL,
		sql_stmt   TEXT NOT NULL,
		target     TEXT NOT NULL,
		status     TEXT NOT NULL DEFAULT 'pending',
		created_at TEXT NOT NULL,
		acked_at   TEXT
	);

	CREATE TABLE IF NOT EXISTS cluster_nodes (
		node_id    TEXT PRIMARY KEY,
		role       TEXT NOT NULL DEFAULT 'slave',
		host       TEXT NOT NULL,
		port       TEXT NOT NULL,
		shard      TEXT,
		status     TEXT NOT NULL DEFAULT 'unknown',
		last_seen  TEXT,
		cpu        REAL DEFAULT 0,
		ram        REAL DEFAULT 0
	);

	INSERT OR IGNORE INTO cluster_nodes VALUES
		('master-1','master','master','8080',NULL,'online',datetime('now'),0,0),
		('worker-1','slave','worker1','8081','Cairo','online',datetime('now'),0,0),
		('worker-2','slave','worker2','8082','Alexandria','online',datetime('now'),0,0),
		('worker-3','slave','worker3','8083','Assiut,Luxor','online',datetime('now'),0,0);
	`
	_, err := e.db.Exec(schema)
	return err
}

// Execute runs any SQL — write ops are master-only
func (e *Engine) Execute(sqlStr string) (*QueryResult, error) {
	start := time.Now()
	upper := strings.ToUpper(strings.TrimSpace(sqlStr))

	if strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH") {
		return e.executeRead(sqlStr, start)
	}
	return e.executeWrite(sqlStr, start)
}

func (e *Engine) executeRead(sqlStr string, start time.Time) (*QueryResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rows, err := e.db.Query(sqlStr)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	result := &QueryResult{
		Columns: cols,
		Rows:    [][]interface{}{},
	}

	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make([]interface{}, len(cols))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = v
			}
		}
		result.Rows = append(result.Rows, row)
	}

	result.Duration = time.Since(start).String()
	return result, nil
}

func (e *Engine) executeWrite(sqlStr string, start time.Time) (*QueryResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	res, err := e.db.Exec(sqlStr)
	if err != nil {
		return nil, fmt.Errorf("exec error: %w", err)
	}

	affected, _ := res.RowsAffected()
	result := &QueryResult{
		Affected: affected,
		Message:  fmt.Sprintf("Query OK, %d row(s) affected", affected),
		Duration: time.Since(start).String(),
	}
	return result, nil
}

// SeedBankData loads the SQL dump into master and partitions per city
func (e *Engine) SeedBankData(sqlFile string) error {
	// Check if already seeded
	var count int
	e.db.QueryRow("SELECT COUNT(*) FROM client").Scan(&count)
	if count > 0 {
		log.Printf("[ENGINE] Already seeded with %d rows, skipping", count)
		return nil
	}

	f, err := os.Open(sqlFile)
	if err != nil {
		return fmt.Errorf("open sql file: %w", err)
	}
	defer f.Close()

	log.Println("[ENGINE] Seeding bank data from SQL dump...")
	tx, err := e.db.Begin()
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	inserted := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(strings.ToUpper(line), "INSERT INTO CLIENT") {
			continue
		}
		// Convert MySQL syntax to SQLite
		line = convertMySQLToSQLite(line)
		if _, err := tx.Exec(line); err != nil {
			log.Printf("[ENGINE] Seed row error: %v", err)
			continue
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("[ENGINE] Seeded %d rows successfully", inserted)
	return nil
}

func convertMySQLToSQLite(s string) string {
	// MySQL ENUM values are just TEXT in SQLite — inserts work as-is
	// Strip the trailing comment  -- row N
	if idx := strings.Index(s, "-- row "); idx != -1 {
		s = strings.TrimSpace(s[:idx])
	}
	return s
}

// GetShardStats returns row counts per city
func (e *Engine) GetShardStats() []ShardInfo {
	rows, err := e.db.Query(`SELECT city, COUNT(*) as cnt FROM client GROUP BY city ORDER BY city`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var shards []ShardInfo
	for rows.Next() {
		var city string
		var cnt int
		rows.Scan(&city, &cnt)
		shards = append(shards, ShardInfo{
			NodeID:   ShardMap[city],
			City:     city,
			RowCount: cnt,
			Status:   "active",
		})
	}
	return shards
}

// GetTableList returns all user tables
func (e *Engine) GetTableList() []string {
	rows, err := e.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
	}
	return tables
}

// LogReplication records a replication event
func (e *Engine) LogReplication(op, sqlStmt, target, status string) {
	e.db.Exec(`INSERT INTO replication_log(op,sql_stmt,target,status,created_at) VALUES(?,?,?,?,datetime('now'))`,
		op, sqlStmt, target, status)
}

// UpdateNodeStatus updates a node's health data
func (e *Engine) UpdateNodeStatus(nodeID, status string, cpu, ram float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.db.Exec(`UPDATE cluster_nodes SET status=?, last_seen=datetime('now'), cpu=?, ram=? WHERE node_id=?`,
		status, cpu, ram, nodeID)
}

// GetClusterNodes returns all cluster node info
func (e *Engine) GetClusterNodes() ([]map[string]interface{}, error) {
	return e.queryRows(`SELECT node_id, role, host, port, shard, status, last_seen, cpu, ram FROM cluster_nodes`)
}

func (e *Engine) queryRows(query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var result []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		m := make(map[string]interface{})
		for i, col := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[col] = string(b)
			} else {
				m[col] = vals[i]
			}
		}
		result = append(result, m)
	}
	return result, nil
}

func (e *Engine) GetReplicationLog(limit int) ([]map[string]interface{}, error) {
	return e.queryRows(`SELECT * FROM replication_log ORDER BY id DESC LIMIT ?`, limit)
}

func (e *Engine) Close() error {
	return e.db.Close()
}
