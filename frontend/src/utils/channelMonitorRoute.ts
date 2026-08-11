import type { Provider } from '@/api/admin/channelMonitor'
import type { GroupPlatform } from '@/types'
import { PROVIDER_ANTHROPIC, PROVIDER_GEMINI } from '@/constants/channelMonitor'

export const ANTIGRAVITY_MONITOR_PATH = '/antigravity'

// Antigravity exposes both Anthropic and Gemini protocols behind its forced
// route. Account mixed_scheduling only affects unprefixed native-group traffic.
export function isMonitorGroupCompatible(
  provider: Provider,
  groupPlatform: GroupPlatform | undefined,
): boolean {
  if (!groupPlatform) return false
  if (provider === PROVIDER_GEMINI || provider === PROVIDER_ANTHROPIC) {
    return groupPlatform === provider || groupPlatform === 'antigravity'
  }
  return groupPlatform === provider
}

export function splitMonitorEndpoint(endpoint: string): {
  origin: string
  useAntigravityRoute: boolean
} {
  const trimmed = endpoint.trim()
  try {
    const parsed = new URL(trimmed)
    const normalizedPath = parsed.pathname.replace(/\/+$/, '')
    if (
      normalizedPath === ANTIGRAVITY_MONITOR_PATH &&
      parsed.search === '' &&
      parsed.hash === ''
    ) {
      return { origin: parsed.origin, useAntigravityRoute: true }
    }
  } catch {
    // Keep invalid/incomplete user input intact so the normal form error remains useful.
  }
  return { origin: trimmed, useAntigravityRoute: false }
}

export function composeMonitorEndpoint(origin: string, useAntigravityRoute: boolean): string {
  const normalizedOrigin = origin.trim().replace(/\/+$/, '')
  if (!useAntigravityRoute || !normalizedOrigin) return normalizedOrigin
  const split = splitMonitorEndpoint(normalizedOrigin)
  if (split.useAntigravityRoute) {
    return `${split.origin}${ANTIGRAVITY_MONITOR_PATH}`
  }
  return `${normalizedOrigin}${ANTIGRAVITY_MONITOR_PATH}`
}
