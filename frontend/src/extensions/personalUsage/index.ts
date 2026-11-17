import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageLog, UsageLog } from '@/types'

export interface CacheDrop {
  previous_usage_id: number
  previous_created_at: string
  gap_seconds: number
  previous_hit_rate: number
  current_hit_rate: number
  drop_points: number
  threshold_points: number
  previous_input_total: number
  current_input_total: number
}

export interface PersonalUsageLog extends UsageLog {
  session_name?: string
  session_identity?: {
    thread_id?: string
    parent_thread_id?: string
    kind?: string
    subagent?: string
    match: 'thread' | 'session_only' | 'unresolved'
  }
  cache_drop?: CacheDrop | null
  cache_monitoring_status?: 'ready' | 'unavailable' | 'identity_missing' | 'not_comparable'
}

export type PersonalAdminUsageLog = AdminUsageLog & PersonalUsageLog

type CacheTokens = Pick<UsageLog, 'input_tokens' | 'cache_read_tokens' | 'cache_creation_tokens'>

export function cacheHitRate(row: CacheTokens): number | null {
  const tokens = [row.input_tokens, row.cache_read_tokens, row.cache_creation_tokens]
  if (tokens.some(value => value == null || !Number.isFinite(value) || value < 0)) return null
  const total = tokens.reduce((sum, value) => sum + value, 0)
  return total > 0 ? row.cache_read_tokens / total : null
}

export function formatCacheHitRate(row: CacheTokens): string {
  const rate = cacheHitRate(row)
  return rate == null ? '—' : `${(rate * 100).toFixed(2)}%`
}

// Keep these thresholds aligned with codex_quota/frontend/src/billing/statsFormat.ts.
export function cacheHitRateColor(row: CacheTokens): string {
  const rate = cacheHitRate(row)
  if (rate == null) return 'text-gray-400 dark:text-gray-500'
  if (rate >= .98) return 'font-bold text-green-800 dark:text-green-500'
  if (rate >= .95) return 'text-green-600 dark:text-green-400'
  if (rate >= .90) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-red-600 dark:text-red-400'
}

// Fork-only translations are co-located to avoid editing upstream locale trees.
const messages = {
  zh: {
    timeClock: '时间：时分秒', timeFull: '时间：完整日期',
    system: '系统', moreMenus: '更多菜单', sessionName: '会话名称', cacheHitRate: '缓存命中率', cacheMonitoring: '掉缓存监测',
    noTitle: '本机未找到会话名称', formula: '缓存读取 ÷（普通输入 + 缓存读取 + 缓存写入）',
    cacheDrop: '掉缓存', noSession: '无会话 ID', unavailable: '暂不可用',
    noDrop: '未检出',
    subagent: '子代理', guardian: 'Guardian 审核', memory: '记忆整理', threadMissing: '线程未记录',
    exactThread: '按客户端执行线程匹配本机最新名称',
    identityMissing: '仅记录了会话关联 ID，无法区分父窗口、子代理和审核请求；不参与掉缓存比较。',
    notComparable: '审核请求不参与掉缓存比较。',
    rule: '同一执行线程、请求类型、API Key、上游账号和模型，相邻请求间隔不超过 5 分钟；命中率下降至少 20 个百分点且缓存读取量减少。缺少线程标识、审核、并发重叠、时间相同、用量不足及压缩请求不作比较。仅依据已记录用量，不能判定掉缓存的原因。',
    previous: '上次', current: '本次', drop: '下降', points: '个百分点', gap: '间隔', seconds: '秒', previousUsage: '上次用量记录',
  },
  en: {
    timeClock: 'Time: clock', timeFull: 'Time: full date',
    system: 'System', moreMenus: 'More menus', sessionName: 'Session name', cacheHitRate: 'Cache hit rate', cacheMonitoring: 'Cache drop monitoring',
    noTitle: 'No local session title found', formula: 'Cache read ÷ (uncached input + cache read + cache write)',
    cacheDrop: 'Cache drop', noSession: 'No session ID', unavailable: 'Unavailable', noDrop: 'Not detected',
    subagent: 'Subagent', guardian: 'Guardian review', memory: 'Memory', threadMissing: 'Thread not recorded',
    exactThread: 'Latest local name matched by client execution thread',
    identityMissing: 'Only a session correlation ID was recorded; parent, subagent and review requests cannot be distinguished. Cache-drop comparison is disabled.',
    notComparable: 'Review requests are excluded from cache-drop comparison.',
    rule: 'Compare adjacent requests within 5 minutes in the same execution thread, request kind, API key, upstream account and model. Flag a drop of at least 20 percentage points with fewer cache reads. Skip missing thread IDs, review, overlapping, equal-time, insufficient-usage and compaction requests. Recorded usage alone cannot establish the cause.',
    previous: 'Previous', current: 'Current', drop: 'Drop', points: 'percentage points', gap: 'Gap', seconds: 'seconds', previousUsage: 'Previous usage record',
  },
}

export function usePersonalUsageLabels() {
  const { locale } = useI18n()
  return computed(() => messages[locale?.value?.startsWith('zh') ? 'zh' : 'en'])
}
