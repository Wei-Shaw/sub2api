import type { ProxyGroup } from '@/types'

export interface FirstServeConfig {
  reuse_scope: 'session' | 'account'
  rotate_seconds: number
  /** Legacy fields are read for existing accounts but are no longer edited or used. */
  ttl_minutes?: number
  ttft_seconds?: number
  max_switches?: number
  cooldown_seconds?: number
  proxy_mode: 'all' | 'selected'
  proxy_ids: number[]
}

export const firstServeFields = [
  { key: 'rotate_seconds', min: 1, max: 86400 }
] as const

export function readFirstServeConfig(extra?: Record<string, unknown>): FirstServeConfig {
  const defaults: FirstServeConfig = {
    rotate_seconds: 240,
    reuse_scope: 'account', proxy_mode: 'all', proxy_ids: []
  }
  const raw = extra?.openai_first_serve
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return defaults
  const config = raw as Partial<FirstServeConfig>
  return { ...defaults, ...config, proxy_ids: Array.isArray(config.proxy_ids) ? [...config.proxy_ids] : [] }
}

export function firstServeIssue(config: FirstServeConfig, groupId: number | null, groups: ProxyGroup[]) {
  if (!['session', 'account'].includes(config.reuse_scope)) return { key: 'scopeInvalid', params: {} }
  // Imports may not have a proxy group yet. Keep the account enabled and let
  // runtime validation report the missing group; selected mode still needs a
  // concrete group to validate its whitelist.
  if (!groupId && config.proxy_mode === 'selected') return { key: 'groupRequired', params: {} }
  for (const field of firstServeFields) {
    if (!Number.isInteger(config[field.key]) || config[field.key] < field.min || config[field.key] > field.max) {
      return { key: 'numberInvalid', params: { field: field.key, min: field.min, max: field.max } }
    }
  }
  if (config.proxy_mode === 'all') return null
  if (new Set(config.proxy_ids).size < 2) return { key: 'selectTwo', params: {} }
  const group = groups.find(item => item.id === groupId)
  const invalid = config.proxy_ids.filter(id => !group?.proxy_ids.includes(id))
  if (invalid.length) return { key: 'outsideGroup', params: { ids: invalid.map(id => `#${id}`).join(', ') } }
  return null
}
