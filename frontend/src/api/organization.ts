import { apiClient } from './client'
import type { CreateOrderResult } from '@/types/payment'
import type {
  AuthResponse,
  CompanyApplication,
  CompanyApplicationDetail,
  CompanyUpgradeEligibility,
	DashboardStats,
	UserBreakdownItem,
	UserSpendingRankingResponse,
	UserUsageTrendPoint,
	AdminOrganization,
	AdminOrganizationDetail,
  EndpointStat,
  FinanceSummary,
  Group,
  IAMMember,
  ManagedPolicy,
  OrganizationContext,
  OrganizationNameChangeRequest,
  OrganizationSubscription,
  OrganizationSpendLimitRule,
  OrganizationSpendUsage,
  OrganizationUsageParams,
  OrganizationUsageStats,
  OrganizationUsageTrendPoint,
  GroupStat,
  ModelStat,
  PaginatedOrganizationUsage,
  UserErrorListParams,
  UserErrorRequest,
  UserErrorRequestDetail,
} from '@/types'

function randomIdempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

export interface IAMLoginRequest {
  principal: string
  password: string
  turnstile_token?: string
  tencent_captcha_ticket?: string
  tencent_captcha_randstr?: string
}

export interface IAMLoginResponse extends AuthResponse {
  organization: OrganizationContext
}

