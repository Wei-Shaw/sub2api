import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { useUserQuotaFilters } from '@/views/user/composables/useUserQuotaFilters'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'

function makeRow(overrides: Partial<LimiterRuntime> = {}): LimiterRuntime {
  return {
    rule_id: 1,
    rule_name: 'rule-1',
    path_id: 10,
    path_index: 1,
    path_summary: { platform: 'anthropic', channel_id: null, group_id: null, account_id: null, model_pattern: null },
    limiter_type: 'rpm',
    window_mode: 'fixed',
    limit_value: 60,
    current: 6,
    utilization_pct: 10,
    counter_mode: 'shared',
    scope_user_id: null,
    is_fallback: false,
    exists: true,
    ...overrides,
  }
}

describe('useUserQuotaFilters', () => {
  it('默认 filters 全部不激活时输出原始 rows', () => {
    const rows = ref([
      makeRow({ rule_id: 1, limiter_type: 'rpm' }),
      makeRow({ rule_id: 2, limiter_type: 'tpm' }),
    ])
    const { filters, filteredRows, hasActiveFilter } = useUserQuotaFilters(rows)
    expect(filters.scope).toBe('all')
    expect(hasActiveFilter.value).toBe(false)
    expect(filteredRows.value).toHaveLength(2)
  })

  it('ruleID 精确匹配', () => {
    const rows = ref([makeRow({ rule_id: 1 }), makeRow({ rule_id: 2 }), makeRow({ rule_id: 1 })])
    const { filters, filteredRows, hasActiveFilter } = useUserQuotaFilters(rows)
    filters.ruleID = 1
    expect(hasActiveFilter.value).toBe(true)
    expect(filteredRows.value).toHaveLength(2)
    expect(filteredRows.value.every((r) => r.rule_id === 1)).toBe(true)
  })

  it('platforms 多选 OR 过滤', () => {
    const rows = ref([
      makeRow({ path_summary: { platform: 'anthropic' } }),
      makeRow({ path_summary: { platform: 'openai' } }),
      makeRow({ path_summary: { platform: 'gemini' } }),
    ])
    const { filters, filteredRows } = useUserQuotaFilters(rows)
    filters.platforms = ['anthropic', 'gemini']
    expect(filteredRows.value).toHaveLength(2)
    expect(filteredRows.value.map((r) => r.path_summary?.platform)).toEqual(['anthropic', 'gemini'])
  })

  it('platforms 空数组不过滤（即使有 path_summary=null 行也保留）', () => {
    const rows = ref([
      makeRow({ path_summary: null }),
      makeRow({ path_summary: { platform: 'anthropic' } }),
    ])
    const { filters, filteredRows } = useUserQuotaFilters(rows)
    expect(filters.platforms).toEqual([])
    expect(filteredRows.value).toHaveLength(2)
  })

  it('scope=global 仅保留 shared / per_user', () => {
    const rows = ref([
      makeRow({ counter_mode: 'shared' }),
      makeRow({ counter_mode: 'per_user' }),
      makeRow({ counter_mode: 'user' }),
    ])
    const { filters, filteredRows } = useUserQuotaFilters(rows)
    filters.scope = 'global'
    expect(filteredRows.value).toHaveLength(2)
    expect(filteredRows.value.every((r) => r.counter_mode !== 'user')).toBe(true)
  })

  it('scope=mine 仅保留 user', () => {
    const rows = ref([
      makeRow({ counter_mode: 'shared' }),
      makeRow({ counter_mode: 'user' }),
    ])
    const { filters, filteredRows } = useUserQuotaFilters(rows)
    filters.scope = 'mine'
    expect(filteredRows.value).toHaveLength(1)
    expect(filteredRows.value[0].counter_mode).toBe('user')
  })

  it('limiterTypes 多选 OR 过滤', () => {
    const rows = ref([
      makeRow({ limiter_type: 'rpm' }),
      makeRow({ limiter_type: 'tpm' }),
      makeRow({ limiter_type: 'concurrency' }),
    ])
    const { filters, filteredRows } = useUserQuotaFilters(rows)
    filters.limiterTypes = ['rpm', 'concurrency']
    expect(filteredRows.value).toHaveLength(2)
    expect(filteredRows.value.map((r) => r.limiter_type).sort()).toEqual(['concurrency', 'rpm'])
  })

  it('status=exceeded 仅保留 utilization >= 100', () => {
    const rows = ref([
      makeRow({ utilization_pct: 50 }),
      makeRow({ utilization_pct: 100 }),
      makeRow({ utilization_pct: 110 }),
    ])
    const { filters, filteredRows } = useUserQuotaFilters(rows)
    filters.status = 'exceeded'
    expect(filteredRows.value).toHaveLength(2)
    expect(filteredRows.value.every((r) => r.utilization_pct >= 100)).toBe(true)
  })

  it('status=healthy 仅保留 utilization < 100', () => {
    const rows = ref([
      makeRow({ utilization_pct: 50 }),
      makeRow({ utilization_pct: 100 }),
    ])
    const { filters, filteredRows } = useUserQuotaFilters(rows)
    filters.status = 'healthy'
    expect(filteredRows.value).toHaveLength(1)
    expect(filteredRows.value[0].utilization_pct).toBe(50)
  })

  it('多 filter 组合 AND 生效（rule + platform + limiterType + status）', () => {
    const rows = ref([
      // 全匹配：rule=1 + anthropic + rpm + 超限
      makeRow({ rule_id: 1, path_summary: { platform: 'anthropic' }, limiter_type: 'rpm', utilization_pct: 100 }),
      // rule 不匹配
      makeRow({ rule_id: 2, path_summary: { platform: 'anthropic' }, limiter_type: 'rpm', utilization_pct: 100 }),
      // platform 不匹配
      makeRow({ rule_id: 1, path_summary: { platform: 'openai' }, limiter_type: 'rpm', utilization_pct: 100 }),
      // limiter_type 不匹配
      makeRow({ rule_id: 1, path_summary: { platform: 'anthropic' }, limiter_type: 'tpm', utilization_pct: 100 }),
      // status 不匹配
      makeRow({ rule_id: 1, path_summary: { platform: 'anthropic' }, limiter_type: 'rpm', utilization_pct: 50 }),
    ])
    const { filters, filteredRows } = useUserQuotaFilters(rows)
    filters.ruleID = 1
    filters.platforms = ['anthropic']
    filters.limiterTypes = ['rpm']
    filters.status = 'exceeded'
    expect(filteredRows.value).toHaveLength(1)
    expect(filteredRows.value[0].rule_id).toBe(1)
  })

  it('ruleOptions 去重 by rule_id 保留首次 rule_name', () => {
    const rows = ref([
      makeRow({ rule_id: 1, rule_name: 'a' }),
      makeRow({ rule_id: 2, rule_name: 'b' }),
      makeRow({ rule_id: 1, rule_name: 'a-dup' }),
    ])
    const { ruleOptions } = useUserQuotaFilters(rows)
    expect(ruleOptions.value).toEqual([
      { id: 1, label: 'a' },
      { id: 2, label: 'b' },
    ])
  })

  it('rule_name 缺失时 ruleOptions 用 #id 占位', () => {
    const rows = ref([makeRow({ rule_id: 7, rule_name: '' })])
    const { ruleOptions } = useUserQuotaFilters(rows)
    expect(ruleOptions.value).toEqual([{ id: 7, label: '#7' }])
  })

  it('platformOptions 去重 + 字典排序', () => {
    const rows = ref([
      makeRow({ path_summary: { platform: 'openai' } }),
      makeRow({ path_summary: { platform: 'anthropic' } }),
      makeRow({ path_summary: { platform: 'openai' } }),
      makeRow({ path_summary: null }),
    ])
    const { platformOptions } = useUserQuotaFilters(rows)
    expect(platformOptions.value).toEqual(['anthropic', 'openai'])
  })

  it('limiterTypeOptions 去重 + 业务序排序', () => {
    const rows = ref([
      makeRow({ limiter_type: 'concurrency' }),
      makeRow({ limiter_type: 'rpm' }),
      makeRow({ limiter_type: 'tpd' }),
      makeRow({ limiter_type: 'rpm' }),
    ])
    const { limiterTypeOptions } = useUserQuotaFilters(rows)
    expect(limiterTypeOptions.value).toEqual(['rpm', 'tpd', 'concurrency'])
  })

  it('resetFilters 清回默认状态', () => {
    const rows = ref([makeRow({})])
    const { filters, resetFilters, hasActiveFilter } = useUserQuotaFilters(rows)
    filters.ruleID = 1
    filters.platforms = ['x']
    filters.scope = 'global'
    filters.limiterTypes = ['rpm']
    filters.status = 'exceeded'
    expect(hasActiveFilter.value).toBe(true)

    resetFilters()
    expect(hasActiveFilter.value).toBe(false)
    expect(filters.ruleID).toBe(null)
    expect(filters.platforms).toEqual([])
    expect(filters.scope).toBe('all')
    expect(filters.limiterTypes).toEqual([])
    expect(filters.status).toBe('all')
  })
})
