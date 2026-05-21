const MASTER = process.env.REACT_APP_MASTER_URL || 'http://localhost:8888';

async function req(method, path, body) {
  const res = await fetch(`${MASTER}${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  });
  return res.json();
}

export const api = {
  heartbeat:       ()      => req('GET',  '/heartbeat'),
  status:          ()      => req('GET',  '/status'),
  query:           (sql)   => req('POST', '/query', { sql }),
  aiTranslate:     (q)     => req('POST', '/ai/translate', { query: q }),
  aiQuery:         (q)     => req('POST', '/ai/query', { query: q }),
  clusterNodes:    ()      => req('GET',  '/cluster/nodes'),
  clusterShards:   ()      => req('GET',  '/cluster/shards'),
  clusterHealth:   ()      => req('GET',  '/cluster/health'),
  replLog:         ()      => req('GET',  '/cluster/replication-log'),
  electionStatus:  ()      => req('GET',  '/election/status'),
  triggerElection: ()      => req('POST', '/election/trigger'),
  triggerFailover: (node)  => req('POST', '/election/failover', { target_node: node }),
  getClients:      (p)     => req('GET',  `/data/clients?${new URLSearchParams(p || {})}`),
  createClient:    (d)     => req('POST', '/data/clients', d),
  deleteClient:    (id)    => req('DELETE', `/data/clients/${id}`),
  getStats:        ()      => req('GET',  '/data/stats'),
  getTables:       ()      => req('GET',  '/tables'),
  createTable:     (sql)   => req('POST', '/tables', { sql }),
  dropTable:       (name)  => req('DELETE', `/tables/${name}`),
};
