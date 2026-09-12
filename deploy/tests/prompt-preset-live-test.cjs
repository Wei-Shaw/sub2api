// Retained local Docker smoke test. Creates a test user/key and keeps all records.
// Reads local signing material in memory; never prints or writes credentials.
const { execFileSync } = require('node:child_process');
const { createHash, createHmac, randomBytes } = require('node:crypto');
const { writeFileSync } = require('node:fs');
const assert = require('node:assert/strict');
const base = 'http://127.0.0.1:8080';
const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8' }).trim();
const sql = query => docker('exec', 'sub2api-postgres', 'psql', '-U', 'sub2api', '-d', 'sub2api', '-tAc', query);
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
async function api(path, method, body, token) {
  const response = await fetch(base + path, { method, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) }, body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(120000) });
  const data = await response.json();
  assert.equal(response.status, 200, `${method} ${path}: HTTP ${response.status}, ${data.message || ''}`);
  return data.data;
}
async function main() {
  const env = Object.fromEntries(JSON.parse(docker('inspect', 'sub2api-dev'))[0].Config.Env.map(line => [line.slice(0, line.indexOf('=')), line.slice(line.indexOf('=') + 1)]));
  const admin = JSON.parse(sql("select row_to_json(u) from (select id,email,password_hash from users where role='admin' and deleted_at is null order by id limit 1) u"));
  const fingerprint = createHash('sha256').update(admin.email.trim().toLowerCase() + '\n' + admin.password_hash).digest().readBigUInt64BE() & 0x7fffffffffffffffn;
  const now = Math.floor(Date.now()/1000);
  const encode = value => Buffer.from(value).toString('base64url');
  const unsigned = encode('{"alg":"HS256","typ":"JWT"}') + '.' + encode(JSON.stringify({ user_id: admin.id, email: admin.email, role: 'admin', exp: now+900, iat: now, nbf: now }).replace(/}$/, ',"token_version":'+fingerprint+'}'));
  const adminToken = unsigned + '.' + createHmac('sha256', env.JWT_SECRET).update(unsigned).digest('base64url');
  const original = await api('/api/v1/admin/prompt-records/recording', 'GET', undefined, adminToken);
  assert.equal(typeof original.filter_preset, 'boolean', 'new source image is not deployed');
  const stamp = Date.now();
  const email = `preset-audit-${stamp}@example.test`;
  const password = randomBytes(24).toString('hex');
  const user = await api('/api/v1/admin/users', 'POST', { email, password, username: '预设过滤审计测试', notes: '本地 Docker 回归测试，按要求保留。', role:'user', balance:1, concurrency:1, allowed_groups:[2] }, adminToken);
  const login = await api('/api/v1/auth/login', 'POST', { email, password });
  assert.ok(login.access_token, 'login must return access token');
  console.log('Real password login: HTTP 200');
  const key = await api('/api/v1/keys', 'POST', { name:`preset-audit-${stamp}`, group_id:2, quota:0.5, expires_in_days:1 }, login.access_token);
  const results = [];
  try {
    for (const enabled of [false, true]) {
      const config = await api('/api/v1/admin/prompt-records/recording', 'PUT', { enabled:true, headers_enabled:true, prompt_enabled:true, filter_preset:enabled }, adminToken);
      assert.equal(config.filter_preset, enabled);
      for (const agent of ['codex', 'claude']) {
        const marker = `preset-audit-${stamp}-${agent}-${enabled}`;
        const preset = `PRESET_${marker}`;
        const identity = agent === 'codex' ? 'You are Codex' : 'You are Claude Code';
        const prefix = agent === 'codex' ? `# AGENTS.md instructions\n<INSTRUCTIONS>${preset}</INSTRUCTIONS>\n<environment_context>${preset}</environment_context>\n` : `<system-reminder>${preset}</system-reminder>\n`;
        const body = { model:'gpt-5.6-terra', stream:false, max_tokens:32, messages:[{role:'system',content:identity+' '+preset},{role:'user',content:prefix+marker+' Reply with OK.'}] };
        const response = await fetch(base+'/v1/chat/completions', { method:'POST', headers:{Authorization:`Bearer ${key.key}`, 'Content-Type':'application/json'}, body:JSON.stringify(body), signal:AbortSignal.timeout(120000) });
        await response.text();
        assert.equal(response.status, 200, `upstream ${agent}: ${response.status}`);
        let record;
        for (let attempt=0; attempt<40; attempt++) {
          const raw = sql(`select row_to_json(r) from (select id,request_body,prompt_text,response_text,response_captured_at from prompt_records where api_key_id=${Number(key.id)} and request_body like '%${marker}%' order by id desc limit 1) r`);
          if (raw) record = JSON.parse(raw);
          if (record?.response_captured_at) break;
          await sleep(250);
        }
        assert.ok(record?.response_captured_at, 'response must be linked');
        assert.equal(record.request_body.includes(preset), !enabled);
        assert.equal(record.prompt_text.includes(preset), !enabled);
        assert.ok(record.request_body.includes(marker));
        if (!enabled) assert.equal(record.request_body, JSON.stringify(body));
        if (enabled) {
          const retained = JSON.parse(record.request_body);
          assert.deepEqual(Object.keys(retained), ['messages']);
          assert.deepEqual(retained.messages, [{ role:'user', content:marker+' Reply with OK.' }]);
          assert.equal(record.prompt_text, marker+' Reply with OK.');
          assert.ok(record.request_body.length < JSON.stringify(body).length / 2, 'filtered request must be materially smaller');
        }
        results.push({agent, filter_preset:enabled, record_id:record.id, response_linked:true, request_bytes:JSON.stringify(body).length, stored_bytes:record.request_body.length});
        console.log(JSON.stringify(results.at(-1)));
      }
    }
  } finally {
    await api('/api/v1/admin/prompt-records/recording', 'PUT', original, adminToken);
    const report = `deploy/tests/prompt-preset-live-results-${stamp}.json`;
    writeFileSync(report, JSON.stringify({user_id:user.id, api_key_id:key.id, login_status:200, original_config:original, results}, null, 2));
    console.log(`Retained report: ${report}`);
  }
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
