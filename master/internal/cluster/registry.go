package cluster

import (
	"os"
	"sort"
	"sync"
)

// Node describes a cluster member.
type Node struct {
	ID         string
	Host       string
	Port       string
	Shard      string
	Priority   int
	CanPromote bool // true only for nodes that can run the Go master API stack
}

// PriorityOrder defines bully ordering (higher index = higher priority).
var PriorityOrder = []string{"worker-1", "worker-2", "worker-3", "master-1"}

// Registry holds cluster-wide master address and term (in-memory + optional persistence).
type Registry struct {
	mu         sync.RWMutex
	selfID     string
	nodes      []Node
	masterID   string
	masterHost string
	masterPort string
	term       int
	electing   bool
}

var global = &Registry{}

func init() {
	global.nodes = defaultNodes()
	global.masterID = "master-1"
	global.masterHost = env("MASTER_HOST", "127.0.0.1")
	global.masterPort = env("MASTER_PORT", "8888")
	global.term = 1
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultNodes() []Node {
	return []Node{
		{ID: "master-1", Host: env("MASTER_HOST", "192.168.1.119"), Port: env("MASTER_PORT", "8888"), Priority: 4, CanPromote: true},
		{ID: "worker-3", Host: env("WORKER3_HOST", "127.0.0.1"), Port: env("WORKER3_PORT", "8083"), Shard: "Assiut,Luxor", Priority: 3, CanPromote: false},
		{ID: "worker-2", Host: env("WORKER2_HOST", "192.168.1.94"), Port: env("WORKER2_PORT", "8082"), Shard: "Alexandria", Priority: 2, CanPromote: false},
		{ID: "worker-1", Host: env("WORKER1_HOST", "192.168.1.155"), Port: env("WORKER1_PORT", "8081"), Shard: "Cairo", Priority: 1, CanPromote: true},
	}
}

// Global returns the process-wide cluster registry.
func Global() *Registry {
	return global
}

// Init configures the registry for this process.
func Init(selfID, selfHost, selfPort string) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.selfID = selfID
	for i := range global.nodes {
		if global.nodes[i].ID == selfID {
			global.nodes[i].Host = selfHost
			global.nodes[i].Port = selfPort
		}
	}
}

func (r *Registry) SelfID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.selfID
}

func (r *Registry) Nodes() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Node, len(r.nodes))
	copy(out, r.nodes)
	return out
}

// ReplicationTargets returns worker nodes that should receive replication from leaderID.
func (r *Registry) ReplicationTargets(leaderID string) []Node {
	all := r.Nodes()
	var out []Node
	for _, n := range all {
		if n.ID == "master-1" || n.ID == leaderID {
			continue
		}
		out = append(out, n)
	}
	return out
}

func (r *Registry) NodeByID(id string) (Node, bool) {
	for _, n := range r.Nodes() {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

func (r *Registry) MasterURL() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return "http://" + r.masterHost + ":" + r.masterPort
}

func (r *Registry) MasterHostPort() (host, port string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.masterHost, r.masterPort
}

func (r *Registry) GetTerm() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.term
}

func (r *Registry) GetLeaderID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.masterID
}

func (r *Registry) Snapshot() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return map[string]interface{}{
		"term":        r.term,
		"leader_id":   r.masterID,
		"master_host": r.masterHost,
		"master_port": r.masterPort,
		"master_url":  "http://" + r.masterHost + ":" + r.masterPort,
		"self_id":     r.selfID,
	}
}

// SetMaster updates the current master (called after election).
func (r *Registry) SetMaster(leaderID, host, port string, term int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if term < r.term {
		return
	}
	r.term = term
	r.masterID = leaderID
	r.masterHost = host
	r.masterPort = port
}

// TryBeginElection returns false if another election is already running locally.
func (r *Registry) TryBeginElection() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.electing {
		return false
	}
	r.electing = true
	return true
}

func (r *Registry) EndElection() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.electing = false
}

// AcceptPropose applies a remote election proposal (term must be strictly greater).
func (r *Registry) AcceptPropose(term int, leaderID, host, port string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if term <= r.term {
		return false
	}
	r.term = term
	r.masterID = leaderID
	r.masterHost = host
	r.masterPort = port
	return true
}

// SelectPromotableWinner picks the highest-priority alive node that can run master APIs.
func SelectPromotableWinner(alive map[string]bool) string {
	order := []string{"master-1", "worker-3", "worker-2", "worker-1"}
	nodes := global.Nodes()
	promotable := make(map[string]bool)
	for _, n := range nodes {
		if n.CanPromote {
			promotable[n.ID] = true
		}
	}
	for _, id := range order {
		if alive[id] && promotable[id] {
			return id
		}
	}
	return ""
}

// SelectElectionCoordinator picks the highest-priority alive node to run the election.
func SelectElectionCoordinator(alive map[string]bool, selfID string) string {
	order := []string{"master-1", "worker-3", "worker-2", "worker-1"}
	for _, id := range order {
		if id == "master-1" {
			continue
		}
		if alive[id] {
			return id
		}
	}
	return selfID
}

// PriorityRank returns sortable priority (higher = more important).
func PriorityRank(id string) int {
	for i, n := range PriorityOrder {
		if n == id {
			return i
		}
	}
	return -1
}

// SortedByPriority returns node IDs sorted high to low priority.
func SortedByPriority(ids []string) []string {
	sort.Slice(ids, func(i, j int) bool {
		return PriorityRank(ids[i]) > PriorityRank(ids[j])
	})
	return ids
}
