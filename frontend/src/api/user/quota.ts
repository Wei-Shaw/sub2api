/**
 * User Quota API endpoints
 *
 * 契约参考：docs/DAILY_QUOTA_CONTRACT.md §3.2 / §5.1
 */

import { apiClient } from '../client'
import type { UserQuotaStatus } from '@/types/quota'

/** 当前用户今日配额概览（只读） */
export async function getMyQuotaStatus(): Promise<UserQuotaStatus> {
  const { data } = await apiClient.get<UserQuotaStatus>('/users/me/quota/status')
  return data
}

export default { getMyQuotaStatus }
