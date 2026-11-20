const http = require('node:http');

// Only synthetic accounts reach this local fixture; authentication is never recorded.
const accounts = new Map();
const requests = [];
const send = (res, status, body) => {
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(body));
};

http.createServer((req, res) => {
  const accountID = req.headers['chatgpt-account-id'] || 'unknown';
  if (req.method === 'GET' && req.url === '/_requests') {
    send(res, 200, requests);
    return;
  }
  if (!accountID.startsWith('fixture-')) {
    send(res, 400, { error: 'Synthetic account header required' });
    return;
  }
  if (!accounts.has(accountID)) {
    accounts.set(accountID, {
      expiresAt: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
      consumed: false,
      consumeCalls: 0,
    });
  }
  const state = accounts.get(accountID);
  const creditID = 'credit-' + accountID;
  const credit = { id: creditID, reset_type: 'codex_rate_limits', status: 'available', expires_at: state.expiresAt };
  const path = '/backend-api/wham/';
  if (req.method === 'GET' && req.url === path + 'usage') {
    requests.push({ account: accountID, action: 'usage' });
    send(res, 200, {
      rate_limit: { primary_window: { used_percent: state.consumed ? 0 : 14, limit_window_seconds: 604800,
        reset_at: Math.floor(Date.now() / 1000) + 86400, reset_after_seconds: 86400 } },
      rate_limit_reset_credits: { available_count: state.consumed ? 0 : 1 },
    });
    return;
  }
  if (req.method === 'GET' && req.url === path + 'rate-limit-reset-credits') {
    requests.push({ account: accountID, action: 'credits' });
    send(res, 200, { available_count: state.consumed ? 0 : 1, credits: state.consumed ? [] : [credit] });
    return;
  }
  if (req.method !== 'POST' || req.url !== path + 'rate-limit-reset-credits/consume') {
    send(res, 501, { error: 'Endpoint outside this fixture contract' });
    return;
  }
  const consume = raw => {
    let body;
    try { body = typeof raw === 'string' ? JSON.parse(raw) : raw; } catch {
      send(res, 400, { error: 'Invalid JSON' });
      return;
    }
    if (!body || body.credit_id !== creditID || !body.redeem_request_id) {
      send(res, 400, { error: 'Targeted credit and stable redeem request required' });
      return;
    }
    state.consumeCalls += 1;
    requests.push({ account: accountID, action: 'consume', credit_id: body.credit_id, redeem_request_id: body.redeem_request_id });
    if (accountID === 'fixture-noop') {
      // An explicit no-op leaves the synthetic card available.
      send(res, 200, { code: 'nothing_to_reset', windows_reset: 0, credit });
      return;
    }
    if (accountID === 'fixture-unknown' && state.consumeCalls === 1) {
      send(res, 200, { code: 'future_unknown', windows_reset: 0, credit });
      return;
    }
    if (accountID === 'fixture-retry' && state.consumeCalls === 1) {
      send(res, 503, { error: 'Synthetic response failure; outcome must be retried with the original identity' });
      return;
    }
    state.consumed = true;
    send(res, 200, { code: 'ok', windows_reset: 1, credit: { ...credit, status: 'redeemed', redeemed_at: new Date().toISOString() } });
  };
  if (req.body !== undefined) {
    consume(req.body);
  } else {
    let raw = '';
    req.on('data', chunk => { raw += chunk; });
    req.on('end', () => consume(raw));
  }
}).listen(process.env.PORT);
