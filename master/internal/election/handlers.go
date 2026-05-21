package election

import (
	"log"
	"net/http"

	"distributed-db/master/internal/cluster"

	"github.com/gin-gonic/gin"
)

// HandlerDeps bundles election HTTP dependencies.
type HandlerDeps struct {
	Promoter    Promoter
	Coordinator *Coordinator
	OnStepDown  func(term int)
}

// RegisterHandlers mounts distributed election endpoints.
func RegisterHandlers(r *gin.Engine, deps *HandlerDeps) {
	r.GET("/election/cluster", func(c *gin.Context) {
		c.JSON(http.StatusOK, cluster.Global().Snapshot())
	})

	r.POST("/election/propose", func(c *gin.Context) {
		var req proposeBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"accepted": false})
			return
		}
		accepted := cluster.Global().AcceptPropose(req.Term, req.LeaderID, req.MasterHost, req.MasterPort)
		if accepted {
			_ = SavePersistedState(PersistedState{Term: req.Term, LeaderID: req.LeaderID})
			log.Printf("[FAILOVER] Accepted proposal term=%d leader=%s", req.Term, req.LeaderID)
		}
		c.JSON(http.StatusOK, gin.H{
			"accepted": accepted,
			"term":     cluster.Global().GetTerm(),
		})
	})

	r.POST("/election/new-master", func(c *gin.Context) {
		var req struct {
			Term       int    `json:"term"`
			NewMaster  string `json:"new_master"`
			MasterHost string `json:"master_host"`
			MasterPort string `json:"master_port"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		if req.Term <= cluster.Global().GetTerm() {
			c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "stale term"})
			return
		}
		cluster.Global().SetMaster(req.NewMaster, req.MasterHost, req.MasterPort, req.Term)
		_ = SavePersistedState(PersistedState{Term: req.Term, LeaderID: req.NewMaster})
		log.Printf("[FAILOVER] New master: %s at %s:%s (term %d)",
			req.NewMaster, req.MasterHost, req.MasterPort, req.Term)

		if deps != nil && deps.Coordinator != nil && deps.Promoter != nil {
			if req.NewMaster == cluster.Global().SelfID() && !deps.Promoter.IsRunning() {
				if n, ok := cluster.Global().NodeByID(req.NewMaster); ok && n.CanPromote {
					go deps.Coordinator.promoteToMaster(req.Term, req.MasterHost, req.MasterPort)
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"status":     "acknowledged",
			"master_url": cluster.Global().MasterURL(),
		})
	})

	r.POST("/election/step-down", func(c *gin.Context) {
		var req struct {
			Term int `json:"term"`
		}
		_ = c.ShouldBindJSON(&req)
		if deps != nil && deps.OnStepDown != nil {
			deps.OnStepDown(req.Term)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/election/failback", func(c *gin.Context) {
		if deps != nil && deps.Coordinator != nil && deps.Promoter != nil {
			if deps.Promoter.IsRunning() && cluster.Global().SelfID() != "master-1" {
				masterNode, ok := cluster.Global().NodeByID("master-1")
				if ok {
					log.Printf("[FAILOVER] Explicit failback requested via API.")
					go deps.Coordinator.ExecuteFailback(masterNode)
					c.JSON(http.StatusOK, gin.H{"status": "failback_initiated", "target": "master-1"})
					return
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "not_promoted_master_or_invalid_state"})
	})
}
