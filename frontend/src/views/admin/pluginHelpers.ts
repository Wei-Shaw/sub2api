export interface PluginInfo {
  name: string
  display_name?: string
  version?: string
  description?: string
  enabled: boolean
  sort_order?: number
  state?: string
  grpc_addr?: string
  http_addr?: string
  restart_count?: number
  builtin?: boolean
  started_at?: string
  last_error?: string
  config?: Record<string, unknown>
  icon_svg?: string
  has_settings?: boolean
  capabilities?: string[]
  uninstalled_at?: string | null
}

export type CapabilityCategory = 'default' | 'declared'

export const CAPABILITY_CATEGORY: Record<string, CapabilityCategory> = {
  'redis.own': 'default',
  'http.register.plugin': 'default',
  'jobs.register': 'default',
  'settings.own.read': 'default',
  'settings.own.write': 'default',
  'db.own.read': 'default',
  'db.own.write': 'default',
  'migrations.apply': 'default',
  'outbound.http': 'default',
  'events.subscribe.lowfreq': 'default',
  'redis.raw': 'declared',
  'secrets.encrypt': 'declared',
  'events.subscribe.gateway': 'declared',
  'http.register.gateway': 'declared',
  'db.core.read': 'declared',
  'db.core.write': 'declared',
  redis_raw_keys: 'declared',
  safe_outbound_http: 'default',
  secret_encryption: 'declared',
  job_scheduler: 'default',
  settings_extension: 'default',
  'events.gateway': 'declared',
}

export function capabilityCategory(name: string): CapabilityCategory {
  return CAPABILITY_CATEGORY[name] ?? 'declared'
}

export type PluginAction = 'enable' | 'disable' | 'restart'

export function dotClass(state?: string, enabled?: boolean): string {
  if (!enabled) return 'bg-gray-400 dark:bg-gray-500'
  switch ((state || '').toLowerCase()) {
    case 'running':
      return 'bg-emerald-500'
    case 'starting':
    case 'restarting':
      return 'bg-amber-500'
    case 'errored':
      return 'bg-red-500'
    default:
      return 'bg-gray-400 dark:bg-gray-500'
  }
}

export function stateLabel(
  p: Pick<PluginInfo, 'enabled' | 'state'>,
  t: (key: string) => string,
): string {
  if (!p.enabled) return t('admin.plugins.stateDisabled')
  switch ((p.state || '').toLowerCase()) {
    case 'running':
      return t('admin.plugins.stateRunning')
    case 'starting':
      return t('admin.plugins.stateStarting')
    case 'restarting':
      return t('admin.plugins.stateRestarting')
    case '':
    case undefined:
      return t('admin.plugins.stateUnknown')
    case 'errored':
      return t('admin.plugins.stateErrored')
    default:
      return p.state || t('admin.plugins.stateUnknown')
  }
}

export function formatUptime(iso: string, nowMs: number): string {
  const ts = Date.parse(iso)
  if (!Number.isFinite(ts)) return '—'
  let secs = Math.max(0, Math.floor((nowMs - ts) / 1000))
  const days = Math.floor(secs / 86400)
  secs -= days * 86400
  const hours = Math.floor(secs / 3600)
  secs -= hours * 3600
  const mins = Math.floor(secs / 60)
  secs -= mins * 60
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  if (mins > 0) return `${mins}m`
  return `${secs}s`
}

export function isZeroTime(iso: string): boolean {
  return iso.startsWith('0001-01-01')
}

export function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

export function formatConfigValue(v: unknown): string {
  if (v === null || v === undefined) return '—'
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}
