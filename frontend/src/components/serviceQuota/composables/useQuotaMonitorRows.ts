/**
 * useQuotaMonitorRows — 给 LimiterRuntime[] 行数组计算 rule/path 列的 rowspan
 * 合并信息，输出可直接 v-for 渲染的 DecoratedRow[]。
 *
 * 算法：
 *   1. 第一遍统计每个 rule_id / (rule_id,path_id) 的总行数
 *   2. 第二遍按出现顺序：组首条赋值 span=count，非首条赋值 0
 *
 * 假设后端 Snapshot 已按 (rule_id, path_id, limiter_type) 排序——同 rule 的
 * 行连续。即便不连续，多个分组各自被认为是首条，渲染上不会错，只是合并不彻底。
 *
 * 抽出来的目的：让 QuotaMonitorTable.vue 模板专注 td 渲染，不混入跨行合并算法。
 */
import { computed, type ComputedRef, type Ref } from 'vue'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'

export interface DecoratedRow extends LimiterRuntime {
  /** 该行所属 rule 的总行数。>0 表示组首（渲染 td 并 rowspan=N）；=0 表示组内非首条（不渲染 td） */
  _ruleSpan: number
  /** 该行所属 (rule, path) 的总行数。同 _ruleSpan 语义 */
  _pathSpan: number
  /** Vue v-for stable key */
  _key: string
}

export function useQuotaMonitorRows(rows: Ref<LimiterRuntime[]>): ComputedRef<DecoratedRow[]> {
  return computed<DecoratedRow[]>(() => {
    const ruleCounts = new Map<number, number>()
    const pathCounts = new Map<string, number>()
    for (const row of rows.value) {
      ruleCounts.set(row.rule_id, (ruleCounts.get(row.rule_id) || 0) + 1)
      const pk = `${row.rule_id}:${row.path_id}`
      pathCounts.set(pk, (pathCounts.get(pk) || 0) + 1)
    }
    const ruleSeen = new Set<number>()
    const pathSeen = new Set<string>()
    return rows.value.map((row): DecoratedRow => {
      const pk = `${row.rule_id}:${row.path_id}`
      let ruleSpan = 0
      if (!ruleSeen.has(row.rule_id)) {
        ruleSeen.add(row.rule_id)
        ruleSpan = ruleCounts.get(row.rule_id) ?? 1
      }
      let pathSpan = 0
      if (!pathSeen.has(pk)) {
        pathSeen.add(pk)
        pathSpan = pathCounts.get(pk) ?? 1
      }
      return {
        ...row,
        _ruleSpan: ruleSpan,
        _pathSpan: pathSpan,
        // _key 不带数组下标，确保 admin/user 端轮询 merge 后即便顺序变化，Vue
        // 仍按业务身份复用同一行 DOM——避免 progressbar 抖动、动画断裂、用户选
        // 中状态丢失。复合 key 与后端 (rule_id, path_id, limiter_type, scope_user_id)
        // 的 merge key 严格对齐。
        _key: `${row.rule_id}-${row.path_id}-${row.limiter_type}-${row.scope_user_id ?? 'shared'}`,
      }
    })
  })
}
