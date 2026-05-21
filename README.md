# DistribDB — Distributed Database System Using Go

> Graduation Project | Faculty of Computer Science  
> A production-style distributed database system demonstrating master-slave architecture, data sharding, replication, leader election, and fault tolerance.

---

## 📐 System Architecture

```
                        ┌─────────────────────────┐
                        │       MASTER NODE        │
                        │        (Go/Gin)          │
                        │                          │
                        │  ┌──────────────────┐    │
                        │  │  Query Engine    │    │
                        │  │  (SQLite + WAL)  │    │
                        │  └──────────────────┘    │
                        │  ┌──────────────────┐    │
                        │  │ Replication Mgr  │    │
                        │  │ (3 Goroutines)   │    │
                        │  └──────────────────┘    │
                        │  ┌──────────────────┐    │
                        │  │ Health Checker   │    │
                        │  │ (Per-node gorout)│    │
                        │  └──────────────────┘    │
                        │  ┌──────────────────┐    │
                        │  │ Leader Election  │    │
                        │  │ (Bully Algorithm)│    │
                        │  └──────────────────┘    │
                        │  ┌──────────────────┐    │
                        │  │ AI Translator    │    │
                        │  │ (NL → SQL)       │    │
                        │  └──────────────────┘    │
                        └────────────┬────────────┘
                                     │ HTTP + JSON
               ┌─────────────────────┼─────────────────────┐
               │                     │                     │
    ┌──────────▼──────┐   ┌──────────▼──────┐   ┌──────────▼──────┐
    │   WORKER 1      │   │   WORKER 2      │   │   WORKER 3      │
    │   (Go/Gin)      │   │ (Python/Flask)  │   │ (Node.js/Express│
    │                 │   │                 │   │  + Socket.IO)   │
    │  Shard: Cairo   │   │ Shard: Alex.    │   │ Shard: Assiut   │
    │  Port: 8081     │   │  Port: 8082     │   │        + Luxor  │
    │                 │   │  + Analytics    │   │  Port: 8083     │
    │  READ queries   │   │  + Reports      │   │  + WebSockets   │
    └─────────────────┘   └─────────────────┘   └─────────────────┘

                        ┌─────────────────────────┐
                        │    FRONTEND DASHBOARD   │
                        │   (React + Recharts)    │
                        │        Port: 3000       │
                        └─────────────────────────┘
```

---

## 🗃️ Data Sharding (Partitioning)

Data is **NOT** stored identically on every node. Instead it is **sharded by city**:

| City        | Node      | Technology |
|-------------|-----------|------------|
| Cairo       | worker-1  | Go         |
| Alexandria  | worker-2  | Python     |
| Assiut      | worker-3  | Node.js    |
| Luxor       | worker-3  | Node.js    |
| Metadata    | master-1  | Go (master)|

---

## ⚙️ Master-Slave Rules

### Master (Write operations only):
- CREATE database / tables
- INSERT / UPDATE / DELETE
- Replication to all workers
- Leader election management
- Health checking

### Slaves (Read operations only):
- SELECT queries
- Search & filter
- Analytics (worker-2 Python special)
- Real-time streaming (worker-3 Node.js WebSocket)

> If a slave needs to write → it sends the request to master → master executes → master replicates to all

---

## 🔄 Replication Flow

```
Client → POST /query (INSERT/UPDATE/DELETE)
           │
           ▼
        MASTER
        Execute SQL locally
           │
           ├── goroutine 1 → POST /replicate → worker-1 (ACK)
           ├── goroutine 2 → POST /replicate → worker-2 (ACK)
           └── goroutine 3 → POST /replicate → worker-3 (ACK)
                                │
                         Log replication_log table
```

**Goroutines & Channels used:**
- `replCh chan ReplicateRequest` — buffered channel (capacity 100)
- 3 goroutine pool reads from channel concurrently
- `sync.WaitGroup` waits for all ACKs
- Results returned via `chan ReplicateResponse`

---

## ♛ Leader Election (Bully Algorithm)

**Priority order:** `master-1 > worker-3 > worker-2 > worker-1`

```
1. Master goes offline
2. Health checker detects (via heartbeat goroutines, every 4s)
3. Election triggered via channel
4. Highest-priority alive node wins
5. New master notifies all other nodes via /election/new-master
6. System continues operating
```

**Can be triggered manually** from the dashboard → "Leader Election" page.

---

