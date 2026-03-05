/**
 * Admin Scheduled Tests API endpoints
 * Handles scheduled test plan management for account connectivity monitoring
 */

import { apiClient REDACTED from '../client'
import type {
  ScheduledTestPlan,
  ScheduledTestResult,
  CreateScheduledTestPlanRequest,
  UpdateScheduledTestPlanRequest
REDACTED from '@/types'

/**
 * List all scheduled test plans for an account
 * @param accountId - Account ID
 * @returns List of scheduled test plans
 */
export async function listByAccount(accountId: number): Promise<ScheduledTestPlan[]> {
  const { data REDACTED = await apiClient.get<ScheduledTestPlan[]>(
    `/admin/accounts/${accountIdREDACTED/scheduled-test-plans`
  )
  return data ?? []
REDACTED

/**
 * Create a new scheduled test plan
 * @param req - Plan creation request
 * @returns Created plan
 */
export async function create(req: CreateScheduledTestPlanRequest): Promise<ScheduledTestPlan> {
  const { data REDACTED = await apiClient.post<ScheduledTestPlan>(
    '/admin/scheduled-test-plans',
    req
  )
  return data
REDACTED

/**
 * Update an existing scheduled test plan
 * @param id - Plan ID
 * @param req - Fields to update
 * @returns Updated plan
 */
export async function update(id: number, req: UpdateScheduledTestPlanRequest): Promise<ScheduledTestPlan> {
  const { data REDACTED = await apiClient.put<ScheduledTestPlan>(
    `/admin/scheduled-test-plans/${idREDACTED`,
    req
  )
  return data
REDACTED

/**
 * Delete a scheduled test plan
 * @param id - Plan ID
 */
export async function deletePlan(id: number): Promise<void> {
  await apiClient.delete(`/admin/scheduled-test-plans/${idREDACTED`)
REDACTED

/**
 * List test results for a plan
 * @param planId - Plan ID
 * @param limit - Optional max number of results to return
 * @returns List of test results
 */
export async function listResults(planId: number, limit?: number): Promise<ScheduledTestResult[]> {
  const { data REDACTED = await apiClient.get<ScheduledTestResult[]>(
    `/admin/scheduled-test-plans/${planIdREDACTED/results`,
    {
      params: limit ? { limit REDACTED : undefined
    REDACTED
  )
  return data ?? []
REDACTED

export const scheduledTestsAPI = {
  listByAccount,
  create,
  update,
  delete: deletePlan,
  listResults
REDACTED

export default scheduledTestsAPI
