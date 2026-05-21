import React, { useState, useEffect, useCallback } from 'react';
import { BarChart, Bar, PieChart, Pie, Cell, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from 'recharts';
import { api } from './utils/api';

const MASTER_PASSWORD = 'master123';

const css = `
  *,*::before,*::after{box-sizing:border-box;margin:0;padding:0;}
  :root{--bg:#080c14;--surface:#0f1724;--surface2:#162035;--border:#1e2d4a;--accent:#3b82f6;--accent2:#06b6d4;--green:#10b981;--red:#ef4444;--amber:#f59e0b;--purple:#8b5cf6;--text:#e2e8f0;--muted:#64748b;}
  body{font-family:'Space Grotesk',sans-serif;background:var(--bg);color:var(--text);}
  .app{display:flex;min-height:100vh;}
  .login-wrap{min-height:100vh;display:flex;align-items:center;justify-content:center;}
  .login-card{background:var(--surface);border:1px solid var(--border);border-radius:16px;padding:40px;width:420px;}
  .login-logo{font-size:28px;font-weight:700;color:var(--accent);margin-bottom:4px;}
  .login-logo span{color:var(--accent2);}
  .login-sub{color:var(--muted);font-size:14px;margin-bottom:28px;}
  .role-wrap{display:flex;gap:10px;margin-bottom:24px;}
  .role-btn{flex:1;padding:12px;border-radius:10px;border:2px solid var(--border);background:transparent;color:var(--muted);cursor:pointer;font-size:14px;font-weight:600;}
  .role-btn.active{border-color:var(--accent);color:var(--accent);background:rgba(59,130,246,.08);}
  .l-input{width:100%;background:var(--surface2);border:1px solid var(--border);border-radius:8px;padding:12px 14px;color:var(--text);font-size:14px;outline:none;margin-bottom:12px;}
  .l-input:focus{border-color:var(--accent);}
  .l-btn{width:100%;padding:13px;background:linear-gradient(135deg,var(--accent),var(--accent2));border:none;border-radius:10px;color:#fff;font-size:15px;font-weight:600;cursor:pointer;margin-top:6px;}
  .l-err{color:var(--red);font-size:13px;margin-top:10px;text-align:center;}
  .l-hint{color:var(--muted);font-size:12px;margin-top:14px;text-align:center;}
  .sidebar{width:220px;min-height:100vh;background:var(--surface);border-right:1px solid var(--border);padding:24px 0;display:flex;flex-direction:column;flex-shrink:0;}
  .logo{padding:0 20px 20px;font-size:18px;font-weight:700;color:var(--accent);border-bottom:1px solid var(--border);}
  .logo span{color:var(--accent2);}
  .logo-role{font-size:11px;color:var(--muted);margin-top:3px;}
  .nav{flex:1;padding-top:14px;}
  .nav-item{display:flex;align-items:center;gap:12px;padding:11px 20px;cursor:pointer;font-size:14px;font-weight:500;color:var(--muted);transition:all .15s;border-left:3px solid transparent;}
  .nav-item:hover{color:var(--text);background:var(--surface2);}
  .nav-item.active{color:var(--accent);border-left-color:var(--accent);background:rgba(59,130,246,.08);}
  .nav-icon{font-size:16px;width:20px;text-align:center;}
  .logout-btn{margin:14px 20px 0;padding:9px;background:rgba(239,68,68,.1);border:1px solid rgba(239,68,68,.2);color:var(--red);border-radius:8px;cursor:pointer;font-size:13px;font-weight:600;}
  .main{flex:1;overflow-y:auto;padding:28px 32px;}
  .page-title{font-size:22px;font-weight:700;margin-bottom:24px;}
  .page-title span{color:var(--muted);font-size:14px;font-weight:400;margin-left:12px;}
  .card{background:var(--surface);border:1px solid var(--border);border-radius:12px;padding:20px;}
  .card-title{font-size:12px;font-weight:600;color:var(--muted);text-transform:uppercase;letter-spacing:.8px;margin-bottom:12px;}
  .grid{display:grid;gap:16px;}
  .grid-4{grid-template-columns:repeat(4,1fr);}
  .grid-2{grid-template-columns:1fr 1fr;}
  .mb16{margin-bottom:16px;}.mt16{margin-top:16px;}
  .banner{background:rgba(245,158,11,.1);border:1px solid rgba(245,158,11,.3);border-radius:10px;padding:13px 18px;margin-bottom:20px;font-size:13px;color:var(--amber);font-weight:500;}
  .nodes-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:14px;}
  .node-card{background:var(--surface);border:1px solid var(--border);border-radius:10px;padding:16px;}
  .node-card.online{border-left:3px solid var(--green);}
  .node-card.offline{border-left:3px solid var(--red);opacity:.7;}
  .nbadge{display:inline-block;padding:2px 8px;border-radius:20px;font-size:10px;font-weight:600;text-transform:uppercase;}
  .bm{background:rgba(59,130,246,.15);color:var(--accent);}
  .bs{background:rgba(16,185,129,.15);color:var(--green);}
  .bgo{background:rgba(6,182,212,.15);color:var(--accent2);}
  .bpy{background:rgba(245,158,11,.15);color:var(--amber);}
  .bno{background:rgba(139,92,246,.15);color:var(--purple);}
  .pulse{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:6px;}
  .pulse.g{background:var(--green);animation:pulse 2s infinite;}
  .pulse.r{background:var(--red);}
  @keyframes pulse{0%,100%{opacity:1}50%{opacity:.4}}
  .meter{height:4px;background:var(--border);border-radius:2px;margin-top:4px;overflow:hidden;}
  .mf{height:100%;border-radius:2px;transition:width .5s;}
  .mc{background:linear-gradient(90deg,var(--accent),var(--accent2));}
  .mr{background:linear-gradient(90deg,var(--purple),var(--red));}
  .terminal{background:#000913;border:1px solid var(--border);border-radius:10px;padding:16px;font-family:'JetBrains Mono',monospace;font-size:13px;}
  .t-header{display:flex;gap:6px;margin-bottom:12px;}
  .dot{width:12px;height:12px;border-radius:50%;}
  .dr{background:var(--red);}.da{background:var(--amber);}.dg{background:var(--green);}
  .t-input{display:flex;gap:8px;align-items:center;background:rgba(255,255,255,.03);border:1px solid var(--border);border-radius:6px;padding:8px 12px;}
  .prompt{color:var(--green);font-weight:600;}
  .t-input input{flex:1;background:transparent;border:none;outline:none;color:var(--text);font-family:'JetBrains Mono',monospace;font-size:13px;}
  .run-btn{background:var(--accent);color:#fff;border:none;border-radius:5px;padding:5px 14px;cursor:pointer;font-size:12px;font-weight:600;}
  .run-btn:disabled{opacity:.4;cursor:not-allowed;}
  .t-out{margin-top:12px;max-height:200px;overflow-y:auto;}
  .ll{font-family:'JetBrains Mono',monospace;font-size:12px;padding:5px 0;border-bottom:1px solid rgba(30,45,74,.4);display:flex;gap:10px;}
  .lt{color:var(--muted);flex-shrink:0;}
  .lok{color:var(--green);}.ler{color:var(--red);}.linf{color:var(--accent2);}
  .dt{width:100%;border-collapse:collapse;font-size:13px;}
  .dt th{text-align:left;padding:8px 12px;color:var(--muted);font-size:11px;text-transform:uppercase;border-bottom:1px solid var(--border);}
  .dt td{padding:9px 12px;border-bottom:1px solid rgba(30,45,74,.5);}
  .dt tr:hover td{background:rgba(255,255,255,.02);}
  .form-row{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:10px;}
  .ff{display:flex;flex-direction:column;gap:5px;}
  .ff label{font-size:12px;color:var(--muted);font-weight:500;}
  .ff input,.ff select{background:var(--surface2);border:1px solid var(--border);border-radius:6px;padding:8px 10px;color:var(--text);font-size:13px;outline:none;}
  .ff input:focus,.ff select:focus{border-color:var(--accent);}
  .ff select option{background:var(--surface2);}
  .sub-btn{background:var(--accent);color:#fff;border:none;border-radius:8px;padding:10px 24px;cursor:pointer;font-weight:600;font-size:14px;margin-top:8px;}
  .req-btn{background:rgba(245,158,11,.15);color:var(--amber);border:1px solid rgba(245,158,11,.3);border-radius:8px;padding:10px 24px;cursor:pointer;font-weight:600;font-size:14px;margin-top:8px;}
  .apr-item{background:var(--surface2);border:1px solid var(--border);border-left:3px solid var(--amber);border-radius:10px;padding:16px;margin-bottom:12px;}
  .apr-done{background:var(--surface2);border:1px solid var(--border);border-radius:10px;padding:14px;margin-bottom:10px;}
  .apr-actions{display:flex;gap:10px;margin-top:12px;}
  .app-btn{background:rgba(16,185,129,.15);color:var(--green);border:1px solid rgba(16,185,129,.3);border-radius:8px;padding:8px 20px;cursor:pointer;font-weight:600;font-size:13px;}
  .app-btn:hover{background:var(--green);color:#fff;}
  .rej-btn{background:rgba(239,68,68,.15);color:var(--red);border:1px solid rgba(239,68,68,.3);border-radius:8px;padding:8px 20px;cursor:pointer;font-weight:600;font-size:13px;}
  .rej-btn:hover{background:var(--red);color:#fff;}
  .ai-in{flex:1;background:var(--surface2);border:1px solid var(--border);border-radius:8px;padding:11px 16px;color:var(--text);font-size:14px;outline:none;}
  .ai-in:focus{border-color:var(--accent);}
  .ai-btn{background:linear-gradient(135deg,var(--accent),var(--accent2));color:#fff;border:none;border-radius:8px;padding:11px 22px;cursor:pointer;font-weight:600;font-size:14px;}
  .sql-pre{background:#000913;border:1px solid var(--border);border-radius:8px;padding:12px 16px;font-family:'JetBrains Mono',monospace;font-size:13px;color:var(--accent2);margin-bottom:12px;}
  .leader{font-size:28px;font-weight:700;color:var(--accent);}
  .dng-btn{background:rgba(239,68,68,.15);color:var(--red);border:1px solid rgba(239,68,68,.3);border-radius:8px;padding:10px 20px;cursor:pointer;font-weight:600;font-size:14px;}
  .dng-btn:hover{background:var(--red);color:#fff;}
  .locked{background:rgba(239,68,68,.08);border:1px solid rgba(239,68,68,.2);border-radius:10px;padding:16px;color:var(--red);font-size:14px;text-align:center;}
  .sh-item{display:flex;align-items:center;gap:12px;padding:10px 0;border-bottom:1px solid rgba(30,45,74,.5);}
  .sh-bar{flex:1;height:8px;background:var(--border);border-radius:4px;overflow:hidden;}
  .sh-fill{height:100%;border-radius:4px;transition:width .6s;}
  .tabs{display:flex;gap:4px;margin-bottom:20px;}
  .tab{padding:8px 18px;border-radius:8px;cursor:pointer;font-size:14px;font-weight:500;color:var(--muted);border:1px solid transparent;}
  .tab.active{color:var(--accent);background:rgba(59,130,246,.1);border-color:rgba(59,130,246,.3);}
  .err{color:var(--red);background:rgba(239,68,68,.08);border:1px solid rgba(239,68,68,.2);border-radius:6px;padding:10px 14px;font-size:13px;}
  .ok{color:var(--green);background:rgba(16,185,129,.08);border:1px solid rgba(16,185,129,.2);border-radius:6px;padding:10px 14px;font-size:13px;}
  .empty{color:var(--muted);font-size:13px;padding:20px 0;text-align:center;}
  ::-webkit-scrollbar{width:6px;}::-webkit-scrollbar-thumb{background:var(--border);border-radius:3px;}
`;

const CITY_COLORS = { Cairo: '#3b82f6', Alexandria: '#06b6d4', Assiut: '#10b981', Luxor: '#f59e0b' };
const PIE_COLORS = ['#3b82f6', '#06b6d4', '#10b981', '#f59e0b', '#8b5cf6'];
const LANG = { 'worker-1': 'go', 'worker-2': 'python', 'worker-3': 'nodejs', 'master-1': 'go' };
const fmt = n => n != null ? parseFloat(n).toLocaleString(undefined, { maximumFractionDigits: 0 }) : '—';
const fmtB = n => n != null ? `${parseFloat(n).toFixed(1)}%` : '—';
const ts = () => new Date().toLocaleTimeString();

let _reqs = [];
let _listeners = [];
const syncReqs = async () => {
  try {
    const r = await api.getRequests();
    _reqs = Array.isArray(r) ? r : [];
    _listeners.forEach(f => f([..._reqs]));
  } catch (e) {}
};
setInterval(syncReqs, 3000);

const addReq = async r => { await api.createRequest(r); await syncReqs(); };
const updateReq = async (id, status) => { await api.updateRequest(id, status); await syncReqs(); };
const useReqs = () => {
  const [reqs, setReqs] = useState(_reqs);
  useEffect(() => { _listeners.push(setReqs); syncReqs(); return () => { _listeners = _listeners.filter(f => f !== setReqs); }; }, []);
  return reqs || [];
};

function LoginPage({ onLogin }) {
  const [role, setRole] = useState('worker');
  const [pass, setPass] = useState('');
  const [name, setName] = useState('');
  const [err, setErr] = useState('');
  const login = () => {
    if (role === 'master') { if (pass === MASTER_PASSWORD) onLogin('master', 'Master Node'); else setErr('❌ Wrong password!'); }
    else { if (!name.trim()) { setErr('Enter your name'); return; } onLogin('worker', name.trim()); }
  };
  return (<><style>{css}</style><div className="login-wrap"><div className="login-card">
    <div className="login-logo">Distrib<span>DB</span></div>
    <div className="login-sub">Distributed Database System</div>
    <div className="role-wrap">
      <button className={`role-btn ${role === 'master' ? 'active' : ''}`} onClick={() => { setRole('master'); setErr(''); }}>👑 Master</button>
      <button className={`role-btn ${role === 'worker' ? 'active' : ''}`} onClick={() => { setRole('worker'); setErr(''); }}>💻 Worker</button>
    </div>
    {role === 'master' ? <><input className="l-input" type="password" placeholder="Master Password" value={pass} onChange={e => setPass(e.target.value)} onKeyDown={e => e.key === 'Enter' && login()} /><div className="l-hint">Only the Master admin knows this</div></>
      : <><input className="l-input" placeholder="Your name (e.g. Ahmed — Worker 1)" value={name} onChange={e => setName(e.target.value)} onKeyDown={e => e.key === 'Enter' && login()} /><div className="l-hint">Workers can view data and send insert requests</div></>}
    {err && <div className="l-err">{err}</div>}
    <button className="l-btn" onClick={login}>{role === 'master' ? '🔑 Login as Master' : '➔ Enter as Worker'}</button>
  </div></div></>);
}

const MASTER_NAV = [
  { key: 'overview', label: 'Overview', icon: '⬡' },
  { key: 'cluster', label: 'Cluster', icon: '◈' },
  { key: 'query', label: 'SQL Terminal', icon: '❯' },
  { key: 'ai', label: 'AI Query', icon: '✦' },
  { key: 'data', label: 'Data Browser', icon: '⊡' },
  { key: 'approvals', label: 'Approvals', icon: '✅' },
  { key: 'replication', label: 'Replication', icon: '⇄' },
  { key: 'election', label: 'Leader Election', icon: '♛' },
];
const WORKER_NAV = [
  { key: 'overview', label: 'Overview', icon: '⬡' },
  { key: 'cluster', label: 'Cluster', icon: '◈' },
  { key: 'query', label: 'SQL Terminal', icon: '❯' },
  { key: 'ai', label: 'AI Query', icon: '✦' },
  { key: 'data', label: 'View Data', icon: '⊡' },
  { key: 'request', label: 'Request Insert', icon: '📤' },
];

export default function App() {
  const [user, setUser] = useState(null);
  const [page, setPage] = useState('overview');
  const [stats, setStats] = useState(null);
  const [nodes, setNodes] = useState([]);
  const [health, setHealth] = useState({});
  const [shards, setShards] = useState([]);
  const reqs = useReqs();
  const pendingCount = reqs.filter(r => r.status === 'pending').length;

  const poll = useCallback(async () => {
    try {
      const [s, n, h, sh] = await Promise.allSettled([api.getStats(), api.clusterNodes(), api.clusterHealth(), api.clusterShards()]);
      if (s.status === 'fulfilled') setStats(s.value);
      if (n.status === 'fulfilled') setNodes(n.value?.nodes || []);
      if (h.status === 'fulfilled') setHealth(h.value || {});
      if (sh.status === 'fulfilled') setShards(sh.value?.shards || []);
    } catch { }
  }, []);

  useEffect(() => { if (!user) return; poll(); const id = setInterval(poll, 5000); return () => clearInterval(id); }, [poll, user]);

  if (!user) return <LoginPage onLogin={(role, name) => { setUser({ role, name }); setPage('overview'); }} />;

  const isMaster = user.role === 'master';
  const nav = isMaster ? MASTER_NAV : WORKER_NAV;

  return (<><style>{css}</style><div className="app">
    <aside className="sidebar">
      <div className="logo">Distrib<span>DB</span><div className="logo-role">{isMaster ? '👑 Master' : `💻 ${user.name}`}</div></div>
      <nav className="nav">
        {nav.map(n => (
          <div key={n.key} className={`nav-item ${page === n.key ? 'active' : ''}`} onClick={() => setPage(n.key)}>
            <span className="nav-icon">{n.icon}</span>{n.label}
            {n.key === 'approvals' && pendingCount > 0 && <span style={{ marginLeft: 'auto', background: 'var(--amber)', color: '#000', borderRadius: '10px', padding: '1px 7px', fontSize: 11, fontWeight: 700 }}>{pendingCount}</span>}
          </div>
        ))}
      </nav>
      <button className="logout-btn" onClick={() => { setUser(null); setPage('overview'); }}>⇤ Logout</button>
    </aside>
    <main className="main">
      {!isMaster && <div className="banner">⚠️ Logged in as <strong>{user.name}</strong> — Read Only. Use "Request Insert" to ask Master.</div>}
      {page === 'overview' && <Overview stats={stats} nodes={nodes} shards={shards} />}
      {page === 'cluster' && <Cluster nodes={nodes} health={health} />}
      {page === 'query' && <QueryTerminal isMaster={isMaster} />}
      {page === 'ai' && <AIQuery />}
      {page === 'data' && <DataBrowser isMaster={isMaster} />}
      {page === 'approvals' && isMaster && <Approvals />}
      {page === 'request' && !isMaster && <RequestInsert workerName={user.name} />}
      {page === 'replication' && isMaster && <ReplicationView />}
      {page === 'election' && <ElectionView isMaster={isMaster} />}
    </main>
  </div></>);
}

function Overview({ stats, nodes, shards }) {
  const total = stats?.total_clients ?? '—';
  const balance = stats?.total_balance ?? null;
  const cityData = (stats?.by_city?.rows || []).map(r => ({ name: r[0], clients: r[1], balance: parseFloat(r[2]) }));
  const typeData = (stats?.by_account_type?.rows || []).map(r => ({ name: r[0], value: r[1] }));
  const online = nodes.filter(n => (n.live_status || n.status) === 'online').length;
  return (<>
    <div className="page-title">Overview <span>Distributed Database System</span></div>
    <div className="grid grid-4 mb16">
      {[{ t: 'Total Clients', v: fmt(total), s: 'Across all shards', c: 'var(--accent)', i: '👥' },
      { t: 'Total Balance', v: balance ? `$${(balance / 1e6).toFixed(1)}M` : '—', s: 'Combined portfolio', c: 'var(--green)', i: '💰' },
      { t: 'Active Nodes', v: `${online}/4`, s: 'Master + 3 workers', c: 'var(--accent2)', i: '◈' },
      { t: 'Shard Count', v: shards.length || 4, s: 'City partitions', c: 'var(--purple)', i: '⬡' },
      ].map(s => (
        <div key={s.t} className="card"><div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <div><div className="card-title">{s.t}</div><div style={{ fontSize: 32, fontWeight: 700, color: s.c, letterSpacing: '-1px' }}>{s.v}</div><div style={{ fontSize: 12, color: 'var(--muted)', marginTop: 4 }}>{s.s}</div></div>
          <div style={{ fontSize: 28, opacity: .5 }}>{s.i}</div>
        </div></div>
      ))}
    </div>
    <div className="grid grid-2 mb16">
      <div className="card"><div className="card-title">Clients by City</div>
        <ResponsiveContainer width="100%" height={200}>
          <BarChart data={cityData} margin={{ left: -20 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,.05)" />
            <XAxis dataKey="name" tick={{ fill: '#64748b', fontSize: 12 }} /><YAxis tick={{ fill: '#64748b', fontSize: 12 }} />
            <Tooltip contentStyle={{ background: '#0f1724', border: '1px solid #1e2d4a', borderRadius: 8 }} />
            <Bar dataKey="clients" radius={[4, 4, 0, 0]}>{cityData.map((d, i) => <Cell key={i} fill={CITY_COLORS[d.name] || '#3b82f6'} />)}</Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
      <div className="card"><div className="card-title">Account Types</div>
        <ResponsiveContainer width="100%" height={200}>
          <PieChart><Pie data={typeData} cx="50%" cy="50%" outerRadius={80} dataKey="value" label={({ name, percent }) => `${name} ${(percent * 100).toFixed(0)}%`} labelLine={false}>
            {typeData.map((_, i) => <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />)}
          </Pie><Tooltip contentStyle={{ background: '#0f1724', border: '1px solid #1e2d4a', borderRadius: 8 }} /></PieChart>
        </ResponsiveContainer>
      </div>
    </div>
    <div className="card"><div className="card-title">Shard Distribution</div>
      {shards.map(s => {
        const max = Math.max(...shards.map(x => x.row_count)); return (
          <div key={s.city} className="sh-item">
            <div style={{ width: 100, fontWeight: 600 }}>{s.city}</div>
            <div style={{ fontSize: 11, color: 'var(--muted)', width: 80 }}>{s.node_id}</div>
            <div className="sh-bar"><div className="sh-fill" style={{ width: `${(s.row_count / max) * 100}%`, background: CITY_COLORS[s.city] }} /></div>
            <div style={{ width: 60, textAlign: 'right', fontFamily: 'monospace', fontSize: 13, color: 'var(--muted)' }}>{fmt(s.row_count)}</div>
          </div>
        );
      })}
    </div>
  </>);
}

function Cluster({ nodes, health }) {
  const enriched = nodes.map(n => ({ ...n, ...(health[n.node_id] || {}), live_status: health[n.node_id]?.status || n.status || 'unknown' }));
  return (<>
    <div className="page-title">Cluster Monitor <span>Real-time node health</span></div>
    <div className="nodes-grid">
      {enriched.map(n => {
        const online = n.live_status === 'online';
        const lang = LANG[n.node_id] || 'go';
        const langClass = lang === 'go' ? 'bgo' : lang === 'python' ? 'bpy' : 'bno';
        return (<div key={n.node_id} className={`node-card ${online ? 'online' : 'offline'}`}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 12 }}>
            <span className={`nbadge ${n.role === 'master' ? 'bm' : 'bs'}`}>{n.role?.toUpperCase()}</span>
            <span className={`nbadge ${langClass}`}>{lang.toUpperCase()}</span>
          </div>
          <div style={{ fontWeight: 700, marginBottom: 4 }}>{n.node_id}</div>
          <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 12 }}>{n.shard || 'All shards'}</div>
          <div style={{ display: 'flex', alignItems: 'center', marginBottom: 10 }}>
            <span className={`pulse ${online ? 'g' : 'r'}`} />
            <span style={{ fontSize: 12, fontWeight: 600, color: online ? 'var(--green)' : 'var(--red)' }}>{online ? 'ONLINE' : 'OFFLINE'}</span>
            {n.latency_ms != null && <span style={{ fontSize: 11, color: 'var(--muted)', marginLeft: 'auto' }}>{n.latency_ms}ms</span>}
          </div>
          <div style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 2 }}>CPU {fmtB(n.cpu)}</div>
          <div className="meter"><div className="mf mc" style={{ width: `${Math.min(100, n.cpu || 0)}%` }} /></div>
          <div style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 2, marginTop: 8 }}>RAM {fmtB(n.ram)}</div>
          <div className="meter"><div className="mf mr" style={{ width: `${Math.min(100, n.ram || 0)}%` }} /></div>
        </div>);
      })}
    </div>
  </>);
}

