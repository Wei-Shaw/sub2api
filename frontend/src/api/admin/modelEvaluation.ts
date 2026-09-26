import { apiClient } from '../client'

export interface EvaluationTarget {
  id: number
  name: string
  type: 'account' | 'group'
}

export interface EvaluationRequest {
  target_type: EvaluationTarget['type']
  target_id: number
  model: string
  effort: string
}

export interface EvaluationResult {
  id: string
  started_at: string
  benchmark: string
  target_type: EvaluationTarget['type']
  target_id: number
  target_name: string
  account_id: number
  account_name: string
  requested_model: string
  upstream_model: string
  reported_model: string
  requested_effort: string
  effective_effort: string
  endpoint: string
  status: 'correct' | 'incorrect' | 'ungraded' | 'error' | 'running' | 'interrupted'
  answer: number | null
  text: string
  input_tokens: number | null
  output_tokens: number | null
  reasoning_tokens: number | null
  cached_tokens: number | null
  duration_ms: number
  request_id: string
  error?: string
}

export interface EvaluationRound {
  round: number
  result?: EvaluationResult
  saved: boolean
  save_error?: string
  error?: string
}

export interface EvaluationReport {
  id: string
  target_type: EvaluationTarget['type']
  target_id: number
  target_name: string
  model: string
  effort: string
  rounds: number
  benchmark: string
  prompt?: string
  visibility: 'private' | 'public'
  created_at: string
  completed: number
  correct: number
  graded: number
  results?: EvaluationRound[]
}

export interface EvaluationHistory {
  items: EvaluationReport[]
  total: number
  page: number
  page_size: number
}

export async function createEvaluation(request: EvaluationRequest & { rounds: number }, signal: AbortSignal) {
  const { data } = await apiClient.post<EvaluationReport>('/admin/model-evaluation/reports', request, { signal })
  return data
}

export async function listEvaluations(target: EvaluationTarget, page: number, signal: AbortSignal) {
  const { data } = await apiClient.get<EvaluationHistory>('/admin/model-evaluation/reports', {
    params: { target_type: target.type, target_id: target.id, page }, signal
  })
  return data
}

export async function getEvaluation(id: string, signal: AbortSignal) {
  const { data } = await apiClient.get<EvaluationReport>(`/admin/model-evaluation/reports/${id}`, { signal })
  return data
}

export async function runEvaluation(id: string, round: number, signal: AbortSignal) {
  const { data } = await apiClient.post<EvaluationRound>(`/admin/model-evaluation/reports/${id}/rounds`, { round }, {
    signal,
    timeout: 190000
  })
  return data
}
