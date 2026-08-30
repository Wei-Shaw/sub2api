const base = (process.env.X5M5X_API_BASE || 'https://us-api.x5m5x.com').replace(/\/$/, '')
const apiKey = process.env.X5M5X_TOKEN_KEY || ''
const concurrency = Math.max(1, Math.min(3, Number(process.env.X5M5X_PROBE_CONCURRENCY || '2')))
const cachePrefixRepeats = Math.max(64, Math.min(400, Number(process.env.X5M5X_CACHE_PREFIX_REPEATS || '220')))
const requestedModels = new Set(
  (process.env.X5M5X_PROBE_MODELS || '')
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
)

if (!apiKey) throw new Error('X5M5X_TOKEN_KEY is required')

const headers = { Authorization: `Bearer ${apiKey}`, Accept: 'application/json' }
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

async function fetchTimeout(url, options = {}, timeoutMs = 120_000) {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)
  try {
    return await fetch(url, { ...options, signal: controller.signal })
  } finally {
    clearTimeout(timer)
  }
}

async function getJSON(path, attempts = 5) {
  let lastError
  for (let attempt = 1; attempt <= attempts; attempt++) {
    try {
      const response = await fetchTimeout(`${base}${path}`, { headers }, 45_000)
      const text = await response.text()
      if (!response.ok) throw new Error(`GET ${path} HTTP ${response.status}: ${text.slice(0, 240)}`)
      return JSON.parse(text)
    } catch (error) {
      lastError = error
      if (attempt < attempts) await sleep(attempt * 1000)
    }
  }
  throw lastError
}

function stat(usage, model) {
  const row = Array.isArray(usage?.model_stats)
    ? usage.model_stats.find(item => item?.model === model)
    : null
  return {
    requests: Number(row?.requests || 0),
    input: Number(row?.input_tokens || 0),
    output: Number(row?.output_tokens || 0),
    write: Number(row?.cache_creation_tokens || 0),
    read: Number(row?.cache_read_tokens || 0),
    cost: Number(row?.actual_cost || 0)
  }
}

function delta(before, after) {
  return {
    requests: after.requests - before.requests,
    input: after.input - before.input,
    output: after.output - before.output,
    cache_write: after.write - before.write,
    cache_read: after.read - before.read,
    cost: Number((after.cost - before.cost).toFixed(12))
  }
}

async function waitLedger(model, before, maxMs = 45_000) {
  const deadline = Date.now() + maxMs
  let current = before
  while (Date.now() < deadline) {
    await sleep(1500)
    const usage = await getJSON('/v1/usage')
    current = stat(usage, model)
    if (current.requests > before.requests) return delta(before, current)
  }
  return delta(before, current)
}