function QueryTerminal({ isMaster }) {
  const [sql, setSql] = useState('SELECT id, name, city, account_type, balance FROM client LIMIT 10');
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [logs, setLogs] = useState([]);
  const addLog = (msg, type) => setLogs(l => [...l.slice(-30), { msg, type, time: ts() }]);
  const run = async () => {
    if (!sql.trim()) return;
    const upper = sql.trim().toUpperCase();
    if (!isMaster && (upper.startsWith('INSERT') || upper.startsWith('UPDATE') || upper.startsWith('DELETE') || upper.startsWith('DROP') || upper.startsWith('CREATE'))) {
      setError('❌ Workers cannot execute write operations! Use Request Insert instead.');
      addLog('Blocked: write operation rejected for worker', 'er');
      return;
    }
    setLoading(true); setError(''); addLog(sql, 'info');
    try {
      const r = await api.query(sql);
      if (r.error) { setError(r.error); addLog(r.error, 'er'); }
      else { setResult(r); addLog(`${r.rows?.length ?? r.affected ?? 0} rows — ${r.duration}`, 'ok'); }
    } catch (e) { setError(e.message); addLog(e.message, 'er'); }
    setLoading(false);
  };
  const examples = ['SELECT * FROM client WHERE city=\'Cairo\' LIMIT 20', 'SELECT city,COUNT(*),AVG(balance) FROM client GROUP BY city', 'SELECT gender,COUNT(*) FROM client GROUP BY gender', 'SELECT * FROM client ORDER BY balance DESC LIMIT 5'];
  return (<>
    <div className="page-title">SQL Terminal <span>Read queries</span></div>
    <div style={{ display: 'flex', gap: 6, marginBottom: 12, flexWrap: 'wrap' }}>
      {examples.map((e, i) => <button key={i} onClick={() => setSql(e)} style={{ background: 'var(--surface2)', border: '1px solid var(--border)', color: 'var(--muted)', padding: '4px 10px', borderRadius: 5, cursor: 'pointer', fontSize: 11 }}>Example {i + 1}</button>)}
    </div>
    <div className="terminal mb16">
      <div className="t-header"><div className="dot dr" /><div className="dot da" /><div className="dot dg" /></div>
      <div className="t-input">
        <span className="prompt">sql❯</span>
        <input value={sql} onChange={e => setSql(e.target.value)} onKeyDown={e => e.key === 'Enter' && run()} placeholder="SELECT ..." />
        <button className="run-btn" onClick={run} disabled={loading}>{loading ? '...' : 'RUN'}</button>
      </div>
      <div className="t-out">{logs.map((l, i) => <div key={i} className="ll"><span className="lt">[{l.time}]</span><span className={`l${l.type}`}>{l.msg}</span></div>)}</div>
    </div>
    {error && <div className="err mb16">{error}</div>}
    {result?.columns && <div className="card"><div className="card-title">{result.rows?.length ?? 0} rows — {result.duration}</div>
      <div style={{ overflowX: 'auto' }}><table className="dt">
        <thead><tr>{result.columns.map(c => <th key={c}>{c}</th>)}</tr></thead>
        <tbody>{result.rows?.map((row, i) => <tr key={i}>{row.map((v, j) => <td key={j}>{v == null ? '—' : String(v)}</td>)}</tr>)}</tbody>
      </table></div>
    </div>}
  </>);
}

