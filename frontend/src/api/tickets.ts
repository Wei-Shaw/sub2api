import { apiClient } from './client'
import type { PaginatedResponse } from '@/types'

export type TicketStatus = 'open' | 'waiting_admin' | 'answered' | 'closed'

export interface TicketMessage {
  id: number
  ticket_id: number
  user_id: number
  user_email?: string
  user_name?: string
  is_admin: boolean
  content: string
  created_at: number
}

export interface Ticket {
  id: number
  user_id: number
  user_email?: string
  user_name?: string
  subject: string
  status: TicketStatus
  last_replied_by: 'user' | 'admin'
  created_at: number
  updated_at: number
  closed_at?: number | null
  message_count: number
  messages?: TicketMessage[]
}

export interface TicketListParams {
  page?: number
  page_size?: number
  status?: string
  search?: string
  sort_by?: string
  sort_order?: string
}

export async function listMyTickets(params: TicketListParams = {}) {
  const { data } = await apiClient.get<PaginatedResponse<Ticket>>('/tickets', { params })
  return data
}

export async function createTicket(payload: { subject: string; content: string }) {
  const { data } = await apiClient.post<Ticket>('/tickets', payload)
  return data
}

export async function getMyTicket(id: number) {
  const { data } = await apiClient.get<Ticket>(`/tickets/${id}`)
  return data
}

export async function replyMyTicket(id: number, content: string) {
  const { data } = await apiClient.post<Ticket>(`/tickets/${id}/messages`, { content })
  return data
}

export async function closeMyTicket(id: number) {
  const { data } = await apiClient.post<Ticket>(`/tickets/${id}/close`)
  return data
}

export async function listAdminTickets(params: TicketListParams = {}) {
  const { data } = await apiClient.get<PaginatedResponse<Ticket>>('/admin/tickets', { params })
  return data
}

export async function getAdminTicket(id: number) {
  const { data } = await apiClient.get<Ticket>(`/admin/tickets/${id}`)
  return data
}

export async function replyAdminTicket(id: number, content: string) {
  const { data } = await apiClient.post<Ticket>(`/admin/tickets/${id}/messages`, { content })
  return data
}

export async function closeAdminTicket(id: number) {
  const { data } = await apiClient.post<Ticket>(`/admin/tickets/${id}/close`)
  return data
}

export async function reopenAdminTicket(id: number) {
  const { data } = await apiClient.post<Ticket>(`/admin/tickets/${id}/reopen`)
  return data
}

const ticketsAPI = {
  listMyTickets,
  createTicket,
  getMyTicket,
  replyMyTicket,
  closeMyTicket,
  listAdminTickets,
  getAdminTicket,
  replyAdminTicket,
  closeAdminTicket,
  reopenAdminTicket
}

export default ticketsAPI
