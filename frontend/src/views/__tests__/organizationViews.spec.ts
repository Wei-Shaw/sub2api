import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OrganizationConsoleView from '@/views/user/OrganizationConsoleView.vue'
import CompanyUpgradeView from '@/views/user/CompanyUpgradeView.vue'
import CompanyApplicationsView from '@/views/admin/CompanyApplicationsView.vue'
import Select from '@/components/common/Select.vue'
import ProfileIAMRecoveryEmailCard from '@/components/user/profile/ProfileIAMRecoveryEmailCard.vue'
import enOrganization from '@/i18n/locales/en/organization'
import zhOrganization from '@/i18n/locales/zh/organization'

const api = vi.hoisted(() => ({
  getContext: vi.fn(), listMembers: vi.fn(), listPolicies: vi.fn(), getUsage: vi.fn(),
	getUsageVideoTask: vi.fn(),
  getUsageStats: vi.fn(), getUsageCharts: vi.fn(), getDashboard: vi.fn(),
  getDashboardSpendingRanking: vi.fn(), getDashboardUserBreakdown: vi.fn(),
  getDashboardUsersTrend: vi.fn(),
  listSubscriptions: vi.fn(), listSubscriptionGroups: vi.fn(),
  listSpendLimits: vi.fn(), getSpendLimitUsage: vi.fn(), upsertSpendLimits: vi.fn(), deleteSpendLimit: vi.fn(),
  getUpgradeEligibility: vi.fn(), getCurrentApplication: vi.fn(),
  listApplications: vi.fn(), listNameChanges: vi.fn(), getApplication: vi.fn(),
  createMember: vi.fn(), setMemberStatus: vi.fn(), resetMemberPassword: vi.fn(),
  setPolicy: vi.fn(), transferBalance: vi.fn(), submitApplication: vi.fn(),
  withdrawApplication: vi.fn(), decideApplication: vi.fn(), decideNameChange: vi.fn()
	, sendRecoveryEmailCode: vi.fn(), verifyRecoveryEmail: vi.fn(), listOrganizations: vi.fn(),
	getOrganization: vi.fn(), setOrganizationStatus: vi.fn(), requestNameChange: vi.fn()
}))

const auth = vi.hoisted(() => ({ user: { id: 99 }, refreshUser: vi.fn() }))
const plaza = vi.hoisted(() => ({ listPlans: vi.fn() }))

vi.mock('@/api', () => ({ organizationAPI: api }))
vi.mock('@/api/plaza', () => ({ plazaAPI: plaza }))
vi.mock('@/stores', () => ({ useAuthStore: () => auth }))
vi.mock('vue-chartjs', () => ({
  Doughnut: { template: '<div data-testid="doughnut-chart" />' },
  Line: { template: '<div data-testid="line-chart" />' },
}))

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {} }, missingWarn: false, fallbackWarn: false })
const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/organization', component: { template: '<div />' } }],
})
const mountOptions = {
  global: {
    plugins: [i18n, createPinia(), router],
    stubs: {
      AppLayout: { template: '<div data-testid="app-layout"><slot /></div>' },
      Doughnut: true,
    },
  },
}

