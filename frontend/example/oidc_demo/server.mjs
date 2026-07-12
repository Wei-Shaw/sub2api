// Sub2API OIDC — minimal Relying Party (RP) demo.
//
// A self-contained BFF (Backend-for-Frontend): the browser only talks to this
// server, and the OIDC client_secret never leaves it. Zero external
// dependencies — only Node built-ins (http, crypto, fs) + global fetch (Node 18+).
//
// Flow implemented:
//   1. Discovery               GET  {issuer}/.well-known/openid-configuration
//   2. Authorization (PKCE)    redirect to {authorization_endpoint}
//   3. Callback + token swap   POST {token_endpoint}  (client_secret_basic)
//   4. ID Token validation     RS256 via JWKS (kid lookup, iss/aud/exp/nonce)
//   5. UserInfo                GET  {userinfo_endpoint}
//   6. Refresh (rotating)      POST {token_endpoint}  grant_type=refresh_token
//
// Run:  cp .env.example .env  &&  edit .env  &&  npm start

import http from 'node:http';
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// ── Tiny .env loader (no dependency) ─────────────────────────────────────────
const __dirname = path.dirname(fileURLToPath(import.meta.url));
function loadDotEnv() {
  const file = path.join(__dirname, '.env');
  if (!fs.existsSync(file)) return;
  for (const raw of fs.readFileSync(file, 'utf8').split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const eq = line.indexOf('=');
    if (eq === -1) continue;
    const key = line.slice(0, eq).trim();
    let val = line.slice(eq + 1).trim();
    if (
      (val.startsWith('"') && val.endsWith('"')) ||
      (val.startsWith("'") && val.endsWith("'"))
    ) {
      val = val.slice(1, -1);
    }
    if (!(key in process.env)) process.env[key] = val;
  }
}
loadDotEnv();

// ── Config ───────────────────────────────────────────────────────────────────
const ISSUER = (process.env.SUB2API_ISSUER_URL || '').replace(/\/+$/, '');
const CLIENT_ID = process.env.SUB2API_CLIENT_ID || '';
const CLIENT_SECRET = process.env.SUB2API_CLIENT_SECRET || '';
const REDIRECT_URI =
  process.env.SUB2API_REDIRECT_URI || 'http://localhost:3000/callback';
const SCOPES = process.env.SUB2API_SCOPES || 'openid profile email offline_access';
const PORT = Number(process.env.PORT || 3000);

if (!ISSUER || !CLIENT_ID || !CLIENT_SECRET) {
  console.error(
    '\n[config] Missing required env. Copy .env.example to .env and fill in:\n' +
      '  SUB2API_ISSUER_URL, SUB2API_CLIENT_ID, SUB2API_CLIENT_SECRET\n'
  );
  process.exit(1);
}

// ── In-memory session store (demo only; use a real store in production) ───────
/** sid -> { login?: {state, nonce, codeVerifier, createdAt}, tokens?: {...}, claims?: {...} } */
const sessions = new Map();
const SID_COOKIE = 'oidc_demo_sid';

function getSession(req) {
  const sid = parseCookies(req)[SID_COOKIE];
  if (sid && sessions.has(sid)) return { sid, sess: sessions.get(sid) };
  const newSid = b64url(crypto.randomBytes(18));
  const sess = {};
  sessions.set(newSid, sess);
  return { sid: newSid, sess, fresh: true };
}

function parseCookies(req) {
  const out = {};
  const header = req.headers.cookie;
  if (!header) return out;
  for (const part of header.split(';')) {
    const i = part.indexOf('=');
    if (i === -1) continue;
    out[part.slice(0, i).trim()] = decodeURIComponent(part.slice(i + 1).trim());
  }
  return out;
}

// ── Crypto helpers ────────────────────────────────────────────────────────────
function b64url(buf) {
  return Buffer.from(buf)
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}
function b64urlDecode(str) {
  return Buffer.from(str.replace(/-/g, '+').replace(/_/g, '/'), 'base64');
}
function pkcePair() {
  const verifier = b64url(crypto.randomBytes(48));
  const challenge = b64url(crypto.createHash('sha256').update(verifier).digest());
  return { verifier, challenge };
}

