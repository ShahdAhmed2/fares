package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"distributed-db/master/internal/cluster"
	"distributed-db/master/internal/election"
	"distributed-db/master/internal/node"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

var (
	db         *sql.DB
	nodeID     string
	shard      string
	port       string
	selfHost   string
	masterPort string
	rt         *node.Runtime
	coord      *election.Coordinator
)

func main() {
	nodeID = getEnv("NODE_ID", "worker-1")
	shard = getEnv("SHARD", "Cairo")
	port = getEnv("WORKER_PORT", "8081")
	selfHost = getEnv("WORKER1_HOST", getEnv("SELF_HOST", "127.0.0.1"))
	masterPort = getEnv("MASTER_PORT", "8888")

	cluster.Init(nodeID, selfHost, port)
	ps := election.LoadPersistedState()
	cluster.Global().SetMaster(ps.LeaderID,
		getEnv("MASTER_HOST", "127.0.0.1"), masterPort, ps.Term)

	log.Printf("[WORKER] Starting %s (shard: %s) worker :%s | failover master :%s",
		nodeID, shard, port, masterPort)

	var err error
	os.MkdirAll("./data", 0755)
	db, err = sql.Open("sqlite3", fmt.Sprintf("./data/%s.db?_journal=WAL", nodeID))
	if err != nil {
		log.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()
	initSchema()

	rt = node.NewRuntime()
	coord = election.NewCoordinator(nodeID, selfHost, port, masterPort, true, rt)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go coord.StartMasterMonitor(ctx)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"*"},
	}))

	r.GET("/heartbeat", heartbeat)
	r.GET("/status", status)
	r.POST("/query", query)
	r.POST("/replicate", replicate)
	r.GET("/data/clients", getClients)
	r.GET("/election/status", electionStatus)

	election.RegisterHandlers(r, &election.HandlerDeps{
		Promoter:    rt,
		Coordinator: coord,
	})

	log.Printf("[WORKER] %s online at :%s (automatic failover enabled)", nodeID, port)
	r.Run(":" + port)
}

func electionStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": gin.H{
			"node_id":    nodeID,
			"term":       cluster.Global().GetTerm(),
			"leader_id":  cluster.Global().GetLeaderID(),
			"master_url": cluster.Global().MasterURL(),
			"is_master":  rt != nil && rt.IsRunning(),
		},
	})
}

func initSchema() {
	schema := `
	CREATE TABLE IF NOT EXISTS client (
		id           INTEGER PRIMARY KEY,
		name         TEXT NOT NULL,
		national_id  TEXT NOT NULL UNIQUE,
		phone        TEXT NOT NULL,
		email        TEXT NOT NULL,
		gender       TEXT NOT NULL,
		birth_date   TEXT NOT NULL,
		city         TEXT NOT NULL,
		address      TEXT NOT NULL,
		account_type TEXT NOT NULL,
		balance      REAL NOT NULL DEFAULT 0,
		created_at   TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_city ON client(city);`
	db.Exec(schema)
}

func heartbeat(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	cpu := 5.0 + rand.Float64()*30
	ram := float64(m.Alloc) / 1024 / 1024
	role := "slave"
	if rt != nil && rt.IsRunning() {
		role = "master"
	}
	c.JSON(200, gin.H{
		"node_id": nodeID,
		"role":    role,
		"shard":   shard,
		"status":  "online",
		"cpu":     cpu,
		"ram":     ram,
		"term":    cluster.Global().GetTerm(),
		"time":    time.Now(),
	})
}

func status(c *gin.Context) {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM client WHERE city = ?", shard).Scan(&count)
	role := "slave"
	if rt != nil && rt.IsRunning() {
		role = "master-promoted"
	}
	c.JSON(200, gin.H{
		"node_id":     nodeID,
		"role":        role,
		"shard":       shard,
		"local_count": count,
		"master_url":  cluster.Global().MasterURL(),
		"status":      "online",
	})
}

func query(c *gin.Context) {
	var req struct {
		SQL string `json:"sql"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	upper := strings.ToUpper(strings.TrimSpace(req.SQL))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		if rt != nil && rt.IsRunning() {
			proxyToLocalMaster(c, req.SQL)
			return
		}
		result, err := forwardWriteToMaster(req.SQL)
		if err != nil {
			c.JSON(502, gin.H{"error": err.Error(), "master_url": cluster.Global().MasterURL()})
			return
		}
		c.JSON(200, result)
		return
	}

	rows, err := db.Query(req.SQL)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var result [][]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		row := make([]interface{}, len(cols))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = v
			}
		}
		result = append(result, row)
	}

	c.JSON(200, gin.H{
		"columns": cols,
		"rows":    result,
		"node_id": nodeID,
		"shard":   shard,
	})
}

func proxyToLocalMaster(c *gin.Context, sql string) {
	url := fmt.Sprintf("http://127.0.0.1:%s/query", masterPort)
	body, _ := json.Marshal(map[string]string{"sql": sql})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", data)
}

func forwardWriteToMaster(sql string) (map[string]interface{}, error) {
	url := cluster.Global().MasterURL() + "/query"
	body, _ := json.Marshal(map[string]string{"sql": sql})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("%v", result["error"])
	}
	return result, nil
}

func replicate(c *gin.Context) {
	var req struct {
		Op     string `json:"op"`
		SQL    string `json:"sql"`
		Origin string `json:"origin"`
		Term   int    `json:"term"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if req.Term > 0 && req.Term < cluster.Global().GetTerm() {
		c.JSON(409, gin.H{"status": "rejected", "reason": "stale term"})
		return
	}

	log.Printf("[WORKER:%s] Replicating %s from %s", nodeID, req.Op, req.Origin)
	_, err := db.Exec(req.SQL)
	if err != nil {
		log.Printf("[WORKER:%s] Replicate note: %v", nodeID, err)
	}

	c.JSON(200, gin.H{"node_id": nodeID, "status": "ok", "op": req.Op})
}

func getClients(c *gin.Context) {
	rows, err := db.Query("SELECT id, name, city, account_type, balance FROM client WHERE city = ? LIMIT 100", shard)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var clients []map[string]interface{}
	for rows.Next() {
		var id int
		var name, city, accountType string
		var balance float64
		rows.Scan(&id, &name, &city, &accountType, &balance)
		clients = append(clients, map[string]interface{}{
			"id": id, "name": name, "city": city,
			"account_type": accountType, "balance": balance,
		})
	}
	c.JSON(200, gin.H{"node_id": nodeID, "shard": shard, "clients": clients})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
