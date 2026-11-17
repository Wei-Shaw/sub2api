import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

export interface Price {
  model: string
  mode: 'token' | 'per_request' | 'unavailable'
  input: string | null
  output: string | null
  cache_read: string | null
  cache_write: string | null
  cache_write_1h?: string | null
  cache_write_30m?: string | null
  cache_write_12h?: string | null
  request: string | null
  reason?: string
  manual?: boolean
}
export interface PricingConfig {
  provider: 'newapi' | 'sub2api'
  base_url: string
  token?: string
  browser_import_id?: string
  user_id?: string
  group: string
  group_id?: string
  automatic: boolean
  interval_minutes: number
  overrides?: Price[]
}
export interface PricingGroup { id?: string; name: string; multiplier: string }
export interface ActivityState {
  configured: boolean
  last_success: string | null
  last_attempt: string | null
  next_attempt: string | null
  last_error: string
  wallet_balance: string | null
  wallet_checked_at: string | null
  wallet_error: string
}
export interface ComparisonAccount {
  id: number; name: string; provider: string; base_url: string; group: string; automatic: boolean
  version_id: number | null; last_success: string | null; next_sync: string | null
  stale: boolean; account_changed: boolean; last_error: string
}
export interface PricingComparison {
  accounts: ComparisonAccount[]
  items: { account_id: number; version_id: number; model: string; multiplier: string; base: Price; effective: Price }[]
  total: number; page: number; page_size: number
}
export interface BrowserImportStart { id: string; ticket: string; expires_at: string }
export interface BrowserImportStatus { id: string; status: 'waiting' | 'validating' | 'ready' | 'failed'; user_id: string; groups?: PricingGroup[]; error?: string }
export interface Catalog {
  group: PricingGroup
  models: Price[]
  supported: number
  unsupported: number
  fingerprint: string
}
export interface Preview { id: string; catalog: Catalog; added: number; changed: number; removed: number }
export interface PricingState {
  source: {
    account_id: number; revision: number; config: PricingConfig; has_credential: boolean
    active_version: number | null; last_success: string | null; last_attempt: string | null
    last_error: string; next_sync: string | null
  }
  catalog: Catalog | null
  runs: { id: number; trigger: string; status: string; message: string; version_id: number | null; created_at: string }[]
  versions: { id: number; effective_from: string; supported: number; unsupported: number }[]
  account_changed: boolean
}
export interface UpstreamEstimate {
  status: 'estimated' | 'not_configured' | 'unavailable' | 'pending'
  amount: string | null
  reason?: string
  version_id?: number
  source?: string
  group?: string
  multiplier?: string
  model?: string
  pricing_at?: string
  synced_at?: string
  price?: Price
  usage?: { input: number; output: number; cache_read: number; cache_write: number; cache_write_5m: number; cache_write_1h: number; cache_write_30m: number; cache_write_12h: number }
  stale?: boolean
}