// ── Discovery + JWKS caches ───────────────────────────────────────────────────
let discoveryCache = null;
async function discovery() {
  if (discoveryCache) return discoveryCache;
  const url = `${ISSUER}/.well-known/openid-configuration`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`discovery failed: ${res.status} ${await res.text()}`);
  discoveryCache = await res.json();
  return discoveryCache;
}

let jwksCache = { keys: [], fetchedAt: 0 };
async function getJwk(kid) {
  const fresh = jwksCache.keys.find((k) => k.kid === kid);
  if (fresh) return fresh;
  // Unknown kid (or first call) -> (re)fetch, throttled to once per 30s.
  if (Date.now() - jwksCache.fetchedAt > 30_000 || jwksCache.keys.length === 0) {
    const { jwks_uri } = await discovery();
    const res = await fetch(jwks_uri);
    if (!res.ok) throw new Error(`jwks fetch failed: ${res.status}`);
    const body = await res.json();
    jwksCache = { keys: body.keys || [], fetchedAt: Date.now() };
  }
  return jwksCache.keys.find((k) => k.kid === kid) || null;
}

// ── ID Token validation (RS256) ───────────────────────────────────────────────
async function verifyIdToken(idToken, expectedNonce) {
  const parts = idToken.split('.');
  if (parts.length !== 3) throw new Error('malformed id_token');
  const [h, p, s] = parts;
  const header = JSON.parse(b64urlDecode(h).toString('utf8'));
  const payload = JSON.parse(b64urlDecode(p).toString('utf8'));

  if (header.alg !== 'RS256') throw new Error(`unexpected alg: ${header.alg}`);

  const jwk = await getJwk(header.kid);
  if (!jwk) throw new Error(`no JWKS key for kid=${header.kid}`);

  const pubKey = crypto.createPublicKey({ key: jwk, format: 'jwk' });
  const ok = crypto.verify(
    'RSA-SHA256',
    Buffer.from(`${h}.${p}`),
    pubKey,
    b64urlDecode(s)
  );
  if (!ok) throw new Error('id_token signature verification failed');

  // Claim checks.
  const { issuer } = await discovery();
  if (payload.iss !== issuer) throw new Error(`iss mismatch: ${payload.iss}`);
  const audOk = Array.isArray(payload.aud)
    ? payload.aud.includes(CLIENT_ID)
    : payload.aud === CLIENT_ID;
  if (!audOk) throw new Error(`aud mismatch: ${payload.aud}`);
  const now = Math.floor(Date.now() / 1000);
  if (typeof payload.exp === 'number' && payload.exp < now) {
    throw new Error('id_token expired');
  }
  if (expectedNonce && payload.nonce !== expectedNonce) {
    throw new Error('nonce mismatch');
  }
  return payload;
}

