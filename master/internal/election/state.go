package election

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const stateFile = "./data/election.state"

// PersistedState is written to disk for term survival across restarts.
type PersistedState struct {
	Term     int    `json:"term"`
	LeaderID string `json:"leader_id"`
}

var persistMu sync.Mutex

func LoadPersistedState() PersistedState {
	persistMu.Lock()
	defer persistMu.Unlock()
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return PersistedState{Term: 1, LeaderID: "master-1"}
	}
	var s PersistedState
	if json.Unmarshal(data, &s) != nil || s.Term < 1 {
		return PersistedState{Term: 1, LeaderID: "master-1"}
	}
	return s
}

func SavePersistedState(s PersistedState) error {
	persistMu.Lock()
	defer persistMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0644)
}
