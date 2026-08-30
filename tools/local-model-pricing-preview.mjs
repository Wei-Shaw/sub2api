import http from 'node:http'
import { createHash } from 'node:crypto'

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
    image_base_prices: [],
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
    image_base_prices: [],
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
    image_base_prices: [],
    image_prices: []
  },
  {
    id: 4,
    platform: 'openai',
    model_name: 'deepseek-v4-flash-0731',
    model_note: '按次线路按单次请求上下文区间计费，不参与按量倍率。',
    billing_mode: 'per_request',
    provider: 'deepseek',
    currency: 'CNY',
    configured: true,
    enabled: true,
    official_prices: null,
    model_multiplier: null,
    effective_multiplier: null,
    display_prices: null,
    per_request: {
      lte_256k: 0.008,
      from_256k_to_512k: 0.012,
      gt_512k: 0.016
    },
    image_base_prices: [],
    image_prices: []
  },
  {
    id: 5,
    platform: 'openai',
    model_name: 'deepseek-v4-pro-0813',
    model_note: '',
    billing_mode: 'per_request',
    provider: 'deepseek',
    currency: 'CNY',
    configured: true,
    enabled: true,
    official_prices: null,
    model_multiplier: null,
    effective_multiplier: null,
    display_prices: null,
    per_request: {
      lte_256k: 0.01,
      from_256k_to_512k: 0.015,
      gt_512k: 0.02
    },
    image_base_prices: [],
    image_prices: []
  }
]

const previewProviderMeta = {
  auto: { display_name: 'Auto', currency: 'CNY', multiplier: null, logo_key: 'auto', sort_order: 10 },
  deepseek: { display_name: 'DeepSeek', currency: 'CNY', multiplier: 0.1, logo_key: 'deepseek', sort_order: 20 },
  zhipu: { display_name: 'GLM', currency: 'CNY', multiplier: 0.125, logo_key: 'zhipu', sort_order: 30 },
  moonshot: { display_name: 'Kimi', currency: 'CNY', multiplier: 0.125, logo_key: 'kimi', sort_order: 40 },
  minimax: { display_name: 'MiniMax', currency: 'CNY', multiplier: 0.125, logo_key: 'minimax', sort_order: 50 },
  qwen: { display_name: 'Qwen', currency: 'CNY', multiplier: 0.125, logo_key: 'qwen', sort_order: 60 },
  mimo: { display_name: 'MiMo', currency: 'CNY', multiplier: 0.125, logo_key: 'mimo', sort_order: 70 },
  hunyuan: { display_name: 'Hunyuan', currency: 'CNY', multiplier: 0.125, logo_key: 'hunyuan', sort_order: 80 },
  openai: { display_name: 'OpenAI', currency: 'USD', multiplier: 0.044, logo_key: 'openai', sort_order: 100 },
  anthropic: { display_name: 'Anthropic', currency: 'USD', multiplier: 0.044, logo_key: 'anthropic', sort_order: 110 },
  gemini: { display_name: 'Gemini', currency: 'USD', multiplier: 0.044, logo_key: 'gemini', sort_order: 120 },
  grok: { display_name: 'Grok', currency: 'USD', multiplier: 0.044, logo_key: 'grok', sort_order: 130 }
}

function inferPreviewProvider(model) {
  const lower = model.toLowerCase()
  if (lower.startsWith('auto')) return 'auto'
  if (lower.includes('deepseek')) return 'deepseek'
  if (lower.includes('glm')) return 'zhipu'
  if (lower.includes('kimi')) return 'moonshot'
  if (lower.includes('minimax')) return 'minimax'
  if (lower.includes('qwen')) return 'qwen'
  if (lower.includes('mimo')) return 'mimo'
  if (lower === 'hy3' || lower.includes('hunyuan')) return 'hunyuan'
  if (lower.includes('claude')) return 'anthropic'
  if (lower.includes('gemini')) return 'gemini'
  if (lower.includes('grok')) return 'grok'
  return 'openai'
}

