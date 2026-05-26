/**
 * useQuotaMonitorFormat — service-quota 监控表格的纯格式化 helpers。
 *
 * 全部基于 vue-i18n（不接受外部参数），admin / user 双视角共用。
 * 提取出来的目的：
 *   1. 让 QuotaMonitorTable.vue 只负责模板渲染，单文件长度回到 ≤ 250 行
 *   2. 让"limiter chip 文案 / 窗口文案 / 计数模式文案"在多处需要时能复用
 *      （比如未来其他卡片视图也想展示同一套 label）
 *
 * 注意：函数本身是纯函数，但通过 useI18n() 拿 t——必须在 setup() 里调用，
 * 不能放到模块顶层；这是 vue-i18n 在组合式 API 下的硬性约束。
 *
 * 倒计时响应式：模块级 sharedNowMs 每秒 tick，resetSeconds/statusText 读取
 * 此 ref，让 Vue 在 1 秒粒度上自动重渲倒计时——即便没人发请求、轮询暂停，
 * "重置: 31s 后" 也会自然走完。tick 用引用计数管理，无消费组件时停止。
 */
import { onScopeDispose, ref, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'
import { formatThousands, formatDailyUsd } from '@/utils/format'

// sharedNowMs 是模块级单例：所有 useQuotaMonitorFormat 调用共享同一份每秒 tick，
// 避免每个表格实例各开一个 setInterval。
const sharedNowMs: Ref<number> = ref(Date.now())
let tickRefcount = 0
let tickHandle: ReturnType<typeof setInterval> | null = null

function acquireNowTick(): void {
  tickRefcount += 1
  if (tickHandle === null) {
    tickHandle = setInterval(() => {
      sharedNowMs.value = Date.now()
    }, 1000)
  }
}

function releaseNowTick(): void {
  tickRefcount = Math.max(0, tickRefcount - 1)
  if (tickRefcount === 0 && tickHandle !== null) {
    clearInterval(tickHandle)
    tickHandle = null
  }
}

/**
 * 暴露给外部（如 useQuotaMonitorPolling、单测）的共享 tick ref。
 * 调用方读取此 ref 后在每秒边界上会触发自身的响应式更新。
 */
export function useSharedQuotaNow(): Ref<number> {
  return sharedNowMs
}

export interface UseQuotaMonitorFormatResult {
  formatLimiter: (type: string) => string
  formatWindow: (mode: string) => string
  formatCounterMode: (mode: string | undefined) => string
  counterModeBadgeClass: (mode: string | undefined) => string
  formatUsageNumbers: (row: LimiterRuntime) => string
  resetSeconds: (row: LimiterRuntime) => number | null
  statusText: (row: LimiterRuntime) => string
}

export function useQuotaMonitorFormat(): UseQuotaMonitorFormatResult {
  const { t } = useI18n()
  acquireNowTick()
  onScopeDispose(() => {
    releaseNowTick()
  })

  function formatLimiter(type: string): string {
    const key = type === 'daily_usd' ? 'dailyUsd' : type
    return t(`admin.serviceQuota.limiters.${key}`, type.toUpperCase())
  }

  function formatWindow(mode: string): string {
    return t(`admin.serviceQuota.windows.${mode}`, mode)
  }

  function formatCounterMode(mode: string | undefined): string {
    if (!mode) return ''
    const key = mode === 'per_user' ? 'perUser' : mode
    return t(`admin.serviceQuota.counterModes.${key}`, mode)
  }

  function counterModeBadgeClass(mode: string | undefined): string {
    if (mode === 'shared') return 'badge-info'
    if (mode === 'user') return 'badge-success'
    return 'badge-gray'
  }

  // daily_usd 是金额，至少 2 位小数 + 千分位；其他都是整数 + 千分位（5,000 / 2,000,000）。
  function formatUsageNumbers(row: LimiterRuntime): string {
    const isUsd = row.limiter_type === 'daily_usd'
    const limitText = isUsd ? formatDailyUsd(row.limit_value, 2) : formatThousands(Math.round(row.limit_value))
    const currentText = isUsd ? formatDailyUsd(row.current, 2) : formatThousands(Math.round(row.current))
    return `${currentText} / ${limitText}`
  }

  // 倒计时：reset_at_unix_ms <= 0 / key 不存在 → 不显示。
  // 读取共享 sharedNowMs（每秒 tick），让 statusText 在模板里被调用时随着秒针自动重渲——
  // 即便后端没下发新数据、用户没点刷新，"X 秒后重置"也照样减小到 0。
  function resetSeconds(row: LimiterRuntime): number | null {
    const resetAt = row.reset_at_unix_ms
    if (!resetAt || resetAt <= 0) return null
    if (!row.exists) return null
    return Math.max(0, Math.ceil((resetAt - sharedNowMs.value) / 1000))
  }

  // statusText 给用量列尾部的状态短文案：
  //   - 有 reset_at 且 key 存在 → "重置: Xs 后"
  //   - 否则 → "现在"（活跃但无窗口/未触发）
  function statusText(row: LimiterRuntime): string {
    const sec = resetSeconds(row)
    if (sec !== null) {
      return t('admin.serviceQuotaMonitor.resetIn', { seconds: sec })
    }
    return t('admin.serviceQuotaMonitor.statusActive')
  }

  return {
    formatLimiter,
    formatWindow,
    formatCounterMode,
    counterModeBadgeClass,
    formatUsageNumbers,
    resetSeconds,
    statusText,
  }
}
