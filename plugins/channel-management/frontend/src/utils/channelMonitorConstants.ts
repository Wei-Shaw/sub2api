/**
 * Channel monitor shared constants.
 *
 * V5 W7 — vendored from host frontend `src/constants/channelMonitor.ts`.
 * Single source of truth for the provider/status string values used by both
 * the admin (ChannelMonitorView) and user-facing (ChannelStatusView) screens.
 */

import type { Provider, MonitorStatus } from '../api/admin/channelMonitor'

export const PROVIDER_OPENAI: Provider = 'openai'
export const PROVIDER_ANTHROPIC: Provider = 'anthropic'
export const PROVIDER_GEMINI: Provider = 'gemini'

export const PROVIDERS: readonly Provider[] = [
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
]

export const STATUS_OPERATIONAL: MonitorStatus = 'operational'
export const STATUS_DEGRADED: MonitorStatus = 'degraded'
export const STATUS_FAILED: MonitorStatus = 'failed'
export const STATUS_ERROR: MonitorStatus = 'error'

/** Default polling interval (seconds) for new monitors. */
export const DEFAULT_INTERVAL_SECONDS = 60