export const organizationAPI = {
  async loginIAM(payload: IAMLoginRequest): Promise<IAMLoginResponse> {
    const { data } = await apiClient.post<IAMLoginResponse>('/auth/iam/login', payload)
    return data
  },
  async getContext(): Promise<{ organization: OrganizationContext; finance?: FinanceSummary }> {
    const { data } = await apiClient.get('/organization/context')
    return data
  },
  async getCurrentApplication(): Promise<CompanyApplication | null> {
    const { data } = await apiClient.get<{ application: CompanyApplication | null }>('/organization/applications/current')
    return data.application
  },
  async getUpgradeEligibility(): Promise<CompanyUpgradeEligibility> {
    const { data } = await apiClient.get<CompanyUpgradeEligibility>('/organization/applications/eligibility')
    return data
  },
  async submitApplication(companyName: string, companySize: string, idempotencyKey: string): Promise<CompanyApplication> {
    const { data } = await apiClient.post('/organization/applications', { company_name: companyName, company_size: companySize, idempotency_key: idempotencyKey })
    return data
  },
  async withdrawApplication(id: number): Promise<CompanyApplication> {
    const { data } = await apiClient.post(`/organization/applications/${id}/withdraw`)
    return data
  },
	async requestNameChange(companyName: string): Promise<void> {
		await apiClient.post('/organization/name-change-requests', { company_name: companyName })
	},
  async listMembers(): Promise<{ items: IAMMember[]; member_limit: number; used_slots: number }> {
    const { data } = await apiClient.get('/organization/members')
    return data
  },
  async getMember(id: number): Promise<IAMMember> {
    const { data } = await apiClient.get<IAMMember>(`/organization/members/${id}`)
    return data
  },
  async createMember(loginName: string, password: string, mustChangePassword = true, recoveryEmail?: string, username?: string): Promise<{ member: IAMMember; initial_password: string }> {
    const { data } = await apiClient.post('/organization/members', {
      login_name: loginName,
      password,
      must_change_password: mustChangePassword,
      recovery_email: recoveryEmail,
      username,
    })
    return data
  },
  async setMemberStatus(id: number, status: IAMMember['status']): Promise<void> {
    await apiClient.patch(`/organization/members/${id}/status`, { status })
  },
  async resetMemberPassword(id: number): Promise<{ initial_password: string }> {
    const { data } = await apiClient.post(`/organization/members/${id}/reset-password`)
    return data
  },
  async changePassword(newPassword: string): Promise<AuthResponse> {
	const { data } = await apiClient.put<AuthResponse>('/organization/password', { new_password: newPassword })
	return data
  },
	async sendRecoveryEmailCode(email: string): Promise<void> {
		await apiClient.post('/organization/recovery-email/send-code', { email })
	},
	async verifyRecoveryEmail(email: string, code: string): Promise<void> {
		await apiClient.post('/organization/recovery-email/verify', { email, code })
	},
  async listPolicies(): Promise<ManagedPolicy[]> {
    const { data } = await apiClient.get<{ items: ManagedPolicy[] }>('/organization/policies')
    return data.items
  },
  async listMemberPolicies(memberID: number): Promise<ManagedPolicy[]> {
    const { data } = await apiClient.get<{ items: ManagedPolicy[] }>(`/organization/members/${memberID}/policies`)
    return data.items
  },
  async setPolicy(memberID: number, policyKey: string, attached: boolean): Promise<void> {
    await apiClient.put(`/organization/members/${memberID}/policies`, { policy_key: policyKey, attached })
  },
  async transferBalance(memberID: number, amount: string, operation: 'allocate' | 'reclaim'): Promise<void> {
    await apiClient.post(`/organization/members/${memberID}/balance`, { amount: String(amount), operation, idempotency_key: randomIdempotencyKey() })
  },
  async transferCompanyBalance(amount: string, operation: 'deposit' | 'withdraw'): Promise<void> {
    await apiClient.post('/organization/company-balance', { amount: String(amount), operation, idempotency_key: randomIdempotencyKey() })
  },
  async getFinance(): Promise<FinanceSummary> {
    const { data } = await apiClient.get('/organization/finance')
    return data
  },
	async getDashboard(): Promise<DashboardStats> {
		const { data } = await apiClient.get<DashboardStats>('/organization/dashboard')
		return data
	},
	async getDashboardSpendingRanking(params: OrganizationUsageParams & { limit?: number } = {}): Promise<UserSpendingRankingResponse> {
		const { data } = await apiClient.get<UserSpendingRankingResponse>('/organization/dashboard/spending-ranking', { params })
		return data
	},
	async getDashboardUserBreakdown(params: OrganizationUsageParams & { limit?: number } = {}): Promise<{ users: UserBreakdownItem[] }> {
		const { data } = await apiClient.get<{ users: UserBreakdownItem[] }>('/organization/dashboard/user-breakdown', { params })
		return data
	},
	async getDashboardUsersTrend(params: OrganizationUsageParams & { limit?: number } = {}): Promise<UserUsageTrendPoint[]> {
		const { data } = await apiClient.get<{ trend: UserUsageTrendPoint[] }>('/organization/dashboard/users-trend', { params })
		return data.trend
	},
  async listSubscriptions(): Promise<OrganizationSubscription[]> {
    const { data } = await apiClient.get<{ subscriptions: OrganizationSubscription[] }>('/organization/subscriptions')
    return data.subscriptions
  },
  async listSubscriptionGroups(): Promise<Group[]> {
    const { data } = await apiClient.get<Group[]>('/organization/subscription-groups')
    return data
  },
  async createSubscription(groupID: number, validityDays: number, notes: string): Promise<OrganizationSubscription> {
    const { data } = await apiClient.post<OrganizationSubscription>('/organization/subscriptions', { group_id: groupID, validity_days: validityDays, notes })
    return data
  },
  async createSubscriptionOrder(payload: {
    plan_id: number
    payment_type: string
    openid?: string
    return_url?: string
    payment_source?: string
    is_mobile?: boolean
  }): Promise<CreateOrderResult> {
    const { data } = await apiClient.post<CreateOrderResult>('/organization/subscription-orders', payload)
    return data
  },
  async cancelSubscription(id: number): Promise<void> {
    await apiClient.delete(`/organization/subscriptions/${id}`)
  },
  async listSpendLimits(): Promise<OrganizationSpendLimitRule[]> {
    const { data } = await apiClient.get<{ items: OrganizationSpendLimitRule[] }>('/organization/spend-limits')
    return data.items
  },
  async upsertSpendLimits(payload: {
    target: 'all' | 'members'
    member_ids?: number[]
    daily_limit_usd?: string | number
    monthly_limit_usd?: string | number
    alert_enabled: boolean
    alert_threshold_pct: number
    additional_recipients: string[]
  }): Promise<OrganizationSpendLimitRule[]> {
    const { data } = await apiClient.put<{ items: OrganizationSpendLimitRule[] }>('/organization/spend-limits', payload)
    return data.items
  },
  async deleteSpendLimit(memberID?: number): Promise<void> {
    await apiClient.delete('/organization/spend-limits', { params: { member_id: memberID } })
  },
  async getSpendLimitUsage(): Promise<OrganizationSpendUsage[]> {
    const { data } = await apiClient.get<{ items: OrganizationSpendUsage[] }>('/organization/spend-limits/usage')
    return data.items
  },
  async getUsage(params: OrganizationUsageParams = {}): Promise<PaginatedOrganizationUsage> {
    const { data } = await apiClient.get('/organization/usage', { params })
    return data
  },
	async searchUsageAPIKeys(query = '', memberID?: number): Promise<Array<{ id: number; name: string }>> {
		const { data } = await apiClient.get<{ items: Array<{ id: number; name: string }> }>('/organization/usage/api-keys/search', {
			params: { q: query || undefined, member_id: memberID },
		})
		return data.items
	},
  async getUsageStats(params: OrganizationUsageParams = {}): Promise<OrganizationUsageStats> {
    const { data } = await apiClient.get<OrganizationUsageStats>('/organization/usage/stats', { params })
    return data
  },
  async getUsageTrend(params: OrganizationUsageParams = {}): Promise<OrganizationUsageTrendPoint[]> {
    const { data } = await apiClient.get<{ items: OrganizationUsageTrendPoint[] }>('/organization/usage/trend', { params })
    return data.items
  },
  async getUsageCharts(params: OrganizationUsageParams = {}): Promise<{
    trend: OrganizationUsageTrendPoint[]
    models: ModelStat[]
    groups: GroupStat[]
    endpoints: EndpointStat[]
  }> {
    const { data } = await apiClient.get('/organization/usage/charts', { params })
    return data
  },
  async listApplications(params: { status?: string; page?: number; page_size?: number } = {}): Promise<{ items: CompanyApplication[]; total: number }> {
    const { data } = await apiClient.get('/admin/organizations/applications', { params })
    return data
  },
  async decideApplication(id: number, decision: 'approve' | 'reject', reason = ''): Promise<CompanyApplication> {
    const { data } = await apiClient.post(`/admin/organizations/applications/${id}/decision`, { decision, reason })
    return data
  },
  async getApplication(id: number): Promise<CompanyApplicationDetail> {
    const { data } = await apiClient.get<CompanyApplicationDetail>(`/admin/organizations/applications/${id}`)
    return data
  },
  async listNameChanges(params: { status?: string; page?: number; page_size?: number } = {}): Promise<{ items: OrganizationNameChangeRequest[]; total: number }> {
    const { data } = await apiClient.get('/admin/organizations/name-change-requests', { params })
    return data
  },
  async getNameChange(id: number): Promise<OrganizationNameChangeRequest> {
    const { data } = await apiClient.get(`/admin/organizations/name-change-requests/${id}`)
    return data
  },
  async decideNameChange(id: number, decision: 'approve' | 'reject', reason = ''): Promise<void> {
    await apiClient.post(`/admin/organizations/name-change-requests/${id}/decision`, { decision, reason })
  },
	async listOrganizations(params: { status?: string; page?: number; page_size?: number } = {}): Promise<{ items: AdminOrganization[]; total: number }> {
		const { data } = await apiClient.get('/admin/organizations', { params })
		return data
	},
	async getUsageErrors(params: UserErrorListParams & { member_id?: number; start?: string; end?: string }): Promise<{ items: UserErrorRequest[]; total: number; page: number; page_size: number; pages: number }> {
		const { data } = await apiClient.get('/organization/usage/errors', { params })
		return data
	},
	async getUsageErrorDetail(id: number): Promise<UserErrorRequestDetail> {
		const { data } = await apiClient.get<UserErrorRequestDetail>(`/organization/usage/errors/${id}`)
		return data
	},
	async getOrganization(id: number): Promise<AdminOrganizationDetail> {
		const { data } = await apiClient.get<AdminOrganizationDetail>(`/admin/organizations/${id}`)
		return data
	},
	async setOrganizationStatus(id: number, status: 'active' | 'suspended'): Promise<void> {
		await apiClient.patch(`/admin/organizations/${id}/status`, { status })
	},
	async assignOrganizationSubscription(id: number, groupId: number, validityDays: number, notes = ''): Promise<OrganizationSubscription> {
		const { data } = await apiClient.post<OrganizationSubscription>(`/admin/organizations/${id}/subscriptions`, {
			group_id: groupId,
			validity_days: validityDays,
			notes,
		})
		return data
	},
	async listAdminOrganizationSubscriptions(params: { page: number; page_size: number; status?: string; group_id?: number; platform?: string; sort_by?: string; sort_order?: 'asc' | 'desc' }, signal?: AbortSignal): Promise<{ items: OrganizationSubscription[]; total: number; pages: number }> {
		const { data } = await apiClient.get('/admin/organizations/subscriptions', { params, signal })
		return data
	},
	async extendAdminOrganizationSubscription(id: number, days: number): Promise<void> {
		await apiClient.post(`/admin/organizations/subscriptions/${id}/extend`, { days })
	},
	async resetAdminOrganizationSubscriptionQuota(id: number): Promise<void> {
		await apiClient.post(`/admin/organizations/subscriptions/${id}/reset-quota`)
	},
	async revokeAdminOrganizationSubscription(id: number): Promise<void> {
		await apiClient.post(`/admin/organizations/subscriptions/${id}/revoke`)
	},
}
