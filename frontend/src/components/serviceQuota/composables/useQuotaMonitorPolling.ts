/**
 * useQuotaMonitorPolling — admin / user 两端共用的"限额监控 + 轮询"状态机。
 *
 * 单端点设计（**唯一**输入：`fetchSnapshot`）：
 *   - **手动刷新 / 初次加载 / 筛选变化**：调 `loadSnapshot()`，整体替换 items 数组
 *     与 `loading` 状态，错误走 `onError` 回调让调用方决定提示。
 *   - **自动刷新轮询**：复用同一个 `fetchSnapshot`，但不写 `loading`（避免按钮转圈
 *     抖动），并且把响应里的 items 按 (rule_id, path_id, limiter_type,
 *     scope_user_id) 复合 key **原地 merge** 进现有 items 的动态字段
 *     （current / utilization_pct / exists / reset_at_unix_ms），保留行对象引用——
 *     Vue 只触发对应单元格的依赖更新，DOM 不重建，progressbar / 滚动位置 / 选中
 *     状态全部保留。
 *
 * 为什么不再拆"轻量 usage 端点"：
 *   增加一个并行端点会逼后端、API client、service 接口、单测各自写两份，
 *   违反 CLAUDE.md「复用」原则。每次轮询响应多传 ~30% 字段（path_summary 大多
 *   为 null + rule_name 短字符串）实测仅 ~5KB 量级，完全可忽略——DOM 复用的
 *   关键不在于"传多少"，在于"merge 时是否替换数组 / 行对象引用"。
 *
 * 倒计时全部由 `useQuotaMonitorFormat` 暴露的共享每秒 tick 驱动，与本 composable
 * 解耦——即便没人发请求，"X 秒后重置"也照样递减。
 */
import { onScopeDispose, ref, type Ref } from 'vue'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'

/** 全量快照的最小契约：admin / user 端的响应都符合此形状 */
export interface QuotaMonitorSnapshot {
  enabled: boolean
  as_of_unix_ms: number
  items: LimiterRuntime[]
  truncated: boolean
}

export interface UseQuotaMonitorPollingOptions {
  /**
   * 唯一的数据获取入口：手动刷新与轮询都调此函数。filter / 鉴权 由调用方闭包捕获。
   */
  fetchSnapshot: () => Promise<QuotaMonitorSnapshot>
  /** 自动刷新档位（秒）——默认 5；调用方自己管理 intervals 列表 */
  defaultIntervalSeconds?: number
  /**
   * 手动刷新失败时的回调。轮询失败默认静默——上次成功的快照保持显示，等下一轮再试，
   * 避免短暂网络抖动让用户看到错误提示后又自动恢复（造成困惑）。
   * 若调用方需要观测轮询失败可用 `onPollingError`。
   */
  onError?: (err: unknown) => void
  /** 轮询失败回调（默认静默吞错）；典型用法是打 console.warn 给运维查看 */
  onPollingError?: (err: unknown) => void
}

export interface UseQuotaMonitorPollingResult {
  snapshot: Ref<QuotaMonitorSnapshot | null>
  loading: Ref<boolean>
  /** 自动刷新启用开关，受调用方 AutoRefreshButton 双向绑定 */
  autoEnabled: Ref<boolean>
  /** 当前选中的自动刷新档位（秒） */
  autoInterval: Ref<number>
  /** 距下次自动刷新的倒计时（秒，每秒 -1） */
  countdown: Ref<number>
  /** 距上次成功更新的秒数（每秒 +1，每次成功响应后清零） */
  secondsSinceUpdate: Ref<number>
  /** 调用方点 refresh 按钮 / 筛选变化时调；整体替换 items 数组并写 loading */
  loadSnapshot: () => Promise<void>
  /** 设置 auto-refresh 开关（true=立即开始倒计时、false=停止 tick） */
  setAutoEnabled: (enabled: boolean) => void
  /** 切换刷新档位（秒）——若自动刷新已开启则立即重启倒计时 */
  setAutoInterval: (seconds: number) => void
  /** 启动整个轮询机制：onMounted 调一次即可。内部已防止重复启动 */
  start: () => void
  /** 显式停止：onBeforeUnmount 调；start 后必须 stop 释放 timer */
  stop: () => void
}

/**
 * 行身份键：与后端 `BuildServiceQuotaCounterKey` 的复合 key 严格对齐。
 * scope_user_id == null/undefined 视为同一类（shared 计数器）。
 */
function rowIdentity(
  ruleID: number,
  pathID: number,
  limiterType: string,
  scopeUserID: number | null | undefined,
): string {
  return `${ruleID}|${pathID}|${limiterType}|${scopeUserID ?? 'shared'}`
}