## 💡 Goroutines & Channels Summary

| Location | Usage |
|----------|-------|
| `health.StartHeartbeatLoop` | 1 goroutine per node → parallel pinging |
| `replication.StartReplicationWorker` | Pool of 3 goroutines reading from `replCh` |
| `replication.replicateToAll` | 1 goroutine per worker → concurrent ACKs |
| `election.MonitorLeadership` | Background goroutine watching for failures |
| `replCh` | Buffered channel (cap 100) for replication queue |
| `results chan ReplicateResponse` | Goroutine-safe ACK collection |

---

## 🛠️ Technology Stack

| Component  | Technology       | Role |
|------------|------------------|------|
| Master     | Go + Gin         | Write controller, cluster manager |
| Worker 1   | Go + Gin         | Cairo shard, read queries |
| Worker 2   | Python + Flask   | Alexandria shard, analytics |
| Worker 3   | Node.js + Express + Socket.IO | Assiut/Luxor, WebSockets, streaming |
| Database   | SQLite (WAL mode)| Local storage per node |
| Frontend   | React + Recharts | Dashboard, real-time monitoring |
| Container  | Docker Compose   | Orchestration |

---

## 🚀 Quick Start

### Option 1: Docker Compose (Recommended)

```bash
# 1. Clone the project
git clone https://github.com/yourname/distributed-db
cd distributed-db

# 2. Copy bank data
cp /path/to/bank_row_by_row.sql ./data/

# 3. Start everything
docker-compose up --build

# 4. Open dashboard
open http://localhost:3000

# Access nodes directly:
# Master:  http://localhost:8080
# Worker1: http://localhost:8081
# Worker2: http://localhost:8082
# Worker3: http://localhost:8083
```

### Option 2: Local Development

```bash
# ── MASTER ──────────────────────────────────────────────────
cd master
go mod tidy
go run ./cmd/main.go

# ── WORKER 1 (Go) ───────────────────────────────────────────
cd worker1
go mod tidy
NODE_ID=worker-1 SHARD=Cairo WORKER_PORT=8081 go run main.go

# ── WORKER 2 (Python) ───────────────────────────────────────
cd worker2
pip install -r requirements.txt
NODE_ID=worker-2 SHARD=Alexandria WORKER_PORT=8082 python app.py

# ── WORKER 3 (Node.js) ──────────────────────────────────────
cd worker3
npm install
NODE_ID=worker-3 SHARD="Assiut,Luxor" WORKER_PORT=8083 node index.js

# ── FRONTEND ────────────────────────────────────────────────
cd frontend
npm install
npm start
```

---

## 📡 API Reference

### Master Node (port 8080)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/heartbeat` | Node liveness check |
| GET | `/status` | Full node status |
| POST | `/query` | Execute SQL query |
| POST | `/ai/translate` | NL → SQL translation |
| POST | `/ai/query` | NL → SQL → Execute |
| GET | `/cluster/nodes` | All cluster nodes |
| GET | `/cluster/shards` | Shard distribution |
| GET | `/cluster/health` | Real-time health |
| GET | `/cluster/replication-log` | Recent replications |
| GET | `/election/status` | Leader election state |
| POST | `/election/trigger` | Start election |
| POST | `/election/failover` | Manual failover |
| GET | `/data/clients` | Paginated client list |
| POST | `/data/clients` | Create client |
| DELETE | `/data/clients/:id` | Delete client |
| GET | `/data/stats` | Aggregate statistics |
| GET | `/tables` | List tables |
| POST | `/tables` | Create table |
| DELETE | `/tables/:name` | Drop table |

### All Worker Nodes

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/heartbeat` | Health ping |
| GET | `/status` | Node status + shard count |
| POST | `/query` | Read-only SQL (SELECT only) |
| POST | `/replicate` | Receive replication from master |
| POST | `/election/new-master` | Notify new master elected |
| GET | `/data/clients` | Shard-local clients |

### Worker 2 (Python) — Special Analytics

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/analytics/summary?city=Alexandria` | Full analytics report |
| GET | `/analytics/top-clients` | Top 10 by balance |

### Worker 3 (Node.js) — Special Streaming

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/stream/live` | SSE real-time data stream |
| WS | `socket.io` | WebSocket live updates |

---

## 📊 Example API Calls

```bash
# Execute SQL query
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"sql": "SELECT * FROM client WHERE city = '\''Cairo'\'' LIMIT 5"}'

