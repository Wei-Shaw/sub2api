/**
 * Admin Scheduled Tests API endpoints
 * Handles scheduled test plan management for account connectivity monitoring
 */

import { apiClient } from '../client'
import type {
  ScheduledTestPlan,
  ScheduledTestResult,
  CreateScheduledTestPlanRequest,
  UpdateScheduledTestPlanRequest,
  BatchCreateScheduledTestPlansRequest,
  BatchCreateScheduledTestPlansResult
} from '@/types'

/**
 * List all scheduled test plans for an account
 * @param accountId - Account ID
 * @returns List of scheduled test plans
 */
export async function listByAccount(accountId: number): Promise<ScheduledTestPlan[]> {
  const { data } = await apiClient.get<ScheduledTestPlan[]>(
    `/admin/accounts/${accountId}/scheduled-test-plans`
  )
  return data ?? []
}

/**
 * Create a new scheduled test plan
 * @param req - Plan creation request
 * @returns Created plan
 */
export async function create(req: CreateScheduledTestPlanRequest): Promise<ScheduledTestPlan> {
  const { data } = await apiClient.post<ScheduledTestPlan>(
    '/admin/scheduled-test-plans',
    req
  )
  return data
}

/**
 * Update an existing scheduled test plan
 * @param id - Plan ID
 * @param req - Fields to update
 * @returns Updated plan
 */
export async function update(id: number, req: UpdateScheduledTestPlanRequest): Promise<ScheduledTestPlan> {
  const { data } = await apiClient.put<ScheduledTestPlan>(
    `/admin/scheduled-test-plans/${id}`,
    req
  )
  return data
}

/**
 * Delete a scheduled test plan
 * @param id - Plan ID
 */
export async function deletePlan(id: number): Promise<void> {
  await apiClient.delete(`/admin/scheduled-test-plans/${id}`)
}

/**
 * List test results for a plan
 * @param planId - Plan ID
 * @param limit - Optional max number of results to return
 * @returns List of test results
 */
export async function listResults(planId: number, limit?: number): Promise<ScheduledTestResult[]> {
  const { data } = await apiClient.get<ScheduledTestResult[]>(
    `/admin/scheduled-test-plans/${planId}/results`,
    {
      params: limit ? { limit } : undefined
    }
  )
  return data ?? []
}

/**
 * Batch create/update scheduled test plans across multiple accounts.
 * 批量为多个账号配置定时健康检测(需求 §7.4)。冲突键为 (account_id, model_id)。
 * @param req - Batch request
 * @returns Per-account result summary
 */
export async function batchCreate(
  req: BatchCreateScheduledTestPlansRequest
): Promise<BatchCreateScheduledTestPlansResult> {
  const { data } = await apiClient.post<BatchCreateScheduledTestPlansResult>(
    '/admin/scheduled-test-plans/batch',
    req
  )
  return data
}

export const scheduledTestsAPI = {
  listByAccount,
  create,
  update,
  delete: deletePlan,
  listResults,
  batchCreate
}

export default scheduledTestsAPI
