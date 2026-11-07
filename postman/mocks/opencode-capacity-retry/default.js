const http = require('node:http');

// Fixtures reproduce observed OpenCode error shapes; store synthetic account labels only.
const requests = [];
http.createServer((req, res) => {
  if (req.method === 'GET' && req.url === '/_requests') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify(requests));
    return;
  }
  if (req.method !== 'POST' || req.url !== '/v1/chat/completions') {
    res.writeHead(501);
    res.end('Endpoint outside this fixture contract');
    return;
  }
  const body = typeof req.body === 'string' ? JSON.parse(req.body) : req.body;
  const model = body.model;
  const account = req.headers.authorization === 'Bearer fixture-account-a' ? 'a' : 'b';
  const attempt = requests.filter(row => row.model === model && row.account === account).length + 1;
  requests.push({ model, account, at: Date.now() });
  const capacity = model.includes('recover') && attempt === 1 ||
    model.includes('exhaust') && account === 'a' || model.includes('cancel');
  if (capacity) {
    const seconds = model.includes('cancel') ? 15 : model.includes('recover') ? 2 : 0;
    res.writeHead(429, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: {
      type: 'server_error',
      message: `Upstream request failed: [ttft_admission_refused] the cell is at capacity; retry shortly (after ${seconds} s)`,
    } }));
    return;
  }
  if (model.includes('invalid')) {
    res.writeHead(400, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: { type: 'server_error', message: 'Param Incorrect', param: 'max_tokens is too large' } }));
    return;
  }
  res.writeHead(200, { 'Content-Type': body.stream ? 'text/event-stream' : 'application/json' });
  const result = {
    id: 'chatcmpl-capacity-fixture', object: 'chat.completion', model,
    choices: [{ index: 0, message: { role: 'assistant', content: 'Recovered' }, finish_reason: 'stop' }],
    usage: { prompt_tokens: 2, completion_tokens: 1, total_tokens: 3 },
  };
  if (body.stream) {
    res.write('data: ' + JSON.stringify({ ...result, object: 'chat.completion.chunk', choices: [{ index: 0, delta: { content: 'Recovered' }, finish_reason: 'stop' }] }) + '\n\n');
    res.end('data: [DONE]\n\n');
  } else {
    res.end(JSON.stringify(result));
  }
}).listen(process.env.PORT);
