import { apiClient } from '../client'
import type { AccountCapacityData } from '@/types'
export interface QualityPolicy {
  enabled: boolean
  interval_minutes: number
  model: string
  effort: string
  service_tier: string
  daily_budget: number
  max_accounts: number
  score_weights: {
    quality: number
    ttft: number
    cache: number
    stability: number
  }
  mode: 'manual' | 'observe' | 'auto'
  endpoint: string
  trace_on_anomaly: boolean
  reference_model: string
  context_check: boolean
  context_chars: number
}
export interface QualityGroup {
  id: number
  name: string
  platform: string
  total: number
  available: number
  policy: QualityPolicy
  next_run_at: string | null
}
export interface QualityAccount {
  capacity?: AccountCapacityData | null
  id: number
  name: string
  platform: string
  type: string
  status: string
  schedulable: boolean
  priority: number
  concurrency: number
  failures: number
  unknown_failure_scope: number
  reliability_known: boolean
  success_rate: number | null
  trace_status: string
  trace_at: string | null
  trace_candidate: string
  needs_review: boolean
  effective_multiplier: number | null
  components: {
    key: string
    score: number | null
    weight: number
    effective_weight: number
    samples: number
    reason: string
  }[]
  requests: number
  ttft_samples: number
  p50_ms: number | null
  p95_ms: number | null
  cache_rate: number | null
  last_request_at: string | null
  candy_status: string
  quality_samples: number
  quality_passed: number
  last_test_at: string | null
  score: number | null
  suggested_weight: number | null
}
export interface QualityPage {
  summary: { scored: number; pending: number; review: number }
  items: QualityAccount[]
  total: number
  page: number
  page_size: number
  window_hours: number
  model: string
  mode: string
}
export type QualityBenchmark = 'candy' | 'modeltrace' | 'context'
export interface QualityCatalog {
  reference_models: string[]
  bank_revision: string
  benchmarks: string[]
}
export interface QualityRun {
  benchmark: QualityBenchmark
  id: number
  status: string
  planned: number
  completed: number
  created_at: string
  finished_at: string | null
  cancel_requested: boolean
}
export interface QualityResult {
  endpoint: string
  requests_used: number
  evidence?: {
    bank_revision?: string
    expected_model?: string
    prompt?: string
    expected_answer?: string
    notice?: string
    attribution?: {
      prediction: string
      probability: number
      used_outputs: number
      results: { model: string; probability: number }[]
    }
  }
  id: number
  run_id: number
  account_id: number
  model: string
  effort: string
  service_tier: string
  benchmark: string
  verdict: string
  answer: string
  detail: string
  duration_ms: number
  created_at: string
}
const base = '/admin/accounts/quality'
export async function listQualityGroups() {
  return (await apiClient.get<QualityGroup[]>(base + '/groups')).data
}
export async function qualityPage(
  id: number,
  params: Record<string, unknown>,
  signal?: AbortSignal
) {
  return (
    await apiClient.get<QualityPage>(base + '/groups/' + id, { params, signal })
  ).data
}
export async function saveQualityPolicy(id: number, policy: QualityPolicy) {
  return (
    await apiClient.put<QualityPolicy>(
      base + '/groups/' + id + '/policy',
      policy
    )
  ).data
}
export async function runQuality(
  id: number,
  accountId = 0,
  benchmark: QualityBenchmark = 'candy'
) {
  return (
    await apiClient.post<QualityRun>(base + '/groups/' + id + '/run', {
      account_id: accountId,
      benchmark
    })
  ).data
}
export async function qualityRuns(id: number) {
  return (await apiClient.get<QualityRun[]>(base + '/groups/' + id + '/runs'))
    .data
}
export async function qualityResults(id: number, accountId = 0) {
  return (
    await apiClient.get<QualityResult[]>(base + '/groups/' + id + '/results', {
      params: { account_id: accountId }
    })
  ).data
}
export async function cancelQuality(id: number) {
  await apiClient.post(base + '/runs/' + id + '/cancel')
}

export async function qualityCatalog() {
  return (await apiClient.get<QualityCatalog>(base + '/catalog')).data
}

export interface QualityRoutingPreview {
  configured_mode: string
  applied: boolean
  reason: string
  snapshot_at?: string
  weights: Record<
    string,
    { multiplier: number; confirmed_bad: boolean; score: number | null }
  >
}
export interface QualityRoutingEvent {
  group_id: number
  account_id: number
  request_id: string
  scope: {
    model: string
    effort: string
    service_tier: string
    endpoint: string
  }
  multiplier: number
  reason: string
  created_at: string
}
export async function qualityRoutingPreview(g: QualityGroup) {
  return (
    await apiClient.get<QualityRoutingPreview>(
      base + '/groups/' + g.id + '/routing',
      {
        params: {
          model: g.policy.model,
          effort: g.policy.effort,
          service_tier: g.policy.service_tier,
          endpoint: g.policy.endpoint
        }
      }
    )
  ).data
}
export async function qualityRoutingEvents(g: number) {
  return (
    await apiClient.get<QualityRoutingEvent[]>(
      base + '/groups/' + g + '/routing-events'
    )
  ).data
}
