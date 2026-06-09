/**
 * Admin Email Broadcasts API endpoints.
 *
 * Bulk announcement emails composed and sent by administrators from the admin
 * panel. Backed by POST /api/v1/admin/email-broadcasts on the server.
 */

import { apiClient } from '../client'

export type EmailBroadcastBodyFormat = 'html' | 'text'
export type EmailBroadcastRecipientsMode = 'all' | 'selected'
export type EmailBroadcastStatus = 'pending' | 'sending' | 'completed' | 'failed'

export interface EmailBroadcast {
  id: number
  subject: string
  body: string
  body_format: EmailBroadcastBodyFormat
  recipients_mode: EmailBroadcastRecipientsMode
  recipient_user_ids?: number[]
  status: EmailBroadcastStatus
  total_count: number
  success_count: number
  failed_count: number
  error_message?: string
  created_by?: number
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

export interface EmailBroadcastSummary {
  id: number
  subject: string
  body_format: EmailBroadcastBodyFormat
  recipients_mode: EmailBroadcastRecipientsMode
  status: EmailBroadcastStatus
  total_count: number
  success_count: number
  failed_count: number
  created_by?: number
  started_at?: string
  finished_at?: string
  created_at: string
}

export interface EmailBroadcastListResult {
  items: EmailBroadcastSummary[]
  total: number
  page: number
  page_size: number
}

export interface CreateEmailBroadcastRequest {
  subject: string
  body: string
  body_format: EmailBroadcastBodyFormat
  recipients_mode: EmailBroadcastRecipientsMode
  recipient_user_ids?: number[]
}

export interface EmailBroadcastRecipientCandidate {
  id: number
  email: string
  username?: string
}

export interface SearchRecipientsResult {
  items: EmailBroadcastRecipientCandidate[]
}

async function list(
  page: number = 1,
  pageSize: number = 20,
  status?: EmailBroadcastStatus
): Promise<EmailBroadcastListResult> {
  const params: Record<string, string | number> = {
    page,
    page_size: pageSize
  }
  if (status) params.status = status
  const { data } = await apiClient.get<EmailBroadcastListResult>('/admin/email-broadcasts', { params })
  return data
}

async function getById(id: number): Promise<EmailBroadcast> {
  const { data } = await apiClient.get<EmailBroadcast>(`/admin/email-broadcasts/${id}`)
  return data
}

async function create(request: CreateEmailBroadcastRequest): Promise<EmailBroadcast> {
  const { data } = await apiClient.post<EmailBroadcast>('/admin/email-broadcasts', request)
  return data
}

async function searchRecipients(query: string, limit: number = 20): Promise<SearchRecipientsResult> {
  const { data } = await apiClient.get<SearchRecipientsResult>('/admin/email-broadcasts/recipients/search', {
    params: { q: query, limit }
  })
  return data
}

export interface PreviewBroadcastRequest {
  subject: string
  body: string
  body_format: EmailBroadcastBodyFormat
}

export interface PreviewBroadcastResult {
  html: string
}

async function preview(
  request: PreviewBroadcastRequest,
  options?: { signal?: AbortSignal }
): Promise<PreviewBroadcastResult> {
  const { data } = await apiClient.post<PreviewBroadcastResult>('/admin/email-broadcasts/preview', request, {
    signal: options?.signal
  })
  return data
}

async function deleteBroadcast(id: number): Promise<void> {
  await apiClient.delete(`/admin/email-broadcasts/${id}`)
}

const emailBroadcastsAPI = {
  list,
  getById,
  create,
  searchRecipients,
  preview,
  delete: deleteBroadcast
}

export default emailBroadcastsAPI
