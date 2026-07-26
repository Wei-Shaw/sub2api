import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OrganizationConsoleView from '@/views/user/OrganizationConsoleView.vue'
import CompanyUpgradeView from '@/views/user/CompanyUpgradeView.vue'
import CompanyApplicationsView from '@/views/admin/CompanyApplicationsView.vue'
import ProfileIAMRecoveryEmailCard from '@/components/user/profile/ProfileIAMRecoveryEmailCard.vue'
import enOrganization from '@/i18n/locales/en/organization'
import zhOrganization from '@/i18n/locales/zh/organization'

const api = vi.hoisted(() => ({
  getContext: vi.fn(), listMembers: vi.fn(), listPolicies: vi.fn(), getUsage: vi.fn(),
  getUpgradeEligibility: vi.fn(), getCurrentApplication: vi.fn(),
  listApplications: vi.fn(), listNameChanges: vi.fn(), getApplication: vi.fn(),
  createMember: vi.fn(), setMemberStatus: vi.fn(), resetMemberPassword: vi.fn(),
  setPolicy: vi.fn(), transferBalance: vi.fn(), submitApplication: vi.fn(),
  withdrawApplication: vi.fn(), decideApplication: vi.fn(), decideNameChange: vi.fn()
	, sendRecoveryEmailCode: vi.fn(), verifyRecoveryEmail: vi.fn(), listOrganizations: vi.fn(),
	getOrganization: vi.fn(), setOrganizationStatus: vi.fn(), requestNameChange: vi.fn()
}))

const auth = vi.hoisted(() => ({ user: { id: 99 }, refreshUser: vi.fn() }))

vi.mock('@/api', () => ({ organizationAPI: api }))
vi.mock('@/stores', () => ({ useAuthStore: () => auth }))

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {} }, missingWarn: false, fallbackWarn: false })
const mountOptions = {
  global: {
    plugins: [i18n, createPinia()],
    stubs: {
      AppLayout: { template: '<div data-testid="app-layout"><slot /></div>' },
    },
  },
}

