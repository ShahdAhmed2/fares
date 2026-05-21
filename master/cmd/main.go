package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	leaderState, err := fetchClusterLeaderState()
	var term int
	var currentLeader string
	var mHost, mPort string

	if err == nil && leaderState.LeaderID != "" && leaderState.LeaderID != "master-1" {
		term = leaderState.Term
		currentLeader = leaderState.LeaderID
		mHost = leaderState.MasterHost
		mPort = leaderState.MasterPort
		log.Printf("[MASTER] Adopting active cluster term %d and leader %s (%s:%s)", term, currentLeader, mHost, mPort)
	} else {
		term = ps.Term
		if err == nil && leaderState.Term > term {
			term = leaderState.Term
		}
		currentLeader = nodeID
		mHost = host
		mPort = port
		log.Printf("[MASTER] No active worker-promoted leader found. Starting as primary leader with term %d", term)
	}

	cluster.Global().SetMaster(currentLeader, mHost, mPort, term)
	_ = election.SavePersistedState(election.PersistedState{Term: term, LeaderID: currentLeader})

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

type leaderInfo struct {
	LeaderID   string
	MasterHost string
	MasterPort string
	Term       int
}

func fetchClusterLeaderState() (leaderInfo, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	var bestInfo leaderInfo
	found := false
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
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		termVal, _ := body.Status["term"].(float64)
		term := int(termVal)
		leaderID, _ := body.Status["leader_id"].(string)
		masterURL, _ := body.Status["master_url"].(string)

		mHost, mPort := "", ""
		if masterURL != "" {
			cleanURL := strings.TrimPrefix(masterURL, "http://")
			parts := strings.Split(cleanURL, ":")
			if len(parts) == 2 {
				mHost = parts[0]
				mPort = parts[1]
			}
		}

		if term > bestInfo.Term {
			bestInfo = leaderInfo{
				LeaderID:   leaderID,
				MasterHost: mHost,
				MasterPort: mPort,
				Term:       term,
			}
			found = true
		}
	}
	if !found {
		return leaderInfo{}, fmt.Errorf("no workers reachable")
	}
	return bestInfo, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
