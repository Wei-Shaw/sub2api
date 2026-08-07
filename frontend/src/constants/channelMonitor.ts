/**
 * Channel monitor shared constants.
 *
 * Single source of truth for provider/status string values used by both the
 * admin (`views/admin/ChannelMonitorView.vue`) and user-facing
 * (`views/user/ChannelStatusView.vue`) screens, plus the shared composable
 * `useChannelMonitorFormat`.
 */

import type { APIMode, Provider, MonitorStatus } from '@/api/admin/channelMonitor'

export const PROVIDER_OPENAI: Provider = 'openai'
export const PROVIDER_ANTHROPIC: Provider = 'anthropic'
export const PROVIDER_GEMINI: Provider = 'gemini'
export const PROVIDER_GROK: Provider = 'grok'
export const PROVIDER_DEEPSEEK: Provider = 'deepseek'
export const PROVIDER_GLM: Provider = 'glm'
export const PROVIDER_KIMI: Provider = 'kimi'

export const DEFAULT_GROK_ENDPOINT = 'https://api.x.ai'
export const DEFAULT_GROK_MODEL = 'grok-4.5'

export const DEFAULT_DEEPSEEK_ENDPOINT = 'https://api.deepseek.com'
export const DEFAULT_GLM_ENDPOINT = 'https://open.bigmodel.cn'
export const DEFAULT_KIMI_ENDPOINT = 'https://api.kimi.com'

/** Coding Plan 配额监控 provider（不发推理请求，查询套餐配额/余额） */
export const CODING_PLAN_PROVIDERS: readonly Provider[] = [
  PROVIDER_DEEPSEEK,
  PROVIDER_GLM,
  PROVIDER_KIMI,
]

export function isCodingPlanProvider(p: Provider | string): boolean {
  return (CODING_PLAN_PROVIDERS as readonly string[]).includes(p)
}

export const API_MODE_CHAT_COMPLETIONS: APIMode = 'chat_completions'
export const API_MODE_RESPONSES: APIMode = 'responses'

export const PROVIDERS: readonly Provider[] = [
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
  PROVIDER_GROK,
  PROVIDER_DEEPSEEK,
  PROVIDER_GLM,
  PROVIDER_KIMI,
]

export const API_MODES: readonly APIMode[] = [
  API_MODE_CHAT_COMPLETIONS,
  API_MODE_RESPONSES,
]

export const STATUS_OPERATIONAL: MonitorStatus = 'operational'
export const STATUS_DEGRADED: MonitorStatus = 'degraded'
export const STATUS_FAILED: MonitorStatus = 'failed'
export const STATUS_ERROR: MonitorStatus = 'error'

export const MONITOR_STATUSES: readonly MonitorStatus[] = [
  STATUS_OPERATIONAL,
  STATUS_DEGRADED,
  STATUS_FAILED,
  STATUS_ERROR,
]

/** Default polling interval (seconds) for new monitors. */
export const DEFAULT_INTERVAL_SECONDS = 60
