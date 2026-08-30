import http from 'node:http'

const host = '127.0.0.1'
const port = Number(process.env.PREVIEW_API_PORT || 8081)
const now = new Date().toISOString()

const tokenModelNames = [
  'claude-fable-5', 'claude-opus-4-7', 'claude-opus-4-8', 'claude-opus-5', 'claude-sonnet-5',
  'deepseek-v4-flash-0731', 'deepseek-v4-flash-vision-exp', 'deepseek-v4-pro-0813',
  'gemini-3.1-pro-preview', 'gemini-3.5-flash', 'gemini-3.6-flash', 'gemini-3.7-flash',
  'glm-5.1', 'glm-5.2', 'glm-5.3', 'glm-5.3-flash',
  'gpt-5.5', 'gpt-5.6-luna', 'gpt-5.6-sol', 'gpt-5.6-terra',
  'grok-4.5', 'grok-4.6', 'hy3', 'kimi-k2.6', 'kimi-k2.7-code', 'kimi-k3',
  'mimo-v2.5', 'mimo-v2.5-pro', 'MiniMax-M2.7', 'MiniMax-M2.7-highspeed', 'MiniMax-M3',
  'qwen3.7-max', 'qwen3.8-max'
]

const perRequestModelNames = [
  'Auto-Model', 'deepseek-v4-flash-0731', 'deepseek-v4-pro-0813',
  'glm-5.1', 'glm-5.2', 'glm-5.3', 'glm-5.3-flash', 'gpt-5.6', 'grok-4.6',
  'kimi-k2.6', 'kimi-k2.7-code', 'MiniMax-M2.7', 'MiniMax-M2.7-highspeed', 'MiniMax-M3'
]

const identityMapping = models => Object.fromEntries(models.map(model => [model, model]))

const tokenChannelPricing = tokenModelNames.map((model, index) => ({
  id: 1000 + index,
  platform: 'openai',
  models: [model],
  billing_mode: 'token',
  input_price: model === 'deepseek-v4-flash-0731' ? 0.000000192 : 0.0000012,
  output_price: model === 'deepseek-v4-flash-0731' ? 0.000000564 : 0.0000036,
  cache_write_price: null,
  cache_read_price: model === 'deepseek-v4-flash-0731' ? 0.000000036 : null,
  fast_multiplier: null,
  flex_multiplier: null,
  image_input_price: null,
  image_output_price: null,
  per_request_price: null,
  intervals: [],
  time_pricing: model.startsWith('deepseek-')
    ? {
        timezone: 'Asia/Shanghai',
        weekdays_only: true,
        periods: [
          { start_time: '09:00', end_time: '12:00', multiplier: 2 },
          { start_time: '14:00', end_time: '18:00', multiplier: 2 }
        ]
      }
    : null
}))

const requestChannelPricing = perRequestModelNames.map((model, index) => {
  const base = model === 'deepseek-v4-flash-0731' ? 0.006 : 0.012
  return {
    id: 2000 + index,
    platform: 'openai',
    models: [model],
    billing_mode: 'per_request',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    fast_multiplier: null,
    flex_multiplier: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: base,
    intervals: [
      { min_tokens: 0, max_tokens: 256000, tier_label: '≤ 256K', per_request_price: base, sort_order: 0 },
      { min_tokens: 256000, max_tokens: 512000, tier_label: '256K–512K', per_request_price: base * 1.5, sort_order: 1 },
      { min_tokens: 512000, max_tokens: null, tier_label: '> 512K', per_request_price: base * 2, sort_order: 2 }
    ].map(item => ({
      ...item,
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      input_multiplier: null,
      output_multiplier: null,
      cache_write_multiplier: null,
      cache_read_multiplier: null
    })),
    time_pricing: null
  }
})

const imageChannelPricing = [{
  id: 3000,
  platform: 'openai',
  models: ['gpt-image-2'],
  billing_mode: 'image',
  input_price: null,
  output_price: null,
  cache_write_price: null,
  cache_read_price: null,
  fast_multiplier: null,
  flex_multiplier: null,
  image_input_price: null,
  image_output_price: null,
  per_request_price: 0.036,
  intervals: [],
  time_pricing: null
}]

