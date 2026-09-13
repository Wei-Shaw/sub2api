/**
 * User Subscription API
 * API for regular users to view their own subscriptions and progress
 */

import { apiClient } from './client'
import type { UserSubscription, SubscriptionProgress } from '@/types'

/**
 * Subscription summary for user dashboard
 */
export interface SubscriptionSummary {
  active_count: number
  subscriptions: Array<{
    id: number
    group_name: string
    status: string
    daily_progress: number | null
    weekly_progress: number | null
    monthly_progress: number | null
    expires_at: string | null
    days_remaining: number | null
  }>
}

/**
 * Get list of current user's subscriptions
 */
export async function getMySubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions')
  return response.data
}

/**
 * Get current user's active subscriptions
 */
export async function getActiveSubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions/active')
  return response.data
}

/**
 * Get progress for all user's active subscriptions
 */
export async function getSubscriptionsProgress(): Promise<SubscriptionProgress[]> {
  const response = await apiClient.get<SubscriptionProgress[]>('/subscriptions/progress')
  return response.data
}

/**
 * Get subscription summary for dashboard display
 */
export async function getSubscriptionSummary(): Promise<SubscriptionSummary> {
  const response = await apiClient.get<SubscriptionSummary>('/subscriptions/summary')
  return response.data
}

/**
 * Get progress for a specific subscription
 */
export async function getSubscriptionProgress(
  subscriptionId: number
): Promise<SubscriptionProgress> {
  const response = await apiClient.get<SubscriptionProgress>(
    `/subscriptions/${subscriptionId}/progress`
  )
  return response.data
}

export default {
  getMySubscriptions,
  getActiveSubscriptions,
  getSubscriptionsProgress,
  getSubscriptionSummary,
  getSubscriptionProgress
}

export interface WeeklyAdvancePreview {
  subscription_id: number
  weekly_window_start: string
  expires_at: string
  new_expires_at: string
  deduct_seconds: number
}

export async function previewAdvanceWeek(id: number): Promise<WeeklyAdvancePreview> {
  return (await apiClient.get<WeeklyAdvancePreview>(`/subscriptions/${id}/advance-week`)).data
}

export async function advanceWeek(preview: WeeklyAdvancePreview): Promise<WeeklyAdvancePreview> {
  return (await apiClient.post<WeeklyAdvancePreview>(`/subscriptions/${preview.subscription_id}/advance-week`, {
    weekly_window_start: preview.weekly_window_start,
    expires_at: preview.expires_at
  })).data
}

export async function setAutoAdvanceWeek(id: number, enabled: boolean): Promise<UserSubscription> {
  return (await apiClient.put<UserSubscription>(`/subscriptions/${id}/auto-advance-week`, { enabled })).data
}

export interface MonthlyAdvancePreview {
  subscription_id: number
  monthly_window_start: string
  weekly_window_start: string | null
  expires_at: string
  new_expires_at: string
  deduct_seconds: number
}

export async function previewAdvanceMonth(id: number): Promise<MonthlyAdvancePreview> {
  return (await apiClient.get<MonthlyAdvancePreview>(`/subscriptions/${id}/advance-month`)).data
}

export async function advanceMonth(preview: MonthlyAdvancePreview): Promise<MonthlyAdvancePreview> {
  return (await apiClient.post<MonthlyAdvancePreview>(`/subscriptions/${preview.subscription_id}/advance-month`, {
    monthly_window_start: preview.monthly_window_start,
    weekly_window_start: preview.weekly_window_start,
    expires_at: preview.expires_at
  })).data
}
