/**
 * 配额规则的前端工具函数
 *
 * 与后端 `backend/internal/service/quota_helpers.go` 的 `normalizeGroupIDs` 保持
 * 语义一致，消除 `UserQuotaLimitModal` 中散落的排序/去重/冲突扫描代码。
 */

/**
 * 规范化分组 ID 数组：去除非正数、去重、升序排序
 *
 * 对应后端 `normalizeGroupIDs(in []int64) []int64` 的行为，用于提交规则前对
 * `group_ids` 做稳定化处理，便于后端比对、也让前端 UI 与后端 DB 展现一致。
 */
export function normalizeGroupIDs(ids: number[]): number[] {
  const seen = new Set<number>()
  const result: number[] = []
  for (const id of ids) {
    if (id > 0 && !seen.has(id)) {
      seen.add(id)
      result.push(id)
    }
  }
  return result.sort((a, b) => a - b)
}

/** 任意带 `group_ids` 字段的规则草稿结构 */
export interface RuleGroupIDs {
  group_ids: number[]
}

/**
 * 扫描多条规则的 group_ids，返回第一个被多条规则同时引用的分组 ID；
 * 若无冲突返回 `null`。
 *
 * 用于校验"同一分组不能被多条规则覆盖"的业务约束。
 */
export function findOverlappingGroupID<T extends RuleGroupIDs>(rules: T[]): number | null {
  const seen = new Set<number>()
  for (const rule of rules) {
    for (const id of rule.group_ids) {
      if (seen.has(id)) return id
      seen.add(id)
    }
  }
  return null
}
