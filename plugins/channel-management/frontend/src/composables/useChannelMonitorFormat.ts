/**
 * Shared formatting helpers for channel monitor views.
 *
 * V5 W7 — vendored from host frontend `src/composables/useChannelMonitorFormat.ts`.
 * 简化范围: 只保留 plugin admin/user 视图实际使用的 helper, 不包含 host
 * dashboard 用到的 hslForPct / providerGradient 等装饰性辅助函数.
 *
 * i18n keys live under `monitorCommon.*` so admin and user views share the
 * same translation source.
 */

import { useI18n } from 'vue-i18n'
import type { MonitorStatus, Provider } from '../api/admin/channelMonitor'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
  STATUS_OPERATIONAL,
  STATUS_DEGRADED,
  STATUS_FAILED,
  STATUS_ERROR,
} from '../utils/channelMonitorConstants'
import {
  platformTagClass,
  platformPickerClass,
  platformGradientClass,
} from '@sub2api/plugin-sdk'

const NEUTRAL_BADGE = 'bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-gray-300'

/** Availability HSL hue multiplier: 0%=red(0) / 50%=yellow(60) / 100%=green(120). */
const HSL_HUE_PER_PERCENT = 1.2
const HSL_SATURATION = 72
const HSL_LIGHTNESS = 42

export interface AvailabilityRow {
  primary_status: MonitorStatus | ''
  availability_7d: number | null | undefined
}

export function useChannelMonitorFormat() {
  const { t } = useI18n()

  function statusLabel(s: MonitorStatus | ''): string {
    if (!s) return t('monitorCommon.status.unknown')
    return t(`monitorCommon.status.${s}`)
  }

  function statusBadgeClass(s: MonitorStatus | ''): string {
    switch (s) {
      case STATUS_OPERATIONAL:
        return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
      case STATUS_DEGRADED:
        return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
      case STATUS_FAILED:
        return 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
      case STATUS_ERROR:
      default:
        return NEUTRAL_BADGE
    }
  }

  function providerLabel(p: Provider | string): string {
    if (p === PROVIDER_OPENAI || p === PROVIDER_ANTHROPIC || p === PROVIDER_GEMINI) {
      return t(`monitorCommon.providers.${p}`)
    }
    return p || '-'
  }

  /** Platform palette delegated to utils/platformColors so admin/user views and channel form share a single switch. */
  function providerBadgeClass(p: Provider | string): string {
    return platformTagClass(p)
  }

  /** Picker palette delegated to utils/platformColors (active/inactive state). */
  function providerPickerClass(p: Provider | string, active: boolean): string {
    return platformPickerClass(p, active)
  }

  function formatLatency(ms: number | null | undefined): string {
    if (ms == null) return t('monitorCommon.latencyEmpty')
    return String(Math.round(ms))
  }

  function formatPercent(v: number | null | undefined): string {
    if (v == null || Number.isNaN(v)) return '-'
    return `${v.toFixed(2)}%`
  }

  function formatAvailability(row: AvailabilityRow): string {
    // T19 Fix D — TODO(channel-mgmt): primary_status 为空当前同时表示
    //   (a) 监控未启用 (尚未跑过任何检查), 与 (b) 启用但 availability_7d=0
    // 两种语义被合并为 '-', 用户无法区分 "未启用" 和 "全部失败".
    // 修复需要 API 在 AvailabilityRow / UserMonitorView 中显式暴露 enabled
    // 字段, 由后端补全后再走 `if (!row.enabled) return ...` 的判断.
    // 当前不引入新字段以保持与 V5 W7/W7.3 schema 一致.
    if (!row.primary_status) return '-'
    return formatPercent(row.availability_7d)
  }

  function formatRelativeTime(iso: string | null | undefined): string {
    if (!iso) return t('monitorCommon.latencyEmpty')
    const ts = Date.parse(iso)
    if (Number.isNaN(ts)) return t('monitorCommon.latencyEmpty')
    const diffSec = Math.max(0, Math.floor((Date.now() - ts) / 1000))
    if (diffSec < 60) return t('monitorCommon.relativeSecondsAgo', { n: diffSec })
    const diffMin = Math.floor(diffSec / 60)
    if (diffMin < 60) return t('monitorCommon.relativeMinutesAgo', { n: diffMin })
    const diffHour = Math.floor(diffMin / 60)
    if (diffHour < 24) return t('monitorCommon.relativeHoursAgo', { n: diffHour })
    const diffDay = Math.floor(diffHour / 24)
    return t('monitorCommon.relativeDaysAgo', { n: diffDay })
  }

  return {
    statusLabel,
    statusBadgeClass,
    providerLabel,
    providerBadgeClass,
    providerPickerClass,
    formatLatency,
    formatPercent,
    formatAvailability,
    formatRelativeTime,
  }
}

/**
 * Map availability percent to an HSL colour (red -> yellow -> green).
 * Returns undefined for null/NaN so callers can fall back to a neutral colour.
 */
export function hslForPct(pct: number | null | undefined): string | undefined {
  if (pct === null || pct === undefined || Number.isNaN(pct)) return undefined
  const clamped = Math.max(0, Math.min(100, pct))
  const hue = clamped * HSL_HUE_PER_PERCENT
  return `hsl(${hue} ${HSL_SATURATION}% ${HSL_LIGHTNESS}%)`
}

/**
 * Tailwind gradient class for the provider icon tile background.
 * Delegates to utils/platformColors so admin/user/MonitorCard share one source.
 */
export function providerGradient(provider: string): string {
  return platformGradientClass(provider)
}