const messages = {
  zh: {
    tab: '上游定价', account: '账号设置', estimate: '上游预计扣费', provider: '站点类型', baseUrl: '面板地址',
    userId: 'New API 用户 ID', userIdHint: '部分站点需要，请填写面板中的用户 ID', token: '面板读取凭证',
    tokenHint: 'New API 可使用面板访问令牌读取分组及用户专属倍率，部分站点还需要用户 ID；与模型调用 Key 分开。Sub2API 使用用户登录 JWT。',
    probeHint: '账号设置中的倍率探测使用 Sub2API 专用接口；New API 请在“上游定价”中填写访问令牌并读取分组。读取的倍率用于上游预计扣费，不自动覆盖账号统计倍率。',
    tokenSaved: '已加密保存；留空保留。更换站点或账号身份后需要重新填写。',
    group: '上游计费分组', readGroups: '读取可用分组', chooseGroup: '请选择准确的计费分组', automatic: '自动同步',
    interval: '同步间隔', minutes: '分钟', preview: '预览价格差异', apply: '应用此预览', sync: '立即同步已保存配置',
    schedule: '保存同步计划', loading: '处理中…', error: '操作失败，请稍后重试', saved: '已应用价格，后续请求将使用此版本',
    note: '按上游账号独立估算，不改变本机扣费。价格表计算结果不等于站点实际账单。',
    baseUrlHint: '填写站点控制台地址；它可能与带 /v1 的模型转发地址不同。',
    lastSuccess: '最近成功', nextSync: '下次同步', never: '尚无', manualOnly: '仅手动', changedAccount: '上游账号地址或 Key 已变化，请重新填写凭证并预览。',
    supported: '可估算模型', unsupported: '暂不支持', model: '上游模型', mode: '计价方式', input: '输入', output: '输出',
    cacheRead: '缓存读取', cacheWrite: '缓存写入（默认）', cache1h: '缓存写入 / 1 小时', cache30m: '缓存写入 / 30 分钟', cache12h: '缓存写入 / 12 小时',
    request: '每次请求（USD）', tokenMode: '按 Token', requestMode: '按次', multiplier: '上游分组倍率', unit: '单价：USD / 百万 Token，尚未乘上游倍率',
    added: '新增', changed: '变更', removed: '移除', unknown: '未知', manual: '人工覆盖', editManual: '编辑人工覆盖',
    overrideHint: '人工覆盖优先于同步价格；留空表示未知，0 表示免费。修改后须重新预览并应用。',
    saveOverride: '加入待应用规则', removeOverride: '移除覆盖', clearEditor: '清空编辑', history: '同步记录', versions: '价格版本',
    rollback: '预览回滚', search: '筛选模型', first100: '最多显示前 100 个匹配模型', noCatalog: '尚未应用价格，请先读取分组并预览',
    expires: '预览 10 分钟内有效，修改配置后需重新预览', stale: '价格可能过期', version: '版本', source: '来源',
    at: '计价时间', synced: '价格验证时间', notConfigured: '未配置', unavailable: '无法估算', pending: '计算中',
    success: '成功', failed: '失败', partial: '缺失单价或不支持的维度不会按零费用处理',
  },
  en: {
    tab: 'Upstream pricing', account: 'Account settings', estimate: 'Estimated upstream cost', provider: 'Site type', baseUrl: 'Panel URL',
    userId: 'New API user ID', userIdHint: 'Required by some sites; use the panel user ID', token: 'Panel access token',
    tokenHint: 'New API panel access tokens can read groups and user-specific multipliers; some sites also require a user ID. Separate from the inference API key. Sub2API requires a user JWT.',
    probeHint: 'The account multiplier probe uses a Sub2API-specific endpoint. For New API, enter a panel token under Upstream pricing and read groups. These multipliers estimate upstream costs without replacing account statistics multipliers.',
    tokenSaved: 'Stored encrypted; leave blank to keep. Enter again after changing the site or account identity.',
    group: 'Upstream billing group', readGroups: 'Read available groups', chooseGroup: 'Select the exact billing group', automatic: 'Automatic sync',
    interval: 'Sync interval', minutes: 'minutes', preview: 'Preview price changes', apply: 'Apply this preview', sync: 'Sync saved settings now',
    schedule: 'Save schedule', loading: 'Working…', error: 'Operation failed. Please try again.', saved: 'Prices applied to subsequent requests',
    note: 'Estimated per upstream account. Local billing is unchanged. Estimates are not settled upstream charges.',
    baseUrlHint: 'Use the panel address, which may differ from the inference URL ending in /v1.',
    lastSuccess: 'Last success', nextSync: 'Next sync', never: 'None yet', manualOnly: 'Manual only', changedAccount: 'The upstream URL or key changed. Enter the panel token again and preview.',
    supported: 'Supported models', unsupported: 'Unsupported', model: 'Upstream model', mode: 'Billing mode', input: 'Input', output: 'Output',
    cacheRead: 'Cache read', cacheWrite: 'Cache write (default)', cache1h: 'Cache write / 1 hour', cache30m: 'Cache write / 30 min', cache12h: 'Cache write / 12 hours',
    request: 'Per request (USD)', tokenMode: 'Per token', requestMode: 'Per request', multiplier: 'Upstream group multiplier', unit: 'USD per million tokens, before the upstream multiplier',
    added: 'Added', changed: 'Changed', removed: 'Removed', unknown: 'Unknown', manual: 'Manual override', editManual: 'Edit manual override',
    overrideHint: 'Overrides take priority over synced prices. Blank means unknown; 0 means free. Preview and apply after editing.',
    saveOverride: 'Add to pending rules', removeOverride: 'Remove override', clearEditor: 'Clear editor', history: 'Sync history', versions: 'Price versions',
    rollback: 'Preview rollback', search: 'Filter models', first100: 'Showing at most 100 matching models', noCatalog: 'Read groups and preview prices to get started',
    expires: 'Previews expire after 10 minutes. Preview again after editing settings.', stale: 'Prices may be stale', version: 'Version', source: 'Source',
    at: 'Pricing time', synced: 'Price validation time', notConfigured: 'Not configured', unavailable: 'Unavailable', pending: 'Pending',
    success: 'Success', failed: 'Failed', partial: 'Unknown prices and unsupported dimensions are never treated as free',
  },
}
export function useUpstreamPricingLabels() {
  const { locale } = useI18n()
  return computed(() => messages[locale?.value?.startsWith('zh') ? 'zh' : 'en'])
}

export function blankPrice(): Price {
  return { model: '', mode: 'token', input: null, output: null, cache_read: null, cache_write: null, request: null }
}
export function formatUpstreamAmount(value?: UpstreamEstimate | null): string {
  if (value?.status !== 'estimated' || value.amount == null || !/^(?:0|[1-9]\d*)(?:\.\d+)?$/.test(value.amount)) return '—'
  return `$${value.amount}`
}
