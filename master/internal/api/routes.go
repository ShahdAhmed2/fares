package api

import (
	"fmt"
    "log"
    "math/rand"
    "net/http"
    "runtime"
    "strings"
    "time"

	"distributed-db/master/internal/cluster"
	"distributed-db/master/internal/election"
	"distributed-db/master/internal/health"
	"distributed-db/master/internal/query"
	"distributed-db/master/internal/replication"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	qe      *query.Engine
	rm      *replication.Manager
	hc      *health.Checker
	le      *election.Leader
	nodeID  string
	ai      *AITranslator
}

// RegisterRoutes registers all API endpoints
func RegisterRoutes(r *gin.Engine, qe *query.Engine, rm *replication.Manager, hc *health.Checker, le *election.Leader, nodeID string) {
	h := &Handler{qe: qe, rm: rm, hc: hc, le: le, nodeID: nodeID, ai: &AITranslator{}}

	// Health & heartbeat
	r.GET("/heartbeat", h.Heartbeat)
	r.GET("/status", h.Status)

	// Query execution (master-only writes)
	r.POST("/query", h.Query)

	// AI translation
	r.POST("/ai/translate", h.AITranslate)
	r.POST("/ai/query", h.AIQuery)

	// Cluster management
	r.GET("/cluster/nodes", h.ClusterNodes)
	r.GET("/cluster/shards", h.ClusterShards)
	r.GET("/cluster/health", h.ClusterHealth)
	r.GET("/cluster/replication-log", h.ReplicationLog)

	// Election
	r.GET("/election/status", h.ElectionStatus)
	r.POST("/election/trigger", h.TriggerElection)
	r.POST("/election/failover", h.TriggerFailover)

	// Replication (called by workers wanting to write)
	r.POST("/replicate", h.ReceiveReplicate)

	// Data APIs
	r.GET("/data/clients", h.GetClients)
	r.GET("/data/stats", h.GetStats)
	r.POST("/data/clients", h.CreateClient)
	r.DELETE("/data/clients/:id", h.DeleteClient)

	// Table management
	r.GET("/tables", h.GetTables)
	r.POST("/tables", h.CreateTable)
	r.DELETE("/tables/:name", h.DropTable)
}

// Heartbeat returns node liveness info
func (h *Handler) Heartbeat(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Simulate CPU/RAM variation for demo realism
	cpu := 10.0 + rand.Float64()*20
	ram := float64(m.Alloc) / 1024 / 1024

	h.hc.SetMasterHealth(cpu, ram)
	c.JSON(http.StatusOK, gin.H{
		"node_id":    h.nodeID,
		"role":       "master",
		"status":     "online",
		"cpu":        cpu,
		"ram":        ram,
		"term":       cluster.Global().GetTerm(),
		"leader_id":  cluster.Global().GetLeaderID(),
		"master_url": cluster.Global().MasterURL(),
		"time":       time.Now(),
	})
}

// Status returns full node status
func (h *Handler) Status(c *gin.Context) {
	electionStatus := h.le.GetStatus()
	c.JSON(http.StatusOK, gin.H{
		"node_id":  h.nodeID,
		"role":     "master",
		"status":   "online",
		"election": electionStatus,
		"tables":   h.qe.GetTableList(),
		"time":     time.Now(),
	})
}

// Query executes a SQL query (master enforces write-only access)
func (h *Handler) Query(c *gin.Context) {
	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[API] Query: %s", req.SQL)

	result, err := h.qe.Execute(req.SQL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Replicate writes to workers
	upper := strings.ToUpper(strings.TrimSpace(req.SQL))
	if strings.HasPrefix(upper, "INSERT") || strings.HasPrefix(upper, "UPDATE") ||
		strings.HasPrefix(upper, "DELETE") || strings.HasPrefix(upper, "CREATE TABLE") ||
		strings.HasPrefix(upper, "DROP TABLE") || strings.HasPrefix(upper, "ALTER") {
		go h.rm.Replicate(extractOp(upper), req.SQL)
	}

	c.JSON(http.StatusOK, result)
}

// AITranslate converts natural language to SQL
func (h *Handler) AITranslate(c *gin.Context) {
	var req struct {
		Query string `json:"query" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result := h.ai.Translate(req.Query)
	c.JSON(http.StatusOK, result)
}

// AIQuery translates and executes in one step
func (h *Handler) AIQuery(c *gin.Context) {
	var req struct {
		Query string `json:"query" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	translation := h.ai.Translate(req.Query)
	if translation.SQL == "" {
		c.JSON(http.StatusOK, gin.H{
			"translation": translation,
			"result":      nil,
		})
		return
	}

	result, err := h.qe.Execute(translation.SQL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"translation": translation,
			"error":       err.Error(),
		})
		return
	}

	upper := strings.ToUpper(strings.TrimSpace(translation.SQL))
	if strings.HasPrefix(upper, "INSERT") || strings.HasPrefix(upper, "UPDATE") ||
		strings.HasPrefix(upper, "DELETE") {
		go h.rm.Replicate(extractOp(upper), translation.SQL)
	}

	c.JSON(http.StatusOK, gin.H{
		"translation": translation,
		"result":      result,
	})
}

