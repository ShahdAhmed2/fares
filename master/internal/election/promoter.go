package election

// Promoter can dynamically start the master API (failover promotion).
type Promoter interface {
	IsRunning() bool
	Start(nodeID, port, dbPath string, term int) error
}
