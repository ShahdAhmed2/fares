package election

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"distributed-db/master/internal/cluster"
)

const (
	masterPingInterval  = 3 * time.Second
	masterDeadThreshold = 10 * time.Second
	electionCooldown    = 5 * time.Second
)

// Coordinator watches the master and runs automatic bully election + promotion.
type Coordinator struct {
	selfID       string
	selfHost     string
	selfPort     string
	masterPort   string
	workerMode   bool
	httpClient   *http.Client
	mu           sync.Mutex
	lastMasterOK time.Time
	masterDead   bool
	lastElection time.Time
	promoter     Promoter
	onPromoted   func()
}

// NewCoordinator creates a failover coordinator for a worker or standby master.
func NewCoordinator(selfID, selfHost, selfPort, masterPort string, workerMode bool, promoter Promoter) *Coordinator {
	cluster.Init(selfID, selfHost, selfPort)
	return &Coordinator{
		selfID:     selfID,
		selfHost:   selfHost,
		selfPort:   selfPort,
		masterPort: masterPort,
		workerMode: workerMode,
		httpClient: &http.Client{Timeout: 3 * time.Second},
		promoter:   promoter,
	}
}

// StartMasterMonitor runs the heartbeat loop that detects master failure.
func (c *Coordinator) StartMasterMonitor(ctx context.Context) {
	log.Printf("[FAILOVER] Master monitor started for %s (interval=%s, dead=%s)",
		c.selfID, masterPingInterval, masterDeadThreshold)

	ticker := time.NewTicker(masterPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkMaster()
		}
	}
}

func (c *Coordinator) checkMaster() {
	if c.promoter != nil && c.promoter.IsRunning() {
		c.mu.Lock()
		c.lastMasterOK = time.Now()
		c.masterDead = false
		c.mu.Unlock()
		// update

		if c.selfID != "master-1" {
			c.checkOriginalMasterRecovery()
		}

		return
	}

	url := cluster.Global().MasterURL() + "/heartbeat"
	start := time.Now()
	resp, err := c.httpClient.Get(url)
	latency := time.Since(start)

	c.mu.Lock()
	defer c.mu.Unlock()

	if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		c.lastMasterOK = time.Now()
		if c.masterDead {
			log.Printf("[FAILOVER] Master reachable again (%dms)", latency.Milliseconds())
		}
		c.masterDead = false
		c.validateRecoveredMaster()
		return
	}
	if resp != nil {
		resp.Body.Close()
	}

	if c.lastMasterOK.IsZero() {
		c.lastMasterOK = time.Now()
		return
	}

	if time.Since(c.lastMasterOK) >= masterDeadThreshold {
		if !c.masterDead {
			c.masterDead = true
			log.Printf("[FAILOVER] Master declared DEAD (no response for %s)", masterDeadThreshold)
			go c.runAutomaticElection()
		}
	}
}