// ClusterNodes returns all cluster node info
func (h *Handler) ClusterNodes(c *gin.Context) {
	nodes, err := h.qe.GetClusterNodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Enrich with live health data
	healthData := h.hc.GetAllHealth()
	for i, node := range nodes {
		if id, ok := node["node_id"].(string); ok {
			if h, ok := healthData[id]; ok {
				nodes[i]["live_status"] = h.Status
				nodes[i]["cpu"] = h.CPU
				nodes[i]["ram"] = h.RAM
				nodes[i]["latency_ms"] = h.Latency
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

// ClusterShards returns shard distribution info
func (h *Handler) ClusterShards(c *gin.Context) {
	shards := h.qe.GetShardStats()
	c.JSON(http.StatusOK, gin.H{
		"shards":    shards,
		"shard_map": query.ShardMap,
	})
}

// ClusterHealth returns real-time health of all nodes
func (h *Handler) ClusterHealth(c *gin.Context) {
	all := h.hc.GetAllHealth()
	c.JSON(http.StatusOK, all)
}

// ReplicationLog returns recent replication events
func (h *Handler) ReplicationLog(c *gin.Context) {
	logs, err := h.qe.GetReplicationLog(50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// ElectionStatus returns current leader election state
func (h *Handler) ElectionStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": h.le.GetStatus(),
		"events": h.le.GetEvents(),
	})
}

// TriggerElection manually starts an election
func (h *Handler) TriggerElection(c *gin.Context) {
	h.le.TriggerElection()
	c.JSON(http.StatusOK, gin.H{"message": "Election triggered", "status": h.le.GetStatus()})
}

// TriggerFailover manually fails over to a worker
func (h *Handler) TriggerFailover(c *gin.Context) {
	var req struct {
		TargetNode string `json:"target_node"`
	}
	c.ShouldBindJSON(&req)
	if req.TargetNode == "" {
		req.TargetNode = "worker-1"
	}
	h.le.SimulateFailover(req.TargetNode)
	c.JSON(http.StatusOK, gin.H{
		"message": "Failover complete",
		"new_leader": req.TargetNode,
	})
}

// ReceiveReplicate handles write requests FROM workers (slave wants to write)
func (h *Handler) ReceiveReplicate(c *gin.Context) {
	var req replication.ReplicateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[API] Replication request from %s: %s", req.Origin, req.Op)

	// Execute on master
	_, err := h.qe.Execute(req.SQL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"node_id": h.nodeID, "status": "error", "message": err.Error()})
		return
	}

	// Replicate to all others
	go h.rm.Replicate(req.Op, req.SQL)

	c.JSON(http.StatusOK, gin.H{"node_id": h.nodeID, "status": "ok"})
}

// GetClients returns paginated client data with optional city filter
func (h *Handler) GetClients(c *gin.Context) {
	city := c.Query("city")
	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")
	search := c.Query("search")

	sql := "SELECT id, name, national_id, phone, email, gender, birth_date, city, address, account_type, balance, created_at FROM client WHERE 1=1"
	if city != "" {
		sql += " AND city = '" + city + "'"
	}
	if search != "" {
		sql += " AND (name LIKE '%" + search + "%' OR email LIKE '%" + search + "%')"
	}
	sql += " ORDER BY id DESC LIMIT " + limit + " OFFSET " + offset

	result, err := h.qe.Execute(sql)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get total count
	countSQL := "SELECT COUNT(*) FROM client WHERE 1=1"
	if city != "" {
		countSQL += " AND city = '" + city + "'"
	}
	if search != "" {
		countSQL += " AND (name LIKE '%" + search + "%' OR email LIKE '%" + search + "%')"
	}
	countResult, _ := h.qe.Execute(countSQL)
	total := int64(0)
	if len(countResult.Rows) > 0 && len(countResult.Rows[0]) > 0 {
		if v, ok := countResult.Rows[0][0].(int64); ok {
			total = v
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": result, "total": total})
}

// GetStats returns aggregate statistics
func (h *Handler) GetStats(c *gin.Context) {
	stats := map[string]interface{}{}

	// Total clients
	r, _ := h.qe.Execute("SELECT COUNT(*) FROM client")
	if len(r.Rows) > 0 {
		stats["total_clients"] = r.Rows[0][0]
	}

	// Total balance
	r, _ = h.qe.Execute("SELECT SUM(balance) FROM client")
	if len(r.Rows) > 0 {
		stats["total_balance"] = r.Rows[0][0]
	}

	// By city
	r, _ = h.qe.Execute("SELECT city, COUNT(*), SUM(balance), AVG(balance) FROM client GROUP BY city")
	stats["by_city"] = gin.H{"columns": r.Columns, "rows": r.Rows}

	// By account type
	r, _ = h.qe.Execute("SELECT account_type, COUNT(*), SUM(balance) FROM client GROUP BY account_type")
	stats["by_account_type"] = gin.H{"columns": r.Columns, "rows": r.Rows}

	// Gender split
	r, _ = h.qe.Execute("SELECT gender, COUNT(*) FROM client GROUP BY gender")
	stats["by_gender"] = gin.H{"columns": r.Columns, "rows": r.Rows}

	// Shards
	stats["shards"] = h.qe.GetShardStats()

	c.JSON(http.StatusOK, stats)
}

// CreateClient inserts a new client
func (h *Handler) CreateClient(c *gin.Context) {
	var client struct {
		Name        string  `json:"name"`
		NationalID  string  `json:"national_id"`
		Phone       string  `json:"phone"`
		Email       string  `json:"email"`
		Gender      string  `json:"gender"`
		BirthDate   string  `json:"birth_date"`
		City        string  `json:"city"`
		Address     string  `json:"address"`
		AccountType string  `json:"account_type"`
		Balance     float64 `json:"balance"`
	}
	if err := c.ShouldBindJSON(&client); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sql := "INSERT INTO client(name,national_id,phone,email,gender,birth_date,city,address,account_type,balance,created_at) VALUES('" +
		client.Name + "','" + client.NationalID + "','" + client.Phone + "','" + client.Email + "','" +
		client.Gender + "','" + client.BirthDate + "','" + client.City + "','" + client.Address + "','" +
		client.AccountType + "'," + formatFloat(client.Balance) + ",date('now'))"

	result, err := h.qe.Execute(sql)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	go h.rm.Replicate("INSERT", sql)
	c.JSON(http.StatusCreated, result)
}

// DeleteClient deletes a client by ID
func (h *Handler) DeleteClient(c *gin.Context) {
	id := c.Param("id")
	sql := "DELETE FROM client WHERE id = " + id
	result, err := h.qe.Execute(sql)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	go h.rm.Replicate("DELETE", sql)
	c.JSON(http.StatusOK, result)
}

// GetTables lists all tables
func (h *Handler) GetTables(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"tables": h.qe.GetTableList()})
}

// CreateTable creates a new table dynamically
func (h *Handler) CreateTable(c *gin.Context) {
	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.qe.Execute(req.SQL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	go h.rm.Replicate("CREATE_TABLE", req.SQL)
	c.JSON(http.StatusCreated, result)
}

// DropTable drops a table
func (h *Handler) DropTable(c *gin.Context) {
	name := c.Param("name")
	sql := "DROP TABLE IF EXISTS " + name
	result, err := h.qe.Execute(sql)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	go h.rm.Replicate("DROP_TABLE", sql)
	c.JSON(http.StatusOK, result)
}

func extractOp(upper string) string {
	ops := []string{"INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER"}
	for _, op := range ops {
		if strings.HasPrefix(upper, op) {
			return op
		}
	}
	return "UNKNOWN"
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%f", f)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