describe('organization views', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.listPolicies.mockResolvedValue([])
    api.listMembers.mockResolvedValue({ items: [], member_limit: 20, used_slots: 0 })
    api.getUsage.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    api.listNameChanges.mockResolvedValue({ items: [], total: 0 })
	api.listOrganizations.mockResolvedValue({ items: [], total: 0 })
  })

  it('does not request owner usage or member APIs for a finance-only IAM user', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'member', actions: ['organization.finance.balance.read'] },
      finance: { balance_source: 'allocated', available: '12', frozen: '0', total: '12' }
    })
    mount(OrganizationConsoleView, mountOptions)
    await flushPromises()

    expect(api.listMembers).not.toHaveBeenCalled()
    expect(api.listPolicies).not.toHaveBeenCalled()
    expect(api.getUsage).not.toHaveBeenCalled()
  })

  it('loads the stable member limit and company usage only for an owner', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' }
    })
    api.listMembers.mockResolvedValue({ items: [], member_limit: 20, used_slots: 0 })
    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()

    expect(wrapper.get('[data-testid="app-layout"]').exists()).toBe(true)
    expect(api.listMembers).toHaveBeenCalledOnce()
    expect(api.getUsage).toHaveBeenCalledOnce()
    expect((wrapper.vm as unknown as { memberLimit: number }).memberLimit).toBe(20)
  })

  it('uses scoped usage filters and never renders admin-only usage fields', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' }
    })
    api.listMembers.mockResolvedValue({
      items: [{ user_id: 42, login_name: 'reader', principal: 'reader@1719905235756637', external_user_id: '201705485041478971', status: 'active', balance: '5', frozen_balance: '0', policy_names: [], must_change_password: false, created_at: '2026-01-01T00:00:00Z' }],
      member_limit: 20,
      used_slots: 1,
    })
    api.getUsage.mockResolvedValue({
      items: [{
        id: 1, member_user_id: 42, member_login: 'reader', api_key_name: 'member-key', model: 'gpt-5',
        input_tokens: 10, output_tokens: 5, actual_cost: '1.25', endpoint: '/v1/responses', status: 'charged',
        duration_ms: 120, created_at: '2026-07-26T00:00:00Z', upstream_account: 'SECRET-UPSTREAM',
        internal_cost: 'SECRET-COST', raw_error: 'SECRET-ERROR',
      }], total: 1, page: 1, page_size: 20, pages: 1,
    })

    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()
    ;(wrapper.vm as unknown as { activeTab: string }).activeTab = 'usage'
    await wrapper.vm.$nextTick()

    const selects = wrapper.findAll('form select')
    await selects[0].setValue('42')
    const inputs = wrapper.findAll('form input')
    await inputs[0].setValue('99')
    await inputs[1].setValue('gpt-5')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(api.getUsage).toHaveBeenLastCalledWith(expect.objectContaining({ member_id: 42, api_key_id: 99, model: 'gpt-5', page: 1 }))
    expect(wrapper.text()).toContain('member-key')
    expect(wrapper.text()).not.toContain('SECRET-UPSTREAM')
    expect(wrapper.text()).not.toContain('SECRET-COST')
    expect(wrapper.text()).not.toContain('SECRET-ERROR')
    expect(wrapper.get('table').classes()).toContain('min-w-[1050px]')
    expect(wrapper.get('table').element.parentElement?.classList.contains('overflow-x-auto')).toBe(true)
  })

  it('renders the configured upgrade fee returned by eligibility', async () => {
    api.getUpgradeEligibility.mockResolvedValue({ eligible: true, fee_amount: '35.50000000', fee_currency: 'USD' })
    api.getCurrentApplication.mockResolvedValue(null)
    const wrapper = mount(CompanyUpgradeView, mountOptions)
    await flushPromises()

    expect(wrapper.get('[data-testid="company-upgrade-content"]').classes()).toContain('w-full')
    expect(wrapper.get('[data-testid="company-upgrade-content"]').classes()).not.toContain('max-w-2xl')
    expect(wrapper.get('[data-testid="upgrade-fee-amount"]').text()).toBe('$35.50')
    expect(enOrganization.organization.upgrade.feeNotice).toContain('frozen from your available balance')
    expect(wrapper.get('[data-testid="upgrade-fee-notice"]').text()).not.toContain('USD')
  })

  it('returns to a new application form after cancelling a pending upgrade', async () => {
    const pendingApplication = {
      id: 42,
      applicant_user_id: 99,
      requested_name: 'Cancelled Company',
      status: 'pending' as const,
      fee_amount: '20.00000000',
      fee_currency: 'USD',
      similar_names: [],
      created_at: '2026-07-26T00:00:00Z',
    }
    api.getUpgradeEligibility
      .mockResolvedValueOnce({
        eligible: false,
        reason: 'application_pending',
        fee_amount: '20.00000000',
        fee_currency: 'USD',
        application: pendingApplication,
      })
      .mockResolvedValueOnce({ eligible: true, fee_amount: '20.00000000', fee_currency: 'USD' })
    api.withdrawApplication.mockResolvedValue({ ...pendingApplication, status: 'withdrawn' })
    api.getCurrentApplication.mockResolvedValue({ ...pendingApplication, status: 'withdrawn' })

    const wrapper = mount(CompanyUpgradeView, mountOptions)
    await flushPromises()
    expect(wrapper.get('[data-testid="company-upgrade-application"]').exists()).toBe(true)

    await wrapper.get('button.btn-secondary').trigger('click')
    await flushPromises()

    expect(api.withdrawApplication).toHaveBeenCalledWith(42)
    expect(auth.refreshUser).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="company-upgrade-application"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="company-upgrade-form"]').exists()).toBe(true)
    expect(zhOrganization.organization.upgrade.withdraw).toBe('中止')
  })

  it('refreshes the current user balance after submitting an upgrade', async () => {
    const pendingApplication = {
      id: 43,
      applicant_user_id: 99,
      requested_name: 'New Company',
      status: 'pending' as const,
      fee_amount: '20.00000000',
      fee_currency: 'USD',
      similar_names: [],
      created_at: '2026-07-27T00:00:00Z',
    }
    api.getUpgradeEligibility.mockResolvedValue({
      eligible: true,
      fee_amount: '20.00000000',
      fee_currency: 'USD',
    })
    api.getCurrentApplication.mockResolvedValue(null)
    api.submitApplication.mockResolvedValue(pendingApplication)

    const wrapper = mount(CompanyUpgradeView, mountOptions)
    await flushPromises()
    await wrapper.get('#company-name').setValue('New Company')
    await wrapper.get('[data-testid="company-upgrade-form"]').trigger('submit')
    await flushPromises()

    expect(api.submitApplication).toHaveBeenCalledWith('New Company', expect.any(String))
    expect(auth.refreshUser).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="company-upgrade-application"]').text()).toContain('New Company')
  })

  it('allows a system administrator to review their own application', async () => {
    api.listApplications.mockResolvedValue({ items: [{
      id: 7, applicant_user_id: 99, requested_name: 'Self Company', status: 'pending',
      fee_amount: '20.00000000', fee_currency: 'USD', similar_names: [], created_at: '2026-01-01T00:00:00Z'
    }], total: 1 })
    const wrapper = mount(CompanyApplicationsView, mountOptions)
    await flushPromises()

    expect(wrapper.get('[data-testid="app-layout"]').exists()).toBe(true)
    expect(wrapper.get('.select-trigger').exists()).toBe(true)
    expect(wrapper.find('select').exists()).toBe(false)
    const actionButtons = wrapper.findAll('tbody button')
    expect(actionButtons).toHaveLength(2)
    expect(actionButtons.every(button => button.attributes('disabled') === undefined)).toBe(true)

    await actionButtons[0].trigger('click')
    await flushPromises()
    expect(api.decideApplication).toHaveBeenCalledWith(7, 'approve', '')
  })

  it('formats USD upgrade fees with a dollar sign and two decimals in admin review', async () => {
    const application = {
      id: 9,
      applicant_user_id: 12,
      requested_name: 'Fee Company',
      status: 'pending' as const,
      fee_amount: '20.00000000',
      fee_currency: 'USD',
      similar_names: [],
      created_at: '2026-01-01T00:00:00Z',
    }
    api.listApplications.mockResolvedValue({ items: [application], total: 1 })
    api.getApplication.mockResolvedValue({ application, audit: [] })

    const wrapper = mount(CompanyApplicationsView, mountOptions)
    await flushPromises()

    expect(wrapper.get('[data-testid="admin-upgrade-fee"]').text()).toBe('$20.00')
    await wrapper.get('tbody tr').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="admin-upgrade-detail-fee"]').text()).toBe('$20.00')
    expect(wrapper.get('[data-testid="company-application-detail"]').text()).not.toContain('USD')
  })

  it('renders terminal approval state without decision controls', async () => {
    api.listApplications.mockResolvedValue({ items: [{
      id: 8, applicant_user_id: 12, requested_name: 'Reviewed Company', status: 'rejected',
      fee_amount: '20.00000000', fee_currency: 'USD', review_reason: 'invalid details', similar_names: [], created_at: '2026-01-01T00:00:00Z'
    }], total: 1 })
    const wrapper = mount(CompanyApplicationsView, mountOptions)
    await flushPromises()

    expect(wrapper.findAll('tbody button')).toHaveLength(0)
    expect(wrapper.text()).toContain('invalid details')
  })

	it('verifies an IAM recovery email without using it as a login identity', async () => {
		api.sendRecoveryEmailCode.mockResolvedValue(undefined)
		api.verifyRecoveryEmail.mockResolvedValue(undefined)
		const wrapper = mount(ProfileIAMRecoveryEmailCard, mountOptions)
		await wrapper.get('input[type="email"]').setValue('iam@example.com')
		await wrapper.get('form').trigger('submit')
		await flushPromises()
		expect(api.sendRecoveryEmailCode).toHaveBeenCalledWith('iam@example.com')

		await wrapper.get('input[inputmode="numeric"]').setValue('123456')
		await wrapper.get('form').trigger('submit')
		await flushPromises()
		expect(api.verifyRecoveryEmail).toHaveBeenCalledWith('iam@example.com', '123456')
	})
})
