const base = (process.env.X5M5X_API_BASE || 'https://us-api.x5m5x.com').replace(/\/$/, '')
const apiKey = process.env.X5M5X_TOKEN_KEY || ''
const budget = Number(process.env.X5M5X_PROBE_BUDGET || '0.05')
const concurrency = Math.max(1, Math.min(5, Number(process.env.X5M5X_PROBE_CONCURRENCY || '3')))
const requestedModels = new Set(
  (process.env.X5M5X_PROBE_MODELS || '')
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
)

if (!apiKey) throw new Error('X5M5X_TOKEN_KEY is required')
if (!Number.isFinite(budget) || budget <= 0) throw new Error('X5M5X_PROBE_BUDGET must be positive')

const commonHeaders = {
  Authorization: `Bearer ${apiKey}`,
  Accept: 'application/json'
}

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

async function fetchWithTimeout(url, options = {}, timeoutMs = 60_000) {
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
      const response = await fetchWithTimeout(`${base}${path}`, { headers: commonHeaders }, 45_000)
      const text = await response.text()
      if (!response.ok) throw new Error(`GET ${path} HTTP ${response.status}: ${text.slice(0, 240)}`)
      return JSON.parse(text)
    } catch (error) {
      lastError = error
      if (attempt < attempts) await sleep(1000 * attempt)
    }
  }
  throw lastError
}

function usageStat(usage, model) {
  const item = Array.isArray(usage?.model_stats)
    ? usage.model_stats.find(entry => entry?.model === model)
    : null
  return {
    requests: Number(item?.requests || 0),
    input: Number(item?.input_tokens || 0),
    output: Number(item?.output_tokens || 0),
    cacheWrite: Number(item?.cache_creation_tokens || 0),
    cacheRead: Number(item?.cache_read_tokens || 0),
    cost: Number(item?.actual_cost || 0)
  }
}

function totalCost(usage) {
  return Number(usage?.usage?.total?.actual_cost || 0)
}

function subtract(before, after) {
  return {
    requests: after.requests - before.requests,
    input: after.input - before.input,
    output: after.output - before.output,
    cacheWrite: after.cacheWrite - before.cacheWrite,
    cacheRead: after.cacheRead - before.cacheRead,
    cost: Number((after.cost - before.cost).toFixed(12))
  }
}

async function waitForLedger(model, before, maxWaitMs = 45_000) {
  const deadline = Date.now() + maxWaitMs
  let last = before
  while (Date.now() < deadline) {
    await sleep(1500)
    const usage = await getJSON('/v1/usage')
    last = usageStat(usage, model)
    if (last.requests > before.requests) return { usage, stat: last, delta: subtract(before, last) }
  }
  return { usage: null, stat: last, delta: subtract(before, last) }
}

let startUsage = await getJSON('/v1/usage')
const startCost = totalCost(startUsage)
let latestTotalCost = startCost
let budgetStopped = false

