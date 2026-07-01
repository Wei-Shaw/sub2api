/**
 * Admin TLS Fingerprint Profile API endpoints
 * Handles TLS fingerprint profile CRUD for administrators
 */

import { apiClient } from '../client'

/**
 * TLS fingerprint profile interface
 */
export interface TLSFingerprintProfile {
  id: number
  name: string
  description: string | null
  enable_grease: boolean
  cipher_suites: number[]
  curves: number[]
  point_formats: number[]
  signature_algorithms: number[]
  alpn_protocols: string[]
  supported_versions: number[]
  key_share_groups: number[]
  psk_modes: number[]
  extensions: number[]
  created_at: string
  updated_at: string
}

/**
 * Create profile request
 */
export interface CreateProfileRequest {
  name: string
  description?: string | null
  enable_grease?: boolean
  cipher_suites?: number[]
  curves?: number[]
  point_formats?: number[]
  signature_algorithms?: number[]
  alpn_protocols?: string[]
  supported_versions?: number[]
  key_share_groups?: number[]
  psk_modes?: number[]
  extensions?: number[]
}

/**
 * Update profile request
 */
export interface UpdateProfileRequest {
  name?: string
  description?: string | null
  enable_grease?: boolean
  cipher_suites?: number[]
  curves?: number[]
  point_formats?: number[]
  signature_algorithms?: number[]
  alpn_protocols?: string[]
  supported_versions?: number[]
  key_share_groups?: number[]
  psk_modes?: number[]
  extensions?: number[]
}

export interface TLSFingerprintCollectorStatus {
  running: boolean
  listen_address: string
  public_base_url: string
  using_generated_cert: boolean
  ca_pem?: string
  session_ttl_seconds: number
  max_records_per_session: number
  started_at?: string
  last_error?: string
}

export interface TLSFingerprintCollectorSession {
  token: string
  expires_at: string
  capture_url: string
  ca_pem?: string
}

export interface TLSFingerprintCaptureRecord {
  id: string
  captured_at: string
  client_kind: string
  request_path: string
  method: string
  user_agent: string
  ja3_raw: string
  ja3_hash: string
  negotiated_alpn: string
  http_proto: string
  profile: TLSFingerprintProfile
  yaml: string
  headers_summary: Record<string, string>
  stainless_summary: Record<string, string>
}

export async function list(): Promise<TLSFingerprintProfile[]> {
  const { data } = await apiClient.get<TLSFingerprintProfile[]>('/admin/tls-fingerprint-profiles')
  return data
}

export async function getById(id: number): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.get<TLSFingerprintProfile>(`/admin/tls-fingerprint-profiles/${id}`)
  return data
}

export async function create(profileData: CreateProfileRequest): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.post<TLSFingerprintProfile>('/admin/tls-fingerprint-profiles', profileData)
  return data
}

export async function update(id: number, updates: UpdateProfileRequest): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.put<TLSFingerprintProfile>(`/admin/tls-fingerprint-profiles/${id}`, updates)
  return data
}

export async function deleteProfile(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/tls-fingerprint-profiles/${id}`)
  return data
}

export async function collectorStatus(): Promise<TLSFingerprintCollectorStatus> {
  const { data } = await apiClient.get<TLSFingerprintCollectorStatus>('/admin/tls-fingerprint-profiles/collector/status')
  return data
}

export async function startCollector(): Promise<TLSFingerprintCollectorStatus> {
  const { data } = await apiClient.post<TLSFingerprintCollectorStatus>('/admin/tls-fingerprint-profiles/collector/start')
  return data
}

export async function stopCollector(): Promise<TLSFingerprintCollectorStatus> {
  const { data } = await apiClient.post<TLSFingerprintCollectorStatus>('/admin/tls-fingerprint-profiles/collector/stop')
  return data
}

export async function createCollectorSession(): Promise<TLSFingerprintCollectorSession> {
  const { data } = await apiClient.post<TLSFingerprintCollectorSession>('/admin/tls-fingerprint-profiles/collector/sessions')
  return data
}

export async function listCollectorCaptures(token: string): Promise<TLSFingerprintCaptureRecord[]> {
  const { data } = await apiClient.get<TLSFingerprintCaptureRecord[]>(`/admin/tls-fingerprint-profiles/collector/sessions/${encodeURIComponent(token)}/captures`)
  return data
}

export async function deleteCollectorSession(token: string): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`/admin/tls-fingerprint-profiles/collector/sessions/${encodeURIComponent(token)}`)
  return data
}

export const tlsFingerprintProfileAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteProfile,
  collectorStatus,
  startCollector,
  stopCollector,
  createCollectorSession,
  listCollectorCaptures,
  deleteCollectorSession
}

export default tlsFingerprintProfileAPI