# AI natural language query
curl -X POST http://localhost:8080/ai/query \
  -H "Content-Type: application/json" \
  -d '{"query": "Show top 5 richest clients from Alexandria"}'

# Get cluster health
curl http://localhost:8080/cluster/health

# Trigger failover to worker-2
curl -X POST http://localhost:8080/election/failover \
  -H "Content-Type: application/json" \
  -d '{"target_node": "worker-2"}'

# Get shard distribution
curl http://localhost:8080/cluster/shards
```

---

## 🗂️ Project Structure

```
distributed-db/
│
├── master/                         # Master Node (Go)
│   ├── cmd/
│   │   └── main.go                 # Entry point
│   ├── internal/
│   │   ├── api/
│   │   │   ├── routes.go           # All HTTP handlers
│   │   │   └── ai_translator.go    # NL → SQL engine
│   │   ├── query/
│   │   │   └── engine.go           # SQLite wrapper + sharding
│   │   ├── replication/
│   │   │   └── manager.go          # Goroutine-based replication
│   │   ├── health/
│   │   │   └── checker.go          # Per-node heartbeat goroutines
│   │   └── election/
│   │       └── leader.go           # Bully algorithm
│   ├── go.mod
│   └── Dockerfile
│
├── worker1/                        # Worker 1 — Go — Cairo
│   ├── main.go
│   ├── go.mod
│   └── Dockerfile
│
├── worker2/                        # Worker 2 — Python — Alexandria
│   ├── app.py
│   ├── requirements.txt
│   └── Dockerfile
│
├── worker3/                        # Worker 3 — Node.js — Assiut+Luxor
│   ├── index.js
│   ├── package.json
│   └── Dockerfile
│
├── frontend/                       # React Dashboard
│   ├── public/index.html
│   ├── src/
│   │   ├── App.jsx                 # Main dashboard (all pages)
│   │   ├── index.js
│   │   └── utils/api.js            # API service layer
│   ├── package.json
│   ├── Dockerfile
│   └── nginx.conf
│
├── data/
│   └── bank_row_by_row.sql         # 3000-row bank dataset
│
├── docker-compose.yml
└── README.md
```

---

## 🧪 Test Scenarios

### 1. Write replication
```bash
# Insert via master
curl -X POST http://localhost:8080/data/clients \
  -H "Content-Type: application/json" \
  -d '{"name":"Ahmed Ali","national_id":"12345678901234","phone":"01012345678","email":"ahmed@test.com","gender":"Male","birth_date":"1990-01-01","city":"Cairo","address":"Test St","account_type":"Savings","balance":50000}'

# Verify on worker-1 (Cairo shard)
curl http://localhost:8081/data/clients
```

### 2. Leader election failover
```bash
# Stop master container
docker stop master

# Trigger election (from worker perspective)
curl -X POST http://localhost:8081/election/trigger

# Or use dashboard → Leader Election → "Failover to worker-2"
```

### 3. AI query
```bash
curl -X POST http://localhost:8080/ai/query \
  -d '{"query":"Show all female clients from Assiut with savings accounts"}'
```

### 4. Shard query
```bash
# Query only Assiut data from worker-3 directly
curl -X POST http://localhost:8083/query \
  -d '{"sql":"SELECT name, balance FROM client WHERE city='\''Assiut'\'' ORDER BY balance DESC LIMIT 10"}'
```

---

## 👨‍💻 Concurrency Architecture Deep-Dive

### Goroutine Graph
```
main()
├── go hc.StartHeartbeatLoop(ctx)
│       ├── go ping(worker-1) [ticker: 4s]
│       ├── go ping(worker-2) [ticker: 4s]
│       └── go ping(worker-3) [ticker: 4s]
│
├── go rm.StartReplicationWorker(ctx)
│       ├── go replicator-0  ← reads from replCh
│       ├── go replicator-1  ← reads from replCh
│       └── go replicator-2  ← reads from replCh
│                   │
│                   └── replicateToAll(req)
│                           ├── go sendToWorker(worker-1) → results chan
│                           ├── go sendToWorker(worker-2) → results chan
│                           └── go sendToWorker(worker-3) → results chan
│
└── go le.MonitorLeadership(ctx)
        ├── ticker.C → checkLeadership()
        └── electionCh → runElection()
```

---

*Built with Go, Python, Node.js, React — Graduation Project 2024/2025*