function AIQuery() {
  const [input, setInput] = useState('');
  const [trans, setTrans] = useState(null);
  const [result, setResult] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const run = async () => {
    if (!input.trim()) return;
    setLoading(true); setError(''); setTrans(null); setResult(null);
    try { const r = await api.aiQuery(input); setTrans(r.translation); if (r.result) setResult(r.result); if (r.error) setError(r.error); }
    catch (e) { setError(e.message); }
    setLoading(false);
  };
  const examples = ['Show all clients from Cairo', 'Count customers in Alexandria', 'Top 5 richest clients', 'Show female clients', 'Average balance by city'];
  return (<>
    <div className="page-title">AI Query ✦ <span>Natural language → SQL</span></div>
    <div style={{ display: 'flex', gap: 6, marginBottom: 16, flexWrap: 'wrap' }}>
      {examples.map((e, i) => <button key={i} onClick={() => setInput(e)} style={{ background: 'var(--surface2)', border: '1px solid var(--border)', color: 'var(--muted)', padding: '5px 12px', borderRadius: 20, cursor: 'pointer', fontSize: 12 }}>{e}</button>)}
    </div>
    <div className="card mb16">
      <div style={{ display: 'flex', gap: 10, marginBottom: 14 }}>
        <input className="ai-in" value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => e.key === 'Enter' && run()} placeholder="Type in plain English..." />
        <button className="ai-btn" onClick={run} disabled={loading}>{loading ? '…' : '✦ Run'}</button>
      </div>
      {trans?.sql && <div className="sql-pre">{trans.sql}</div>}
      {trans?.explanation && <div style={{ fontSize: 12, color: 'var(--muted)' }}>{trans.explanation}</div>}
    </div>
    {error && <div className="err mb16">{error}</div>}
    {result?.columns && <div className="card"><div className="card-title">{result.rows?.length ?? 0} rows</div>
      <div style={{ overflowX: 'auto' }}><table className="dt">
        <thead><tr>{result.columns.map(c => <th key={c}>{c}</th>)}</tr></thead>
        <tbody>{result.rows?.map((row, i) => <tr key={i}>{row.map((v, j) => <td key={j}>{v == null ? '—' : String(v)}</td>)}</tr>)}</tbody>
      </table></div>
    </div>}
  </>);
}