function scaledPrice(value, multiplier) {
  return Math.round(value * multiplier * 1e6) / 1e6
}

function genericTokenModel(model, index) {
  const provider = inferPreviewProvider(model)
  const meta = previewProviderMeta[provider]
  const input = meta.currency === 'CNY' ? 1 + (index % 5) * 1.1 : 0.5 + (index % 5) * 0.75
  const output = input * (3 + (index % 2))
  const cacheRead = input * 0.1
  const multiplier = meta.multiplier ?? 1
  return {
    id: 100 + index,
    platform: 'openai',
    model_name: model,
    model_note: '',
    billing_mode: 'token',
    provider,
    currency: meta.currency,
    configured: true,
    enabled: true,
    official_prices: {
      input_per_million: input,
      output_per_million: output,
      cache_write_per_million: null,
      cache_read_per_million: cacheRead
    },
    model_multiplier: null,
    effective_multiplier: multiplier,
    display_prices: {
      input_per_million: scaledPrice(input, multiplier),
      output_per_million: scaledPrice(output, multiplier),
      cache_write_per_million: null,
      cache_read_per_million: scaledPrice(cacheRead, multiplier)
    },
    per_request: null,
    image_base_prices: [],
    image_prices: []
  }
}

function genericPerRequestModel(model, index) {
  const provider = inferPreviewProvider(model)
  const meta = previewProviderMeta[provider]
  const base = Math.round((0.005 + (index % 6) * 0.001) * 1e6) / 1e6
  return {
    id: 200 + index,
    platform: 'openai',
    model_name: model,
    model_note: '',
    billing_mode: 'per_request',
    provider,
    currency: meta.currency,
    configured: true,
    enabled: true,
    official_prices: null,
    model_multiplier: null,
    effective_multiplier: null,
    display_prices: null,
    per_request: {
      lte_256k: base,
      from_256k_to_512k: base * 1.5,
      gt_512k: base * 2
    },
    image_base_prices: [],
    image_prices: []
  }
}

const previewDisplayModels = [
  ...deepSeekModels,
  ...tokenModelNames.filter(model => !model.toLowerCase().includes('deepseek')).map(genericTokenModel),
  ...perRequestModelNames.filter(model => !model.toLowerCase().includes('deepseek')).map(genericPerRequestModel),
  {
    id: 300,
    platform: 'openai',
    model_name: 'gpt-image-2',
    model_note: '支持多种图片尺寸，本站价格按规格展示。',
    billing_mode: 'image',
    provider: 'openai',
    currency: 'USD',
    configured: true,
    enabled: true,
    official_prices: null,
    model_multiplier: null,
    effective_multiplier: 1.2,
    display_prices: null,
    per_request: null,
    image_base_prices: [
      { label: '1K', price: 0.04 },
      { label: '2K', price: 0.07 }
    ],
    image_prices: [
      { label: '1K', price: 0.048 },
      { label: '2K', price: 0.084 }
    ]
  }
]

const previewCatalogProviders = Object.entries(previewProviderMeta)
  .map(([provider, meta]) => ({
    provider,
    display_name: meta.display_name,
    provider_note: provider === 'deepseek'
      ? 'DeepSeek 按量展示平常价格；工作日北京时间 09:00–12:00、14:00–18:00 为高峰时段。'
      : '',
    per_request_note: provider === 'deepseek'
      ? '按次价格按单次请求总上下文分三档，与按量高峰时段及展示倍率无关。'
      : '',
    image_note: provider === 'openai' ? '生图价格按图片规格独立展示。' : '',
    currency: meta.currency,
    logo_key: meta.logo_key,
    logo_url: '',
    configured_multiplier: meta.multiplier,
    effective_multiplier: meta.multiplier ?? 1,
    sort_order: meta.sort_order,
    models: previewDisplayModels.filter(model => model.provider === provider)
  }))
  .filter(provider => provider.models.length > 0)

