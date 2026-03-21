import type { CodexUsageSnapshot } from '@/types'

export interface ResolvedCodexUsageWindow {
  usedPercent: number | null
  resetAt: string | null
}

type WindowKind = '5h' | '7d'

function asNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && value.trim() !== '') {
    const n = Number(value)
    if (Number.isFinite(n)) return n
  }
  return null
}

function asString(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed === '' ? null : trimmed
}

function asISOTime(value: unknown): string | null {
  const raw = asString(value)
  if (!raw) return null
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return null
  return date.toISOString()
}

function resolveLegacy5h(snapshot: Record<string, unknown>): {
  used: number | null
  resetAfterSeconds: number | null
} {
  const primaryWindow = asNumber(snapshot.codex_primary_window_minutes)
  const secondaryWindow = asNumber(snapshot.codex_secondary_window_minutes)
  const primaryUsed = asNumber(snapshot.codex_primary_used_percent)
  const secondaryUsed = asNumber(snapshot.codex_secondary_used_percent)
  const primaryReset = asNumber(snapshot.codex_primary_reset_after_seconds)
  const secondaryReset = asNumber(snapshot.codex_secondary_reset_after_seconds)

  if (primaryWindow != null && primaryWindow <= 360) {
    return { used: primaryUsed, resetAfterSeconds: primaryReset }
  }
  if (secondaryWindow != null && secondaryWindow <= 360) {
    return { used: secondaryUsed, resetAfterSeconds: secondaryReset }
  }
  return { used: secondaryUsed, resetAfterSeconds: secondaryReset }
}

function resolveLegacy7d(snapshot: Record<string, unknown>): {
  used: number | null
  resetAfterSeconds: number | null
} {
  const primaryWindow = asNumber(snapshot.codex_primary_window_minutes)
  const secondaryWindow = asNumber(snapshot.codex_secondary_window_minutes)
  const primaryUsed = asNumber(snapshot.codex_primary_used_percent)
  const secondaryUsed = asNumber(snapshot.codex_secondary_used_percent)
  const primaryReset = asNumber(snapshot.codex_primary_reset_after_seconds)
  const secondaryReset = asNumber(snapshot.codex_secondary_reset_after_seconds)

  if (primaryWindow != null && primaryWindow >= 10000) {
    return { used: primaryUsed, resetAfterSeconds: primaryReset }
  }
  if (secondaryWindow != null && secondaryWindow >= 10000) {
    return { used: secondaryUsed, resetAfterSeconds: secondaryReset }
  }
  return { used: primaryUsed, resetAfterSeconds: primaryReset }
}

function resolveFromSeconds(
  snapshot: Record<string, unknown>,
  resetAfterSeconds: number | null
): string | null {
  if (resetAfterSeconds == null) return null

  const baseRaw = asString(snapshot.codex_usage_updated_at)
  const base = baseRaw ? new Date(baseRaw) : new Date()
  if (Number.isNaN(base.getTime())) return null

  const sec = Math.max(0, resetAfterSeconds)
  const resetAt = new Date(base.getTime() + sec * 1000)
  return resetAt.toISOString()
}

function applyExpiredRule(
  window: ResolvedCodexUsageWindow,
  now: Date
): ResolvedCodexUsageWindow {
  if (window.usedPercent == null || !window.resetAt) return window
  const resetDate = new Date(window.resetAt)
  if (Number.isNaN(resetDate.getTime())) return window
  if (resetDate.getTime() <= now.getTime()) {
    return { usedPercent: 0, resetAt: resetDate.toISOString() }
  }
  return window
}

export function resolveCodexUsageWindow(
  snapshot: (CodexUsageSnapshot & Record<string, unknown>) | null | undefined,
  window: WindowKind,
  now: Date = new Date()
): ResolvedCodexUsageWindow {
  if (!snapshot) {
    return { usedPercent: null, resetAt: null }
  }

  const typedSnapshot = snapshot as Record<string, unknown>
  let usedPercent: number | null
  let resetAfterSeconds: number | null
  let resetAt: string | null

  if (window === '5h') {
    usedPercent = asNumber(typedSnapshot.codex_5h_used_percent)
    resetAfterSeconds = asNumber(typedSnapshot.codex_5h_reset_after_seconds)
    resetAt = asISOTime(typedSnapshot.codex_5h_reset_at)
    if (usedPercent == null || (resetAfterSeconds == null && !resetAt)) {
      const legacy = resolveLegacy5h(typedSnapshot)
      if (usedPercent == null) usedPercent = legacy.used
      if (resetAfterSeconds == null) resetAfterSeconds = legacy.resetAfterSeconds
    }
  } else {
    usedPercent = asNumber(typedSnapshot.codex_7d_used_percent)
    resetAfterSeconds = asNumber(typedSnapshot.codex_7d_reset_after_seconds)
    resetAt = asISOTime(typedSnapshot.codex_7d_reset_at)
    if (usedPercent == null || (resetAfterSeconds == null && !resetAt)) {
      const legacy = resolveLegacy7d(typedSnapshot)
      if (usedPercent == null) usedPercent = legacy.used
      if (resetAfterSeconds == null) resetAfterSeconds = legacy.resetAfterSeconds
    }
  }

  if (!resetAt) {
    resetAt = resolveFromSeconds(typedSnapshot, resetAfterSeconds)
  }

  return applyExpiredRule({ usedPercent, resetAt }, now)
}