function DataBrowser({ isMaster }) {
  const [clients, setClients] = useState([]);
  const [total, setTotal] = useState(0);
  const [city, setCity] = useState('');
  const [search, setSearch] = useState('');
  const [pg, setPg] = useState(0);
  const [loading, setLoading] = useState(false);
  const [tab, setTab] = useState('browse');
  const [form, setForm] = useState({ name: '', national_id: '', phone: '', email: '', gender: 'Male', birth_date: '1990-01-01', city: 'Cairo', address: '', account_type: 'Savings', balance: 0 });
  const [msg, setMsg] = useState('');
  const limit = 20;
  const load = useCallback(async () => {
    setLoading(true);
    const r = await api.getClients({ city, search, limit, offset: pg * limit });
    setClients(r.data?.rows || []); setTotal(r.total || 0); setLoading(false);
  }, [city, search, pg]);
  useEffect(() => { load(); }, [load]);
  const del = async (id) => { if (!isMaster) return; if (!window.confirm(`Delete #${id}?`)) return; await api.deleteClient(id); load(); };
  const submit = async () => {
    setMsg('');
    const r = await api.createClient({ ...form, balance: parseFloat(form.balance) || 0 });
    if (r.error) setMsg('Error: ' + r.error);
    else { setMsg('✓ Inserted and replicated!'); load(); setTab('browse'); }
  };
  return (<>
    <div className="page-title">{isMaster ? 'Data Browser' : 'View Data'} <span>{fmt(total)} records</span></div>
    {isMaster && <div className="tabs">
      <div className={`tab ${tab === 'browse' ? 'active' : ''}`} onClick={() => setTab('browse')}>Browse</div>
      <div className={`tab ${tab === 'insert' ? 'active' : ''}`} onClick={() => setTab('insert')}>Add Client</div>
    </div>}
    {tab === 'browse' && <>
      <div style={{ display: 'flex', gap: 10, marginBottom: 16 }}>
        <select value={city} onChange={e => { setCity(e.target.value); setPg(0); }} style={{ background: 'var(--surface2)', border: '1px solid var(--border)', color: 'var(--text)', padding: '8px 12px', borderRadius: 8, fontSize: 13, outline: 'none' }}>
          <option value="">All Cities</option>
          {['Cairo', 'Alexandria', 'Assiut', 'Luxor'].map(c => <option key={c}>{c}</option>)}
        </select>
        <input placeholder="Search..." value={search} onChange={e => { setSearch(e.target.value); setPg(0); }} style={{ flex: 1, background: 'var(--surface2)', border: '1px solid var(--border)', color: 'var(--text)', padding: '8px 12px', borderRadius: 8, fontSize: 13, outline: 'none' }} />
        <button className="run-btn" onClick={load}>Search</button>
      </div>
      <div className="card">
        {loading ? <div className="empty">Loading...</div> : <div style={{ overflowX: 'auto' }}><table className="dt">
          <thead><tr><th>ID</th><th>Name</th><th>City</th><th>Gender</th><th>Account</th><th>Balance</th><th>Created</th>{isMaster && <th></th>}</tr></thead>
          <tbody>{clients.map((row, i) => (
            <tr key={i}>
              <td style={{ color: 'var(--muted)', fontFamily: 'monospace' }}>{row[0]}</td>
              <td style={{ fontWeight: 500 }}>{row[1]}</td>
              <td><span style={{ background: `${CITY_COLORS[row[7]]}20`, color: CITY_COLORS[row[7]], padding: '2px 8px', borderRadius: 4, fontSize: 11, fontWeight: 600 }}>{row[7]}</span></td>
              <td>{row[5]}</td><td style={{ fontSize: 11 }}>{row[9]}</td>
              <td style={{ fontFamily: 'monospace', color: 'var(--green)' }}>${fmt(row[10])}</td>
              <td style={{ color: 'var(--muted)', fontSize: 12 }}>{row[11]}</td>
              {isMaster && <td><button onClick={() => del(row[0])} style={{ background: 'transparent', border: 'none', color: 'var(--red)', cursor: 'pointer', fontSize: 14 }}>✕</button></td>}
            </tr>
          ))}</tbody>
        </table></div>}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 12, fontSize: 13, color: 'var(--muted)' }}>
          <span>Page {pg + 1} of {Math.ceil(total / limit) || 1}</span>
          <div style={{ display: 'flex', gap: 8 }}>
            <button onClick={() => setPg(p => Math.max(0, p - 1))} disabled={pg === 0} className="run-btn">←</button>
            <button onClick={() => setPg(p => p + 1)} disabled={(pg + 1) * limit >= total} className="run-btn">→</button>
          </div>
        </div>
      </div>
    </>}
    {tab === 'insert' && isMaster && <div className="card">
      <div className="card-title">👑 Add Client — Master Only</div>
      <div className="form-row">
        {[['name', 'Full Name'], ['national_id', 'National ID'], ['phone', 'Phone'], ['email', 'Email'], ['address', 'Address']].map(([k, l]) => (
          <div key={k} className="ff"><label>{l}</label><input value={form[k]} onChange={e => setForm(f => ({ ...f, [k]: e.target.value }))} /></div>
        ))}
        <div className="ff"><label>Birth Date</label><input type="date" value={form.birth_date} onChange={e => setForm(f => ({ ...f, birth_date: e.target.value }))} /></div>
        <div className="ff"><label>Gender</label><select value={form.gender} onChange={e => setForm(f => ({ ...f, gender: e.target.value }))}><option>Male</option><option>Female</option></select></div>
        <div className="ff"><label>City</label><select value={form.city} onChange={e => setForm(f => ({ ...f, city: e.target.value }))}>{['Cairo', 'Alexandria', 'Assiut', 'Luxor'].map(c => <option key={c}>{c}</option>)}</select></div>
        <div className="ff"><label>Account Type</label><select value={form.account_type} onChange={e => setForm(f => ({ ...f, account_type: e.target.value }))}>{['Savings', 'Current', 'Fixed Deposit', 'Business'].map(t => <option key={t}>{t}</option>)}</select></div>
        <div className="ff"><label>Balance</label><input type="number" value={form.balance} onChange={e => setForm(f => ({ ...f, balance: e.target.value }))} /></div>
      </div>
      <button className="sub-btn" onClick={submit}>✓ Insert & Replicate</button>
      {msg && <div className={msg.startsWith('✓') ? 'ok mt16' : 'err mt16'}>{msg}</div>}
    </div>}
  </>);
}

