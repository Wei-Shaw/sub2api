import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'
import type { AdminInvoiceListParams, AdminInvoiceRequest, InvoiceRequest } from '@/types/invoice'

export const adminInvoiceAPI = {
  list(params?: AdminInvoiceListParams) {
    return apiClient.get<BasePaginationResponse<AdminInvoiceRequest>>('/admin/payment/invoices', { params })
  },

  get(id: number) {
    return apiClient.get<AdminInvoiceRequest>(`/admin/payment/invoices/${id}`)
  },

  reject(id: number, reason: string) {
    return apiClient.post<InvoiceRequest>(`/admin/payment/invoices/${id}/reject`, { reason })
  },

  complete(id: number, invoiceNo: string, file: File) {
    const form = new FormData()
    form.append('invoice_no', invoiceNo)
    form.append('file', file)
    return apiClient.post<InvoiceRequest>(`/admin/payment/invoices/${id}/complete`, form, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  }
}
