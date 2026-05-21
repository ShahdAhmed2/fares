package node

import (
	"context"
	"log"
	"sync"
	"time"

	"distributed-db/master/internal/masterboot"
)

// Runtime runs the master HTTP API dynamically (failover promotion).
type Runtime struct {
	mu    sync.Mutex
	stack *masterboot.Stack
}

// NewRuntime creates an empty master runtime.
func NewRuntime() *Runtime {
	return &Runtime{}
}

// IsRunning reports whether the master API server is active.
func (rt *Runtime) IsRunning() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.stack != nil
}

// Start brings up the full master stack on the given port.
func (rt *Runtime) Start(nodeID, port, dbPath string, term int) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.stack != nil {
		return nil
	}
	stack, err := masterboot.Start(nodeID, port, dbPath, term, nil)
	if err != nil {
		return err
	}
	rt.stack = stack
	return nil
}

// Stop shuts down the promoted master server.
func (rt *Runtime) Stop() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.stack == nil {
		return nil
	}
	log.Printf("[RUNTIME] Stopping promoted master on %s", rt.stack.Port)
	if rt.stack.Cancel != nil {
		rt.stack.Cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if rt.stack.Server != nil {
		_ = rt.stack.Server.Shutdown(ctx)
	}
	if rt.stack.QE != nil {
		_ = rt.stack.QE.Close()
	}
	rt.stack = nil
	return nil
}