const publicSettings = {
  site_name: 'Sub2API Local Preview',
  site_logo: '',
  site_version: 'local',
  model_plaza_enabled: true,
  model_plaza_require_auth: false,
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
  providers: previewCatalogProviders
}

function recalculatePreviewPrices() {
  for (const provider of catalog.providers) {
    const inheritedMultiplier = catalog.global_multiplier * (provider.configured_multiplier ?? 1)
    provider.effective_multiplier = inheritedMultiplier
    for (const model of provider.models) {
      if (model.billing_mode !== 'token' && model.billing_mode !== 'image') continue
      const multiplier = model.model_multiplier ?? inheritedMultiplier
      model.effective_multiplier = multiplier
      if (model.billing_mode === 'token' && model.official_prices) {
        model.display_prices = Object.fromEntries(
          Object.entries(model.official_prices).map(([key, value]) => [
            key,
            value == null ? null : scaledPrice(Number(value), multiplier)
          ])
        )
      }
      if (model.billing_mode === 'image') {
        model.image_prices = (model.image_base_prices ?? []).map(tier => ({
          label: tier.label,
          price: scaledPrice(tier.price, multiplier)
        }))
      }
    }
  }
  catalog.updated_at = new Date().toISOString()
}

const officialReferenceUrls = {
  deepseek: 'https://api-docs.deepseek.com/zh-cn/quick_start/pricing',
  zhipu: 'https://open.bigmodel.cn/pricing',
  moonshot: 'https://platform.kimi.com/docs/pricing/chat',
  minimax: 'https://platform.minimaxi.com/docs/guides/pricing-paygo',
  qwen: 'https://help.aliyun.com/zh/model-studio/model-pricing',
  mimo: 'https://mimo.mi.com/docs/zh-CN/price/pay-as-you-go',
  hunyuan: 'https://cloud.tencent.com/document/product/1729',
  openai: 'https://developers.openai.com/api/docs/pricing',
  anthropic: 'https://platform.claude.com/docs/en/about-claude/pricing',
  gemini: 'https://ai.google.dev/gemini-api/docs/pricing',
  grok: 'https://docs.x.ai/developers/pricing'
}

function officialValues(model) {
  return {
    input_per_million: model.official_prices?.input_per_million ?? null,
    output_per_million: model.official_prices?.output_per_million ?? null,
    cache_write_per_million: model.official_prices?.cache_write_per_million ?? null,
    cache_read_per_million: model.official_prices?.cache_read_per_million ?? null
  }
}

function candidateOfficialValue(component, fallback) {
  const value = component?.official
  if (value == null || value === '') return fallback
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback
}

function officialProposalHash(modelId, proposed) {
  return createHash('sha256').update(`${modelId}\0${JSON.stringify(proposed)}`).digest('hex')
}