function RequestInsert({ workerName }) {
  const [form, setForm] = useState({ name: '', national_id: '', phone: '', email: '', gender: 'Male', birth_date: '1990-01-01', city: 'Cairo', address: '', account_type: 'Savings', balance: 0 });
  const [msg, setMsg] = useState('');
  const reqs = useReqs();
  const myReqs = reqs.filter(r => r.worker === workerName);
  const send = async () => {
    if (!form.name || !form.national_id) { setMsg('❌ Fill name and National ID'); return; }
    await addReq({ ...form, worker: workerName });
    setMsg('✅ Request sent to Master! Waiting for approval...');
    setForm({ name: '', national_id: '', phone: '', email: '', gender: 'Male', birth_date: '1990-01-01', city: 'Cairo', address: '', account_type: 'Savings', balance: 0 });
  };
  const sc = s => s === 'approved' ? 'var(--green)' : s === 'rejected' ? 'var(--red)' : 'var(--amber)';
  const si = s => s === 'approved' ? '✅' : s === 'rejected' ? '❌' : '⏳';
  return (<>
    <div className="page-title">Request Insert 📤 <span>Ask Master to add data</span></div>
    <div className="card mb16">
      <div className="card-title">New Insert Request</div>
      <div className="form-row">
        {[['name', 'Full Name *'], ['national_id', 'National ID *'], ['phone', 'Phone'], ['email', 'Email'], ['address', 'Address']].map(([k, l]) => (
          <div key={k} className="ff"><label>{l}</label><input value={form[k]} onChange={e => setForm(f => ({ ...f, [k]: e.target.value }))} /></div>
        ))}
        <div className="ff"><label>Birth Date</label><input type="date" value={form.birth_date} onChange={e => setForm(f => ({ ...f, birth_date: e.target.value }))} /></div>
        <div className="ff"><label>Gender</label><select value={form.gender} onChange={e => setForm(f => ({ ...f, gender: e.target.value }))}><option>Male</option><option>Female</option></select></div>
        <div className="ff"><label>City</label><select value={form.city} onChange={e => setForm(f => ({ ...f, city: e.target.value }))}>{['Cairo', 'Alexandria', 'Assiut', 'Luxor'].map(c => <option key={c}>{c}</option>)}</select></div>
        <div className="ff"><label>Account Type</label><select value={form.account_type} onChange={e => setForm(f => ({ ...f, account_type: e.target.value }))}>{['Savings', 'Current', 'Fixed Deposit', 'Business'].map(t => <option key={t}>{t}</option>)}</select></div>
        <div className="ff"><label>Balance</label><input type="number" value={form.balance} onChange={e => setForm(f => ({ ...f, balance: e.target.value }))} /></div>
      </div>
      <button className="req-btn" onClick={send}>📤 Send Request to Master</button>
      {msg && <div className={msg.startsWith('✅') ? 'ok mt16' : 'err mt16'}>{msg}</div>}
    </div>
    {myReqs.length > 0 && <div className="card"><div className="card-title">My Requests</div>
      {myReqs.slice().reverse().map(r => (
        <div key={r.id} className="apr-done" style={{ borderLeft: `3px solid ${sc(r.status)}` }}>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <div style={{ fontWeight: 600 }}>{r.name}</div>
            <div style={{ color: sc(r.status), fontWeight: 600 }}>{si(r.status)} {r.status.toUpperCase()}</div>
          </div>
          <div style={{ fontSize: 12, color: 'var(--muted)', marginTop: 4 }}>{r.city} — {r.account_type} — ${fmt(r.balance)} — {r.time}</div>
        </div>
      ))}
    </div>}
  </>);
}

