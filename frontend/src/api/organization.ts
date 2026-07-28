import { apiClient } from './client'
import type {
  AuthResponse,
  CompanyApplication,
  CompanyApplicationDetail,
  CompanyUpgradeEligibility,
	AdminOrganization,
	AdminOrganizationDetail,
  FinanceSummary,
  IAMMember,
  ManagedPolicy,
  OrganizationContext,
  OrganizationNameChangeRequest,
  OrganizationUsageParams,
  OrganizationUsageStats,
  OrganizationUsageTrendPoint,
  PaginatedOrganizationUsage,
} from '@/types'

export interface IAMLoginRequest {
  principal: string
  password: string
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
  async submitApplication(companyName: string, englishName: string, companySize: string, idempotencyKey: string): Promise<CompanyApplication> {
    const { data } = await apiClient.post('/organization/applications', { company_name: companyName, english_name: englishName, company_size: companySize, idempotency_key: idempotencyKey })
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
  async createMember(loginName: string, password: string, mustChangePassword = true, recoveryEmail?: string): Promise<{ member: IAMMember; initial_password: string }> {
    const { data } = await apiClient.post('/organization/members', {
      login_name: loginName,
      password,
      must_change_password: mustChangePassword,
      recovery_email: recoveryEmail,
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
    await apiClient.post(`/organization/members/${memberID}/balance`, { amount, operation, idempotency_key: crypto.randomUUID() })
  },
  async getFinance(): Promise<FinanceSummary> {
    const { data } = await apiClient.get('/organization/finance')
    return data
  },
  async getUsage(params: OrganizationUsageParams = {}): Promise<PaginatedOrganizationUsage> {
    const { data } = await apiClient.get('/organization/usage', { params })
    return data
  },
  async getUsageStats(params: OrganizationUsageParams = {}): Promise<OrganizationUsageStats> {
    const { data } = await apiClient.get<OrganizationUsageStats>('/organization/usage/stats', { params })
    return data
  },
  async getUsageTrend(params: OrganizationUsageParams = {}): Promise<OrganizationUsageTrendPoint[]> {
    const { data } = await apiClient.get<{ items: OrganizationUsageTrendPoint[] }>('/organization/usage/trend', { params })
    return data.items
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
	async getOrganization(id: number): Promise<AdminOrganizationDetail> {
		const { data } = await apiClient.get<AdminOrganizationDetail>(`/admin/organizations/${id}`)
		return data
	},
	async setOrganizationStatus(id: number, status: 'active' | 'suspended'): Promise<void> {
		await apiClient.patch(`/admin/organizations/${id}/status`, { status })
	},
}
