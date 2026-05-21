// Worker 1 moved to master module for failover integration.
// Run from the master directory:
//
//   cd master
//   go run ./cmd/worker1
//
// Or: go build -o worker1.exe ./cmd/worker1
package main

import "log"

func main() {
	log.Fatal("Run worker-1 from the master folder: cd master && go run ./cmd/worker1")
}