function previewGroup(id, name, description, models, allowImages = false) {
  return {
    id,
    name,
    description,
    platform: 'openai',
    rate_multiplier: 1,
    rpm_limit: 0,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'standard',
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    long_context_pricing_enabled: true,
    allow_image_generation: allowImages,
    allow_batch_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    batch_image_discount_multiplier: 0.5,
    batch_image_hold_multiplier: 0.6,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    video_price_480p: null,
    video_price_720p: null,
    video_price_1080p: null,
    web_search_price_per_call: null,
    search_price_per_1k: null,
    audio_realtime_price_per_min: null,
    audio_tts_price_per_million_chars: null,
    audio_stt_price_per_hour: null,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    allow_live: false,
    require_oauth_only: false,
    require_privacy_set: false,
    profit_control_enabled: false,
    profit_min_margin: 0,
    profit_safety_buffer: 0,
    model_routing: null,
    model_routing_enabled: false,
    mcp_xml_inject: false,
    account_count: 1,
    active_account_count: 1,
    rate_limited_account_count: 0,
    models_list_config: { enabled: true, models },
    model_pricing: [],
    sort_order: id * 10,
    created_at: now,
    updated_at: now
  }
}

const previewGroups = [
  previewGroup(2, '按量分组【成功率百分之99+】', '完全对接官方模型，适合企业级用户', tokenModelNames),
  previewGroup(3, '按次分组【成功率百分之95+】', '按上下文区间扣费', perRequestModelNames),
  previewGroup(4, '生图分组-按次扣费', '生图稳定版', ['gpt-image-2'], true)
]

function previewChannel(id, name, description, groupId, modelPricing, mappingModels) {
  return {
    id,
    name,
    description,
    status: 'active',
    billing_model_source: 'requested',
    restrict_models: true,
    features_config: {},
    group_ids: [groupId],
    model_pricing: modelPricing,
    model_mapping: { openai: identityMapping(mappingModels) },
    apply_pricing_to_account_stats: false,
    account_stats_pricing_rules: [],
    created_at: now,
    updated_at: now
  }
}

const previewChannels = [
  previewChannel(1, 'x5m5x-payg-channel', '按量渠道 · 33 个模型', 2, tokenChannelPricing, tokenModelNames),
  previewChannel(2, 'x5m5x-per-request-channel', '按次渠道 · 14 个模型', 3, requestChannelPricing, perRequestModelNames),
  previewChannel(3, 'x5m5x-image-channel', '生图渠道 · 1 个模型', 4, imageChannelPricing, ['gpt-image-2'])
]

const previewMonitors = [
  ['按量渠道健康检查', 'deepseek-v4-flash-0731', 99.96, 420],
  ['按次渠道健康检查', 'Auto-Model', 96.80, 610],
  ['生图渠道健康检查', 'gpt-image-2', 98.50, 1850]
].map(([name, model, availability, latency], index) => ({
  id: index + 1,
  name,
  provider: 'openai',
  api_mode: 'chat_completions',
  endpoint: 'https://api.x5m5x.com/v1',
  api_key_masked: 'sk-****preview',
  primary_model: model,
  extra_models: [],
  group_name: previewGroups[index].name,
  enabled: true,
  interval_seconds: 300,
  jitter_seconds: 15,
  last_checked_at: now,
  created_by: 1,
  created_at: now,
  updated_at: now,
  primary_status: 'operational',
  primary_latency_ms: latency,
  availability_7d: availability,
  extra_models_status: [],
  template_id: null,
  extra_headers: {},
  body_override_mode: 'off',
  body_override: null,
  check_mode: 'probe',
  account_id: null,
  latest_quota: null
}))

const previewUser = {
  id: 1001,
  username: 'Preview User',
  email: 'preview@local.test',
  role: 'user',
  balance: 100,
  frozen_balance: 0,
  concurrency: 3,
  status: 'active',
  allowed_groups: null,
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: now,
  updated_at: now,
  run_mode: 'standard'
}

const previewAdmin = {
  ...previewUser,
  id: 1,
  username: 'Preview Admin',
  email: 'admin@local.test',
  role: 'admin'
}

