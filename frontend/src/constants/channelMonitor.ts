/**
 * Channel monitor shared constants.
 *
 * Single source of truth for provider/status string values used by both the
 * admin (`views/admin/ChannelMonitorView.vue`) and user-facing
 * (`views/user/ChannelStatusView.vue`) screens, plus the shared composable
 * `useChannelMonitorFormat`.
 */

import type { APIMode, CheckMode, Provider, MonitorStatus } from '@/api/admin/channelMonitor'

export const PROVIDER_OPENAI: Provider = 'openai'
export const PROVIDER_ANTHROPIC: Provider = 'anthropic'
export const PROVIDER_GEMINI: Provider = 'gemini'
export const PROVIDER_GROK: Provider = 'grok'
export const PROVIDER_ANTIGRAVITY: Provider = 'antigravity'
export const PROVIDER_KIMI: Provider = 'kimi'
export const PROVIDER_ZHIPU: Provider = 'zhipu'
export const PROVIDER_DEEPSEEK: Provider = 'deepseek'

export const DEFAULT_GROK_ENDPOINT = 'https://api.x.ai'
export const DEFAULT_GROK_MODEL = 'grok-4.5'

/** 国产 provider 的官方 endpoint（探活模式预填；配额模式可留空）。 */
export const DEFAULT_KIMI_ENDPOINT = 'https://api.moonshot.cn'
export const DEFAULT_ZHIPU_ENDPOINT = 'https://open.bigmodel.cn'
export const DEFAULT_DEEPSEEK_ENDPOINT = 'https://api.deepseek.com'

export const CHECK_MODE_PROBE: CheckMode = 'probe'
export const CHECK_MODE_QUOTA: CheckMode = 'quota'
export const CHECK_MODE_QUOTA_PROBE: CheckMode = 'quota_probe'

export const API_MODE_CHAT_COMPLETIONS: APIMode = 'chat_completions'
export const API_MODE_RESPONSES: APIMode = 'responses'

export const PROVIDERS: readonly Provider[] = [
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
  PROVIDER_GROK,
  PROVIDER_ANTIGRAVITY,
  PROVIDER_KIMI,
  PROVIDER_ZHIPU,
  PROVIDER_DEEPSEEK,
]

/** 仅支持配额模式（无探活 adapter）的 provider。 */
export const QUOTA_ONLY_PROVIDERS: readonly Provider[] = [PROVIDER_ANTIGRAVITY]

export const CHECK_MODES: readonly CheckMode[] = [
  CHECK_MODE_PROBE,
  CHECK_MODE_QUOTA,
  CHECK_MODE_QUOTA_PROBE,
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
