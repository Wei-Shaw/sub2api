/**
 * Admin User Quota API endpoints
 *
 * 契约参考：docs/DAILY_QUOTA_CONTRACT.md §3.1 / §5.1
 */

import { apiClient } from '../client'
import type {
  UserQuotaView,
  UpdateUserQuotaRequest,
  QuotaRule,
  CreateRuleRequest,
  UpdateRuleRequest,
} from '@/types/quota'

/** 获取指定用户的配额视图（含 resolved + today_usage） */
export async function getUserQuota(userId: number): Promise<UserQuotaView> {
  const { data } = await apiClient.get<UserQuotaView>(`/admin/users/${userId}/quota`)
  return data
}

/** 更新用户级 enabled + 总限额（不含规则） */
export async function updateUserQuota(
  userId: number,
  body: UpdateUserQuotaRequest,
): Promise<void> {
  await apiClient.put(`/admin/users/${userId}/quota`, body)
}

/** 列出指定用户的所有分组规则 */
export async function listUserQuotaRules(userId: number): Promise<QuotaRule[]> {
  const { data } = await apiClient.get<QuotaRule[]>(`/admin/users/${userId}/quota/rules`)
  return data
}

/** 新建规则 */
export async function createUserQuotaRule(
  userId: number,
  body: CreateRuleRequest,
): Promise<QuotaRule> {
  const { data } = await apiClient.post<QuotaRule>(`/admin/users/${userId}/quota/rules`, body)
  return data
}

/** 更新规则 */
export async function updateUserQuotaRule(
  userId: number,
  ruleId: number,
  body: UpdateRuleRequest,
): Promise<QuotaRule> {
  const { data } = await apiClient.put<QuotaRule>(
    `/admin/users/${userId}/quota/rules/${ruleId}`,
    body,
  )
  return data
}

/** 删除规则 */
export async function deleteUserQuotaRule(userId: number, ruleId: number): Promise<void> {
  await apiClient.delete(`/admin/users/${userId}/quota/rules/${ruleId}`)
}

/**
 * 批量替换规则（单事务幂等全量覆盖）
 *
 * 后端会：
 *   1. 校验所有入参（分组存在性、订阅类型拒绝、组 ID 重叠）
 *   2. 单事务内全量替换该用户的规则（未出现的历史规则被删除，其余新建或更新）
 *   3. 返回替换后的完整规则列表
 */
export async function replaceUserQuotaRules(
  userId: number,
  rules: CreateRuleRequest[],
): Promise<QuotaRule[]> {
  const { data } = await apiClient.put<QuotaRule[]>(
    `/admin/users/${userId}/quota/rules`,
    { rules },
  )
  return data
}

export const userQuotaAPI = {
  getUserQuota,
  updateUserQuota,
  listUserQuotaRules,
  createUserQuotaRule,
  updateUserQuotaRule,
  deleteUserQuotaRule,
  replaceUserQuotaRules,
}

export default userQuotaAPI
