/**
 * 用户端服务限额 API
 * 后端会强制注入当前用户 user_id，shared 模式仍可见，
 * path_summary / counter_mode / scope_user_id 在用户视角会被抹空。
 */

import apiClient from '@/api/client'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'

export interface MyQuotaSnapshot {
  enabled: boolean
  as_of_unix_ms: number
  items: LimiterRuntime[]
  truncated: boolean
}

/** 拉取本用户的服务限额运行时快照 */
export async function getMyServiceQuota(): Promise<MyQuotaSnapshot> {
  const { data } = await apiClient.get<MyQuotaSnapshot>('/service-quotas/my')
  return data
}