function Approvals() {
  const reqs = useReqs();
  const [msg, setMsg] = useState('');
  const approve = async (req) => {
    try {
      const r = await api.createClient({ ...req, balance: parseFloat(req.balance) || 0 });
      if (r.error) { setMsg('❌ ' + r.error); return; }
      await updateReq(req.id, 'approved');
      setMsg(`✅ Approved & inserted: ${req.name}`);
    } catch (e) { setMsg('❌ ' + e.message); }
  };
  const reject = async req => { await updateReq(req.id, 'rejected'); setMsg(`❌ Rejected: ${req.name}`); };
  const pending = reqs.filter(r => r.status === 'pending');
  const done = reqs.filter(r => r.status !== 'pending');
  return (<>
    <div className="page-title">Approvals ✅ <span>Review worker requests</span></div>
    {msg && <div className={msg.startsWith('✅') ? 'ok mb16' : 'err mb16'}>{msg}</div>}
    <div className="card mb16">
      <div className="card-title">⏳ Pending ({pending.length})</div>
      {pending.length === 0 ? <div className="empty">No pending requests</div> :
        pending.map(r => (
          <div key={r.id} className="apr-item">
            <div style={{ fontWeight: 700, fontSize: 15, marginBottom: 4 }}>{r.name}</div>
            <div style={{ fontSize: 12, color: 'var(--muted)' }}>From: <strong style={{ color: 'var(--amber)' }}>{r.worker}</strong> | {r.city} | {r.account_type} | ${fmt(r.balance)}</div>
            <div style={{ fontSize: 12, color: 'var(--muted)', marginTop: 2 }}>ID: {r.national_id} | {r.phone} | {r.email}</div>
            <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 2 }}>{r.time}</div>
            <div className="apr-actions">
              <button className="app-btn" onClick={() => approve(r)}>✅ Approve & Insert</button>
              <button className="rej-btn" onClick={() => reject(r)}>❌ Reject</button>
            </div>
          </div>
        ))
      }
    </div>
    {done.length > 0 && <div className="card"><div className="card-title">Processed ({done.length})</div>
      {done.slice().reverse().map(r => (
        <div key={r.id} className="apr-done" style={{ borderLeft: `3px solid ${r.status === 'approved' ? 'var(--green)' : 'var(--red)'}` }}>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <div style={{ fontWeight: 600 }}>{r.name} <span style={{ fontSize: 12, color: 'var(--muted)', fontWeight: 400 }}>from {r.worker}</span></div>
            <div style={{ color: r.status === 'approved' ? 'var(--green)' : 'var(--red)', fontWeight: 600 }}>{r.status === 'approved' ? '✅ APPROVED' : '❌ REJECTED'}</div>
          </div>
          <div style={{ fontSize: 12, color: 'var(--muted)', marginTop: 4 }}>{r.city} — ${fmt(r.balance)} — {r.time}</div>
        </div>
      ))}
    </div>}
  </>);
}

