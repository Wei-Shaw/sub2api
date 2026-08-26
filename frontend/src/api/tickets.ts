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

export interface TicketPage {
  items: Ticket[]
  total: number
  page: number
  page_size: number
  pages: number
}

type TicketListParams = { page?: number; page_size?: number }

export const ticketsAPI = {
  async list(params?: TicketListParams) { const { data } = await apiClient.get<TicketPage>('/tickets', { params }); return data },
  async create(payload: { title: string; description: string; images?: string[] }) { const { data } = await apiClient.post<Ticket>('/tickets', payload); return data },
  async get(id: number) { const { data } = await apiClient.get<Ticket>(`/tickets/${id}`); return data },
  async reply(id: number, payload: { content: string; images?: string[] }) { const { data } = await apiClient.post<TicketMessage>(`/tickets/${id}/messages`, payload); return data },
  async close(id: number) { const { data } = await apiClient.post<{ status: string }>(`/tickets/${id}/close`); return data },
}

export const adminTicketsAPI = {
  async list(params?: TicketListParams) { const { data } = await apiClient.get<TicketPage>('/admin/tickets', { params }); return data },
  async get(id: number) { const { data } = await apiClient.get<Ticket>(`/admin/tickets/${id}`); return data },
  async reply(id: number, payload: { content: string; images?: string[] }) { const { data } = await apiClient.post<TicketMessage>(`/admin/tickets/${id}/messages`, payload); return data },
  async close(id: number) { const { data } = await apiClient.post<{ status: string }>(`/admin/tickets/${id}/close`); return data },
  async remove(id: number) { const { data } = await apiClient.delete<{ status: string }>(`/admin/tickets/${id}`); return data },
}

export default ticketsAPI