const deepSeekModels = [
  {
    id: 1,
    platform: 'openai',
    model_name: 'deepseek-v4-flash-0731',
    model_note: '',
    billing_mode: 'token',
    provider: 'deepseek',
    currency: 'CNY',
    configured: true,
    enabled: true,
    official_prices: {
      input_per_million: 1.6,
      output_per_million: 4.7,
      cache_write_per_million: null,
      cache_read_per_million: 0.1
    },
    model_multiplier: 0.1,
    effective_multiplier: 0.1,
    display_prices: {
      input_per_million: 0.16,
      output_per_million: 0.47,
      cache_write_per_million: null,
      cache_read_per_million: 0.03
    },
    per_request: null,
    image_prices: []
  },
  {
    id: 2,
    platform: 'openai',
    model_name: 'deepseek-v4-pro-0813',
    model_note: '',
    billing_mode: 'token',
    provider: 'deepseek',
    currency: 'CNY',
    configured: true,
    enabled: true,
    official_prices: {
      input_per_million: 4.7,
      output_per_million: 13.9,
      cache_write_per_million: null,
      cache_read_per_million: 0.2
    },
    model_multiplier: 0.1,
    effective_multiplier: 0.1,
    display_prices: {
      input_per_million: 0.47,
      output_per_million: 1.39,
      cache_write_per_million: null,
      cache_read_per_million: 0.06
    },
    per_request: null,
    image_prices: []
  },
  {
    id: 3,
    platform: 'openai',
    model_name: 'deepseek-v4-flash-vision-exp',
    model_note: '新模型上线初期资源较为紧张，当前价格偏高；待资源供应充足后将适时下调。',
    billing_mode: 'token',
    provider: 'deepseek',
    currency: 'CNY',
    configured: true,
    enabled: true,
    official_prices: {
      input_per_million: 1.6,
      output_per_million: 4.7,
      cache_write_per_million: null,
      cache_read_per_million: 0.1
    },
    model_multiplier: 0.375,
    effective_multiplier: 0.375,
    display_prices: {
      input_per_million: 0.6,
      output_per_million: 1.7625,
      cache_write_per_million: null,
      cache_read_per_million: 0.03
    },
    per_request: null,
    image_prices: []
  }
]

const publicSettings = {
  site_name: 'Sub2API Local Preview',
  site_logo: '',
  site_version: 'local',
  model_plaza_enabled: true,
  model_plaza_require_auth: true,
  backend_mode_enabled: false,
  channel_monitor_enabled: true,
  channel_monitor_mode: 'v1',
  channel_monitor_default_interval_seconds: 300,
  channel_monitor_hide_throughput: true,
  channel_monitor_show_quota: false,
  available_channels_enabled: true,
  payment_enabled: false,
  affiliate_enabled: false,
  risk_control_enabled: false,
  plugin_management_enabled: false,
  compact_home_enabled: false,
  custom_menu_items: []
}

const catalog = {
  global_multiplier: 1,
  updated_at: now,
  providers: [
    {
      provider: 'deepseek',
      display_name: 'DeepSeek',
      provider_note: 'DeepSeek 平常价格展示；高峰期为工作日北京时间 09:00–12:00、14:00–18:00，高峰价格按平常价格 ×2 计算。',
      currency: 'CNY',
      logo_key: 'deepseek',
      logo_url: '',
      configured_multiplier: 0.1,
      effective_multiplier: 0.1,
      models: deepSeekModels
    }
  ]
}

function send(res, data, status = 200) {
  const body = JSON.stringify({ code: 0, message: 'ok', data })
  res.writeHead(status, {
    'Content-Type': 'application/json; charset=utf-8',
    'Cache-Control': 'no-store',
    'Content-Length': Buffer.byteLength(body)
  })
  res.end(body)
}

function readBody(req) {
  return new Promise((resolve) => {
    let body = ''
    req.setEncoding('utf8')
    req.on('data', chunk => { body += chunk })
    req.on('end', () => resolve(body))
  })
}

function adminProvider() {
  const provider = catalog.providers[0]
  return {
    provider: provider.provider,
    display_name: provider.display_name,
    provider_note: provider.provider_note,
    currency: provider.currency,
    multiplier: provider.configured_multiplier,
    logo_key: provider.logo_key,
    logo_url: provider.logo_url,
    sort_order: 20,
    updated_at: catalog.updated_at
  }
}

function adminModels() {
  return deepSeekModels.map((model, index) => ({
    id: model.id,
    platform: model.platform,
    model_name: model.model_name,
    provider: model.provider,
    billing_mode: model.billing_mode,
    currency: model.currency,
    enabled: model.enabled,
    sort_order: 100 + index,
    model_note: model.model_note,
    official_input_per_million: model.official_prices?.input_per_million ?? null,
    official_output_per_million: model.official_prices?.output_per_million ?? null,
    official_cache_write_per_million: model.official_prices?.cache_write_per_million ?? null,
    official_cache_read_per_million: model.official_prices?.cache_read_per_million ?? null,
    model_multiplier: model.model_multiplier,
    per_request_lte_256k: null,
    per_request_256k_512k_override: null,
    per_request_gt_512k_override: null,
    image_prices: [],
    created_at: now,
    updated_at: catalog.updated_at
  }))
}