describe('organization views', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await router.replace('/organization')
    api.listPolicies.mockResolvedValue([])
    api.listMembers.mockResolvedValue({ items: [], member_limit: 20, used_slots: 0 })
    api.getUsage.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    api.getUsageStats.mockResolvedValue({ requests: 0, input_tokens: 0, output_tokens: 0, actual_cost: '0' })
    api.getUsageCharts.mockResolvedValue({ trend: [], models: [], groups: [], endpoints: [] })
    api.getDashboard.mockResolvedValue({ total_api_keys: 0, active_api_keys: 0, total_accounts: 0, normal_accounts: 0, today_requests: 0, total_requests: 0, today_new_users: 0, total_users: 0, today_tokens: 0, total_tokens: 0, rpm: 0, tpm: 0, average_duration_ms: 0, active_users: 0, today_actual_cost: 0, today_account_cost: 0, today_cost: 0, total_actual_cost: 0, total_account_cost: 0, total_cost: 0 })
    api.getDashboardSpendingRanking.mockResolvedValue({ ranking: [], total_actual_cost: 0, total_requests: 0, total_tokens: 0 })
    api.getDashboardUserBreakdown.mockResolvedValue({ users: [] })
    api.getDashboardUsersTrend.mockResolvedValue([])
    api.listSubscriptions.mockResolvedValue([])
    api.listSubscriptionGroups.mockResolvedValue([])
    api.listSpendLimits.mockResolvedValue([])
    api.getSpendLimitUsage.mockResolvedValue([])
    api.upsertSpendLimits.mockResolvedValue([])
    plaza.listPlans.mockResolvedValue({ cards: [] })
    api.listNameChanges.mockResolvedValue({ items: [], total: 0 })
	api.listOrganizations.mockResolvedValue({ items: [], total: 0 })
  })

  it('restores the selected owner tab from the URL and keeps tab changes addressable', async () => {
    await router.replace('/organization?tab=usage')
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' },
    })
    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()

    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe('usage')
    await wrapper.get('#organization-tab-subscriptions').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.tab).toBe('subscriptions')
  })

  it('shows the enterprise subscription rate in subscription details', async () => {
    await router.replace('/organization?tab=subscriptions')
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' },
    })
    api.listSubscriptions.mockResolvedValue([{
      id: 9,
      organization_id: 1,
      group_id: 7,
      group_name: 'Enterprise 0.2x',
      platform: 'openai',
      subscription_type: 'monthly',
      rate_multiplier: 0.2,
      starts_at: '2026-01-01T00:00:00Z',
      expires_at: '2027-01-01T00:00:00Z',
      status: 'active',
      daily_usage_usd: '0',
      weekly_usage_usd: '0',
      monthly_usage_usd: '0',
      assigned_at: '2026-01-01T00:00:00Z',
      created_at: '2026-01-01T00:00:00Z',
    }])

    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()

    expect(wrapper.text()).toContain('organization.subscriptions.rate')
    expect(wrapper.text()).toContain('0.2x')
  })

  it('uses the shared subscription group rate for enterprise plan cards', async () => {
    await router.replace('/organization?tab=subscriptions')
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' },
    })
    api.listSubscriptionGroups.mockResolvedValue([{
      id: 7,
      name: 'Shared 0.2x Group',
      platform: 'openai',
      rate_multiplier: 0.2,
    }])
    plaza.listPlans.mockResolvedValue({
      cards: [{
        id: 11,
        name: 'Monthly Plan',
        description: '',
        price: 99,
        validity_days: 30,
        validity_unit: 'days',
        features: '',
        group_id: 7,
        group_name: 'Stale Group Name',
        platform: 'openai',
        rate_multiplier: 0,
        models: [],
        models_overflow: 0,
      }],
    })

    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()

    expect(api.listSubscriptionGroups).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Shared 0.2x Group')
    expect(wrapper.text()).toContain('0.2x')
    expect(wrapper.text()).not.toContain('Stale Group Name')
  })

  it('shows the console to IAM users while limiting finance and member data by permission', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'member', actions: [] },
      finance: { balance_source: 'shared' }
    })
    api.listMembers.mockResolvedValue({
      items: [
        { user_id: 99, account_id: '4714045153338228', login_name: 'self', principal: 'self@company.opentk.ai', status: 'active', balance: '5', frozen_balance: '0', policy_names: ['CompanySharedBalanceUse'], must_change_password: false, created_at: '2026-01-01T00:00:00Z' },
        { user_id: 100, account_id: '8392014857201143', login_name: 'other-member', principal: 'other@company.opentk.ai', status: 'active', balance: '8', frozen_balance: '0', policy_names: [], must_change_password: false, created_at: '2026-01-01T00:00:00Z' },
      ],
      member_limit: 20,
      used_slots: 2,
    })
    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()

    expect(wrapper.get('[data-testid="organization-header"]').classes()).toContain('card')
    expect(wrapper.get('[data-testid="company-finance-summary"]').classes()).toContain('card')
    expect(wrapper.get('[data-testid="organization-members"]').classes()).toContain('card')
    expect(wrapper.get('#organization-tab-finance').exists()).toBe(true)
    expect(wrapper.find('#organization-tab-subscriptions').exists()).toBe(false)
    expect(wrapper.get('[data-testid="company-finance-summary"]').text().match(/organization\.finance\.noPermission/g)).toHaveLength(3)
    expect(wrapper.get('[data-testid="organization-members"]').text()).toContain('self')
    expect(wrapper.get('[data-testid="organization-members"]').text()).not.toContain('other-member')
    expect(wrapper.get('[data-testid="member-username-99"]').text()).toBe('self')
    expect(wrapper.find('th.text-right').exists()).toBe(false)
    expect(api.listMembers).toHaveBeenCalledOnce()
    expect(api.listPolicies).not.toHaveBeenCalled()
    expect(api.getUsage).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="member-policy-CompanySharedBalanceUse"]').trigger('mouseenter')
    await flushPromises()
    const tooltips = document.body.querySelectorAll('[role="tooltip"]')
    expect(tooltips[tooltips.length - 1]?.textContent).toContain('organization.policyMeta.CompanySharedBalanceUse.description')
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
    expect(api.getUsageCharts).toHaveBeenCalledTimes(2)
    expect(api.getDashboard).toHaveBeenCalledOnce()
    expect(api.getDashboardUsersTrend).toHaveBeenCalledOnce()
    expect(api.getUsageCharts).toHaveBeenCalledWith(expect.objectContaining({ granularity: 'hour', start: expect.any(String), end: expect.any(String) }))
    expect((wrapper.vm as unknown as { memberLimit: number }).memberLimit).toBe(20)
  })

  it('shows company today cost like the user dashboard with totals underneath', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' },
    })
    api.getDashboard.mockResolvedValue({
      total_api_keys: 0, active_api_keys: 0, total_accounts: 0, normal_accounts: 0,
      today_requests: 0, total_requests: 0, today_tokens: 0, total_tokens: 0,
      rpm: 0, tpm: 0, average_duration_ms: 0, active_users: 0,
      today_actual_cost: 1.25, today_account_cost: 1.75, today_cost: 2.5,
      total_actual_cost: 10, total_account_cost: 15, total_cost: 20,
    })

    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()
    ;(wrapper.vm as unknown as { activeTab: string }).activeTab = 'dashboard'
    await wrapper.vm.$nextTick()

    const card = wrapper.get('[data-testid="organization-today-cost"]')
    expect(card.text()).toContain('dashboard.todayCost')
    expect(card.text()).toContain('$1.25 / $2.50')
    expect(card.text()).toContain('common.total: $10.00 / $20.00')
    expect(card.text()).not.toContain('$1.75')
    expect(card.text()).not.toContain('$15.00')
  })

  it('loads model usage for a selected dashboard spending user', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' },
    })
    api.getDashboardSpendingRanking.mockResolvedValue({
      ranking: [{ user_id: 42, username: 'Finance Reader', email: 'reader@c123456789012345.opentk.ai', actual_cost: 1.25, requests: 7, tokens: 1000 }],
      total_actual_cost: 1.25,
      total_requests: 7,
      total_tokens: 1000,
    })

    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()
    ;(wrapper.vm as unknown as { activeTab: string }).activeTab = 'dashboard'
    await wrapper.vm.$nextTick()

    const rankingButton = wrapper.findAll('button').find((button) => button.text() === 'admin.dashboard.viewSpendingRanking')
    await rankingButton!.trigger('click')
    expect(wrapper.get('[data-testid="spending-ranking-row-42"]').text()).toContain('Finance Reader')
    expect(wrapper.get('[data-testid="spending-ranking-row-42"]').text()).not.toContain('reader@c123456789012345.opentk.ai')
    api.getUsageCharts.mockResolvedValueOnce({
      trend: [], groups: [], endpoints: [],
      models: [{ model: 'gpt-5.4', requests: 7, input_tokens: 600, output_tokens: 400, cache_creation_tokens: 0, cache_read_tokens: 0, total_tokens: 1000, cost: 2, actual_cost: 1.25 }],
    })
    await wrapper.get('[data-testid="spending-ranking-row-42"]').trigger('click')
    await flushPromises()

    expect(api.getUsageCharts).toHaveBeenLastCalledWith(expect.objectContaining({ member_id: 42, start: expect.any(String), end: expect.any(String) }))
    expect(wrapper.get('[data-testid="spending-ranking-detail-42"]').text()).toContain('gpt-5.4')
  })

  it('lets an owner configure selected-member limits and shows effective usage', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' },
    })
    api.listMembers.mockResolvedValue({
      items: [{ user_id: 42, account_id: '8392014857201143', username: 'Finance Reader', login_name: 'reader', principal: 'reader@company.opentk.ai', status: 'active', balance: '0', frozen_balance: '0', recovery_email: 'reader@example.com', recovery_email_verified_at: '2026-01-01T00:00:00Z', policy_names: [], must_change_password: false, created_at: '2026-01-01T00:00:00Z' }],
      member_limit: 20,
      used_slots: 1,
    })
    api.listSpendLimits.mockResolvedValue([{ id: 1, organization_id: 1, member_user_id: 42, member_login: 'reader', member_username: 'Finance Reader', daily_limit_usd: '10', monthly_limit_usd: '50', alert_enabled: false, alert_threshold_pct: 80, additional_recipients: [], revision: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' }])
    api.getSpendLimitUsage.mockResolvedValue([{ member_user_id: 42, member_login: 'reader', member_username: 'Finance Reader', daily_used_usd: '10', monthly_used_usd: '8', daily_limit_usd: '10', monthly_limit_usd: '50' }])

    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()
    expect(wrapper.get('[data-testid="organization-members"]').text()).toContain('Finance Reader')
    await wrapper.get('#organization-tab-limits').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="spend-limit-form"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="spend-limit-form"]').classes()).toContain('card')
    expect(wrapper.get('[data-testid="spend-limit-rules"]').text()).toContain('Finance Reader')
    expect(wrapper.get('[data-testid="spend-limit-rules"]').text()).toContain('reader')
    expect(wrapper.get('[data-testid="spend-limit-usage"]').classes()).toContain('card')
    expect(wrapper.get('[data-testid="spend-limit-usage"]').text()).toContain('Finance Reader')
    expect(wrapper.get('[data-testid="spend-limit-usage"]').text()).toContain('$10.00 / $10.00')
    expect(wrapper.get('[data-testid="spend-limit-usage"]').text()).not.toContain('US$')
    expect(wrapper.get('[data-testid="spend-limit-daily-42"]').classes()).toContain('text-red-600')
    expect(wrapper.get('[data-testid="spend-limit-monthly-42"]').classes()).not.toContain('text-red-600')
    const vm = wrapper.vm as unknown as {
      spendLimitForm: { target: 'all' | 'members'; daily: string; monthly: string; alertEnabled: boolean; threshold: number; recipients: string }
      spendLimitMemberIDs: number[]
      saveSpendLimit: () => Promise<void>
    }
    vm.spendLimitForm.target = 'members'
    vm.spendLimitForm.daily = '10'
    vm.spendLimitForm.monthly = '50'
    vm.spendLimitForm.alertEnabled = true
    vm.spendLimitForm.threshold = 80
    vm.spendLimitMemberIDs = [42]
    await wrapper.vm.$nextTick()

    const granularity = wrapper.get('[data-testid="spend-limit-granularity"]')
    const dailyLimit = wrapper.get('#spend-limit-daily')
    expect(granularity.element.compareDocumentPosition(dailyLimit.element) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0)
    const alertSettings = wrapper.get('[data-testid="spend-limit-alert-settings"]')
    expect(alertSettings.find('#spend-limit-threshold').exists()).toBe(true)
    expect(alertSettings.find('#spend-limit-recipient-input').exists()).toBe(true)
    expect(wrapper.get('#spend-limit-member-emails option').attributes('value')).toBe('reader@example.com')
    await wrapper.get('#spend-limit-recipient-input').setValue('reader@example.com')
    await wrapper.get('#spend-limit-recipient-input').trigger('keydown', { key: 'Enter' })
    vm.spendLimitForm.recipients += ', ops@example.com, finance@example.com'
    await vm.saveSpendLimit()

    expect(api.upsertSpendLimits).toHaveBeenCalledWith({
      target: 'members', member_ids: [42], daily_limit_usd: '10', monthly_limit_usd: '50',
      alert_enabled: true, alert_threshold_pct: 80, additional_recipients: ['reader@example.com', 'ops@example.com', 'finance@example.com'],
    })
  })

  it('shows only self spend usage to an IAM member without mutation controls', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'member', actions: [] },
      finance: { balance_source: 'shared' },
    })
    api.getSpendLimitUsage.mockResolvedValue([{ member_user_id: 99, member_login: 'self', daily_used_usd: '1', monthly_used_usd: '3' }])
    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()
    await wrapper.get('#organization-tab-limits').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="spend-limit-form"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="spend-limit-usage-username-99"]').text()).toBe('self')
    expect(wrapper.get('[data-testid="spend-limit-usage"]').text()).toContain('$1.00 · organization.spendLimits.unlimited')
    expect(api.listSpendLimits).not.toHaveBeenCalled()
  })

  it('uses scoped usage filters and never renders admin-only usage fields', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' }
    })
    api.listMembers.mockResolvedValue({
      items: [{ user_id: 42, account_id: '8392014857201143', login_name: 'reader', principal: 'reader@1719905235756637.opentk.ai', status: 'active', balance: '5', frozen_balance: '0', policy_names: [], must_change_password: false, created_at: '2026-01-01T00:00:00Z' }],
      member_limit: 20,
      used_slots: 1,
    })
    api.getUsage.mockResolvedValue({
      items: [{
        id: 1, member_user_id: 42, member_login: 'reader', api_key_name: 'member-key', model: 'gpt-5',
        input_tokens: 10, output_tokens: 5, actual_cost: '1.25', total_cost: '1', rate_multiplier: 1.25,
        input_cost: 0.5, output_cost: 0.75, cache_creation_cost: 0, cache_read_cost: 0,
        image_input_tokens: 0, image_input_cost: 0, image_output_tokens: 0, image_output_cost: 0,
        endpoint: '/v1/responses', status: 'charged', balance_source: 'subscription',
        group_id: 7, group_name: 'Enterprise', request_type: 'stream', billing_type: 1, billing_mode: 'token',
        image_count: 0, image_urls: [], cos_urls: [], ip_address: '203.0.113.10', user_agent: 'test-agent',
        duration_ms: 120, created_at: '2026-07-26T00:00:00Z', upstream_account: 'SECRET-UPSTREAM',
        internal_cost: 'SECRET-COST', raw_error: 'SECRET-ERROR',
      }], total: 1, page: 1, page_size: 20, pages: 1,
    })
    api.getUsageCharts.mockResolvedValue({
      trend: [],
      models: [{ model: 'gpt-5', requests: 1, input_tokens: 10, output_tokens: 5, total_tokens: 15, actual_cost: 1.25 }],
      groups: [{ group_id: 7, group_name: 'Enterprise', requests: 1, total_tokens: 15, actual_cost: 1.25 }],
      endpoints: [],
    })

    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()
    ;(wrapper.vm as unknown as { activeTab: string }).activeTab = 'usage'
    await wrapper.vm.$nextTick()

    const vm = wrapper.vm as unknown as {
      usageFilters: { memberId: string; apiKeyId: string; model: string; groupId: string; billingType: string; billingMode: string }
      searchUsage: () => Promise<void>
    }
    vm.usageFilters.memberId = '42'
    vm.usageFilters.apiKeyId = '99'
    vm.usageFilters.model = 'gpt-5'
    vm.usageFilters.groupId = '7'
    vm.usageFilters.billingType = '1'
    vm.usageFilters.billingMode = 'token'
    expect(wrapper.get('[data-testid="organization-usage-details"]').classes()).toContain('card')
    expect(wrapper.get('[data-testid="organization-usage-filters"]').classes()).toContain('flex')
    expect(wrapper.get('[data-testid="organization-usage-model-filter"]').classes()).toContain('sm:min-w-[220px]')
    const modelFilter = wrapper.get('[data-testid="organization-usage-model-filter"]').findComponent(Select)
    expect(modelFilter.props('searchable')).toBe(true)
    expect(modelFilter.props('options')).toEqual(expect.arrayContaining([
      expect.objectContaining({ value: '', label: 'admin.usage.allModels' }),
      expect.objectContaining({ value: 'gpt-5', label: 'gpt-5' }),
    ]))
    expect(wrapper.find('input[placeholder="organization.usage.searchApiKeyPlaceholder"]').exists()).toBe(true)
    expect(wrapper.findAll('button').some(button => button.text() === 'common.search')).toBe(false)
    await vm.searchUsage()
    await flushPromises()

    expect(api.getUsage).toHaveBeenLastCalledWith(expect.objectContaining({ member_id: 42, api_key_id: 99, model: 'gpt-5', group_id: 7, billing_type: 1, billing_mode: 'token', page: 1 }))
    expect(wrapper.text()).toContain('member-key')
    expect(wrapper.text()).not.toContain('SECRET-UPSTREAM')
    expect(wrapper.text()).not.toContain('SECRET-COST')
    expect(wrapper.text()).not.toContain('SECRET-ERROR')
    expect(wrapper.text()).toContain('$1.250000')
    expect(wrapper.text()).toContain('organization.balanceSource.subscription')
    expect(wrapper.text()).toContain('Enterprise')
    expect(wrapper.text()).toContain('203.0.113.10')
    expect(wrapper.text()).toContain('test-agent')
    expect(wrapper.get('span[title="test-agent"]').classes()).toEqual(expect.arrayContaining(['text-sm', 'text-gray-600', 'max-w-[320px]', 'truncate']))
    const tokenCostDetail = wrapper.get('[data-testid="organization-usage-cost-detail-1"]')
    await tokenCostDetail.trigger('mouseenter')
    await wrapper.vm.$nextTick()
    expect(document.body.textContent).toContain('usage.inputTokenPrice')
    expect(document.body.textContent).toContain('usage.outputTokenPrice')
    await tokenCostDetail.trigger('mouseleave')
    const usageTable = wrapper.findAll('table').find(table => table.classes().includes('min-w-[1900px]'))
    expect(usageTable).toBeDefined()
    expect(usageTable!.element.parentElement?.classList.contains('overflow-x-auto')).toBe(true)
  })

	it('renders organization video results without a detail action and keeps numbered pagination', async () => {
		api.getContext.mockResolvedValue({
			organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
			finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' },
		})
		api.getUsage.mockResolvedValue({
			items: [{
				id: 88, member_user_id: 42, member_login: 'reader', member_username: '', api_key_name: 'video-key',
				model: 'bytedance/seedance-2.5/text-to-video', input_tokens: 0, output_tokens: 0,
				cache_creation_tokens: 0, cache_read_tokens: 0, cache_creation_5m_tokens: 0, cache_creation_1h_tokens: 0,
				actual_cost: '1.25', total_cost: '1.25', rate_multiplier: 1, endpoint: '/api/v1/model/video',
				group_name: 'Enterprise', request_type: 'sync', billing_type: 0, billing_mode: 'video', image_count: 0,
				video_count: 1, video_resolution: '1080p', video_duration_seconds: 17,
				image_urls: ['https://cdn.example.test/video.mp4'], cos_urls: [], ip_address: '', user_agent: '', status: 'charged',
				created_at: '2026-08-23T13:16:31Z', task_id: 17,
			}, {
				id: 89, member_user_id: 42, member_login: 'reader', member_username: '', api_key_name: 'image-key',
				model: 'gpt-image-2', input_tokens: 0, output_tokens: 0,
				cache_creation_tokens: 0, cache_read_tokens: 0, cache_creation_5m_tokens: 0, cache_creation_1h_tokens: 0,
				actual_cost: '0.40', total_cost: '0.40', rate_multiplier: 1, endpoint: '/v1/images/generations',
				group_name: 'Enterprise', request_type: 'sync', billing_type: 0, billing_mode: 'image', image_count: 2,
				image_size: '2K', image_size_source: 'output', image_urls: [], cos_urls: [], ip_address: '', user_agent: '', status: 'charged',
				created_at: '2026-08-23T13:15:31Z',
			}],
			total: 45, page: 1, page_size: 20, pages: 3,
		})
		await router.replace('/organization?tab=usage')

		const wrapper = mount(OrganizationConsoleView, mountOptions)
		await flushPromises()
		;(wrapper.vm as unknown as { activeTab: string }).activeTab = 'usage'
		await wrapper.vm.$nextTick()
		const videoResult = wrapper.get('[data-testid="organization-video-result-88-0"]')
		expect(videoResult.attributes('href')).toBe('https://cdn.example.test/video.mp4')
		expect(videoResult.get('video').attributes('src')).toBe('https://cdn.example.test/video.mp4')
		expect(wrapper.find('img[src="https://cdn.example.test/video.mp4"]').exists()).toBe(false)
		expect(wrapper.find('[data-testid="organization-video-detail-88"]').exists()).toBe(false)
		expect(api.getUsageVideoTask).not.toHaveBeenCalled()
		const videoCostDetail = wrapper.get('[data-testid="organization-usage-cost-detail-88"]')
		const imageCostDetail = wrapper.get('[data-testid="organization-usage-cost-detail-89"]')
		await videoCostDetail.trigger('mouseenter')
		await wrapper.vm.$nextTick()
		expect(document.body.textContent).toContain('1080p')
		expect(document.body.textContent).toContain('17s')
		await videoCostDetail.trigger('mouseleave')
		await imageCostDetail.trigger('mouseenter')
		await wrapper.vm.$nextTick()
		expect(document.body.textContent).toContain('2K')
		expect(wrapper.text()).toContain('$1.250000')
		expect(wrapper.text()).toContain('21:16:31')
		expect(wrapper.text()).not.toMatch(/\b(?:AM|PM)\b/i)
		expect(wrapper.text()).toContain('2')
		expect(wrapper.text()).toContain('3')
		wrapper.unmount()
	})

  it('shows total tokens with input, output, and cache details like admin usage', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' },
    })
    api.getUsageStats.mockResolvedValue({
      requests: 3,
      input_tokens: 100,
      output_tokens: 50,
      cache_creation_tokens: 12,
      cache_read_tokens: 22,
      total_tokens: 184,
      actual_cost: '1.25',
    })

    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()
    ;(wrapper.vm as unknown as { activeTab: string }).activeTab = 'usage'
    await wrapper.vm.$nextTick()

    const tokenCard = wrapper.get('[data-testid="usage-token-summary"]')
    expect(tokenCard.text()).toContain('usage.totalTokens')
    expect(tokenCard.text()).toContain('184')
    expect(tokenCard.text()).toContain('usage.in: 100')
    expect(tokenCard.text()).toContain('usage.out: 50')
    expect(tokenCard.text()).toContain('usage.cacheTotal: 34')
    expect(wrapper.get('[data-testid="usage-cache-detail"]').text()).toContain('12')
    expect(wrapper.get('[data-testid="usage-cache-detail"]').text()).toContain('22')
  })

  it('creates an IAM member with the company ID suffix and password options', async () => {
    api.getContext.mockResolvedValue({
      organization: { organization_id: 1, account_id: '1719905235756637', company_id: 'c123456789012345', company_name: 'Example', organization_status: 'active', membership_status: 'active', role: 'owner', actions: [] },
      finance: { balance_source: 'self', available: '100', frozen: '0', total: '100' }
    })
    const wrapper = mount(OrganizationConsoleView, mountOptions)
    await flushPromises()
    ;(wrapper.vm as unknown as { activeTab: string }).activeTab = 'finance'
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="create-iam-member"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="iam-principal-suffix"]').text()).toBe('@c123456789012345.opentk.ai')
    const mustChange = wrapper.get<HTMLInputElement>('[data-testid="must-change-password"]')
    expect(mustChange.element.checked).toBe(true)

    await wrapper.get('[data-testid="generate-iam-password"]').trigger('click')
    const passwordInput = wrapper.get<HTMLInputElement>('#iam-member-password')
    const generatedPassword = passwordInput.element.value
    expect(generatedPassword).toHaveLength(24)
    expect(passwordInput.attributes('type')).toBe('text')

    await wrapper.get('#iam-member-login-name').setValue('finance.reader')
    await wrapper.get('#iam-member-username').setValue('Finance Reader')
    await mustChange.setValue(false)
    api.createMember.mockResolvedValue({
      member: { principal: 'finance.reader@c123456789012345.opentk.ai' },
      initial_password: generatedPassword,
    })
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(api.createMember).toHaveBeenCalledWith('finance.reader', generatedPassword, false, undefined, 'Finance Reader')
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
    wrapper.findComponent(Select).vm.$emit('update:modelValue', '1-20')
    await flushPromises()
    await wrapper.get('[data-testid="company-upgrade-form"]').trigger('submit')
    await flushPromises()

    expect(api.submitApplication).toHaveBeenCalledWith('New Company', '1-20', expect.any(String))
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
