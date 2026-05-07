export type InvoiceStatus = 'pending' | 'completed' | 'rejected'
export type InvoiceType = 'general' | 'vat_special'

export interface InvoiceProfile {
  id: number
  title: string
  tax_number: string
  email: string
  address?: string | null
  phone?: string | null
  bank_name?: string | null
  bank_account?: string | null
  invoice_type: InvoiceType
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface InvoiceProfilePayload {
  title: string
  tax_number: string
  email: string
  address?: string | null
  phone?: string | null
  bank_name?: string | null
  bank_account?: string | null
  invoice_type: InvoiceType
}

export interface InvoiceProfileSnapshot {
  title: string
  tax_number: string
  email: string
  address?: string | null
  phone?: string | null
  bank_name?: string | null
  bank_account?: string | null
  invoice_type?: InvoiceType
}

export interface InvoiceOrder {
  id: number
  out_trade_no: string
  pay_amount: number
  payment_type: string
  order_type: string
  status: string
  created_at: string
  completed_at?: string | null
}

export interface InvoiceRequest {
  id: number
  profile_id?: number | null
  serial_no: string
  status: InvoiceStatus
  profile_snapshot: InvoiceProfileSnapshot
  total_amount: number
  reject_reason?: string | null
  created_at: string
  updated_at: string
  completed_at?: string | null
  orders: InvoiceOrder[]

  invoice_no?: string | null
  invoice_file_name?: string | null
  invoice_file_size?: number | null
  invoice_file_mime?: string | null
  has_file?: boolean
  processed_by?: number | null
  processed_at?: string | null

  has_refunded_orders?: boolean
  voided_at?: string | null
  voided_reason?: string | null
}

export interface AdminInvoiceRequest extends InvoiceRequest {
  user_id: number
  username?: string
  user_email?: string
}

export interface AdminInvoiceListParams {
  page?: number
  page_size?: number
  status?: InvoiceStatus | ''
  keyword?: string
  user_id?: number
  start_time?: string
  end_time?: string
}

export interface InvoiceRequestPayload {
  profile_id: number
  order_ids: number[]
}

export interface InvoiceRequestListParams {
  page?: number
  page_size?: number
  status?: InvoiceStatus | ''
  start_time?: string
  end_time?: string
}

export interface InvoiceOrderListParams {
  page?: number
  page_size?: number
}
