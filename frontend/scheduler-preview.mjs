import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve('../backend/internal/web/dist')
const user = { id: 1, username: 'preview', email: 'preview@example.test', role: 'admin', status: 'active', balance: 0 }
const now = new Date().toISOString()
const model_cooldowns = ['cooldown', 'awaiting_probe', 'probing', 'recovering'].map((state, i) => ({
  account_id: i < 2 ? 1 : i, model: i === 0 ? '*' : 'gpt-5.6-sol', failure_streak: 2,
  last_failure_at: now, block_until: now, remaining_seconds: i === 0 ? 10 : 0, state,
  reason: i === 0 ? 'cross_model_502' : 'upstream_503', probes_in_flight: i > 1 ? 1 : 0, recovery_successes: i === 3 ? 1 : 0
}))

http.createServer((request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1')
  const name = url.pathname
  response.setHeader('Cache-Control', 'no-store')
  if (name.startsWith('/api/')) {
    let data = { items: [], total: 0, account: {}, enabled: true }
    if (name.endsWith('/auth/me') || name.endsWith('/users/me')) data = user
    if (name.includes('announcements')) data = []
    if (name.endsWith('/settings/public')) data = { site_name: 'BUAGA API', ops_monitoring_enabled: true, registration_enabled: true, setup_completed: true }
    if (name.endsWith('/ops/scheduler')) data = { generated_at: now, metrics: {}, model_cooldowns }
    if (name.endsWith('/account-availability') || name.endsWith('/concurrency')) data = { enabled: true, account: {1:{account_name:'Preview upstream A',is_available:true},2:{account_name:'Preview upstream B',is_available:true},3:{account_name:'Preview upstream C',is_available:true}} }
    if (name.endsWith('/dashboard/overview')) data = { error_count_total: 0, ttft: {p95_ms:200} }
    response.setHeader('Content-Type', 'application/json')
    response.end(JSON.stringify({ code: 0, data }))
    return
  }
  const file = path.join(root, name === '/' || !path.extname(name) ? 'index.html' : name)
  if (!file.startsWith(root + '/') || !fs.existsSync(file)) { response.statusCode = 404; response.end(); return }
  response.setHeader('Content-Type', {'.js':'text/javascript','.css':'text/css','.html':'text/html','.svg':'image/svg+xml','.json':'application/json'}[path.extname(file)] || 'application/octet-stream')
  response.end(fs.readFileSync(file))
}).listen(18761, '127.0.0.1', () => console.log('preview http://127.0.0.1:18761'))
