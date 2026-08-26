import { apiClient } from './client'

export interface TicketMessage {
  id: number
  ticket_id: number
  sender_type: 'user' | 'admin'
  sender_id: number
  content: string
  images: string[]
  created_at: string
}

export interface Ticket {
  id: number
  user_email?: string
  user_name?: string
  title: string
  description: string
  status: 'pending' | 'processing' | 'closed'
  created_at: string
  updated_at: string
  messages?: TicketMessage[]
}

export const ticketsAPI = {
  async list() { const { data } = await apiClient.get<Ticket[]>('/tickets'); return data },
  async create(payload: { title: string; description: string; images?: string[] }) { const { data } = await apiClient.post<Ticket>('/tickets', payload); return data },
  async get(id: number) { const { data } = await apiClient.get<Ticket>(`/tickets/${id}`); return data },
  async reply(id: number, payload: { content: string; images?: string[] }) { const { data } = await apiClient.post<TicketMessage>(`/tickets/${id}/messages`, payload); return data },
  async close(id: number) { const { data } = await apiClient.post<{ status: string }>(`/tickets/${id}/close`); return data },
}

export const adminTicketsAPI = {
  async list() { const { data } = await apiClient.get<Ticket[]>('/admin/tickets'); return data },
  async get(id: number) { const { data } = await apiClient.get<Ticket>(`/admin/tickets/${id}`); return data },
  async reply(id: number, payload: { content: string; images?: string[] }) { const { data } = await apiClient.post<TicketMessage>(`/admin/tickets/${id}/messages`, payload); return data },
  async close(id: number) { const { data } = await apiClient.post<{ status: string }>(`/admin/tickets/${id}/close`); return data },
}

export default ticketsAPI
