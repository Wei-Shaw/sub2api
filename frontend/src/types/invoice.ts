export type InvoiceStatus = 'pending' | 'completed' | 'rejected'

export interface InvoiceProfile {
  id: number
  title: string
  tax_number: string
  email: string
  address?: string | null
  phone?: string | null
  bank_name?: string | null
  bank_account?: string | null
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
}

export interface InvoiceProfileSnapshot {
  title: string
  tax_number: string
  email: string
  address?: string | null
  phone?: string | null
  bank_name?: string | null
  bank_account?: string | null
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
