export function applyInterceptWarmup(
  credentials: Record<string, unknown>,
  enabled: boolean,
  mode: 'create' | 'edit'
): void {
  if (enabled) {
    credentials.intercept_warmup_requests = true
  } else if (mode === 'edit') {
    delete credentials.intercept_warmup_requests
  }
}

export const ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY = 'antigravity_project_id'

export function applyAntigravityProjectID(
  credentials: Record<string, unknown>,
  projectId: string,
  mode: 'create' | 'edit'
): void {
  const trimmed = projectId.trim()
  if (trimmed) {
    credentials[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY] = trimmed
  } else if (mode === 'edit') {
    delete credentials[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]
  }
}

// ========== 请求头覆写（anthropic/openai 的 api_key 账号 + grok 的 api_key/oauth 账号） ==========

export const HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY = 'header_override_enabled'
export const HEADER_OVERRIDES_CREDENTIAL_KEY = 'header_overrides'

export interface HeaderOverrideRow {
  name: string
  value: string
}

/** 请求头覆写资格（与后端 IsHeaderOverrideEligible 保持一致） */
export function isHeaderOverrideCapable(platform: string, type: string): boolean {
  if (platform === 'anthropic' || platform === 'openai') {
    return type === 'apikey'
  }
  if (platform === 'grok') {
    return type === 'apikey' || type === 'oauth'
  }
  return false
}

/** 禁止覆写的请求头（与后端 headerOverrideBlockedNames 保持一致） */
const HEADER_OVERRIDE_BLOCKED_NAMES = new Set([
  'host',
  'content-length',
  'content-type',
  'transfer-encoding',
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'proxy-connection',
  'te',
  'trailer',
  'upgrade',
  'authorization',
  'x-api-key',
  'x-goog-api-key',
  'cookie',
  'accept-encoding',
  'sec-websocket-key',
  'sec-websocket-version',
  'sec-websocket-extensions',
  'sec-websocket-protocol',
  'sec-websocket-accept',
  'session_id',
  'conversation_id',
  'x-codex-turn-state',
  'x-codex-turn-metadata',
  'chatgpt-account-id',
  'x-claude-code-session-id',
  'x-client-request-id',
  'x-grok-conv-id'
])

/** RFC 7230 token：合法的 HTTP header 名称字符集 */
const HEADER_NAME_PATTERN = /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/

function isValidHeaderOverrideName(name: string): boolean {
  return HEADER_NAME_PATTERN.test(name)
}

/** 与后端 maxHeaderOverride* 常量保持一致 */
const HEADER_OVERRIDE_MAX_ENTRIES = 64
const HEADER_OVERRIDE_MAX_NAME_LENGTH = 200
const HEADER_OVERRIDE_MAX_VALUE_LENGTH = 8192

/** header value 不允许包含控制字符（与后端 httpguts.ValidHeaderFieldValue 对齐） */
// eslint-disable-next-line no-control-regex
const HEADER_VALUE_INVALID_PATTERN = /[\x00-\x08\x0a-\x1f\x7f]/

/** 长度限制按 UTF-8 字节计（与后端 Go len() 对齐，避免多字节值前端放行后端 400） */
const HEADER_TEXT_ENCODER = new TextEncoder()
function utf8ByteLength(value: string): number {
  return HEADER_TEXT_ENCODER.encode(value).length
}

/**
 * 校验请求头覆写行，返回首个错误的 i18n key（无错误返回 null）。
 * 名称为空但值非空 → invalidName；名称非法 → invalidName；
 * 禁止覆写 → blockedName；大小写不敏感重名 → duplicateName；
 * 值含控制字符或超长 → invalidValue；条目过多 → tooManyEntries。
 */
export function validateHeaderOverrideRows(
  rows: HeaderOverrideRow[]
): 'invalidName' | 'blockedName' | 'duplicateName' | 'invalidValue' | 'tooManyEntries' | null {
  const seen = new Set<string>()
  for (const row of rows) {
    const name = row.name.trim()
    const value = row.value.trim()
    if (!name) {
      if (value) return 'invalidName'
      continue
    }
    if (!isValidHeaderOverrideName(name) || name.length > HEADER_OVERRIDE_MAX_NAME_LENGTH) {
      return 'invalidName'
    }
    const lower = name.toLowerCase()
    if (HEADER_OVERRIDE_BLOCKED_NAMES.has(lower)) return 'blockedName'
    if (seen.has(lower)) return 'duplicateName'
    if (
      HEADER_VALUE_INVALID_PATTERN.test(value) ||
      utf8ByteLength(value) > HEADER_OVERRIDE_MAX_VALUE_LENGTH
    ) {
      return 'invalidValue'
    }
    seen.add(lower)
  }
  if (seen.size > HEADER_OVERRIDE_MAX_ENTRIES) return 'tooManyEntries'
  return null
}