async function send(model, body, session) {
  const before = stat(await getJSON('/v1/usage'), model)
  let responseUsage = null
  let error = null
  try {
    const response = await fetchTimeout(`${base}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        ...headers,
        'Content-Type': 'application/json',
        'X-Session-Id': session
      },
      body: JSON.stringify(body)
    })
    const text = await response.text()
    if (!response.ok) error = `HTTP ${response.status}: ${text.slice(0, 240)}`
    else responseUsage = JSON.parse(text).usage || null
  } catch (caught) {
    error = `transport_error: ${caught.message}`
  }

  // Always inspect the ledger before any subsequent request.
  const observed = await waitLedger(model, before, error ? 15_000 : 45_000)
  if (observed.requests !== 1) return { error: error || `ledger_request_delta_${observed.requests}`, usage: responseUsage, delta: observed }
  return { error, usage: responseUsage, delta: observed }
}

function priceFromResidual(row, inputPrice, outputPrice) {
  return row.cost - row.input * inputPrice / 1_000_000 - row.output * outputPrice / 1_000_000
}

function solveCache(rows, inputPrice, outputPrice) {
  const usable = rows.filter(row => row?.delta?.requests === 1 && !row.error)
  const observations = usable.map(row => ({
    write: row.delta.cache_write,
    read: row.delta.cache_read,
    residual: priceFromResidual(row.delta, inputPrice, outputPrice)
  }))
  let writePrice = null
  let readPrice = null

  for (const item of observations) {
    if (item.write > 0 && item.read === 0) writePrice = item.residual * 1_000_000 / item.write
    if (item.read > 0 && item.write === 0) readPrice = item.residual * 1_000_000 / item.read
  }
  if ((writePrice == null || readPrice == null) && observations.length >= 2) {
    for (let i = 0; i < observations.length - 1; i++) {
      for (let j = i + 1; j < observations.length; j++) {
        const a = observations[i]
        const b = observations[j]
        const determinant = a.write * b.read - b.write * a.read
        if (Math.abs(determinant) < 1e-9) continue
        writePrice = (a.residual * b.read - b.residual * a.read) * 1_000_000 / determinant
        readPrice = (a.write * b.residual - b.write * a.residual) * 1_000_000 / determinant
      }
    }
  }

  const normalize = value => Number.isFinite(value) && value >= -1e-8 ? Number(Math.max(0, value).toFixed(8)) : null
  return {
    cache_write_per_million: normalize(writePrice),
    cache_read_per_million: normalize(readPrice),
    observed_write_tokens: observations.reduce((sum, row) => sum + row.write, 0),
    observed_read_tokens: observations.reduce((sum, row) => sum + row.read, 0)
  }
}

function decodeText(html) {
  return html.replace(/<[^>]+>/g, '').replaceAll('&nbsp;', ' ').trim()
}

function parsePrice(html) {
  const value = decodeText(html).replace(/[\u00a5\uffe5,]/g, '').trim()
  return /^\d+(?:\.\d+)?$/.test(value) ? Number(value) : null
}

async function declaredPrices() {
  const response = await fetchTimeout('https://api.x5m5x.com/pricing/', { headers: { Accept: 'text/html' } }, 45_000)
  const html = await response.text()
  const map = new Map()
  const rows = html.match(/<tr\b(?=[^>]*\bclass=["'][^"']*\btoken-model\b[^"']*["'])[^>]*>.*?<\/tr>/gis) || []
  for (const row of rows) {
    const name = row.match(/\bdata-model=["']([^"']+)["']/i)?.[1]
    if (!name) continue
    const cell = label => row.match(new RegExp(`<td\\b(?=[^>]*\\bdata-label=["']${label}["'])[^>]*>(.*?)<\\/td>`, 'is'))?.[1] || ''
    const strong = content => [...content.matchAll(/<strong\b[^>]*>(.*?)<\/strong>/gis)].map(match => parsePrice(match[1]))
    const input = strong(cell('输入'))[0]
    const output = strong(cell('输出'))[0]
    map.set(name.toLowerCase(), { input, output })
  }
  return map
}

const priceMap = await declaredPrices()
const modelResponse = await getJSON('/v1/models')
const allModels = (Array.isArray(modelResponse?.data) ? modelResponse.data : modelResponse)
  .map(item => typeof item === 'string' ? item : item?.id)
  .filter(Boolean)
const models = allModels.filter(model => requestedModels.size === 0 || requestedModels.has(model))

const prefix = 'stable system cache knowledge alpha beta gamma delta epsilon zeta eta theta '.repeat(cachePrefixRepeats)
const results = []
let cursor = 0

async function probeModel(model) {
  const prices = priceMap.get(model.toLowerCase())
  if (!prices || prices.input == null || prices.output == null) {
    return { model, error: 'missing_declared_input_output_for_residual' }
  }

  const session = `pricing-cache-${model}-${crypto.randomUUID()}`
  const plainBody = {
    model,
    messages: [
      { role: 'system', content: prefix },
      { role: 'user', content: 'Reply exactly A.' }
    ],
    max_tokens: 1,
    stream: false
  }
  const first = await send(model, plainBody, session)
  const second = first.delta?.requests === 1 ? await send(model, plainBody, session) : { error: 'first_request_not_billed_once' }
  let rows = [first, second]

  // If normal sticky repetition produces no cache telemetry, try the standard
  // cache_control shape once. Unsupported models fail closed without retries.
  const noCache = rows.every(row => !row?.delta?.cache_write && !row?.delta?.cache_read)
  let explicit = []
  if (noCache) {
    const explicitSession = `pricing-cache-explicit-${model}-${crypto.randomUUID()}`
    const explicitBody = {
      model,
      messages: [
        {
          role: 'system',
          content: [{ type: 'text', text: prefix, cache_control: { type: 'ephemeral' } }]
        },
        { role: 'user', content: 'Reply exactly A.' }
      ],
      max_tokens: 1,
      stream: false
    }
    const explicitFirst = await send(model, explicitBody, explicitSession)
    const explicitSecond = explicitFirst.delta?.requests === 1
      ? await send(model, explicitBody, explicitSession)
      : { error: 'explicit_first_request_not_billed_once' }
    explicit = [explicitFirst, explicitSecond]
    rows = rows.concat(explicit)
  }

  return {
    model,
    input_per_million: prices.input,
    output_per_million: prices.output,
    ...solveCache(rows, prices.input, prices.output),
    plain: [first, second],
    explicit
  }
}

async function worker() {
  while (true) {
    const index = cursor++
    if (index >= models.length) return
    const result = await probeModel(models[index])
    results.push(result)
    console.log(`PROGRESS ${results.length}/${models.length} ${result.model} read=${result.cache_read_per_million ?? '-'} write=${result.cache_write_per_million ?? '-'}`)
  }
}

const startUsage = await getJSON('/v1/usage')
const startCost = Number(startUsage?.usage?.total?.actual_cost || 0)
await Promise.all(Array.from({ length: concurrency }, () => worker()))
const endUsage = await getJSON('/v1/usage')
const endCost = Number(endUsage?.usage?.total?.actual_cost || 0)

console.log(`RESULT ${JSON.stringify({
  models: results.sort((a, b) => a.model.localeCompare(b.model)),
  start_cost: startCost,
  end_cost: endCost,
  run_cost: Number((endCost - startCost).toFixed(12))
})}`)
