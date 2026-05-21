"""
Worker 2 — Python Flask Service
Shard: Alexandria
Special tasks: Analytics, Report Generation, Data aggregation
"""
import os
import sqlite3
import json
import time
import random
import threading
from datetime import datetime
from flask import Flask, request, jsonify
from flask_cors import CORS
import failover
import urllib.request

app = Flask(__name__)
CORS(app)

NODE_ID = os.environ.get("NODE_ID", "worker-2")
SHARD   = os.environ.get("SHARD", "Alexandria")
PORT    = int(os.environ.get("WORKER_PORT", "8082"))
DB_PATH = f"./data/{NODE_ID}.db"

# ─── Database setup ───────────────────────────────────────────────────────────

def get_db():
    os.makedirs("./data", exist_ok=True)
    conn = sqlite3.connect(DB_PATH, check_same_thread=False)
    conn.row_factory = sqlite3.Row
    return conn

def init_schema():
    conn = get_db()
    conn.execute("""
        CREATE TABLE IF NOT EXISTS client (
            id           INTEGER PRIMARY KEY,
            name         TEXT NOT NULL,
            national_id  TEXT NOT NULL UNIQUE,
            phone        TEXT NOT NULL,
            email        TEXT NOT NULL,
            gender       TEXT NOT NULL,
            birth_date   TEXT NOT NULL,
            city         TEXT NOT NULL,
            address      TEXT NOT NULL,
            account_type TEXT NOT NULL,
            balance      REAL NOT NULL DEFAULT 0,
            created_at   TEXT NOT NULL
        )
    """)
    conn.execute("CREATE INDEX IF NOT EXISTS idx_city ON client(city)")
    conn.commit()
    conn.close()
    print(f"[{NODE_ID}] Schema initialized for shard '{SHARD}'")

# ─── Routes ──────────────────────────────────────────────────────────────────

@app.route("/heartbeat")
def heartbeat():
    """Health probe for master node"""
    import psutil
    cpu = psutil.cpu_percent(interval=None) or random.uniform(5, 40)
    ram = psutil.virtual_memory().percent
    return jsonify({
        "node_id": NODE_ID,
        "role": "slave",
        "language": "python",
        "shard": SHARD,
        "status": "online",
        "cpu": cpu,
        "ram": ram,
        "time": datetime.utcnow().isoformat()
    })


@app.route("/status")
def status():
    conn = get_db()
    count = conn.execute("SELECT COUNT(*) FROM client WHERE city=?", (SHARD,)).fetchone()[0]
    conn.close()
    return jsonify({
        "node_id": NODE_ID,
        "role": "slave",
        "language": "python-flask",
        "shard": SHARD,
        "local_count": count,
        "status": "online"
    })


@app.route("/query", methods=["POST"])
def query():
    data = request.get_json()
    sql  = data.get("sql", "").strip().upper()

    if not sql.startswith("SELECT") and not sql.startswith("WITH"):
        try:
            body = json.dumps({"sql": data.get("sql", "")}).encode()
            req = urllib.request.Request(
                f"{failover.master_url()}/query",
                data=body,
                method="POST",
                headers={"Content-Type": "application/json"},
            )
            with urllib.request.urlopen(req, timeout=10) as resp:
                return jsonify(json.loads(resp.read().decode())), resp.status
        except Exception as e:
            return jsonify({"error": str(e), "master_url": failover.master_url()}), 502

    try:
        conn = get_db()
        cur  = conn.execute(data["sql"])
        cols = [d[0] for d in cur.description] if cur.description else []
        rows = [list(r) for r in cur.fetchall()]
        conn.close()
        return jsonify({"columns": cols, "rows": rows, "node_id": NODE_ID, "shard": SHARD})
    except Exception as e:
        return jsonify({"error": str(e)}), 400