func (c *Coordinator) validateRecoveredMaster() {
	statusURL := cluster.Global().MasterURL() + "/election/status"
	resp, err := c.httpClient.Get(statusURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var body struct {
		Status map[string]interface{} `json:"status"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return
	}
	remoteTerm, _ := body.Status["term"].(float64)
	localTerm := cluster.Global().GetTerm()
	if int(remoteTerm) < localTerm && c.workerMode {
		log.Printf("[FAILOVER] Recovered master term %d < cluster term %d — requesting step-down",
			int(remoteTerm), localTerm)
		c.postStepDown(cluster.Global().MasterURL(), localTerm)
	}
}

func (c *Coordinator) postStepDown(masterURL string, term int) {
	body, _ := json.Marshal(map[string]interface{}{"term": term})
	c.httpClient.Post(masterURL+"/election/step-down", "application/json", bytes.NewReader(body))
}

// update
func (c *Coordinator) checkOriginalMasterRecovery() {
	masterNode, ok := cluster.Global().NodeByID("master-1")
	if !ok {
		return
	}

	url := fmt.Sprintf("http://%s:%s/heartbeat", masterNode.Host, masterNode.Port)
	resp, err := c.httpClient.Get(url)

	if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		log.Printf("[FAILOVER] Original master (master-1) is back online. Initiating failback...")
		c.ExecuteFailback(masterNode)
	} else if resp != nil {
		resp.Body.Close()
	}
}

// ExecuteFailback contains the thread-safe logic to step down and return leadership.
func (c *Coordinator) ExecuteFailback(masterNode cluster.Node) {
	if err := c.promoter.Stop(); err != nil {
		log.Printf("[FAILOVER] Failed to stop promoter during failback: %v", err)
		return
	}

	newTerm := cluster.Global().GetTerm() + 1
	cluster.Global().SetMaster("master-1", masterNode.Host, masterNode.Port, newTerm)
	_ = SavePersistedState(PersistedState{Term: newTerm, LeaderID: "master-1"})

	c.broadcastNewMaster(newTerm, "master-1", masterNode.Host, masterNode.Port)
	log.Printf("[FAILOVER] Stepped down safely. master-1 is the leader again.")
}

// runAutomaticElection executes bully election across workers (no UI).
func (c *Coordinator) runAutomaticElection() {
	c.mu.Lock()
	if time.Since(c.lastElection) < electionCooldown {
		c.mu.Unlock()
		return
	}
	c.lastElection = time.Now()
	c.mu.Unlock()

	if !cluster.Global().TryBeginElection() {
		return
	}
	defer cluster.Global().EndElection()

	alive := c.pingCluster()
	alive[c.selfID] = true

	coordinator := cluster.SelectElectionCoordinator(alive, c.selfID)
	if coordinator != c.selfID {
		log.Printf("[FAILOVER] Election coordinator is %s (not %s), waiting for proposal", coordinator, c.selfID)
		time.Sleep(2 * time.Second)
		return
	}

	log.Printf("[FAILOVER] %s coordinating automatic election", c.selfID)

	maxTerm := c.fetchMaxTerm()
	newTerm := maxTerm + 1

	winner := cluster.SelectPromotableWinner(alive)
	if winner == "" {
		log.Println("[FAILOVER] No promotable winner found")
		return
	}

	winNode, ok := cluster.Global().NodeByID(winner)
	if !ok {
		return
	}

	masterHost := winNode.Host
	masterPort := c.masterPort
	if winner == c.selfID {
		masterPort = c.masterPort
	}

	acks := c.broadcastPropose(newTerm, winner, masterHost, masterPort)
	quorum := c.quorumRequired()

	log.Printf("[FAILOVER] Election term=%d leader=%s acks=%d/%d", newTerm, winner, acks, quorum)

	if acks < quorum {
		log.Printf("[FAILOVER] Election failed quorum")
		return
	}

	cluster.Global().SetMaster(winner, masterHost, masterPort, newTerm)
	_ = SavePersistedState(PersistedState{Term: newTerm, LeaderID: winner})

	c.broadcastNewMaster(newTerm, winner, masterHost, masterPort)

	if winner == c.selfID && winNode.CanPromote {
		c.promoteToMaster(newTerm, masterHost, masterPort)
	}
}

func (c *Coordinator) quorumRequired() int {
	n := len(cluster.Global().Nodes()) - 1 // exclude static master-1 entry
	if n <= 0 {
		return 1
	}
	return (n / 2) + 1
}

func (c *Coordinator) pingCluster() map[string]bool {
	alive := make(map[string]bool)
	for _, n := range cluster.Global().Nodes() {
		if n.ID == c.selfID {
			continue
		}
		url := fmt.Sprintf("http://%s:%s/heartbeat", n.Host, n.Port)
		resp, err := c.httpClient.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			alive[n.ID] = true
			resp.Body.Close()
		} else if resp != nil {
			resp.Body.Close()
		}
	}
	return alive
}

func (c *Coordinator) fetchMaxTerm() int {
	maxT := cluster.Global().GetTerm()
	for _, n := range cluster.Global().Nodes() {
		if n.ID == c.selfID {
			continue
		}
		url := fmt.Sprintf("http://%s:%s/election/status", n.Host, n.Port)
		resp, err := c.httpClient.Get(url)
		if err != nil {
			continue
		}
		var body struct {
			Status map[string]interface{} `json:"status"`
		}
		json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if t, ok := body.Status["term"].(float64); ok && int(t) > maxT {
			maxT = int(t)
		}
	}
	return maxT
}

type proposeBody struct {
	Term       int    `json:"term"`
	LeaderID   string `json:"leader_id"`
	MasterHost string `json:"master_host"`
	MasterPort string `json:"master_port"`
}

func (c *Coordinator) broadcastPropose(term int, leaderID, host, port string) int {
	body, _ := json.Marshal(proposeBody{
		Term: term, LeaderID: leaderID, MasterHost: host, MasterPort: port,
	})
	acks := 0
	if c.acceptProposeLocally(term, leaderID, host, port) {
		acks++
	}
	for _, n := range cluster.Global().Nodes() {
		if n.ID == c.selfID || n.ID == "master-1" {
			continue
		}
		url := fmt.Sprintf("http://%s:%s/election/propose", n.Host, n.Port)
		resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			continue
		}
		var res struct {
			Accepted bool `json:"accepted"`
		}
		json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		if res.Accepted {
			acks++
		}
	}
	return acks
}

func (c *Coordinator) acceptProposeLocally(term int, leaderID, host, port string) bool {
	return cluster.Global().AcceptPropose(term, leaderID, host, port)
}

func (c *Coordinator) broadcastNewMaster(term int, leaderID, host, port string) {
	body, _ := json.Marshal(map[string]interface{}{
		"term":        term,
		"new_master":  leaderID,
		"master_host": host,
		"master_port": port,
	})
	for _, n := range cluster.Global().Nodes() {
		if n.ID == c.selfID || n.ID == "master-1" {
			continue
		}
		url := fmt.Sprintf("http://%s:%s/election/new-master", n.Host, n.Port)
		c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	}
}

func (c *Coordinator) promoteToMaster(term int, host, port string) {
	if c.promoter == nil {
		log.Println("[FAILOVER] No promoter available for promotion")
		return
	}
	dbPath := fmt.Sprintf("./data/%s.db", c.selfID)
	if err := c.promoter.Start(c.selfID, port, dbPath, term); err != nil {
		log.Printf("[FAILOVER] Promotion failed: %v", err)
		return
	}
	log.Printf("[FAILOVER] *** %s is now MASTER at :%s (term %d) ***", c.selfID, port, term)
	if c.onPromoted != nil {
		c.onPromoted()
	}
}
