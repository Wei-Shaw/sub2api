import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, patch } = vi.hoisted(() => ({
  get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn()
}))

vi.mock('@/api/client', () => ({ apiClient: { get, post, put, patch } }))

import { organizationAPI } from '@/api/organization'

describe('organization API', () => {
  beforeEach(() => vi.clearAllMocks())

  it('submits canonical IAM login fields without an organization scope', async () => {
    post.mockResolvedValue({ data: { access_token: 'token' } })
    await organizationAPI.loginIAM({ principal: 'finance@1719905235756637.opentk.ai', password: 'secret' })
    expect(post).toHaveBeenCalledWith('/auth/iam/login', {
      principal: 'finance@1719905235756637.opentk.ai', password: 'secret'
    })
  })

  it('derives member and policy scope from authenticated routes', async () => {
    post.mockResolvedValue({ data: { member: {}, initial_password: 'one-time' } })
    put.mockResolvedValue({ data: {} })
    await organizationAPI.createMember('reader', 'initial-password', false, undefined, 'Reader User')
    await organizationAPI.setPolicy(42, 'CompanyFinanceReadOnly', true)
    expect(post).toHaveBeenCalledWith('/organization/members', {
      login_name: 'reader',
      password: 'initial-password',
      must_change_password: false,
      recovery_email: undefined,
      username: 'Reader User',
    })
    expect(put).toHaveBeenCalledWith('/organization/members/42/policies', { policy_key: 'CompanyFinanceReadOnly', attached: true })
  })

  it('uses backend eligibility fee and dedicated admin detail routes', async () => {
    get.mockResolvedValueOnce({ data: { eligible: true, fee_amount: '20.00000000', fee_currency: 'USD' } })
    get.mockResolvedValueOnce({ data: { application: { id: 7 }, audit: [] } })
    expect(await organizationAPI.getUpgradeEligibility()).toMatchObject({ eligible: true, fee_currency: 'USD' })
    expect(await organizationAPI.getApplication(7)).toMatchObject({ audit: [] })
    expect(get).toHaveBeenNthCalledWith(1, '/organization/applications/eligibility')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/organizations/applications/7')
  })

  it('assigns an enterprise subscription through the admin organization route', async () => {
    post.mockResolvedValue({ data: { id: 9, organization_id: 4, group_id: 7 } })

    await organizationAPI.assignOrganizationSubscription(4, 7, 30, 'admin grant')

    expect(post).toHaveBeenCalledWith('/admin/organizations/4/subscriptions', {
      group_id: 7,
      validity_days: 30,
      notes: 'admin grant',
    })
  })

  it('lists enterprise subscriptions for the admin table', async () => {
    get.mockResolvedValue({ data: { items: [{ id: 9 }], total: 1, pages: 1 } })

    await organizationAPI.listAdminOrganizationSubscriptions({ page: 1, page_size: 20, status: 'active' })

    expect(get).toHaveBeenCalledWith('/admin/organizations/subscriptions', {
      params: { page: 1, page_size: 20, status: 'active' },
      signal: undefined,
    })
  })

  it('manages enterprise subscriptions through dedicated admin routes', async () => {
    post.mockResolvedValue({ data: { success: true } })

    await organizationAPI.extendAdminOrganizationSubscription(9, 30)
    await organizationAPI.resetAdminOrganizationSubscriptionQuota(9)
    await organizationAPI.revokeAdminOrganizationSubscription(9)

    expect(post).toHaveBeenNthCalledWith(1, '/admin/organizations/subscriptions/9/extend', { days: 30 })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/organizations/subscriptions/9/reset-quota')
    expect(post).toHaveBeenNthCalledWith(3, '/admin/organizations/subscriptions/9/revoke')
  })

  it('sends a unique command key for every allocation command', async () => {
    post.mockResolvedValue({ data: {} })
    await organizationAPI.transferBalance(9, '12.50', 'allocate')
    expect(post).toHaveBeenCalledWith('/organization/members/9/balance', expect.objectContaining({
      amount: '12.50', operation: 'allocate', idempotency_key: expect.any(String)
    }))
  })

  it('provides member policy and organization usage query clients', async () => {
    get
      .mockResolvedValueOnce({ data: { user_id: 42, login_name: 'reader' } })
      .mockResolvedValueOnce({ data: { items: [{ key: 'CompanyFinanceReadOnly' }] } })
      .mockResolvedValueOnce({ data: { requests: 2, input_tokens: 10, output_tokens: 5, actual_cost: '1.25' } })
      .mockResolvedValueOnce({ data: { items: [{ bucket: '2026-07-26T00:00:00Z', requests: 2 }] } })
      .mockResolvedValueOnce({ data: { trend: [], models: [], groups: [], endpoints: [] } })

    expect(await organizationAPI.getMember(42)).toMatchObject({ login_name: 'reader' })
    expect(await organizationAPI.listMemberPolicies(42)).toHaveLength(1)
    expect(await organizationAPI.getUsageStats({ member_id: 42 })).toMatchObject({ requests: 2 })
    expect(await organizationAPI.getUsageTrend({ model: 'gpt-5' })).toHaveLength(1)
    expect(await organizationAPI.getUsageCharts({ endpoint: '/v1/responses' })).toMatchObject({ models: [] })

    expect(get).toHaveBeenNthCalledWith(1, '/organization/members/42')
    expect(get).toHaveBeenNthCalledWith(2, '/organization/members/42/policies')
    expect(get).toHaveBeenNthCalledWith(3, '/organization/usage/stats', { params: { member_id: 42 } })
    expect(get).toHaveBeenNthCalledWith(4, '/organization/usage/trend', { params: { model: 'gpt-5' } })
    expect(get).toHaveBeenNthCalledWith(5, '/organization/usage/charts', { params: { endpoint: '/v1/responses' } })
  })

  it('loads organization-scoped dashboard statistics', async () => {
    get.mockResolvedValue({ data: { total_api_keys: 3, today_requests: 12 } })

    await expect(organizationAPI.getDashboard()).resolves.toMatchObject({ total_api_keys: 3 })
    expect(get).toHaveBeenCalledWith('/organization/dashboard')
  })

  it('uses organization-scoped error list and detail routes', async () => {
    get
      .mockResolvedValueOnce({ data: { items: [{ id: 12 }], total: 1, page: 1, page_size: 20, pages: 1 } })
      .mockResolvedValueOnce({ data: { id: 12, status_code: 429 } })

    await organizationAPI.getUsageErrors({ page: 1, page_size: 20, member_id: 9, model: 'gpt-5.6' })
    await organizationAPI.getUsageErrorDetail(12)

    expect(get).toHaveBeenNthCalledWith(1, '/organization/usage/errors', {
      params: { page: 1, page_size: 20, member_id: 9, model: 'gpt-5.6' },
    })
    expect(get).toHaveBeenNthCalledWith(2, '/organization/usage/errors/12')
  })

	it('uses dedicated recovery verification and organization lifecycle routes', async () => {
		post.mockResolvedValue({ data: {} })
		patch.mockResolvedValue({ data: {} })
		await organizationAPI.sendRecoveryEmailCode('iam@example.com')
		await organizationAPI.verifyRecoveryEmail('iam@example.com', '123456')
		await organizationAPI.setOrganizationStatus(8, 'suspended')
		expect(post).toHaveBeenNthCalledWith(1, '/organization/recovery-email/send-code', { email: 'iam@example.com' })
		expect(post).toHaveBeenNthCalledWith(2, '/organization/recovery-email/verify', { email: 'iam@example.com', code: '123456' })
		expect(patch).toHaveBeenCalledWith('/admin/organizations/8/status', { status: 'suspended' })
	})
})