function requestUser(req) {
  const authorization = String(req.headers.authorization || '')
  return authorization.includes('admin') ? previewAdmin : previewUser
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || '/', `http://${host}:${port}`)
  const path = url.pathname

  if (req.method === 'OPTIONS') {
    res.writeHead(204)
    res.end()
    return
  }
  if (path === '/health') {
    send(res, { status: 'ok' })
    return
  }
  if (path === '/api/v1/settings/public') {
    send(res, publicSettings)
    return
  }
  if (path === '/api/v1/auth/login' && req.method === 'POST') {
    const raw = await readBody(req)
    let email = ''
    try { email = String(JSON.parse(raw).email || '') } catch { email = '' }
    const isAdmin = email.toLowerCase().startsWith('admin@')
    send(res, {
      access_token: isAdmin ? 'local-preview-admin-token' : 'local-preview-token',
      token_type: 'Bearer',
      expires_in: 86400,
      user: isAdmin ? previewAdmin : previewUser
    })
    return
  }
  if (path === '/api/v1/auth/me') {
    send(res, requestUser(req))
    return
  }
  if (path === '/api/v1/model-prices') {
    send(res, catalog)
    return
  }
  if (path === '/api/v1/admin/compliance') {
    send(res, { required: false })
    return
  }
  if (path === '/api/v1/admin/display-pricing/settings') {
    if (req.method === 'PUT') {
      const raw = await readBody(req)
      try { catalog.global_multiplier = Number(JSON.parse(raw).global_multiplier || 1) } catch { /* preview only */ }
    }
    send(res, { global_multiplier: catalog.global_multiplier, updated_at: catalog.updated_at })
    return
  }
  if (path === '/api/v1/admin/display-pricing/providers') {
    send(res, { items: [adminProvider()] })
    return
  }
  if (path === '/api/v1/admin/display-pricing/providers/deepseek' && req.method === 'PUT') {
    const raw = await readBody(req)
    try {
      const payload = JSON.parse(raw)
      catalog.providers[0].display_name = String(payload.display_name || 'DeepSeek')
      catalog.providers[0].provider_note = String(payload.provider_note || '')
      catalog.providers[0].logo_key = String(payload.logo_key || 'deepseek')
      catalog.providers[0].logo_url = String(payload.logo_url || '')
      catalog.providers[0].configured_multiplier = payload.multiplier == null ? null : Number(payload.multiplier)
    } catch { /* preview only */ }
    send(res, adminProvider())
    return
  }
  if (path === '/api/v1/admin/display-pricing/models') {
    send(res, { items: adminModels() })
    return
  }
  if (path.startsWith('/api/v1/admin/display-pricing/models/') && req.method === 'PUT') {
    const id = Number(path.split('/').at(-1))
    const raw = await readBody(req)
    const model = deepSeekModels.find(item => item.id === id)
    if (model) {
      try { model.model_note = String(JSON.parse(raw).model_note || '') } catch { /* preview only */ }
    }
    send(res, adminModels().find(item => item.id === id) || null)
    return
  }
  if (path === '/api/v1/admin/display-pricing/discovered-models') {
    send(res, {
      items: deepSeekModels.map(model => ({
        platform: model.platform,
        model_name: model.model_name,
        billing_mode: model.billing_mode,
        provider: model.provider,
        configured: true
      }))
    })
    return
  }
  if (path === '/api/v1/admin/groups/usage-summary') {
    send(res, previewGroups.map((group, index) => ({
      group_id: group.id,
      today_cost: 1.25 + index,
      yesterday_cost: 0.8 + index,
      total_cost: 32.5 + index * 10
    })))
    return
  }
  if (path === '/api/v1/admin/groups/capacity-summary') {
    send(res, previewGroups.map(group => ({
      group_id: group.id,
      concurrency_used: 0,
      concurrency_max: 3,
      sessions_used: 0,
      sessions_max: 3,
      rpm_used: 0,
      rpm_max: 0
    })))
    return
  }
  if (path === '/api/v1/admin/groups/live-capability') {
    send(res, { supported: true })
    return
  }
  if (path === '/api/v1/admin/groups/all') {
    send(res, previewGroups)
    return
  }
  if (path === '/api/v1/admin/groups' && req.method === 'GET') {
    send(res, {
      items: previewGroups,
      total: previewGroups.length,
      page: 1,
      page_size: previewGroups.length,
      pages: 1
    })
    return
  }
  if (/^\/api\/v1\/admin\/groups\/\d+$/.test(path)) {
    const id = Number(path.split('/').at(-1))
    const group = previewGroups.find(item => item.id === id)
    if (req.method === 'PUT' && group) {
      const raw = await readBody(req)
      try { Object.assign(group, JSON.parse(raw)) } catch { /* preview only */ }
    }
    send(res, group || null)
    return
  }
  if (path === '/api/v1/admin/channels/pricing/sync-models') {
    send(res, { models: tokenModelNames })
    return
  }
  if (path === '/api/v1/admin/channels/model-pricing') {
    send(res, { found: false })
    return
  }
  if (path === '/api/v1/admin/channels' && req.method === 'GET') {
    send(res, { items: previewChannels, total: previewChannels.length })
    return
  }
  if (/^\/api\/v1\/admin\/channels\/\d+$/.test(path)) {
    const id = Number(path.split('/').at(-1))
    const channel = previewChannels.find(item => item.id === id)
    if (req.method === 'PUT' && channel) {
      const raw = await readBody(req)
      try { Object.assign(channel, JSON.parse(raw)) } catch { /* preview only */ }
    }
    send(res, channel || null)
    return
  }
  if (path === '/api/v1/admin/channel-monitors' && req.method === 'GET') {
    send(res, {
      items: previewMonitors,
      total: previewMonitors.length,
      page: 1,
      page_size: previewMonitors.length,
      pages: 1
    })
    return
  }
  if (/^\/api\/v1\/admin\/channel-monitors\/\d+\/run$/.test(path) && req.method === 'POST') {
    const id = Number(path.split('/').at(-2))
    const monitor = previewMonitors.find(item => item.id === id)
    send(res, {
      results: monitor ? [{
        model: monitor.primary_model,
        status: 'operational',
        latency_ms: monitor.primary_latency_ms,
        ping_latency_ms: Math.round(monitor.primary_latency_ms / 2),
        message: 'Local preview check passed',
        checked_at: new Date().toISOString(),
        quota: null
      }] : []
    })
    return
  }
  if (/^\/api\/v1\/admin\/channel-monitors\/\d+\/history$/.test(path)) {
    const id = Number(path.split('/').at(-2))
    const monitor = previewMonitors.find(item => item.id === id)
    send(res, {
      items: monitor ? [{
        id: 1,
        model: monitor.primary_model,
        status: 'operational',
        latency_ms: monitor.primary_latency_ms,
        ping_latency_ms: Math.round(monitor.primary_latency_ms / 2),
        message: 'Local preview check passed',
        checked_at: now,
        quota: null
      }] : []
    })
    return
  }
  if (/^\/api\/v1\/admin\/channel-monitors\/\d+$/.test(path)) {
    const id = Number(path.split('/').at(-1))
    const monitor = previewMonitors.find(item => item.id === id)
    if (req.method === 'PUT' && monitor) {
      const raw = await readBody(req)
      try { Object.assign(monitor, JSON.parse(raw)) } catch { /* preview only */ }
    }
    send(res, monitor || null)
    return
  }
  if (path === '/api/v1/channel-monitors') {
    send(res, {
      items: previewMonitors.map(monitor => ({
        id: monitor.id,
        name: monitor.name,
        provider: monitor.provider,
        group_name: monitor.group_name,
        primary_model: monitor.primary_model,
        primary_status: monitor.primary_status,
        primary_latency_ms: monitor.primary_latency_ms,
        primary_ping_latency_ms: Math.round(monitor.primary_latency_ms / 2),
        availability_7d: monitor.availability_7d,
        extra_models: [],
        timeline: []
      }))
    })
    return
  }
  if (path.includes('/announcements')) {
    send(res, { items: [], total: 0, unread_count: 0 })
    return
  }
  if (path.includes('/subscriptions')) {
    send(res, [])
    return
  }
  if (path.includes('/groups/available')) {
    send(res, [])
    return
  }
  if (path.endsWith('/keys')) {
    send(res, { items: [], total: 0 })
    return
  }

  send(res, { items: [], total: 0 })
})

server.listen(port, host, () => {
  console.log(`Local model-pricing preview API listening on http://${host}:${port}`)
})
