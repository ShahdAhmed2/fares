package replication

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
	"distributed-db/master/internal/query"
)

// ReplicateRequest is sent to worker nodes
type ReplicateRequest struct {
	Op      string `json:"op"`
	SQL     string `json:"sql"`
	Origin  string `json:"origin"`
	LogID   int64  `json:"log_id"`
}

// ReplicateResponse is returned by workers
type ReplicateResponse struct {
	NodeID  string `json:"node_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// WorkerNode represents a slave node in the cluster
type WorkerNode struct {
	ID     string
	Host   string
	Port   string
	Shard  string
	Status string
}

// Manager handles all replication across nodes
type Manager struct {
	nodeID  string
	qe      *query.Engine
	workers []WorkerNode
	mu      sync.RWMutex
	replCh  chan ReplicateRequest
	httpCl  *http.Client
}

// NewManager creates a replication manager.
func NewManager(nodeID string, qe *query.Engine) *Manager {
	m := &Manager{
		nodeID:  nodeID,
		qe:      qe,
		replCh:  make(chan ReplicateRequest, 100),
		httpCl:  &http.Client{Timeout: 5 * time.Second},
		workers: loadWorkersFromCluster(nodeID),
	}
	return m
}

func loadWorkersFromCluster(leaderID string) []WorkerNode {
	var workers []WorkerNode
	for _, n := range cluster.Global().ReplicationTargets(leaderID) {
		workers = append(workers, WorkerNode{
			ID: n.ID, Host: n.Host, Port: n.Port, Shard: n.Shard, Status: "online",
		})
	}
	return workers
}

// StartReplicationWorker starts the background replication processor
func (m *Manager) StartReplicationWorker(ctx context.Context) {
	log.Println("[REPLICATION] Starting replication worker pool (3 concurrent replicators)")

	// Use 3 goroutines for concurrent replication to different nodes
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			log.Printf("[REPLICATION] Replicator goroutine %d started", workerID)
			for {
				select {
				case <-ctx.Done():
					log.Printf("[REPLICATION] Replicator %d stopping", workerID)
					return
				case req, ok := <-m.replCh:
					if !ok {
						return
					}
					m.replicateToAll(req)
				}
			}
		}(i)
	}
	wg.Wait()
}

// Replicate queues a replication request
func (m *Manager) Replicate(op, sql string) {
	req := ReplicateRequest{
		Op:     op,
		SQL:    sql,
		Origin: m.nodeID,
		LogID:  int64(cluster.Global().GetTerm()),
	}
	select {
	case m.replCh <- req:
	default:
		log.Println("[REPLICATION] Warning: replication channel full, dropping event")
	}
}

// replicateToAll sends the SQL to all worker nodes concurrently
func (m *Manager) replicateToAll(req ReplicateRequest) {
	m.mu.Lock()
	m.workers = loadWorkersFromCluster(m.nodeID)
	workers := make([]WorkerNode, len(m.workers))
	copy(workers, m.workers)
	m.mu.Unlock()

	var wg sync.WaitGroup
	results := make(chan ReplicateResponse, len(workers))

	for _, w := range workers {
		wg.Add(1)
		go func(worker WorkerNode) {
			defer wg.Done()
			resp := m.sendToWorker(worker, req)
			results <- resp
		}(w)
	}

	// Close results channel when all goroutines done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect acknowledgments
	acked := 0
	for resp := range results {
		if resp.Status == "ok" {
			acked++
			log.Printf("[REPLICATION] ACK from %s", resp.NodeID)
		} else {
			log.Printf("[REPLICATION] NACK from %s: %s", resp.NodeID, resp.Message)
		}
	}

	status := "partial"
	if acked == len(workers) {
		status = "success"
	} else if acked == 0 {
		status = "failed"
	}

	m.qe.LogReplication(req.Op, req.SQL, "all-nodes", status)
	log.Printf("[REPLICATION] Op=%s status=%s acked=%d/%d", req.Op, status, acked, len(workers))
}

func (m *Manager) sendToWorker(w WorkerNode, req ReplicateRequest) ReplicateResponse {
	if req.LogID == 0 {
		req.LogID = int64(cluster.Global().GetTerm())
	}
	body, _ := json.Marshal(map[string]interface{}{
		"op": req.Op, "sql": req.SQL, "origin": req.Origin, "term": req.LogID,
	})
	url := fmt.Sprintf("http://%s:%s/replicate", w.Host, w.Port)

	resp, err := m.httpCl.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		m.mu.Lock()
		for i, worker := range m.workers {
			if worker.ID == w.ID {
				m.workers[i].Status = "offline"
			}
		}
		m.mu.Unlock()
		return ReplicateResponse{NodeID: w.ID, Status: "error", Message: err.Error()}
	}
	defer resp.Body.Close()

	var r ReplicateResponse
	json.NewDecoder(resp.Body).Decode(&r)
	if r.NodeID == "" {
		r.NodeID = w.ID
	}
	return r
}

// UpdateWorkerStatus updates a worker node's known status
func (m *Manager) UpdateWorkerStatus(nodeID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, w := range m.workers {
		if w.ID == nodeID {
			m.workers[i].Status = status
			break
		}
	}
}

// GetWorkers returns current worker list
func (m *Manager) GetWorkers() []WorkerNode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]WorkerNode, len(m.workers))
	copy(result, m.workers)
	return result
}

// PromoteWorker promotes a worker node to master (during failover)
func (m *Manager) PromoteWorker(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, w := range m.workers {
		if w.ID == nodeID {
			m.workers[i].Status = "master"
			log.Printf("[REPLICATION] Promoted %s to master role", nodeID)

			// Notify all other workers
			go m.notifyPromotion(nodeID)
			return nil
		}
	}
	return fmt.Errorf("worker %s not found", nodeID)
}

func (m *Manager) notifyPromotion(newMasterID string) {
	for _, w := range m.workers {
		if w.ID == newMasterID {
			continue
		}
		url := fmt.Sprintf("http://%s:%s/election/new-master", w.Host, w.Port)
		body, _ := json.Marshal(map[string]string{"new_master": newMasterID})
		m.httpCl.Post(url, "application/json", bytes.NewReader(body))
	}
}
