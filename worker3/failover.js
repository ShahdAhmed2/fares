/**
 * Automatic master failure detection and distributed election (worker side).
 */
const http = require('http');

const NODE_ID = process.env.NODE_ID || 'worker-3';
const SELF_HOST = process.env.WORKER3_HOST || process.env.SELF_HOST || '127.0.0.1';
const WORKER_PORT = process.env.WORKER_PORT || '8083';
const MASTER_HOST = process.env.MASTER_HOST || '127.0.0.1';
const MASTER_PORT = process.env.MASTER_PORT || '8888';
const WORKER1_HOST = process.env.WORKER1_HOST || '127.0.0.1';
const WORKER1_PORT = process.env.WORKER1_PORT || '8081';
const WORKER2_HOST = process.env.WORKER2_HOST || '127.0.0.1';
const WORKER2_PORT = process.env.WORKER2_PORT || '8082';

const PING_INTERVAL = 3000;
const DEAD_MS = 10000;
const ELECTION_COOLDOWN = 5000;

const state = {
  term: 1,
  leader_id: 'master-1',
  master_host: MASTER_HOST,
  master_port: MASTER_PORT,
  electing: false,
};

const PEERS = [
  { id: 'worker-1', host: WORKER1_HOST, port: WORKER1_PORT },
  { id: 'worker-2', host: WORKER2_HOST, port: WORKER2_PORT },
  { id: 'worker-3', host: SELF_HOST, port: WORKER_PORT },
];
const PROMOTABLE = new Set(['worker-1']);

let lastOk = Date.now();
let lastElection = 0;
let masterDead = false;

function masterUrl() {
  return `http://${state.master_host}:${state.master_port}`;
}

function getStatus() {
  return {
    node_id: NODE_ID,
    term: state.term,
    leader_id: state.leader_id,
    master_url: masterUrl(),
    master_host: state.master_host,
    master_port: state.master_port,
  };
}

function request(method, url, body) {
  return new Promise((resolve, reject) => {
    const u = new URL(url);
    const data = body ? JSON.stringify(body) : null;
    const req = http.request({
      hostname: u.hostname,
      port: u.port,
      path: u.pathname,
      method,
      headers: data ? { 'Content-Type': 'application/json', 'Content-Length': data.length } : {},
      timeout: 3000,
    }, res => {
      let raw = '';
      res.on('data', c => raw += c);
      res.on('end', () => {
        try { resolve(JSON.parse(raw || '{}')); } catch { resolve({}); }
      });
    });
    req.on('error', reject);
    req.on('timeout', () => { req.destroy(); reject(new Error('timeout')); });
    if (data) req.write(data);
    req.end();
  });
}

async function ping(url) {
  try {
    await request('GET', url);
    return true;
  } catch {
    return false;
  }
}

async function alivePeers() {
  const alive = { [NODE_ID]: true };
  for (const p of PEERS) {
    if (p.id === NODE_ID) continue;
    if (await ping(`http://${p.host}:${p.port}/heartbeat`)) alive[p.id] = true;
  }
  return alive;
}

async function maxTerm() {
  let t = state.term;
  for (const p of PEERS) {
    if (p.id === NODE_ID) continue;
    try {
      const r = await request('GET', `http://${p.host}:${p.port}/election/status`);
      const term = r.status?.term || 0;
      if (term > t) t = term;
    } catch {}
  }
  return t;
}

function selectCoordinator(alive) {
  for (const id of ['worker-3', 'worker-2', 'worker-1']) {
    if (alive[id]) return id;
  }
  return NODE_ID;
}

function selectWinner(alive) {
  for (const id of ['worker-3', 'worker-2', 'worker-1']) {
    if (alive[id] && PROMOTABLE.has(id)) return id;
  }
  return null;
}

function applyPropose(data) {
  if (data.term <= state.term) return false;
  state.term = data.term;
  state.leader_id = data.leader_id;
  state.master_host = data.master_host;
  state.master_port = data.master_port;
  return true;
}

function applyNewMaster(data) {
  if ((data.term || 0) <= state.term) return false;
  state.term = data.term;
  state.leader_id = data.new_master || state.leader_id;
  state.master_host = data.master_host || state.master_host;
  state.master_port = data.master_port || state.master_port;
  return true;
}

async function runElection() {
  const now = Date.now();
  if (now - lastElection < ELECTION_COOLDOWN || state.electing) return;
  state.electing = true;
  lastElection = now;

  try {
    const alive = await alivePeers();
    const coord = selectCoordinator(alive);
    if (coord !== NODE_ID) {
      console.log(`[${NODE_ID}] Election coordinator is ${coord}`);
      return;
    }

    const winner = selectWinner(alive);
    if (!winner) {
      console.log(`[${NODE_ID}] No promotable master candidate`);
      return;
    }

    const newTerm = (await maxTerm()) + 1;
    const host = winner === 'worker-1' ? WORKER1_HOST : SELF_HOST;
    const port = MASTER_PORT;
    const body = { term: newTerm, leader_id: winner, master_host: host, master_port: port };

    let acks = applyPropose(body) ? 1 : 0;
    for (const p of PEERS) {
      if (p.id === NODE_ID) continue;
      try {
        const r = await request('POST', `http://${p.host}:${p.port}/election/propose`, body);
        if (r.accepted) acks++;
      } catch {}
    }

    const quorum = 2;
    console.log(`[${NODE_ID}] Election term=${newTerm} leader=${winner} acks=${acks}/${quorum}`);
    if (acks < quorum) return;

    for (const p of PEERS) {
      if (p.id === NODE_ID) continue;
      try {
        await request('POST', `http://${p.host}:${p.port}/election/new-master`, {
          term: newTerm, new_master: winner, master_host: host, master_port: port,
        });
      } catch {}
    }
    console.log(`[${NODE_ID}] Failover complete → master at ${host}:${port}`);
  } finally {
    state.electing = false;
  }
}

function startMonitor() {
  console.log(`[${NODE_ID}] Master monitor started (dead threshold=${DEAD_MS}ms)`);
  setInterval(async () => {
    if (await ping(`${masterUrl()}/heartbeat`)) {
      lastOk = Date.now();
      if (masterDead) console.log(`[${NODE_ID}] Master is back online`);
      masterDead = false;
    } else if (Date.now() - lastOk >= DEAD_MS) {
      if (!masterDead) {
        masterDead = true;
        console.log(`[${NODE_ID}] MASTER DEAD — triggering automatic election`);
        runElection();
      }
    }
  }, PING_INTERVAL);
}

module.exports = { masterUrl, getStatus, applyPropose, applyNewMaster, startMonitor, request };
