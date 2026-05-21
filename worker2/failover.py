"""
Automatic master failure detection and distributed election (worker side).
"""
import json
import os
import threading
import time
import urllib.request
import urllib.error

NODE_ID = os.environ.get("NODE_ID", "worker-2")
SELF_HOST = os.environ.get("WORKER2_HOST", os.environ.get("SELF_HOST", "127.0.0.1"))
WORKER_PORT = os.environ.get("WORKER_PORT", "8082")
MASTER_HOST = os.environ.get("MASTER_HOST", "127.0.0.1")
MASTER_PORT = os.environ.get("MASTER_PORT", "8888")
WORKER1_HOST = os.environ.get("WORKER1_HOST", "127.0.0.1")
WORKER1_PORT = os.environ.get("WORKER1_PORT", "8081")
WORKER3_HOST = os.environ.get("WORKER3_HOST", "127.0.0.1")
WORKER3_PORT = os.environ.get("WORKER3_PORT", "8083")

PING_INTERVAL = 3
DEAD_SECONDS = 10
ELECTION_COOLDOWN = 5

_state = {
    "term": 1,
    "leader_id": "master-1",
    "master_host": MASTER_HOST,
    "master_port": MASTER_PORT,
    "electing": False,
}
_lock = threading.Lock()
_last_ok = time.time()
_last_election = 0.0
_master_dead = False

PEERS = [
    {"id": "worker-1", "host": WORKER1_HOST, "port": WORKER1_PORT},
    {"id": "worker-2", "host": SELF_HOST, "port": WORKER_PORT},
    {"id": "worker-3", "host": WORKER3_HOST, "port": WORKER3_PORT},
]
PRIORITY = ["worker-3", "worker-2", "worker-1"]
PROMOTABLE = {"worker-1"}


def master_url():
    with _lock:
        return f"http://{_state['master_host']}:{_state['master_port']}"


def get_status():
    with _lock:
        return {
            "node_id": NODE_ID,
            "term": _state["term"],
            "leader_id": _state["leader_id"],
            "master_url": master_url(),
            "master_host": _state["master_host"],
            "master_port": _state["master_port"],
        }


def _http(method, url, data=None, timeout=3):
    body = json.dumps(data).encode() if data is not None else None
    req = urllib.request.Request(
        url, data=body, method=method,
        headers={"Content-Type": "application/json"} if body else {},
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode())


def _ping(url):
    try:
        _http("GET", url)
        return True
    except Exception:
        return False


def _alive_peers():
    alive = {NODE_ID: True}
    for p in PEERS:
        if p["id"] == NODE_ID:
            continue
        if _ping(f"http://{p['host']}:{p['port']}/heartbeat"):
            alive[p["id"]] = True
    return alive


def _max_term():
    t = _state["term"]
    for p in PEERS:
        if p["id"] == NODE_ID:
            continue
        try:
            r = _http("GET", f"http://{p['host']}:{p['port']}/election/status")
            term = r.get("status", {}).get("term", 0)
            if term > t:
                t = term
        except Exception:
            pass
    return t


def _select_coordinator(alive):
    for nid in ["worker-3", "worker-2", "worker-1"]:
        if alive.get(nid):
            return nid
    return NODE_ID


def _select_winner(alive):
    for nid in ["worker-3", "worker-2", "worker-1"]:
        if alive.get(nid) and nid in PROMOTABLE:
            return nid
    return None


def _propose(term, leader, host, port):
    body = {"term": term, "leader_id": leader, "master_host": host, "master_port": port}
    acks = 0
    if _apply_propose(body):
        acks += 1
    for p in PEERS:
        if p["id"] == NODE_ID:
            continue
        try:
            r = _http("POST", f"http://{p['host']}:{p['port']}/election/propose", body)
            if r.get("accepted"):
                acks += 1
        except Exception:
            pass
    return acks


def _apply_propose(data):
    global _state
    with _lock:
        if data["term"] <= _state["term"]:
            return False
        _state["term"] = data["term"]
        _state["leader_id"] = data["leader_id"]
        _state["master_host"] = data["master_host"]
        _state["master_port"] = data["master_port"]
        return True


def apply_new_master(data):
    with _lock:
        if data.get("term", 0) <= _state["term"]:
            return False
        _state["term"] = data["term"]
        _state["leader_id"] = data.get("new_master", data.get("leader_id", _state["leader_id"]))
        _state["master_host"] = data.get("master_host", _state["master_host"])
        _state["master_port"] = data.get("master_port", _state["master_port"])
        return True


def _broadcast_new_master(term, leader, host, port):
    body = {"term": term, "new_master": leader, "master_host": host, "master_port": port}
    for p in PEERS:
        if p["id"] == NODE_ID:
            continue
        try:
            _http("POST", f"http://{p['host']}:{p['port']}/election/new-master", body)
        except Exception:
            pass


def run_election():
    global _last_election, _master_dead
    now = time.time()
    if now - _last_election < ELECTION_COOLDOWN:
        return
    with _lock:
        if _state["electing"]:
            return
        _state["electing"] = True
    _last_election = now

    try:
        alive = _alive_peers()
        coord = _select_coordinator(alive)
        if coord != NODE_ID:
            print(f"[{NODE_ID}] Election coordinator is {coord}")
            return

        winner = _select_winner(alive)
        if not winner:
            print(f"[{NODE_ID}] No promotable master candidate")
            return

        new_term = _max_term() + 1
        host = WORKER1_HOST if winner == "worker-1" else SELF_HOST
        port = MASTER_PORT

        acks = _propose(new_term, winner, host, port)
        quorum = 2
        print(f"[{NODE_ID}] Election term={new_term} leader={winner} acks={acks}/{quorum}")

        if acks < quorum:
            return

        _broadcast_new_master(new_term, winner, host, port)
        print(f"[{NODE_ID}] Failover complete → master at {host}:{port}")
    finally:
        with _lock:
            _state["electing"] = False


def _monitor_loop():
    global _last_ok, _master_dead
    print(f"[{NODE_ID}] Master monitor started (dead threshold={DEAD_SECONDS}s)")
    while True:
        if _ping(f"{master_url()}/heartbeat"):
            _last_ok = time.time()
            if _master_dead:
                print(f"[{NODE_ID}] Master is back online")
            _master_dead = False
        elif time.time() - _last_ok >= DEAD_SECONDS:
            if not _master_dead:
                _master_dead = True
                print(f"[{NODE_ID}] MASTER DEAD — triggering automatic election")
                threading.Thread(target=run_election, daemon=True).start()
        time.sleep(PING_INTERVAL)


def start():
    t = threading.Thread(target=_monitor_loop, daemon=True)
    t.start()
