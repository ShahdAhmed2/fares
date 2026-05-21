package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"distributed-db/master/internal/cluster"
)

// NodeHealth holds health status of a node
type NodeHealth struct {
	NodeID    string    `json:"node_id"`
	Status    string    `json:"status"` // online, offline, degraded
	CPU       float64   `json:"cpu"`
	RAM       float64   `json:"ram"`
	Latency   int64     `json:"latency_ms"`
	LastSeen  time.Time `json:"last_seen"`
}

// Checker monitors the health of all cluster nodes
type Checker struct {
	nodeID  string
	mu      sync.RWMutex
	nodes   map[string]*NodeHealth
	client  *http.Client
	workers []workerAddr
}

type workerAddr struct {
	id   string
	host string
	port string
}

// NewChecker creates a new health checker.
func NewChecker(nodeID string) *Checker {
	c := &Checker{
		nodeID: nodeID,
		nodes:  make(map[string]*NodeHealth),
		client: &http.Client{Timeout: 3 * time.Second},
	}
	for _, n := range cluster.Global().ReplicationTargets(nodeID) {
		c.workers = append(c.workers, workerAddr{id: n.ID, host: n.Host, port: n.Port})
		c.nodes[n.ID] = &NodeHealth{NodeID: n.ID, Status: "unknown"}
	}
	c.nodes[nodeID] = &NodeHealth{NodeID: nodeID, Status: "online", CPU: 0, RAM: 0}
	return c
}

// StartHeartbeatLoop continuously polls all workers — one goroutine per node
func (c *Checker) StartHeartbeatLoop(ctx context.Context) {
	log.Println("[HEALTH] Starting heartbeat loop with per-node goroutines")

	var wg sync.WaitGroup
	for _, w := range c.workers {
		wg.Add(1)
		go func(worker workerAddr) {
			defer wg.Done()
			ticker := time.NewTicker(4 * time.Second)
			defer ticker.Stop()

			log.Printf("[HEALTH] Heartbeat goroutine started for %s", worker.id)
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					c.ping(worker)
				}
			}
		}(w)
	}
	wg.Wait()
}

func (c *Checker) ping(w workerAddr) {
	start := time.Now()
	url := fmt.Sprintf("http://%s:%s/heartbeat", w.host, w.port)

	resp, err := c.client.Get(url)
	latency := time.Since(start).Milliseconds()

	c.mu.Lock()
	defer c.mu.Unlock()

	node := c.nodes[w.id]
	if node == nil {
		node = &NodeHealth{NodeID: w.id}
		c.nodes[w.id] = node
	}

	if err != nil {
		if node.Status != "offline" {
			log.Printf("[HEALTH] Node %s went OFFLINE: %v", w.id, err)
		}
		node.Status = "offline"
		node.LastSeen = time.Now()
		return
	}
	defer resp.Body.Close()

	var hb struct {
		CPU float64 `json:"cpu"`
		RAM float64 `json:"ram"`
	}
	json.NewDecoder(resp.Body).Decode(&hb)

	if node.Status != "online" {
		log.Printf("[HEALTH] Node %s came back ONLINE (latency: %dms)", w.id, latency)
	}
	node.Status = "online"
	node.CPU = hb.CPU
	node.RAM = hb.RAM
	node.Latency = latency
	node.LastSeen = time.Now()
}

// GetAllHealth returns snapshot of all nodes
func (c *Checker) GetAllHealth() map[string]*NodeHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]*NodeHealth)
	for k, v := range c.nodes {
		copy := *v
		result[k] = &copy
	}
	return result
}

// IsOnline checks if a specific node is online
func (c *Checker) IsOnline(nodeID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if n, ok := c.nodes[nodeID]; ok {
		return n.Status == "online"
	}
	return false
}

// GetOfflineNodes returns list of offline node IDs
func (c *Checker) GetOfflineNodes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var offline []string
	for id, n := range c.nodes {
		if n.Status == "offline" {
			offline = append(offline, id)
		}
	}
	return offline
}

// SetMasterHealth updates the master's own health metrics
func (c *Checker) SetMasterHealth(cpu, ram float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n, ok := c.nodes[c.nodeID]; ok {
		n.CPU = cpu
		n.RAM = ram
		n.LastSeen = time.Now()
		n.Status = "online"
	}
}