async function buildOfficialSyncPreview() {
  const fetchedAt = new Date().toISOString()
  let remote
  let warning = null
  try {
    const response = await fetch('https://sub2.herohao.top/pricing/api/pricing', {
      headers: { Accept: 'application/json', 'User-Agent': 'sub2api-local-preview/1' },
      signal: AbortSignal.timeout(10000)
    })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    remote = await response.json()
  } catch (error) {
    warning = `聚合候选源暂时不可用：${error instanceof Error ? error.message : String(error)}`
    remote = { token: { models: [] } }
  }
  const remoteModels = new Map((remote.token?.models ?? []).map(item => [String(item.model), item]))
  const items = previewDisplayModels
    .filter(model => model.billing_mode === 'token')
    .map(model => {
      const current = officialValues(model)
      const source = remoteModels.get(model.model_name)
      let proposed = null
      let applicable = false
      let reason = ''
      if (model.currency !== 'CNY') reason = 'currency_mismatch'
      else if (!source) reason = 'candidate_not_found'
      else if (!source.enabled) reason = 'candidate_disabled'
      else {
        proposed = {
          input_per_million: candidateOfficialValue(source.prices?.input, current.input_per_million),
          output_per_million: candidateOfficialValue(source.prices?.output, current.output_per_million),
          cache_write_per_million: candidateOfficialValue(source.prices?.cacheWrite, current.cache_write_per_million),
          cache_read_per_million: candidateOfficialValue(source.prices?.cacheRead, current.cache_read_per_million)
        }
        applicable = true
      }
      const changed = applicable && JSON.stringify(current) !== JSON.stringify(proposed)
      return {
        model_id: model.id,
        model_name: model.model_name,
        provider: model.provider,
        billing_mode: model.billing_mode,
        currency: model.currency,
        current,
        proposed,
        changed,
        diff: {
          input_per_million: current.input_per_million !== proposed?.input_per_million,
          output_per_million: current.output_per_million !== proposed?.output_per_million,
          cache_write_per_million: current.cache_write_per_million !== proposed?.cache_write_per_million,
          cache_read_per_million: current.cache_read_per_million !== proposed?.cache_read_per_million,
          has_changes: changed
        },
        applicable,
        reason,
        source: 'herohao_aggregate',
        confidence: 'unverified',
        source_updated_at: source?.updatedAt ?? remote.token?.databaseUpdatedAt ?? null,
        official_reference_url: officialReferenceUrls[model.provider] ?? '',
        expected_updated_at: model.updated_at ?? catalog.updated_at,
        proposal_hash: proposed ? officialProposalHash(model.id, proposed) : ''
      }
    })
  return { items, fetched_at: remote.fetchedAt ?? fetchedAt, warning: remote.warning ?? warning }
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

function adminProvider(provider) {
  return {
    provider: provider.provider,
    display_name: provider.display_name,
    provider_note: provider.provider_note,
    per_request_note: provider.per_request_note,
    image_note: provider.image_note,
    currency: provider.currency,
    multiplier: provider.configured_multiplier,
    logo_key: provider.logo_key,
    logo_url: provider.logo_url,
    sort_order: provider.sort_order,
    updated_at: catalog.updated_at
  }
}

function adminModels() {
  return previewDisplayModels.map((model, index) => ({
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
    official_price_source: model.official_price_source ?? 'manual',
    official_price_source_url: model.official_price_source_url ?? '',
    official_price_synced_at: model.official_price_synced_at ?? null,
    model_multiplier: model.model_multiplier,
    per_request_lte_256k: model.per_request?.lte_256k ?? null,
    per_request_256k_512k_override: model.per_request?.from_256k_to_512k ?? null,
    per_request_gt_512k_override: model.per_request?.gt_512k ?? null,
    image_prices: model.image_base_prices ?? [],
    created_at: now,
    updated_at: model.updated_at ?? catalog.updated_at
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
      recalculatePreviewPrices()
    }
    send(res, { global_multiplier: catalog.global_multiplier, updated_at: catalog.updated_at })
    return
  }
  if (path === '/api/v1/admin/display-pricing/providers') {
    send(res, { items: catalog.providers.map(adminProvider) })
    return
  }
  if (path === '/api/v1/admin/display-pricing/official-sync/preview' && req.method === 'POST') {
    send(res, await buildOfficialSyncPreview())
    return
  }
  if (path === '/api/v1/admin/display-pricing/official-sync/apply' && req.method === 'POST') {
    const raw = await readBody(req)
    let selections = []
    try { selections = JSON.parse(raw).models ?? [] } catch { selections = [] }
    const preview = await buildOfficialSyncPreview()
    const candidates = new Map(preview.items.map(item => [item.model_id, item]))
    const selected = selections.map(selection => ({ selection, candidate: candidates.get(Number(selection.model_id)) }))
    const valid = selected.length > 0 && selected.every(({ selection, candidate }) =>
      candidate?.applicable &&
      candidate.proposed &&
      candidate.expected_updated_at === selection.expected_updated_at &&
      candidate.proposal_hash === selection.proposal_hash
    )
    if (!valid) {
      send(res, { error: 'official price candidate changed; preview again' }, 409)
      return
    }
    const syncedAt = new Date().toISOString()
    for (const { candidate } of selected) {
      const model = previewDisplayModels.find(item => item.id === candidate.model_id)
      if (!model) continue
      model.official_prices = { ...candidate.proposed }
      model.official_price_source = 'herohao_aggregate'
      model.official_price_source_url = 'https://sub2.herohao.top/pricing/api/pricing'
      model.official_price_synced_at = syncedAt
      model.updated_at = syncedAt
    }
    recalculatePreviewPrices()
    send(res, { applied_count: selected.length, synced_at: syncedAt })
    return
  }
  if (path.startsWith('/api/v1/admin/display-pricing/providers/') && req.method === 'PUT') {
    const providerKey = decodeURIComponent(path.split('/').at(-1) || '')
    const provider = catalog.providers.find(item => item.provider === providerKey)
    const raw = await readBody(req)
    if (provider) try {
      const payload = JSON.parse(raw)
      provider.display_name = String(payload.display_name || provider.display_name)
      provider.provider_note = String(payload.provider_note || '')
      provider.per_request_note = String(payload.per_request_note || '')
      provider.image_note = String(payload.image_note || '')
      provider.logo_key = String(payload.logo_key || provider.provider)
      provider.logo_url = String(payload.logo_url || '')
      provider.configured_multiplier = payload.multiplier == null ? null : Number(payload.multiplier)
      recalculatePreviewPrices()
    } catch { /* preview only */ }
    send(res, provider ? adminProvider(provider) : null, provider ? 200 : 404)
    return
  }
  if (path === '/api/v1/admin/display-pricing/models') {
    send(res, { items: adminModels() })
    return
  }
  if (path.startsWith('/api/v1/admin/display-pricing/models/') && req.method === 'PUT') {
    const id = Number(path.split('/').at(-1))
    const raw = await readBody(req)
    const model = previewDisplayModels.find(item => item.id === id)
    if (model) {
      try {
        const payload = JSON.parse(raw)
        model.model_note = String(payload.model_note || '')
        model.official_price_source = String(payload.official_price_source || 'manual')
        model.official_price_source_url = String(payload.official_price_source_url || '')
        model.official_price_synced_at = payload.official_price_synced_at || null
        model.updated_at = new Date().toISOString()
        model.model_multiplier = payload.model_multiplier == null ? null : Number(payload.model_multiplier)
        if (model.billing_mode === 'token') {
          model.official_prices = {
            input_per_million: payload.official_input_per_million == null ? null : Number(payload.official_input_per_million),
            output_per_million: payload.official_output_per_million == null ? null : Number(payload.official_output_per_million),
            cache_write_per_million: payload.official_cache_write_per_million == null ? null : Number(payload.official_cache_write_per_million),
            cache_read_per_million: payload.official_cache_read_per_million == null ? null : Number(payload.official_cache_read_per_million)
          }
        }
        if (model.billing_mode === 'per_request') {
          const base = Number(payload.per_request_lte_256k ?? 0)
          model.per_request = {
            lte_256k: base,
            from_256k_to_512k: payload.per_request_256k_512k_override == null ? base * 1.5 : Number(payload.per_request_256k_512k_override),
            gt_512k: payload.per_request_gt_512k_override == null ? base * 2 : Number(payload.per_request_gt_512k_override)
          }
        }
        if (model.billing_mode === 'image') {
          model.image_base_prices = Array.isArray(payload.image_prices)
            ? payload.image_prices.map(tier => ({ label: String(tier.label), price: Number(tier.price) }))
            : []
        }
        recalculatePreviewPrices()
      } catch { /* preview only */ }
    }
    send(res, adminModels().find(item => item.id === id) || null)
    return
  }
  if (path === '/api/v1/admin/display-pricing/discovered-models') {
    send(res, {
      items: previewDisplayModels.map(model => ({
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
