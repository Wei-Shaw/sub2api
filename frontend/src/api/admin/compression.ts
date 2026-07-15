import { apiClient } from '../client'

export type CompressionMode = 'off' | 'observe' | 'enforce'

export interface CompressionPolicy {
  enabled: boolean
  mode: CompressionMode
  intensity: 'safe' | 'balanced' | 'aggressive'
  profile_version_id?: string
  filter_pack_version_id?: string
  rollout_percent: number
  holdout_percent: number
  allowed_protocols: string[]
  min_candidate_tokens: number
  min_savings_tokens: number
  max_body_bytes: number
  max_result_bytes: number
  max_duration_ms: number
  allow_request_override: boolean
  revision: number
  config_hash: string
}

export interface CompressionStatus {
  deployment_enabled: boolean
  mode: CompressionMode
  runtime: { emergency_stopped: boolean; revision: number; reason?: string }
  policy: CompressionPolicy
  engine_available: boolean
  telemetry: { recorded: number; dropped: number; applied: number; skipped: number; failed: number }
}

const compressionAPI = {
  async status(): Promise<CompressionStatus> {
    const { data } = await apiClient.get<CompressionStatus>('/admin/prompt-compression/status')
    return data
  },
  async update(payload: Partial<CompressionPolicy>): Promise<CompressionPolicy> {
    const { data } = await apiClient.put<CompressionPolicy>('/admin/prompt-compression/config', payload)
    return data
  },
  async emergencyStop(reason: string): Promise<CompressionStatus> {
    const { data } = await apiClient.post<CompressionStatus>('/admin/prompt-compression/emergency-stop', { reason })
    return data
  },
  async resume(reason: string): Promise<CompressionStatus> {
    const { data } = await apiClient.post<CompressionStatus>('/admin/prompt-compression/resume', { reason })
    return data
  },
  async preview(protocol: string, body: unknown): Promise<Record<string, unknown>> {
    const { data } = await apiClient.post<Record<string, unknown>>('/admin/prompt-compression/preview', { protocol, body })
    return data
  },
}

export default compressionAPI
