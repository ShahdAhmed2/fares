package election

import (
	"context"
	"log"
	"sync"
	"time"

	"distributed-db/master/internal/cluster"
	"distributed-db/master/internal/health"
	"distributed-db/master/internal/replication"
)

// State represents the election state.
type State string

const (
	StateLeader    State = "leader"
	StateFollower  State = "follower"
	StateCandidate State = "candidate"
)

// Leader manages election state on the active master process.
type Leader struct {
	nodeID     string
	state      State
	term       int
	leaderID   string
	isPrimary  bool
	mu         sync.RWMutex
	rm         *replication.Manager
	hc         *health.Checker
	electionCh chan struct{}
	events     []ElectionEvent
}

// ElectionEvent records an election action.
type ElectionEvent struct {
	Time   time.Time `json:"time"`
	Event  string    `json:"event"`
	NodeID string    `json:"node_id"`
	Term   int       `json:"term"`
}

// NewLeader creates the leader election manager.
func NewLeader(nodeID string, rm *replication.Manager, hc *health.Checker, isPrimary bool) *Leader {
	ps := LoadPersistedState()
	term := ps.Term
	if term < 1 {
		term = 1
	}
	leaderID := ps.LeaderID
	if leaderID == "" {
		leaderID = nodeID
	}
	state := StateFollower
	if isPrimary && leaderID == nodeID {
		state = StateLeader
	}
	mh, mp := cluster.Global().MasterHostPort()
	cluster.Global().SetMaster(leaderID, mh, mp, term)

	return &Leader{
		nodeID:     nodeID,
		state:      state,
		term:       term,
		leaderID:   leaderID,
		isPrimary:  isPrimary,
		rm:         rm,
		hc:         hc,
		electionCh: make(chan struct{}, 1),
		events:     []ElectionEvent{},
	}
}

// SetTerm updates the current term (after promotion).
func (l *Leader) SetTerm(t int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.term = t
	l.leaderID = l.nodeID
	l.state = StateLeader
}

// MonitorLeadership watches worker health on the active master.
func (l *Leader) MonitorLeadership(ctx context.Context) {
	log.Println("[ELECTION] Leadership monitor started")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.checkLeadership()
		case <-l.electionCh:
			l.addEvent("manual_election_skipped", l.nodeID)
		}
	}
}

func (l *Leader) checkLeadership() {
	if !l.isPrimary {
		return
	}
	offline := l.hc.GetOfflineNodes()
	if len(offline) > 0 {
		log.Printf("[ELECTION] Warning: offline nodes: %v", offline)
		l.addEvent("nodes_offline", l.nodeID)
	}
}

// TriggerElection is kept for API compatibility (automatic failover uses Coordinator).
func (l *Leader) TriggerElection() {
	select {
	case l.electionCh <- struct{}{}:
	default:
	}
}

// SimulateFailover is kept for manual testing only.
func (l *Leader) SimulateFailover(targetNodeID string) {
	log.Printf("[ELECTION] Manual failover to %s (use automatic failover in production)", targetNodeID)
	l.addEvent("manual_failover", targetNodeID)
}

// GetStatus returns current election status.
func (l *Leader) GetStatus() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return map[string]interface{}{
		"node_id":    l.nodeID,
		"state":      string(l.state),
		"term":       l.term,
		"leader_id":  cluster.Global().GetLeaderID(),
		"master_url": cluster.Global().MasterURL(),
	}
}

// GetEvents returns election history.
func (l *Leader) GetEvents() []ElectionEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()
	events := make([]ElectionEvent, len(l.events))
	copy(events, l.events)
	return events
}

func (l *Leader) addEvent(event, nodeID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, ElectionEvent{
		Time:   time.Now(),
		Event:  event,
		NodeID: nodeID,
		Term:   l.term,
	})
	if len(l.events) > 100 {
		l.events = l.events[len(l.events)-100:]
	}
}