function ReplicationView() {
  const [logs, setLogs] = useState([]);
  const load = async () => { const r = await api.replLog(); setLogs(r.logs || []); };
  useEffect(() => { load(); const id = setInterval(load, 4000); return () => clearInterval(id); }, []);
  const sc = s => s === 'success' ? 'var(--green)' : s === 'failed' ? 'var(--red)' : 'var(--amber)';
  return (<>
    <div className="page-title">Replication Log <span>Master → Worker sync</span></div>
    <div style={{ display: 'flex', gap: 12, marginBottom: 16 }}>
      {[['Goroutines', '3 parallel', '⚡'], ['Channel', 'Buffered (100)', '⇄'], ['Strategy', 'Broadcast all', '◈']].map(([t, d, i]) => (
        <div key={t} className="card" style={{ flex: 1 }}><div style={{ fontSize: 24, marginBottom: 6 }}>{i}</div><div style={{ fontWeight: 600 }}>{t}</div><div style={{ fontSize: 12, color: 'var(--muted)' }}>{d}</div></div>
      ))}
    </div>
    <div className="card"><div className="card-title">Recent Events</div>
      {logs.length === 0 ? <div className="empty">No events yet. Insert a record to see this.</div> :
        logs.map((l, i) => (
          <div key={i} className="ll">
            <span className="lt">{l.created_at}</span>
            <span style={{ color: 'var(--accent2)', width: 80, flexShrink: 0 }}>{l.op}</span>
            <span style={{ color: sc(l.status), width: 70, flexShrink: 0, fontWeight: 600 }}>{l.status}</span>
            <span style={{ flex: 1, color: 'var(--muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{l.sql_stmt}</span>
          </div>
        ))
      }
    </div>
  </>);
}

function ElectionView({ isMaster }) {
  const [status, setStatus] = useState(null);
  const [events, setEvents] = useState([]);
  const [msg, setMsg] = useState('');
  const load = async () => { const r = await api.electionStatus(); setStatus(r.status); setEvents(r.events || []); };
  useEffect(() => { load(); const id = setInterval(load, 5000); return () => clearInterval(id); }, []);
  return (<>
    <div className="page-title">Leader Election ♛ <span>Bully Algorithm</span></div>
    {status && <div className="card mb16">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 4 }}>Current Leader</div>
          <div className="leader">{status.leader_id}</div>
          <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
            <span style={{ background: 'rgba(59,130,246,.15)', color: 'var(--accent)', padding: '4px 12px', borderRadius: 20, fontSize: 12, fontWeight: 600 }}>Term #{status.term}</span>
            <span style={{ background: 'rgba(16,185,129,.15)', color: 'var(--green)', padding: '4px 12px', borderRadius: 20, fontSize: 12, fontWeight: 600 }}>{status.state?.toUpperCase()}</span>
          </div>
        </div>
        {isMaster
          ? <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <button className="dng-btn" onClick={async () => { await api.triggerElection(); setMsg('Election triggered!'); load(); }}>⚡ Trigger Election</button>
            {['worker-1', 'worker-2', 'worker-3'].map(n => (
              <button key={n} className="dng-btn" onClick={async () => { await api.triggerFailover(n); setMsg(`Failover to ${n}`); load(); }}>Failover → {n}</button>
            ))}
          </div>
          : <div className="locked">🔒 Only Master can control elections</div>
        }
      </div>
    </div>}
    {msg && <div className="ok mb16">{msg}</div>}
    <div className="card"><div className="card-title">Election History</div>
      {events.length === 0 ? <div className="empty">No events yet.</div> :
        events.slice().reverse().map((e, i) => (
          <div key={i} className="ll">
            <span className="lt">{new Date(e.time).toLocaleTimeString()}</span>
            <span className="linf" style={{ width: 160 }}>{e.event}</span>
            <span>{e.node_id}</span>
            <span style={{ marginLeft: 'auto', color: 'var(--muted)' }}>term #{e.term}</span>
          </div>
        ))
      }
    </div>
  </>);
}