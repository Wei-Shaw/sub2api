import { apiClient } from '../client'

export type VideoProvider = 'mock' | 'seedance' | 'kling'
export type VideoTaskType = 'text_to_video' | 'image_to_video' | 'reference_to_video'
export type VideoTaskStatus = 'queued' | 'submitted' | 'running' | 'succeeded' | 'failed' | 'cancelled'
export type VideoDeliveryStatus = 'processing' | 'archiving' | 'deliverable' | 'delivery_failed'
export type VideoTaskNextAction =
  | 'poll'
  | 'archive'
  | 'download'
  | 'review_delivery'
  | 'reconcile_dispatch'
  | 'review_settlement'
  | 'none'

export interface VideoProviderAccount {
  id: number
  provider: VideoProvider
  display_name: string
  enabled: boolean
  api_key_configured: boolean
  masked_key: string
  base_url: string
  default_model: string
  rate_limit_per_minute: number
  metadata_json: Record<string, unknown>
  key_status: string
  health_status: string
  diagnostic_type: string
  suggested_action: string
  priority: number
  current_inflight: number
  today_tasks: number
  today_failures: number
  last_error: string
  last_test_at: string
  route_available: boolean
  route_skip_reason: string
  created_at: string
  updated_at: string
}

export interface VideoProviderPayload {
  provider?: VideoProvider
  display_name?: string
  enabled?: boolean
  api_key?: string
  base_url?: string
  default_model?: string
  rate_limit_per_minute?: number
  metadata_json?: Record<string, unknown>
}

export interface VideoProviderTestResult {
  provider: VideoProvider
  configured: boolean
  reachable: boolean
  message: string
  normalized_status: string
  payload_preview?: Record<string, unknown>
}

export interface VideoTaskSummary {
  id: number
  provider_account_id: number
  provider_account_name: string
  provider: VideoProvider
  model: string
  task_type: VideoTaskType
  prompt: string
  status: VideoTaskStatus
  result_url: string
  result_url_expires_at?: string | null
  result_url_expiry_source?: 'url_query' | 'estimated' | 'unknown' | string
  local_asset_path?: string
  local_asset_saved_at?: string | null
  local_asset_available?: boolean
  dispatch_state?: string
  settlement_status?: string
  archive_status?: string
  capture_status?: string
  delivery_status?: VideoDeliveryStatus
  next_action?: VideoTaskNextAction | string
  error_message: string
  cost_estimate: number
  currency?: string // 'CNY' = Seedance 真实计费；'USD' = 其他（旧后端可能缺失）
  created_by: number
  created_by_email: string
  created_by_name: string
  created_by_label: string
  created_at: string
  updated_at: string
  completed_at: string | null
}

export interface VideoProviderStatus {
  provider: VideoProvider
  display_name: string
  enabled: boolean
  api_key_configured: boolean
  masked_key: string
  default_model: string
  key_status: string
  health_status: string
  diagnostic_type: string
  suggested_action: string
  route_available: boolean
  route_skip_reason: string
  priority: number
  current_inflight: number
  last_error: string
  last_test_at: string
  updated_at: string
  today_tasks: number
  running_tasks: number
  failed_tasks: number
}

export interface VideoHealthDiagnostic {
  provider: VideoProvider
  display_name: string
  route_account: string
  key_status: string
  last_test_at: string
  exception_type: string
  impact_tasks: number
  recent_error: string
  suggested_action: string
  status: string
}

export interface VideoUsageSummary {
  provider: VideoProvider
  model: string
  status: VideoTaskStatus
  count: number
  cost_estimate: number
  duration: number
}

export interface VideoDashboard {
  today_tasks: number
  success_rate: number
  failed_tasks: number
  queued_tasks: number
  running_tasks: number
  provider_status: VideoProviderStatus[]
  health_diagnostics: VideoHealthDiagnostic[]
  recent_failures: VideoTaskSummary[]
  recent_successes: VideoTaskSummary[]
  usage_overview: VideoUsageSummary[]
}

export interface VideoTaskEvent {
  id: number
  video_task_id: number
  event_type: string
  message: string
  payload_json: Record<string, unknown>
  created_at: string
}

export interface VideoTask extends VideoTaskSummary {
  negative_prompt: string
  reference_image_url: string
  reference_video_url: string
  aspect_ratio: string
  duration: number
  resolution: string
  upstream_task_id: string
  routing_strategy: string
  routing_reason: string
  events?: VideoTaskEvent[]
}

export interface VideoTaskCreatePayload {
  provider_account_id?: number
  task_type: VideoTaskType
  model?: string
  prompt: string
  negative_prompt?: string
  reference_image_url?: string
  reference_video_url?: string
  aspect_ratio?: string
  duration?: number
  resolution?: string
}

export interface VideoTaskListParams {
  page?: number
  page_size?: number
  status?: VideoTaskStatus | ''
  provider?: VideoProvider | ''
}