/**
 * 把 next.items 按行身份合并进 prev.items：
 *   - 命中现有行 → 原地更新 4 个会变字段（current / utilization_pct / exists /
 *     reset_at_unix_ms），保留行对象引用
 *   - 未命中（规则被删 / 上次 truncated 边界）→ 直接落入新数组尾部
 *   - prev 中存在但 next 不再返回 → 一并丢弃（与"整体替换"语义一致），保证轮询
 *     之后视图始终等价于刚拉了一次快照
 *
 * 抽出来纯函数便于单测与本地推理；不修改入参，返回新数组。
 */
export function mergeSnapshotItems(
  prev: LimiterRuntime[],
  next: LimiterRuntime[],
): LimiterRuntime[] {
  if (prev.length === 0) return next.slice()
  const prevIndex = new Map<string, LimiterRuntime>()
  for (const row of prev) {
    prevIndex.set(
      rowIdentity(row.rule_id, row.path_id, row.limiter_type, row.scope_user_id),
      row,
    )
  }
  return next.map((incoming) => {
    const key = rowIdentity(
      incoming.rule_id,
      incoming.path_id,
      incoming.limiter_type,
      incoming.scope_user_id,
    )
    const existing = prevIndex.get(key)
    if (!existing) return incoming
    existing.current = incoming.current
    existing.utilization_pct = incoming.utilization_pct
    existing.exists = incoming.exists
    existing.reset_at_unix_ms = incoming.reset_at_unix_ms
    return existing
  })
}

export function useQuotaMonitorPolling(
  options: UseQuotaMonitorPollingOptions,
): UseQuotaMonitorPollingResult {
  const defaultInterval = options.defaultIntervalSeconds ?? 5

  const snapshot = ref<QuotaMonitorSnapshot | null>(null)
  const loading = ref(false)
  const autoEnabled = ref(true)
  const autoInterval = ref<number>(defaultInterval)
  const countdown = ref<number>(defaultInterval)
  const secondsSinceUpdate = ref(0)

  let countdownTimer: ReturnType<typeof setInterval> | null = null
  let asOfTimer: ReturnType<typeof setInterval> | null = null

  /**
   * 手动刷新与初次加载共用入口：整体替换 items 数组、写 loading、错误抛 onError。
   * loading=true 期间二次调用直接 return，防止筛选高频变更触发并发请求。
   */
  async function loadSnapshot(): Promise<void> {
    if (loading.value) return
    loading.value = true
    try {
      snapshot.value = await options.fetchSnapshot()
      secondsSinceUpdate.value = 0
    } catch (err: unknown) {
      options.onError?.(err)
    } finally {
      loading.value = false
    }
  }

  /**
   * 轮询路径：复用 fetchSnapshot，但不写 loading，merge 而不替换 items 数组。
   * 上一份基线缺失（首屏未到达 / 上次失败的恢复路径）时退化为 loadSnapshot。
   */
  async function pollSnapshot(): Promise<void> {
    if (snapshot.value === null) {
      await loadSnapshot()
      return
    }
    try {
      const next = await options.fetchSnapshot()
      const current = snapshot.value
      if (current === null) return
      current.enabled = next.enabled
      current.as_of_unix_ms = next.as_of_unix_ms
      current.truncated = next.truncated
      current.items = mergeSnapshotItems(current.items, next.items)
      secondsSinceUpdate.value = 0
    } catch (err: unknown) {
      options.onPollingError?.(err)
    }
  }

  function clearCountdownTimer(): void {
    if (countdownTimer !== null) {
      clearInterval(countdownTimer)
      countdownTimer = null
    }
  }

  function startCountdownTimer(): void {
    clearCountdownTimer()
    if (!autoEnabled.value) return
    countdown.value = autoInterval.value
    countdownTimer = setInterval(() => {
      countdown.value -= 1
      if (countdown.value <= 0) {
        countdown.value = autoInterval.value
        pollSnapshot()
      }
    }, 1000)
  }

  function setAutoEnabled(enabled: boolean): void {
    autoEnabled.value = enabled
    if (enabled) {
      startCountdownTimer()
    } else {
      clearCountdownTimer()
    }
  }

  function setAutoInterval(seconds: number): void {
    autoInterval.value = seconds
    if (autoEnabled.value) startCountdownTimer()
  }

  let started = false
  function start(): void {
    if (started) return
    started = true
    startCountdownTimer()
    if (asOfTimer === null) {
      asOfTimer = setInterval(() => {
        secondsSinceUpdate.value += 1
      }, 1000)
    }
  }

  function stop(): void {
    started = false
    clearCountdownTimer()
    if (asOfTimer !== null) {
      clearInterval(asOfTimer)
      asOfTimer = null
    }
  }

  onScopeDispose(() => {
    stop()
  })

  return {
    snapshot,
    loading,
    autoEnabled,
    autoInterval,
    countdown,
    secondsSinceUpdate,
    loadSnapshot,
    setAutoEnabled,
    setAutoInterval,
    start,
    stop,
  }
}