async function runSample(model, spec) {
  const beforeUsage = await getJSON('/v1/usage')
  latestTotalCost = Math.max(latestTotalCost, totalCost(beforeUsage))
  const spentBefore = latestTotalCost - startCost
  if (spentBefore >= budget) {
    budgetStopped = true
    return { kind: spec.kind, error: 'budget_exhausted' }
  }

  const before = usageStat(beforeUsage, model)
  const session = `pricing-probe-${crypto.randomUUID()}`
  const body = JSON.stringify({
    model,
    messages: [{ role: 'user', content: spec.content }],
    max_tokens: spec.maxTokens,
    temperature: 0,
    stream: false
  })

  let responseInfo = null
  let postError = null
  try {
    const response = await fetchWithTimeout(`${base}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        ...commonHeaders,
        'Content-Type': 'application/json',
        'X-Session-Id': session
      },
      body
    }, 120_000)
    const text = await response.text()
    if (!response.ok) {
      postError = `HTTP ${response.status}: ${text.slice(0, 240)}`
    } else {
      const parsed = JSON.parse(text)
      responseInfo = {
        model: parsed.model || '',
        prompt_tokens: Number(parsed.usage?.prompt_tokens || 0),
        completion_tokens: Number(parsed.usage?.completion_tokens || 0),
        cached_tokens: Number(parsed.usage?.prompt_tokens_details?.cached_tokens || 0)
      }
    }
  } catch (error) {
    postError = `transport_error: ${error.message}`
  }

  // Even after a transport error, inspect the ledger before considering any retry.
  const ledger = await waitForLedger(model, before, postError ? 20_000 : 45_000)
  if (ledger.usage) {
    latestTotalCost = Math.max(latestTotalCost, totalCost(ledger.usage))
  }
  if (ledger.delta.requests !== 1) {
    return {
      kind: spec.kind,
      error: postError || `ledger_request_delta_${ledger.delta.requests}`,
      response: responseInfo,
      delta: ledger.delta
    }
  }
  if (ledger.delta.input < 0 || ledger.delta.output < 0 || ledger.delta.cost < 0) {
    return { kind: spec.kind, error: 'negative_ledger_delta', response: responseInfo, delta: ledger.delta }
  }
  return { kind: spec.kind, response: responseInfo, delta: ledger.delta, post_warning: postError }
}

function solveThree(samples) {
  let best = null
  for (let a = 0; a < samples.length - 2; a++) {
    for (let b = a + 1; b < samples.length - 1; b++) {
      for (let c = b + 1; c < samples.length; c++) {
        const r0 = samples[a].delta
        const r1 = samples[b].delta
        const r2 = samples[c].delta
        const di1 = r1.input - r0.input
        const do1 = r1.output - r0.output
        const dc1 = r1.cost - r0.cost
        const di2 = r2.input - r0.input
        const do2 = r2.output - r0.output
        const dc2 = r2.cost - r0.cost
        const determinant = di1 * do2 - di2 * do1
        if (Math.abs(determinant) < 1e-9) continue
        const inputPerToken = (dc1 * do2 - dc2 * do1) / determinant
        const outputPerToken = (di1 * dc2 - di2 * dc1) / determinant
        const fixed = r0.cost - r0.input * inputPerToken - r0.output * outputPerToken
        const candidate = { inputPerToken, outputPerToken, fixed, determinant: Math.abs(determinant) }
        if (!best || candidate.determinant > best.determinant) best = candidate
      }
    }
  }
  if (!best) return null
  let maxResidual = 0
  for (const sample of samples) {
    const row = sample.delta
    const predicted = row.input * best.inputPerToken + row.output * best.outputPerToken + best.fixed
    maxResidual = Math.max(maxResidual, Math.abs(predicted - row.cost))
  }
  return {
    input_per_million: best.inputPerToken * 1_000_000,
    output_per_million: best.outputPerToken * 1_000_000,
    fixed_per_request: best.fixed,
    max_residual: maxResidual
  }
}

function solveNoFixed(samples) {
  let best = null
  for (let a = 0; a < samples.length - 1; a++) {
    for (let b = a + 1; b < samples.length; b++) {
      const x = samples[a].delta
      const y = samples[b].delta
      const determinant = x.input * y.output - y.input * x.output
      if (Math.abs(determinant) < 1e-9) continue
      const inputPerToken = (x.cost * y.output - y.cost * x.output) / determinant
      const outputPerToken = (x.input * y.cost - y.input * x.cost) / determinant
      const candidate = { inputPerToken, outputPerToken, determinant: Math.abs(determinant) }
      if (!best || candidate.determinant > best.determinant) best = candidate
    }
  }
  if (!best) return null
  let maxResidual = 0
  for (const sample of samples) {
    const row = sample.delta
    const predicted = row.input * best.inputPerToken + row.output * best.outputPerToken
    maxResidual = Math.max(maxResidual, Math.abs(predicted - row.cost))
  }
  return {
    input_per_million: best.inputPerToken * 1_000_000,
    output_per_million: best.outputPerToken * 1_000_000,
    fixed_per_request: 0,
    max_residual: maxResidual,
    assumption: 'no_fixed_request_fee'
  }
}

function round(value, digits = 8) {
  return Number.isFinite(value) ? Number(value.toFixed(digits)) : null
}

async function probeModel(model) {
  const nonce = () => crypto.randomUUID().replaceAll('-', '')
  const specs = [
    {
      kind: 'short_input',
      maxTokens: 1,
      content: `Probe ${nonce()}. Return exactly the single letter A.`
    },
    {
      kind: 'medium_input',
      maxTokens: 1,
      content: `Probe ${nonce()}. ${'calibrationword '.repeat(96)} Return exactly the single letter A.`
    },
    {
      kind: 'output_mix',
      maxTokens: 32,
      content: `Probe ${nonce()}. Write the integers 1 through 40 separated by commas, with no other text.`
    }
  ]

  const samples = []
  for (const spec of specs) {
    if (budgetStopped) break
    const sample = await runSample(model, spec)
    samples.push(sample)
  }

  let valid = samples.filter(sample => !sample.error && sample.delta?.requests === 1)
  const outputVariation = new Set(valid.map(sample => sample.delta.output)).size > 1
  if (!budgetStopped && valid.length >= 2 && !outputVariation) {
    const fallback = await runSample(model, {
      kind: 'output_fallback',
      maxTokens: 64,
      content: `Probe ${nonce()}. Write a forty-word factual sentence and nothing else.`
    })
    samples.push(fallback)
    valid = samples.filter(sample => !sample.error && sample.delta?.requests === 1)
  }

  const full = valid.length >= 3 ? solveThree(valid) : null
  const solved = full || (valid.length >= 2 ? solveNoFixed(valid) : null)
  const clean = solved && solved.input_per_million >= 0 && solved.output_per_million >= 0
  const maxCost = Math.max(0, ...valid.map(sample => sample.delta.cost))
  const tolerance = Math.max(5e-8, maxCost * 0.01)
  const confidence = clean && solved.max_residual <= tolerance
    ? (full ? 'measured' : 'measured_assuming_no_fixed_fee')
    : 'unresolved'

  return {
    model,
    confidence,
    input_per_million: clean ? round(solved.input_per_million, 8) : null,
    output_per_million: clean ? round(solved.output_per_million, 8) : null,
    fixed_per_request: clean ? round(solved.fixed_per_request, 12) : null,
    max_residual: clean ? round(solved.max_residual, 12) : null,
    samples
  }
}

const modelResponse = await getJSON('/v1/models')
const models = (Array.isArray(modelResponse?.data) ? modelResponse.data : modelResponse)
  .map(item => typeof item === 'string' ? item : item?.id)
  .filter(Boolean)
  .filter(model => requestedModels.size === 0 || requestedModels.has(model))

const results = []
let nextModel = 0

async function worker(workerID) {
  while (!budgetStopped) {
    const index = nextModel++
    if (index >= models.length) return
    const model = models[index]
    const result = await probeModel(model)
    results.push(result)
    console.log(`PROGRESS ${results.length}/${models.length} ${model} ${result.confidence}`)
  }
  console.log(`WORKER ${workerID} stopped by budget`)
}

await Promise.all(Array.from({ length: concurrency }, (_, index) => worker(index + 1)))

const endUsage = await getJSON('/v1/usage')
const endCost = totalCost(endUsage)
const output = {
  base,
  model_count: models.length,
  completed_models: results.length,
  budget,
  budget_stopped: budgetStopped,
  start_cost: startCost,
  end_cost: endCost,
  run_cost: Number((endCost - startCost).toFixed(12)),
  results: results.sort((a, b) => a.model.localeCompare(b.model))
}

console.log(`RESULT ${JSON.stringify(output)}`)
