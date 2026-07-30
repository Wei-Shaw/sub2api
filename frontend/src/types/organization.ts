export type OrganizationRole = 'owner' | 'member'
export type OrganizationStatus = 'active' | 'suspended'
export type IAMMemberStatus = 'active' | 'disabled' | 'archived'

export interface OrganizationContext {
  organization_id: number
  account_id: string
  company_id: string
  owner_user_id: number
  company_name: string
  organization_status: OrganizationStatus
  membership_id: number
  role: OrganizationRole
  membership_status: IAMMemberStatus
  authz_generation: number
  policy_names: string[]
  actions: string[]
  effective_at: string
}

export interface CompanyApplication {
  id: number
  applicant_user_id: number
  applicant_email?: string
  requested_name: string
  company_size?: string
  status: 'pending' | 'approved' | 'rejected' | 'withdrawn'
  fee_amount: string
  fee_currency: string
  reviewer_user_id?: number
  review_reason?: string
  organization_id?: number
  similar_names: string[]
  created_at: string
  decided_at?: string
}

export interface CompanyUpgradeEligibility {
  eligible: boolean
  reason?: string
  fee_amount: string
  fee_currency: string
  application?: CompanyApplication
}

export interface OrganizationAuditEvent {
  id: number
  actor_user_id?: number
  subject_user_id?: number
  action: string
  result: string
  correlation_id?: string
  metadata: Record<string, unknown>
  created_at: string
}

export interface CompanyApplicationDetail {
  application: CompanyApplication
  audit: OrganizationAuditEvent[]
}

export interface OrganizationNameChangeRequest {
  id: number
  organization_id: number
  applicant_user_id: number
  company_name: string
  old_name: string
  new_name: string
  status: 'pending' | 'approved' | 'rejected' | 'withdrawn'
  reviewer_user_id?: number
  review_reason?: string
  similar_names: string[]
  created_at: string
  decided_at?: string
}

export interface AdminOrganization {
	id: number
	account_id: string
	company_id: string
	name: string
	status: OrganizationStatus
	owner_user_id: number
	owner_email?: string
	member_count: number
	member_limit: number
	effective_at: string
	created_at: string
}

export interface AdminOrganizationDetail {
	organization: AdminOrganization
	audit: OrganizationAuditEvent[]
}

export interface IAMMember {
  user_id: number
  external_user_id: string
  login_name: string
  principal: string
  status: IAMMemberStatus
  balance: string
  frozen_balance: string
  recovery_email?: string
  recovery_email_verified_at?: string
  must_change_password: boolean
  policy_names: string[]
  created_at: string
}

export interface ManagedPolicy {
  id: number
  key: string
  display_name: string
  type: 'system'
  description: string
  version: number
  actions: string[]
}

export interface FinanceSummary {
  balance_source: 'self' | 'allocated' | 'shared'
  available?: string
  frozen?: string
  total?: string
  company_available?: string
  company_frozen?: string
  company_total?: string
}

export interface OrganizationSubscription {
  id: number
  organization_id: number
  group_id: number
  group_name: string
  platform: string
  subscription_type: string
  starts_at: string
  expires_at: string
  status: 'active' | 'expired' | 'cancelled'
  daily_limit_usd?: string
  weekly_limit_usd?: string
  monthly_limit_usd?: string
  daily_usage_usd: string
  weekly_usage_usd: string
  monthly_usage_usd: string
  notes?: string
  assigned_by?: number
  assigned_at: string
  created_at: string
}

export interface OrganizationUsageRow {
  id: number
  member_user_id: number
  member_login: string
  api_key_name: string
  model: string
  input_tokens: number
  output_tokens: number
  actual_cost: string
  endpoint: string
  status: string
  duration_ms?: number
  created_at: string
  balance_source?: 'self' | 'allocated' | 'shared'
}

export interface OrganizationUsageParams {
  start?: string
  end?: string
  member_id?: number
  api_key_id?: number
  model?: string
  endpoint?: string
  status?: string
  page?: number
  page_size?: number
}

export interface PaginatedOrganizationUsage {
  items: OrganizationUsageRow[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface OrganizationUsageStats {
  requests: number
  input_tokens: number
  output_tokens: number
  actual_cost: string
}

export interface OrganizationUsageTrendPoint {
  bucket: string
  requests: number
  tokens: number
  actual_cost: string
}
