import { apiClient REDACTED from '@/api/client'

export interface AdminComplianceAcknowledgement {
  version: string
  document_zh: string
  document_en: string
  admin_user_id: number
  ip_address?: string
  user_agent?: string
  accepted_at: string
REDACTED

export interface AdminComplianceStatus {
  required: boolean
  version: string
  document_path_zh: string
  document_path_en: string
  document_url_zh: string
  document_url_en: string
  ack_phrase_zh: string
  ack_phrase_en: string
  acknowledgement?: AdminComplianceAcknowledgement
REDACTED

export interface AcceptAdminComplianceRequest {
  phrase: string
  language: string
REDACTED

export const adminComplianceAPI = {
  async getStatus(): Promise<AdminComplianceStatus> {
    const { data REDACTED = await apiClient.get<AdminComplianceStatus>('/admin/compliance')
    return data
  REDACTED,

  async accept(payload: AcceptAdminComplianceRequest): Promise<AdminComplianceStatus> {
    const { data REDACTED = await apiClient.post<AdminComplianceStatus>('/admin/compliance/accept', payload)
    return data
  REDACTED
REDACTED

export default adminComplianceAPI
