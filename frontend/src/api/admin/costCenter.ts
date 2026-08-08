import apiClient from '../client'

export interface CostCenterSummary {
  cash_income: number
  realized_income: number
  promotional_consumption: number
  upstream_cost: number
  settled_expenses: number
  pending_forecast: number
  cash_profit: number
  operating_profit: number
  profit_margin: number
  unknown_source_amount: number
  deferred_subscription_usd: number
  expired_entitlement_usd: number
}

export interface CostCenterEvent {
  id: number
  event_type: string
  status: string
  source_type: string
  source_id?: string | null
  account_id?: number | null
  category: string
  amount_usd: number
  occurred_at: string
  note: string
  platform?: string
  model?: string
  user_id?: number | null
  group_id?: number | null
  plan_id?: number | null
}

export interface CostCenterEventPage { items: CostCenterEvent[]; total: number; page: number; page_size: number; pages: number }

export interface CostCenterFilters { start: string; end: string; account_id?: number; user_id?: number; group_id?: number; plan_id?: number; category?: string; source_type?: string; platform?: string; model?: string }

const costCenterAPI = {
  getSummary(filters: CostCenterFilters) { return apiClient.get<CostCenterSummary>('/admin/cost-center/summary', { params: filters }) },
  getEvents(filters: CostCenterFilters & { page?: number; page_size?: number }) { return apiClient.get<CostCenterEventPage>('/admin/cost-center/events', { params: filters }) },
  createExpense(data: { amount_usd: number; category: string; account_id?: number; occurred_at?: string; note?: string; status?: string }) { return apiClient.post<CostCenterEvent>('/admin/cost-center/expenses', data) },
  updateEventStatus(id: number, status: string, reason: string) { return apiClient.patch<CostCenterEvent>(`/admin/cost-center/events/${id}/status`, { status, reason }) },
  reverseEvent(id: number, reason: string) { return apiClient.post<CostCenterEvent>(`/admin/cost-center/events/${id}/reverse`, { reason }) },
  reconcile(filters: CostCenterFilters) { return apiClient.get<{ unknown_events: number; pending_events: number; duplicate_keys: number }>('/admin/cost-center/reconciliation', { params: filters }) },
}

export default costCenterAPI