// ── Token endpoint calls (client_secret_basic) ────────────────────────────────
async function tokenRequest(form) {
  const { token_endpoint } = await discovery();
  const basic = Buffer.from(`${CLIENT_ID}:${CLIENT_SECRET}`).toString('base64');
  const res = await fetch(token_endpoint, {
    method: 'POST',
    headers: {
      Authorization: `Basic ${basic}`,
      'Content-Type': 'application/x-www-form-urlencoded',
      Accept: 'application/json',
    },
    body: new URLSearchParams(form).toString(),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(body.error_description || body.error || `token ${res.status}`);
    err.detail = body;
    throw err;
  }
  return body;
}

async function fetchUserInfo(accessToken) {
  const { userinfo_endpoint } = await discovery();
  const res = await fetch(userinfo_endpoint, {
    headers: { Authorization: `Bearer ${accessToken}`, Accept: 'application/json' },
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(body.error_description || body.error || `userinfo ${res.status}`);
    err.detail = body;
    throw err;
  }
  return body;
}

// ── HTTP response helpers ─────────────────────────────────────────────────────
function send(res, status, headers, body) {
  res.writeHead(status, headers);
  res.end(body);
}
function html(res, status, body, extraHeaders = {}) {
  send(res, status, { 'Content-Type': 'text/html; charset=utf-8', ...extraHeaders }, body);
}
function redirect(res, location, setCookie) {
  const headers = { Location: location };
  if (setCookie) headers['Set-Cookie'] = setCookie;
  send(res, 302, headers, '');
}
function sidCookie(sid) {
  return `${SID_COOKIE}=${sid}; HttpOnly; SameSite=Lax; Path=/; Max-Age=3600`;
}
function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}
function pre(obj) {
  return `<pre>${esc(JSON.stringify(obj, null, 2))}</pre>`;
}

// ── Pages ─────────────────────────────────────────────────────────────────────
function page(inner) {
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sub2API OIDC Demo</title>
<style>
  :root { color-scheme: light dark; }
  body { font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
         max-width: 820px; margin: 40px auto; padding: 0 20px; line-height: 1.55; }
  h1 { font-size: 1.5rem; } h2 { font-size: 1.1rem; margin-top: 1.8rem; }
  a.btn, button { display: inline-block; padding: 8px 16px; border-radius: 8px;
         border: 1px solid #4f46e5; background: #4f46e5; color: #fff; cursor: pointer;
         font-size: .95rem; text-decoration: none; margin-right: 8px; }
  button.secondary, a.btn.secondary { background: transparent; color: #4f46e5; }
  pre { background: rgba(127,127,127,.12); padding: 12px 14px; border-radius: 8px;
        overflow-x: auto; font-size: .82rem; }
  .muted { opacity: .7; font-size: .9rem; }
  .err { background: rgba(220,38,38,.12); border: 1px solid #dc2626; padding: 12px 14px;
         border-radius: 8px; }
  code { background: rgba(127,127,127,.18); padding: 1px 5px; border-radius: 4px; }
</style></head><body>
<h1>Sub2API OIDC — RP Demo</h1>
${inner}
<hr style="margin-top:2.5rem;opacity:.2">
<p class="muted">Issuer: <code>${esc(ISSUER)}</code> &middot; Client: <code>${esc(CLIENT_ID)}</code>
&middot; Scopes: <code>${esc(SCOPES)}</code></p>
</body></html>`;
}

function homeLoggedOut() {
  return page(`
<p>You are <strong>not logged in</strong>. Start the Authorization Code + PKCE flow:</p>
<p><a class="btn" href="/login">Login with Sub2API</a></p>
<p class="muted">This server keeps the client secret server-side and verifies the ID Token
signature (RS256) against the provider JWKS.</p>`);
}

function homeLoggedIn(sess) {
  const t = sess.tokens || {};
  const expiresInfo = t.obtainedAt
    ? `expires_in ${esc(t.expires_in)}s (obtained ${new Date(t.obtainedAt).toLocaleTimeString()})`
    : '';
  return page(`
<p>You are <strong>logged in</strong>. ${esc(expiresInfo)}</p>

<form method="POST" action="/refresh" style="display:inline">
  <button ${t.refresh_token ? '' : 'disabled title="no refresh_token (request offline_access)"'}>Refresh tokens</button>
</form>
<a class="btn secondary" href="/userinfo">Fetch UserInfo (live)</a>
<form method="POST" action="/logout" style="display:inline">
  <button class="secondary">Logout</button>
</form>

<h2>ID Token claims (verified)</h2>
${pre(sess.claims || {})}

<h2>Token response</h2>
${pre({
    token_type: t.token_type,
    expires_in: t.expires_in,
    scope: t.scope,
    access_token: t.access_token ? `${String(t.access_token).slice(0, 12)}… (${t.access_token.length} chars)` : undefined,
    refresh_token: t.refresh_token ? `${String(t.refresh_token).slice(0, 12)}… (rotating)` : '(none)',
    id_token: t.id_token ? `${String(t.id_token).slice(0, 24)}…` : undefined,
  })}

${sess.userinfo ? `<h2>UserInfo</h2>${pre(sess.userinfo)}` : ''}
`);
}

function errorPage(res, status, title, detail) {
  html(
    res,
    status,
    page(`<div class="err"><strong>${esc(title)}</strong>
${detail ? `<pre>${esc(typeof detail === 'string' ? detail : JSON.stringify(detail, null, 2))}</pre>` : ''}
</div><p><a class="btn secondary" href="/">Back home</a></p>`)
  );
}

// ── Router ─────────────────────────────────────────────────────────────────────
const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://localhost:${PORT}`);
  const { sid, sess, fresh } = getSession(req);
  const setCookie = fresh ? sidCookie(sid) : undefined;

  try {
    // Home
    if (req.method === 'GET' && url.pathname === '/') {
      const body = sess.tokens ? homeLoggedIn(sess) : homeLoggedOut();
      return html(res, 200, body, setCookie ? { 'Set-Cookie': setCookie } : {});
    }

    // Start login: PKCE + state + nonce, then redirect to authorize.
    if (req.method === 'GET' && url.pathname === '/login') {
      const { authorization_endpoint } = await discovery();
      const { verifier, challenge } = pkcePair();
      const state = b64url(crypto.randomBytes(18));
      const nonce = b64url(crypto.randomBytes(18));
      sess.login = { state, nonce, codeVerifier: verifier, createdAt: Date.now() };

      const auth = new URL(authorization_endpoint);
      auth.search = new URLSearchParams({
        client_id: CLIENT_ID,
        redirect_uri: REDIRECT_URI,
        response_type: 'code',
        scope: SCOPES,
        state,
        nonce,
        code_challenge: challenge,
        code_challenge_method: 'S256',
      }).toString();
      return redirect(res, auth.toString(), setCookie);
    }

    // OAuth callback.
    if (req.method === 'GET' && url.pathname === '/callback') {
      const params = url.searchParams;
      if (params.get('error')) {
        return errorPage(res, 400, `Authorization error: ${params.get('error')}`,
          params.get('error_description') || '');
      }
      const login = sess.login;
      if (!login) return errorPage(res, 400, 'No pending login in session');
      if (params.get('state') !== login.state) {
        return errorPage(res, 400, 'state mismatch (possible CSRF)');
      }
      const code = params.get('code');
      if (!code) return errorPage(res, 400, 'missing authorization code');

      const tok = await tokenRequest({
        grant_type: 'authorization_code',
        code,
        redirect_uri: REDIRECT_URI,
        code_verifier: login.codeVerifier,
      });

      let claims = {};
      if (tok.id_token) claims = await verifyIdToken(tok.id_token, login.nonce);

      sess.tokens = { ...tok, obtainedAt: Date.now() };
      sess.claims = claims;
      delete sess.login;
      delete sess.userinfo;
      return redirect(res, '/', setCookie);
    }

    // Live UserInfo.
    if (req.method === 'GET' && url.pathname === '/userinfo') {
      if (!sess.tokens?.access_token) return redirect(res, '/', setCookie);
      sess.userinfo = await fetchUserInfo(sess.tokens.access_token);
      return redirect(res, '/', setCookie);
    }

    // Refresh (rotating token; persist the new one, discard the old).
    if (req.method === 'POST' && url.pathname === '/refresh') {
      if (!sess.tokens?.refresh_token) return redirect(res, '/', setCookie);
      const tok = await tokenRequest({
        grant_type: 'refresh_token',
        refresh_token: sess.tokens.refresh_token,
      });
      let claims = sess.claims;
      if (tok.id_token) claims = await verifyIdToken(tok.id_token, undefined);
      sess.tokens = { ...sess.tokens, ...tok, obtainedAt: Date.now() };
      sess.claims = claims;
      delete sess.userinfo;
      return redirect(res, '/', setCookie);
    }

    // Logout (local session only).
    if (req.method === 'POST' && url.pathname === '/logout') {
      sessions.delete(sid);
      return redirect(res, '/', `${SID_COOKIE}=; Path=/; Max-Age=0`);
    }

    return errorPage(res, 404, 'Not found', url.pathname);
  } catch (e) {
    console.error('[error]', e);
    return errorPage(res, 500, e.message || 'Internal error', e.detail);
  }
});

server.listen(PORT, () => {
  console.log(`\nSub2API OIDC demo running:  http://localhost:${PORT}`);
  console.log(`  issuer:       ${ISSUER}`);
  console.log(`  client_id:    ${CLIENT_ID}`);
  console.log(`  redirect_uri: ${REDIRECT_URI}`);
  console.log(`  scopes:       ${SCOPES}\n`);
});
