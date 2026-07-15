import { apiClient } from '../client'

export type UpstreamSiteType = 'auto' | 'newapi' | 'sub2api'
export type UpstreamCredentialMode = 'password' | 'token' | 'api_key'
export type UpstreamPlatform = 'openai' | 'anthropic' | 'gemini' | 'grok'
export type UpstreamHealth = 'unknown' | 'healthy' | 'error'

export interface UpstreamCredentials {
  username?: string
  password?: string
  access_token?: string
  refresh_token?: string
  cookie?: string
  user_id?: string
  api_key?: string
}

export interface FixedRouteInput {
  remote_group_key: string
  remote_group_name: string
  platform: UpstreamPlatform
  models?: string[]
  group_rate: number
  schedulable?: boolean
}

export interface UpstreamStation {
  id: number
  name: string
  site_type: UpstreamSiteType
  base_url: string
  credential_mode: UpstreamCredentialMode
  credential_configured: boolean
  recharge_multiplier: number
  recharge_source: 'manual' | 'auto'
  balance?: number | null
  enabled: boolean
  auto_sync: boolean
  health_status: UpstreamHealth
  last_error?: string
  last_sync_at?: string | null
  last_test_at?: string | null
  created_at: string
  updated_at: string
}

export interface UpstreamRoute {
  id: number
  station_id: number
  remote_group_key: string
  remote_group_name: string
  platform: UpstreamPlatform
  models: string[]
  group_rate: number
  recharge_multiplier: number
  effective_rate: number
  fixed_route: boolean
  remote_api_key_id?: string
  managed_account_id?: number | null
  schedulable: boolean
  health_status: UpstreamHealth
  last_error?: string
  last_test_at?: string | null
  last_sync_at?: string | null
}

export interface UpstreamSyncLog {
  id: number
  station_id: number
  action: string
  success: boolean
  message?: string
  detail?: string
  created_at: string
}

export interface UpstreamSyncResult {
  station_id: number
  synced_routes: number
  created_keys: number
  errors: string[]
}

export interface UpstreamTestResult {
  site_type: string
  balance?: number
  group_count: number
  route_count: number
}

export interface CreateUpstreamStationParams {
  name: string
  site_type: UpstreamSiteType
  base_url: string
  credential_mode: UpstreamCredentialMode
  credentials: UpstreamCredentials
  recharge_multiplier: number
  recharge_source: 'manual' | 'auto'
  enabled: boolean
  auto_sync: boolean
  fixed_routes?: FixedRouteInput[]
}

export type UpdateUpstreamStationParams = Partial<Omit<CreateUpstreamStationParams, 'fixed_routes'>>

export interface UpdateUpstreamRouteParams {
  remote_group_name?: string
  models?: string[]
  group_rate?: number
  recharge_multiplier?: number
  schedulable?: boolean
}

async function list(): Promise<UpstreamStation[]> {
  const { data } = await apiClient.get<UpstreamStation[]>('/admin/upstream-stations')
  return data
}

async function get(id: number): Promise<UpstreamStation> {
  const { data } = await apiClient.get<UpstreamStation>(`/admin/upstream-stations/${id}`)
  return data
}

async function create(params: CreateUpstreamStationParams): Promise<UpstreamStation> {
  const { data } = await apiClient.post<UpstreamStation>('/admin/upstream-stations', params)
  return data
}

async function update(id: number, params: UpdateUpstreamStationParams): Promise<UpstreamStation> {
  const { data } = await apiClient.put<UpstreamStation>(`/admin/upstream-stations/${id}`, params)
  return data
}

async function del(id: number): Promise<void> {
  await apiClient.delete(`/admin/upstream-stations/${id}`)
}

async function test(id: number): Promise<UpstreamTestResult> {
  const { data } = await apiClient.post<UpstreamTestResult>(`/admin/upstream-stations/${id}/test`)
  return data
}

async function sync(id: number): Promise<UpstreamSyncResult> {
  const { data } = await apiClient.post<UpstreamSyncResult>(`/admin/upstream-stations/${id}/sync`)
  return data
}

async function syncAll(): Promise<UpstreamSyncResult[]> {
  const { data } = await apiClient.post<UpstreamSyncResult[]>('/admin/upstream-stations/sync-all')
  return data
}

async function listRoutes(stationId: number): Promise<UpstreamRoute[]> {
  const { data } = await apiClient.get<UpstreamRoute[]>(`/admin/upstream-stations/${stationId}/routes`)
  return data
}

async function createFixedRoute(stationId: number, params: FixedRouteInput): Promise<UpstreamRoute> {
  const { data } = await apiClient.post<UpstreamRoute>(`/admin/upstream-stations/${stationId}/routes`, params)
  return data
}

async function updateRoute(id: number, params: UpdateUpstreamRouteParams): Promise<UpstreamRoute> {
  const { data } = await apiClient.put<UpstreamRoute>(`/admin/upstream-routes/${id}`, params)
  return data
}

async function testRoute(id: number): Promise<UpstreamSyncResult> {
  const { data } = await apiClient.post<UpstreamSyncResult>(`/admin/upstream-routes/${id}/test`)
  return data
}

async function setRouteSchedulable(id: number, schedulable: boolean): Promise<void> {
  await apiClient.post(`/admin/upstream-routes/${id}/schedulable`, { schedulable })
}

async function listLogs(stationId: number, limit = 50): Promise<UpstreamSyncLog[]> {
  const { data } = await apiClient.get<UpstreamSyncLog[]>(`/admin/upstream-stations/${stationId}/logs`, {
    params: { limit },
  })
  return data
}

export const upstreamStationsAPI = {
  list,
  get,
  create,
  update,
  del,
  test,
  sync,
  syncAll,
  listRoutes,
  createFixedRoute,
  updateRoute,
  testRoute,
  setRouteSchedulable,
  listLogs,
}

export default upstreamStationsAPI
