package masterboot

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"distributed-db/master/internal/api"
	"distributed-db/master/internal/cluster"
	"distributed-db/master/internal/election"
	"distributed-db/master/internal/health"
	"distributed-db/master/internal/query"
	"distributed-db/master/internal/replication"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Stack holds a running promoted master instance.
type Stack struct {
	NodeID string
	Port   string
	DBPath string
	QE     *query.Engine
	RM     *replication.Manager
	HC     *health.Checker
	LE     *election.Leader
	Cancel context.CancelFunc
	Server *http.Server
}

// Start launches the full master HTTP API and background workers.
func Start(nodeID, port, dbPath string, term int, electionDeps *election.HandlerDeps) (*Stack, error) {
	log.Printf("[BOOT] Promoting %s to master on :%s (db=%s, term=%d)", nodeID, port, dbPath, term)

	qe, err := query.NewEngine(dbPath)
	if err != nil {
		return nil, fmt.Errorf("query engine: %w", err)
	}

	host, _ := cluster.Global().MasterHostPort()
	if n, ok := cluster.Global().NodeByID(nodeID); ok {
		host = n.Host
	}
	cluster.Global().SetMaster(nodeID, host, port, term)
	_ = election.SavePersistedState(election.PersistedState{Term: term, LeaderID: nodeID})

	rm := replication.NewManager(nodeID, qe)
	hc := health.NewChecker(nodeID)
	le := election.NewLeader(nodeID, rm, hc, true)
	le.SetTerm(term)

	ctx, cancel := context.WithCancel(context.Background())
	go hc.StartHeartbeatLoop(ctx)
	go rm.StartReplicationWorker(ctx)
	go le.MonitorLeadership(ctx)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		AllowCredentials: true,
	}))

	api.RegisterRoutes(r, qe, rm, hc, le, nodeID)
	if electionDeps != nil {
		election.RegisterHandlers(r, electionDeps)
	} else {
		election.RegisterHandlers(r, &election.HandlerDeps{})
	}

	srv := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[BOOT] Master server error: %v", err)
		}
	}()

	log.Printf("[BOOT] Master API active at http://0.0.0.0:%s", port)
	return &Stack{
		NodeID: nodeID, Port: port, DBPath: dbPath,
		QE: qe, RM: rm, HC: hc, LE: le, Cancel: cancel, Server: srv,
	}, nil
}
