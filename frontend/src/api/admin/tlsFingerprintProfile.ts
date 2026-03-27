/**
 * Admin TLS Fingerprint Profile API endpoints
 * Handles TLS fingerprint profile CRUD for administrators
 */

import { apiClient REDACTED from '../client'

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
REDACTED

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
REDACTED

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
REDACTED

export async function list(): Promise<TLSFingerprintProfile[]> {
  const { data REDACTED = await apiClient.get<TLSFingerprintProfile[]>('/admin/tls-fingerprint-profiles')
  return data
REDACTED

export async function getById(id: number): Promise<TLSFingerprintProfile> {
  const { data REDACTED = await apiClient.get<TLSFingerprintProfile>(`/admin/tls-fingerprint-profiles/${idREDACTED`)
  return data
REDACTED

export async function create(profileData: CreateProfileRequest): Promise<TLSFingerprintProfile> {
  const { data REDACTED = await apiClient.post<TLSFingerprintProfile>('/admin/tls-fingerprint-profiles', profileData)
  return data
REDACTED

export async function update(id: number, updates: UpdateProfileRequest): Promise<TLSFingerprintProfile> {
  const { data REDACTED = await apiClient.put<TLSFingerprintProfile>(`/admin/tls-fingerprint-profiles/${idREDACTED`, updates)
  return data
REDACTED

export async function deleteProfile(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/tls-fingerprint-profiles/${idREDACTED`)
  return data
REDACTED

export const tlsFingerprintProfileAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteProfile
REDACTED

export default tlsFingerprintProfileAPI
