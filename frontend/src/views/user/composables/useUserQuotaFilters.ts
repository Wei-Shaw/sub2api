/**
 * useUserQuotaFilters — "我的限额"页面的本地 filter 状态 + 过滤逻辑。
 *
 * 与 admin 端 useServiceQuotaFilters 解耦：
 *   - admin 用 ServiceQuotaRule（配置静态行）作为输入；本 composable 用
 *     LimiterRuntime（运行时快照行）作为输入
 *   - admin 可按渠道/分组/账号筛选；user 视角这些字段被后端抹空（audit 3
 *     脱敏决定），所以本 composable 不暴露这些维度
 *
 * 设计：
 *   - filters 是 reactive 状态，view 直接 v-model 绑控件
 *   - filteredRows 是 computed，输入 ref 变更或 filters 变更都会自动重算
 *   - 过滤纯函数式：相同输入 + 相同 filters 永远返回相同输出，方便 spec 测试
 *
 * 不与 useServiceQuotaFilters 复用：两者输入数据模型不同（rule vs runtime），
 * 强行复用会让任一侧字段都得"包容另一侧"，违反"接口最小化"。
 */
import { computed, reactive, type ComputedRef, type Ref } from 'vue'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'

/**
 * 限额范围：
 *   - all：不筛选（默认）
 *   - global：仅全局共享 / 全员独立计数（counter_mode in {shared, per_user}）
 *   - mine：仅指定到我的（counter_mode = user）
 *
 * 与 QuotaMonitorTable.vue isGlobalRule 同源语义：global = "全部用户都受影响"
 */
export type UserQuotaScopeFilter = 'all' | 'global' | 'mine'

/**
 * 状态过滤：
 *   - all：默认
 *   - exceeded：utilization_pct >= 100 已超限（实际上后端可能在 acquire 阶段就拒绝，
 *              这里展示的是 snapshot 上看到的瞬时值；判定阈值 100 与 admin 监控页对齐）
 *   - healthy：utilization_pct < 100 健康
 */
export type UserQuotaStatusFilter = 'all' | 'exceeded' | 'healthy'

export interface UserQuotaFilterState {
  /** 规则 ID 精确匹配；0/null 表示不筛选 */
  ruleID: number | null
  /** 平台多选（path_summary.platform 命中任意一项保留）；空数组 = 不筛选 */
  platforms: string[]
  /** 限额范围 */
  scope: UserQuotaScopeFilter
  /** 限流器类型多选；空数组 = 不筛选 */
  limiterTypes: string[]
  /** 状态 */
  status: UserQuotaStatusFilter
}

export interface RuleOption {
  id: number
  label: string
}

export interface UseUserQuotaFiltersResult {
  filters: UserQuotaFilterState
  filteredRows: ComputedRef<LimiterRuntime[]>
  /** 当前快照内出现过的所有 rule（去重 by id），供下拉选择 */
  ruleOptions: ComputedRef<RuleOption[]>
  /** 当前快照内出现过的所有 platform（去重，按字典序），供 chip 渲染 */
  platformOptions: ComputedRef<string[]>
  /** 当前快照内出现过的所有 limiter_type（去重，按 chip 排序），供 chip 渲染 */
  limiterTypeOptions: ComputedRef<string[]>
  /** 是否有任意 filter 处于激活状态（用于"重置"按钮启用判断） */
  hasActiveFilter: ComputedRef<boolean>
  resetFilters: () => void
}

const SCOPE_GLOBAL_MODES = new Set<string>(['shared', 'per_user'])
const SCOPE_MINE_MODES = new Set<string>(['user'])

/** 与 admin LimiterEditor allTypes 顺序保持一致，让 chip 在视觉上稳定 */
const LIMITER_TYPE_ORDER: Record<string, number> = {
  rpm: 0,
  tpm: 1,
  tpd: 2,
  daily_usd: 3,
  concurrency: 4,
}

function sortLimiterTypes(types: string[]): string[] {
  return [...types].sort((a, b) => {
    const ai = LIMITER_TYPE_ORDER[a] ?? 99
    const bi = LIMITER_TYPE_ORDER[b] ?? 99
    return ai - bi
  })
}

function blankFilters(): UserQuotaFilterState {
  return {
    ruleID: null,
    platforms: [],
    scope: 'all',
    limiterTypes: [],
    status: 'all',
  }
}

export function useUserQuotaFilters(rows: Ref<LimiterRuntime[]>): UseUserQuotaFiltersResult {
  const filters = reactive<UserQuotaFilterState>(blankFilters())

  const ruleOptions = computed<RuleOption[]>(() => {
    const seen = new Map<number, string>()
    for (const row of rows.value) {
      if (!seen.has(row.rule_id)) seen.set(row.rule_id, row.rule_name || `#${row.rule_id}`)
    }
    return Array.from(seen.entries()).map(([id, label]) => ({ id, label }))
  })

  const platformOptions = computed<string[]>(() => {
    const seen = new Set<string>()
    for (const row of rows.value) {
      const p = row.path_summary?.platform
      if (p) seen.add(p)
    }
    return Array.from(seen).sort()
  })

  const limiterTypeOptions = computed<string[]>(() => {
    const seen = new Set<string>()
    for (const row of rows.value) {
      if (row.limiter_type) seen.add(row.limiter_type)
    }
    return sortLimiterTypes(Array.from(seen))
  })

  const hasActiveFilter = computed<boolean>(() =>
    filters.ruleID !== null ||
    filters.platforms.length > 0 ||
    filters.scope !== 'all' ||
    filters.limiterTypes.length > 0 ||
    filters.status !== 'all',
  )

  const filteredRows = computed<LimiterRuntime[]>(() => {
    return rows.value.filter((row) => {
      if (filters.ruleID !== null && row.rule_id !== filters.ruleID) return false

      if (filters.platforms.length > 0) {
        const p = row.path_summary?.platform || ''
        if (!filters.platforms.includes(p)) return false
      }

      if (filters.scope === 'global') {
        if (!SCOPE_GLOBAL_MODES.has(row.counter_mode || '')) return false
      } else if (filters.scope === 'mine') {
        if (!SCOPE_MINE_MODES.has(row.counter_mode || '')) return false
      }

      if (filters.limiterTypes.length > 0) {
        if (!filters.limiterTypes.includes(row.limiter_type)) return false
      }

      if (filters.status === 'exceeded') {
        if (!(row.utilization_pct >= 100)) return false
      } else if (filters.status === 'healthy') {
        if (!(row.utilization_pct < 100)) return false
      }

      return true
    })
  })

  function resetFilters(): void {
    Object.assign(filters, blankFilters())
  }

  return {
    filters,
    filteredRows,
    ruleOptions,
    platformOptions,
    limiterTypeOptions,
    hasActiveFilter,
    resetFilters,
  }
}
