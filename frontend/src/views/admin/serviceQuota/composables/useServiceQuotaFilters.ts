/**
 * useServiceQuotaFilters — ConfigView 的筛选状态 + 列表过滤逻辑。
 *
 * 接受规则列表 ref，返回筛选 state + 过滤后的 computed。
 * 单元筛选规则未与表格组件耦合（filter 字段名 == 后端 ServiceQuotaRule 字段名），
 * 让管理端 / 用户端共享筛选逻辑（如果后续 user 也需要规则列表）。
 */
import { computed, reactive, type ComputedRef, type Ref } from 'vue'
import type { ServiceQuotaRule } from '@/api/admin/serviceQuota'

export interface ServiceQuotaFilterState {
  /** counter_mode 精确匹配；空字符串表示不筛选 */
  counterMode: string
  /** is_fallback 字符串化匹配（'true'/'false'）；空字符串表示不筛选 */
  fallback: string
  /** enabled 字符串化匹配（'true'/'false'）；空字符串表示不筛选 */
  enabled: string
}

export interface UseServiceQuotaFiltersResult {
  filters: ServiceQuotaFilterState
  filteredRules: ComputedRef<ServiceQuotaRule[]>
  resetFilters: () => void
}

export function useServiceQuotaFilters(rules: Ref<ServiceQuotaRule[]>): UseServiceQuotaFiltersResult {
  const filters = reactive<ServiceQuotaFilterState>({
    counterMode: '',
    fallback: '',
    enabled: '',
  })

  const filteredRules = computed(() =>
    rules.value.filter((rule) => {
      if (filters.counterMode && rule.counter_mode !== filters.counterMode) return false
      if (filters.fallback && String(rule.is_fallback) !== filters.fallback) return false
      if (filters.enabled && String(rule.enabled) !== filters.enabled) return false
      return true
    }),
  )

  function resetFilters(): void {
    filters.counterMode = ''
    filters.fallback = ''
    filters.enabled = ''
  }

  return { filters, filteredRules, resetFilters }
}
