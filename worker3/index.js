/**
 * Worker 3 — Node.js Express Service
 * Shard: Assiut + Luxor
 * Special tasks: Real-time data streaming, WebSocket support, Background jobs
 */

const express = require('express');
const cors    = require('cors');
const http    = require('http');
const { Server } = require('socket.io');
const Database = require('better-sqlite3');
const os = require('os');
const path = require('path');
const fs = require('fs');
const failover = require('./failover');

const NODE_ID = process.env.NODE_ID    || 'worker-3';
const SHARD   = process.env.SHARD      || 'Assiut,Luxor';
const PORT    = parseInt(process.env.WORKER_PORT || '8083');
const DB_PATH = `./data/${NODE_ID}.db`;

// ─── Setup ──────────────────────────────────────────────────────────────────

fs.mkdirSync('./data', { recursive: true });

const db  = new Database(DB_PATH, { verbose: null });
const app = express();
const srv = http.createServer(app);
const io  = new Server(srv, { cors: { origin: '*' } });

app.use(cors());
app.use(express.json());

// ─── Schema ──────────────────────────────────────────────────────────────────

db.exec(`
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
  );
  CREATE INDEX IF NOT EXISTS idx_city ON client(city);
`);

console.log(`[${NODE_ID}] Schema initialized for shard '${SHARD}'`);

// ─── Routes ──────────────────────────────────────────────────────────────────

app.get('/heartbeat', (req, res) => {
  const cpus  = os.cpus();
  const loads = os.loadavg();
  const cpu   = Math.min(100, (loads[0] / cpus.length) * 100) || (Math.random() * 35 + 5);
  const ram   = (1 - os.freemem() / os.totalmem()) * 100;

  res.json({
    node_id:  NODE_ID,
    role:     'slave',
    language: 'nodejs',
    shard:    SHARD,
    status:   'online',
    cpu:      parseFloat(cpu.toFixed(1)),
    ram:      parseFloat(ram.toFixed(1)),
    time:     new Date().toISOString()
  });
});

app.get('/status', (req, res) => {
  const cities  = SHARD.split(',').map(c => `'${c.trim()}'`).join(',');
  const count   = db.prepare(`SELECT COUNT(*) as cnt FROM client WHERE city IN (${cities})`).get();
  res.json({
    node_id:     NODE_ID,
    role:        'slave',
    language:    'nodejs-express',
    shard:       SHARD,
    local_count: count.cnt,
    status:      'online'
  });
});

app.post('/query', async (req, res) => {
  const sql    = (req.body.sql || '').trim().toUpperCase();

  if (!sql.startsWith('SELECT') && !sql.startsWith('WITH')) {
    try {
      const r = await failover.request('POST', `${failover.masterUrl()}/query`, { sql: req.body.sql });
      return res.json(r);
    } catch (e) {
      return res.status(502).json({ error: e.message, master_url: failover.masterUrl() });
    }
  }

  try {
    const stmt = db.prepare(req.body.sql);
    const cols = stmt.columns().map(c => c.name);
    const rows = stmt.all().map(r => Object.values(r));
    res.json({ columns: cols, rows, node_id: NODE_ID, shard: SHARD });
  } catch (e) {
    res.status(400).json({ error: e.message });
  }
});

app.post('/replicate', (req, res) => {
  const { op, sql, origin } = req.body;
  console.log(`[${NODE_ID}] Replicating ${op} from ${origin}`);

  try {
    db.exec(sql);
  } catch (e) {
    console.log(`[${NODE_ID}] Replicate note: ${e.message}`);
  }

  // Broadcast to connected WebSocket clients
  io.emit('replication', { op, origin, time: new Date().toISOString() });

  res.json({ node_id: NODE_ID, status: 'ok', op });
});

app.get('/election/status', (req, res) => {
  res.json({ status: failover.getStatus() });
});

app.post('/election/propose', (req, res) => {
  const accepted = failover.applyPropose(req.body || {});
  res.json({ accepted, term: failover.getStatus().term });
});

app.post('/election/new-master', (req, res) => {
  failover.applyNewMaster(req.body || {});
  const { new_master } = req.body || {};
  console.log(`[${NODE_ID}] New master elected: ${new_master} at ${failover.masterUrl()}`);
  io.emit('election', { new_master, master_url: failover.masterUrl(), time: new Date().toISOString() });
  res.json({ status: 'acknowledged', master_url: failover.masterUrl() });
});

app.get('/data/clients', (req, res) => {
  const cities = SHARD.split(',').map(c => `'${c.trim()}'`).join(',');
  const rows   = db.prepare(
    `SELECT id, name, city, account_type, balance FROM client WHERE city IN (${cities}) LIMIT 100`
  ).all();
  res.json({ node_id: NODE_ID, shard: SHARD, clients: rows });
});

// Special: streaming analytics via SSE
app.get('/stream/live', (req, res) => {
  res.setHeader('Content-Type', 'text/event-stream');
  res.setHeader('Cache-Control', 'no-cache');
  res.setHeader('Connection', 'keep-alive');
  res.flushHeaders();

  const cities = SHARD.split(',').map(c => `'${c.trim()}'`).join(',');
  const interval = setInterval(() => {
    const count = db.prepare(`SELECT COUNT(*) as cnt FROM client WHERE city IN (${cities})`).get();
    const bal   = db.prepare(`SELECT SUM(balance) as total FROM client WHERE city IN (${cities})`).get();
    res.write(`data: ${JSON.stringify({
      node_id: NODE_ID,
      count: count.cnt,
      total_balance: bal.total,
      timestamp: new Date().toISOString()
    })}\n\n`);
  }, 3000);

  req.on('close', () => clearInterval(interval));
});

// ─── WebSocket ───────────────────────────────────────────────────────────────

io.on('connection', socket => {
  console.log(`[${NODE_ID}] WebSocket client connected`);
  socket.emit('welcome', { node_id: NODE_ID, shard: SHARD });

  // Background job: push random stats every 5s
  const job = setInterval(() => {
    const cities = SHARD.split(',').map(c => `'${c.trim()}'`).join(',');
    try {
      const count = db.prepare(`SELECT COUNT(*) as cnt FROM client WHERE city IN (${cities})`).get();
      socket.emit('stats', { count: count.cnt, time: new Date().toISOString() });
    } catch {}
  }, 5000);

  socket.on('disconnect', () => {
    clearInterval(job);
    console.log(`[${NODE_ID}] Client disconnected`);
  });
});

// ─── Start ───────────────────────────────────────────────────────────────────

failover.startMonitor();

srv.listen(PORT, '0.0.0.0', () => {
  console.log(`[${NODE_ID}] Node.js/Express worker online at :${PORT} serving shard '${SHARD}'`);
});
