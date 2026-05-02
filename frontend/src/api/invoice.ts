import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'
import type {
  InvoiceOrder,
  InvoiceOrderListParams,
  InvoiceProfile,
  InvoiceProfilePayload,
  InvoiceRequest,
  InvoiceRequestListParams,
  InvoiceRequestPayload
} from '@/types/invoice'

export const invoiceAPI = {
  listProfiles() {
    return apiClient.get<InvoiceProfile[]>('/invoice/profiles')
  },

  createProfile(data: InvoiceProfilePayload) {
    return apiClient.post<InvoiceProfile>('/invoice/profiles', data)
  },

  updateProfile(id: number, data: InvoiceProfilePayload) {
    return apiClient.put<InvoiceProfile>(`/invoice/profiles/${id}`, data)
  },

  deleteProfile(id: number) {
    return apiClient.delete(`/invoice/profiles/${id}`)
  },

  setDefaultProfile(id: number) {
    return apiClient.put<InvoiceProfile>(`/invoice/profiles/${id}/default`)
  },

  listInvoiceableOrders(params?: InvoiceOrderListParams) {
    return apiClient.get<BasePaginationResponse<InvoiceOrder>>('/invoice/orders', { params })
  },

  createRequest(data: InvoiceRequestPayload) {
    return apiClient.post<InvoiceRequest>('/invoice/requests', data)
  },

  listRequests(params?: InvoiceRequestListParams) {
    return apiClient.get<BasePaginationResponse<InvoiceRequest>>('/invoice/requests', { params })
  }
}