/** 行数组 → credentials 存储对象（名称小写化，丢弃空行） */
export function buildHeaderOverridesObject(rows: HeaderOverrideRow[]): Record<string, string> {
  const result: Record<string, string> = {}
  for (const row of rows) {
    const name = row.name.trim().toLowerCase()
    if (!name) continue
    result[name] = row.value.trim()
  }
  return result
}

/** credentials 存储对象 → 行数组（按名称排序保证稳定展示） */
export function splitHeaderOverridesObject(record: unknown): HeaderOverrideRow[] {
  if (!record || typeof record !== 'object' || Array.isArray(record)) return []
  return Object.entries(record as Record<string, unknown>)
    .filter(([, value]) => typeof value === 'string')
    .map(([name, value]) => ({ name, value: value as string }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

/**
 * 解析粘贴的 JSON 文本为请求头覆写行。
 * 仅接受扁平 JSON 对象；值允许 string/number/boolean（统一转字符串），
 * 其余类型或非对象输入返回 null 表示格式非法。键为空白的条目直接丢弃。
 */
export function parseHeaderOverridesJson(text: string): HeaderOverrideRow[] | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null
  const rows: HeaderOverrideRow[] = []
  for (const [rawName, rawValue] of Object.entries(parsed as Record<string, unknown>)) {
    const name = rawName.trim()
    if (!name) continue
    if (
      typeof rawValue !== 'string' &&
      typeof rawValue !== 'number' &&
      typeof rawValue !== 'boolean'
    ) {
      return null
    }
    rows.push({ name, value: String(rawValue).trim() })
  }
  return rows.sort((a, b) => a.name.localeCompare(b.name))
}

/** 请求头覆写行 → 便于迁移/备份的 JSON 文本（跳过名称为空的占位行） */
export function serializeHeaderOverrideRows(rows: HeaderOverrideRow[]): string {
  const record: Record<string, string> = {}
  for (const row of rows) {
    const name = row.name.trim()
    if (!name) continue
    record[name] = row.value.trim()
  }
  return JSON.stringify(record, null, 2)
}

// ========== Grok 自定义转发地址（base_url 仅改写转发端点，凭证生命周期不受影响） ==========

/** OAuth 账号建号/刷新默认写入的 CLI 网关 host——只有它视同"未定制"。 */
const GROK_DEFAULT_GATEWAY_HOST = 'cli-chat-proxy.grok.com'

/**
 * 判断 Grok 账号存储的 base_url 是否为主动指定的上游端点。
 * 运营方可在官方 API / 区域 API / 第三方转发地址之间手动切换（应对单端点
 * 不可用），这些值都必须回显（开关开启 + 显示地址）。仅默认 CLI 网关
 * （建号/刷新自动写入）、空值与无法解析的值视为"未定制"（与后端
 * GetGrokBaseURL 的回落语义对齐），用于 OAuth 账号编辑时决定开关初始状态。
 */
export function isCustomGrokBaseUrl(value: unknown): boolean {
  if (typeof value !== 'string') return false
  const trimmed = value.trim()
  if (!trimmed) return false
  let parsed: URL
  try {
    parsed = new URL(trimmed)
  } catch {
    return false
  }
  return parsed.hostname.toLowerCase() !== GROK_DEFAULT_GATEWAY_HOST
}

export interface GrokBaseUrlPreset {
  /** i18n 子键：admin.accounts.grokCustomBaseUrl.presets.<labelKey> */
  labelKey?: 'cli' | 'official'
  /** 字面标签（如区域标识 us-east-1），专有名词不参与 i18n */
  label?: string
  url: string
}

/**
 * Grok 快捷端点（仅供快速填充，输入框仍可自由填写任意转发地址）。
 * 官方端点偶发不可用时，运营方靠这组预设在端点间手动切换。
 */
export const GROK_BASE_URL_PRESETS: GrokBaseUrlPreset[] = [
  { labelKey: 'cli', url: 'https://cli-chat-proxy.grok.com/v1' },
  { labelKey: 'official', url: 'https://api.x.ai/v1' },
  { label: 'us-east-1', url: 'https://us-east-1.api.x.ai/v1' },
  { label: 'us-west-2', url: 'https://us-west-2.api.x.ai/v1' },
  { label: 'eu-west-1', url: 'https://eu-west-1.api.x.ai/v1' }
]

/**
 * 将请求头覆写写入 credentials。
 * create 模式：关闭时不写入任何字段；edit 模式：关闭时删除字段（全量替换语义）。
 */
export function applyHeaderOverride(
  credentials: Record<string, unknown>,
  enabled: boolean,
  rows: HeaderOverrideRow[],
  mode: 'create' | 'edit'
): void {
  if (enabled) {
    credentials[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY] = true
    credentials[HEADER_OVERRIDES_CREDENTIAL_KEY] = buildHeaderOverridesObject(rows)
  } else if (mode === 'edit') {
    delete credentials[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]
    delete credentials[HEADER_OVERRIDES_CREDENTIAL_KEY]
  }
}

// ===== OpenAI plan_type (ChatGPT 订阅档位) 手动覆盖 =====

export interface PlanTypeOption {
  value: string
  label: string
  // 兼容 common/Select.vue 的 SelectOption(含索引签名)
  [key: string]: unknown
}

/**
 * plan_type 值的友好显示标签，镜像 PlatformTypeBadge 的映射
 * （canonical 值 chatgptpro 显示为 Pro，team 显示为 Team）。未知值原样返回。
 */
export function planTypeDisplayLabel(value: string): string {
  switch (value.trim().toLowerCase()) {
    case 'plus':
      return 'Plus'
    case 'pro':
    case 'chatgptpro':
      return 'Pro'
    case 'free':
      return 'Free'
    case 'team':
      return 'Team'
    default:
      return value
  }
}

/**
 * 从凭据里读取 plan_type，仅接受字符串（脏数据 42/true 等一律视为空，
 * 避免被当作合法自定义项保留）。
 */
export function readPlanType(credentials: Record<string, unknown> | undefined | null): string {
  const v = credentials?.plan_type
  return typeof v === 'string' ? v : ''
}

/**
 * 构建 plan_type 下拉选项：清空 + Plus/Pro/Free 预设。
 * 若当前值是某预设的别名（如 chatgptpro↔Pro），用当前的 canonical 值占据该
 * 标签位（保留 canonical，显示友好标签，避免重复项）；若是完全预设外的值
 * （如 team 或异常值），追加为一项，避免编辑时下拉丢失原值。
 */
export function buildPlanTypeOptions(current: string, clearLabel: string): PlanTypeOption[] {
  const cur = (current || '').trim()
  const curLabel = cur ? planTypeDisplayLabel(cur) : ''
  const presets: PlanTypeOption[] = [
    { value: 'plus', label: 'Plus' },
    { value: 'pro', label: 'Pro' },
    { value: 'free', label: 'Free' }
  ]
  const opts: PlanTypeOption[] = [{ value: '', label: clearLabel }]
  for (const p of presets) {
    if (cur && p.value !== cur.toLowerCase() && p.label === curLabel) {
      // 当前值是该预设的别名：用 canonical 当前值占位，标签仍显示友好名
      opts.push({ value: cur, label: p.label })
    } else {
      opts.push(p)
    }
  }
  if (cur && !opts.some(o => o.value.toLowerCase() === cur.toLowerCase())) {
    opts.push({ value: cur, label: planTypeDisplayLabel(cur) })
  }
  return opts
}

/**
 * 把手动选择的 plan_type 写入凭据：非空则设置，空则删除该键（清空/自动识别）。
 * 直接修改传入对象并返回。
 */
export function applyPlanType(
  credentials: Record<string, unknown>,
  planType: string
): Record<string, unknown> {
  const pt = (planType || '').trim()
  if (pt) {
    credentials.plan_type = pt
  } else {
    delete credentials.plan_type
  }
  return credentials
}
