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
    await organizationAPI.createMember('reader', 'initial-password', false)
    await organizationAPI.setPolicy(42, 'CompanyFinanceReadOnly', true)
    expect(post).toHaveBeenCalledWith('/organization/members', {
      login_name: 'reader',
      password: 'initial-password',
      must_change_password: false,
      recovery_email: undefined,
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

    expect(await organizationAPI.getMember(42)).toMatchObject({ login_name: 'reader' })
    expect(await organizationAPI.listMemberPolicies(42)).toHaveLength(1)
    expect(await organizationAPI.getUsageStats({ member_id: 42 })).toMatchObject({ requests: 2 })
    expect(await organizationAPI.getUsageTrend({ model: 'gpt-5' })).toHaveLength(1)

    expect(get).toHaveBeenNthCalledWith(1, '/organization/members/42')
    expect(get).toHaveBeenNthCalledWith(2, '/organization/members/42/policies')
    expect(get).toHaveBeenNthCalledWith(3, '/organization/usage/stats', { params: { member_id: 42 } })
    expect(get).toHaveBeenNthCalledWith(4, '/organization/usage/trend', { params: { model: 'gpt-5' } })
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