@app.route("/replicate", methods=["POST"])
def replicate():
    """Accept replication events from master"""
    data = request.get_json()
    op   = data.get("op", "UNKNOWN")
    sql  = data.get("sql", "")
    origin = data.get("origin", "master")

    print(f"[{NODE_ID}] Replicating {op} from {origin}")

    try:
        conn = get_db()
        conn.execute(sql)
        conn.commit()
        conn.close()
    except Exception as e:
        print(f"[{NODE_ID}] Replicate note: {e}")

    return jsonify({"node_id": NODE_ID, "status": "ok", "op": op})


@app.route("/election/status")
def election_status():
    return jsonify({"status": failover.get_status()})


@app.route("/election/propose", methods=["POST"])
def election_propose():
    data = request.get_json() or {}
    accepted = failover._apply_propose(data)
    return jsonify({"accepted": accepted, "term": failover.get_status()["term"]})


@app.route("/election/new-master", methods=["POST"])
def new_master():
    data = request.get_json() or {}
    failover.apply_new_master(data)
    print(f"[{NODE_ID}] New master elected: {data.get('new_master')} at {failover.master_url()}")
    return jsonify({"status": "acknowledged", "master_url": failover.master_url()})


@app.route("/data/clients")
def get_clients():
    conn = get_db()
    rows = conn.execute(
        "SELECT id, name, city, account_type, balance FROM client WHERE city=? LIMIT 100",
        (SHARD,)
    ).fetchall()
    conn.close()
    return jsonify({
        "node_id": NODE_ID,
        "shard": SHARD,
        "clients": [dict(r) for r in rows]
    })


@app.route("/analytics/summary")
def analytics_summary():
    """Special analytics endpoint — Python's strength"""
    conn = get_db()
    city = request.args.get("city", SHARD)

    total     = conn.execute("SELECT COUNT(*) FROM client WHERE city=?", (city,)).fetchone()[0]
    total_bal = conn.execute("SELECT SUM(balance) FROM client WHERE city=?", (city,)).fetchone()[0] or 0
    avg_bal   = conn.execute("SELECT AVG(balance) FROM client WHERE city=?", (city,)).fetchone()[0] or 0
    max_bal   = conn.execute("SELECT MAX(balance) FROM client WHERE city=?", (city,)).fetchone()[0] or 0
    min_bal   = conn.execute("SELECT MIN(balance) FROM client WHERE city=?", (city,)).fetchone()[0] or 0

    by_type = conn.execute(
        "SELECT account_type, COUNT(*), SUM(balance) FROM client WHERE city=? GROUP BY account_type",
        (city,)
    ).fetchall()

    by_gender = conn.execute(
        "SELECT gender, COUNT(*) FROM client WHERE city=? GROUP BY gender",
        (city,)
    ).fetchall()

    conn.close()

    return jsonify({
        "node_id": NODE_ID,
        "city": city,
        "analytics": {
            "total_clients": total,
            "total_balance": round(total_bal, 2),
            "avg_balance":   round(avg_bal, 2),
            "max_balance":   round(max_bal, 2),
            "min_balance":   round(min_bal, 2),
            "by_account_type": [
                {"type": r[0], "count": r[1], "total_balance": round(r[2], 2)}
                for r in by_type
            ],
            "by_gender": [{"gender": r[0], "count": r[1]} for r in by_gender]
        }
    })


@app.route("/analytics/top-clients")
def top_clients():
    """Top 10 clients by balance"""
    conn = get_db()
    rows = conn.execute(
        "SELECT id, name, city, account_type, balance FROM client ORDER BY balance DESC LIMIT 10"
    ).fetchall()
    conn.close()
    return jsonify({
        "node_id": NODE_ID,
        "top_clients": [dict(r) for r in rows]
    })


if __name__ == "__main__":
    init_schema()
    failover.start()
    print(f"[{NODE_ID}] Python Flask worker starting on port {PORT} for shard '{SHARD}'")
    app.run(host="0.0.0.0", port=PORT, debug=False, threaded=True)