export interface VideoTaskListResponse {
  items: VideoTask[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface DramaEngineProfile {
  id: string
  provider_code: string
  engine_family: string
  model_name: string
  model_version: string
  mode: string
  best_for_scene_types: string[]
  weak_for_scene_types: string[]
  credential_required: boolean
  real_provider_verified: boolean
  internal_safe_mode_only: boolean
  source_note: string
}

export interface DramaProviderPoolItem {
  provider_account_status: string
  provider_code: string
  display_name: string
  credential_configured: boolean
  real_provider_verified: boolean
  fallback_ready: boolean
  suitable_scene_types: string[]
  unsuitable_scene_types: string[]
  supported_modes: string[]
  routing_reason: string
  skipped_reason: string
  current_inflight: number
  failure_count_today: number
  safe_demo_mode: boolean
}

export interface DramaSkillCard {
  employee_alias: string
  scope_type: string
  drama_types: string[]
  scene_types: string[]
  engine_modes: string[]
  total_tasks: number
  approval_status: string
  version: string
}

export interface DramaSkillAnalysisExport {
  id: string
  export_type: string
  target_ai_model: string
  anonymized: boolean
  schema_version: string
  export_json: Record<string, unknown>
  analysis_prompt: string
  analysis_prompts: Record<string, string>
  created_at: string
}

async function listProviders(): Promise<{ items: VideoProviderAccount[] }> {
  const { data } = await apiClient.get<{ items: VideoProviderAccount[] }>('/admin/video/providers')
  return data
}

async function createProvider(payload: VideoProviderPayload): Promise<VideoProviderAccount> {
  const { data } = await apiClient.post<VideoProviderAccount>('/admin/video/providers', payload)
  return data
}

async function updateProvider(id: number, payload: VideoProviderPayload): Promise<VideoProviderAccount> {
  const { data } = await apiClient.patch<VideoProviderAccount>(`/admin/video/providers/${id}`, payload)
  return data
}

async function testProvider(id: number): Promise<VideoProviderTestResult> {
  const { data } = await apiClient.post<VideoProviderTestResult>(`/admin/video/providers/${id}/test`)
  return data
}

async function dashboard(): Promise<VideoDashboard> {
  const { data } = await apiClient.get<VideoDashboard>('/admin/video/dashboard')
  return data
}

async function engineCapabilityMatrix(): Promise<{ items: DramaEngineProfile[] }> {
  const { data } = await apiClient.get<{ items: DramaEngineProfile[] }>('/admin/engine-capability-matrix')
  return data
}

async function providerPool(): Promise<{ items: DramaProviderPoolItem[] }> {
  const { data } = await apiClient.get<{ items: DramaProviderPoolItem[] }>('/admin/provider-pool')
  return data
}

async function routingEvents(): Promise<{ items: Record<string, unknown>[] }> {
  const { data } = await apiClient.get<{ items: Record<string, unknown>[] }>('/admin/routing-events')
  return data
}

async function skillCards(): Promise<{ items: DramaSkillCard[] }> {
  const { data } = await apiClient.get<{ items: DramaSkillCard[] }>('/admin/skill-cards')
  return data
}

async function createSkillAnalysisExport(payload: { target_ai_model: 'gemini' | 'gpt' | 'kimi' | 'all' }): Promise<DramaSkillAnalysisExport> {
  const { data } = await apiClient.post<DramaSkillAnalysisExport>('/admin/skill-analysis/exports', payload)
  return data
}

async function listTaskProviders(): Promise<{ items: VideoProviderAccount[] }> {
  const { data } = await apiClient.get<{ items: VideoProviderAccount[] }>('/video/providers')
  return data
}

async function listTasks(params: VideoTaskListParams = {}, signal?: AbortSignal): Promise<VideoTaskListResponse> {
  const { data } = await apiClient.get<VideoTaskListResponse>('/video/tasks', { params, signal })
  return data
}

async function createTask(payload: VideoTaskCreatePayload): Promise<VideoTask> {
  const { data } = await apiClient.post<VideoTask>('/video/tasks', payload)
  return data
}

async function getTask(id: number, signal?: AbortSignal): Promise<VideoTask> {
  const { data } = await apiClient.get<VideoTask>(`/video/tasks/${id}`, { signal })
  return data
}

async function cancelTask(id: number): Promise<VideoTask> {
  const { data } = await apiClient.post<VideoTask>(`/video/tasks/${id}/cancel`)
  return data
}

async function getLocalAssetBlob(taskId: number): Promise<Blob> {
  const { data } = await apiClient.get<Blob>(`/video/tasks/${taskId}/local-asset`, {
    responseType: 'blob',
  })
  return data
}

/** Authenticated local archive preview/download path (relative to API base). */
export function localAssetURL(taskId: number): string {
  return `/video/tasks/${taskId}/local-asset`
}

export const videoAdminAPI = {
  listProviders,
  createProvider,
  updateProvider,
  testProvider,
  dashboard,
  engineCapabilityMatrix,
  providerPool,
  routingEvents,
  skillCards,
  createSkillAnalysisExport,
}

export const videoTaskAPI = {
  listProviders: listTaskProviders,
  list: listTasks,
  create: createTask,
  get: getTask,
  cancel: cancelTask,
  localAssetURL,
  getLocalAssetBlob,
}

export default videoAdminAPI
