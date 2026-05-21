package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"distributed-db/master/internal/api"
	"distributed-db/master/internal/cluster"
	"distributed-db/master/internal/election"
	"distributed-db/master/internal/health"
	"distributed-db/master/internal/replication"
	"distributed-db/master/internal/query"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	port := getEnv("MASTER_PORT", "8888")
	nodeID := getEnv("NODE_ID", "master-1")
	host := getEnv("MASTER_HOST", "127.0.0.1")

	cluster.Init(nodeID, host, port)

	ps := election.LoadPersistedState()
	remoteTerm := fetchMaxClusterTerm()
	if remoteTerm > ps.Term {
		ps.Term = remoteTerm
		log.Printf("[MASTER] Adopting cluster term %d (newer than local persisted)", remoteTerm)
	}
	cluster.Global().SetMaster(nodeID, host, port, ps.Term)
	_ = election.SavePersistedState(election.PersistedState{Term: ps.Term, LeaderID: nodeID})

	log.Printf("[MASTER] Starting node %s on %s:%s (term %d)", nodeID, host, port, ps.Term)

	qe, err := query.NewEngine("./data/master.db")
	if err != nil {
		log.Fatalf("[MASTER] Failed to init query engine: %v", err)
	}
	defer qe.Close()

	rm := replication.NewManager(nodeID, qe)
	hc := health.NewChecker(nodeID)
	le := election.NewLeader(nodeID, rm, hc, true)
	le.SetTerm(ps.Term)

	if err := qe.SeedBankData("./data/bank_row_by_row.sql"); err != nil {
		log.Printf("[MASTER] Warning: could not seed data: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	election.RegisterHandlers(r, &election.HandlerDeps{
		OnStepDown: func(term int) {
			if term > cluster.Global().GetTerm() {
				log.Printf("[MASTER] Step-down requested (cluster term %d > local %d) — stopping write leader",
					term, cluster.Global().GetTerm())
				cluster.Global().SetMaster(nodeID, host, port, term)
			}
		},
	})

	srv := &http.Server{Addr: ":" + port, Handler: r}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[MASTER] Server error: %v", err)
		}
	}()

	log.Printf("[MASTER] Node %s running at http://0.0.0.0:%s", nodeID, port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[MASTER] Shutting down gracefully...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
}

func fetchMaxClusterTerm() int {
	client := &http.Client{Timeout: 2 * time.Second}
	maxT := 0
	for _, n := range cluster.Global().Nodes() {
		if n.ID == "master-1" {
			continue
		}
		url := fmt.Sprintf("http://%s:%s/election/status", n.Host, n.Port)
		resp, err := client.Get(url)
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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
