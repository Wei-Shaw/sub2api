import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UpstreamBillingView from '../UpstreamBillingView.vue'

const { api, state, showSuccess } = vi.hoisted(() => ({
  api: { report: vi.fn(), connections: vi.fn(), accounts: vi.fn(), save: vi.fn(), sync: vi.fn(), source: vi.fn() },
  state: { isAdmin: false },
  showSuccess: vi.fn()
}))
vi.mock('@/api/upstreamBilling', () => ({ upstreamBillingAPI: api }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => state }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError: vi.fn() }) }))
vi.mock('vue-router', async importOriginal => ({ ...await importOriginal<typeof import('vue-router')>(), useRoute: () => ({ query: { month: '2026-09' } }) }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))

const allocation = {
  connection_id: 1, connection_name: 'Private connection', provider: 'azure', resource_id: 'private-resource',
  account_id: 2, account_name: 'Private account', api_key_id: 3, api_key_name: 'My virtual key', user_id: 4,
  requests: 7, tokens: 123, local_cost: '1.50000000', allocated_cost: '2.34000000', currency: 'USD',
  method: 'local_cost_weighted', period_start: '2026-09-01T00:00:00Z', period_end: '2026-10-01T00:00:00Z'
}
const report = {
  month: '2026-09', is_admin: false, items: [allocation],
  totals: [{ currency: 'USD', official_cost: '99', allocated_cost: '2.34', unmatched_cost: '96.66' }, { currency: 'CNY', allocated_cost: '8.76' }],
  bills: [{ connection_id: 1, resource_id: 'private-resource', description: 'Private bill', amount: '99', currency: 'USD', period_start: allocation.period_start, period_end: allocation.period_end }]
}
const connection = {
  id: 1, name: 'Private connection', provider: 'azure', settings: { tenant_id: 'tenant', client_id: 'client', subscription_id: 'subscription', resource_id: 'resource' },
  has_credentials: true, enabled: true, sync_interval_hours: 24, bindings: []
}

function render() {
  return mount(UpstreamBillingView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        Icon: true, Select: true, Toggle: true, Pagination: true
      }
    }
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  state.isAdmin = false
  api.report.mockResolvedValue(report)
  api.connections.mockResolvedValue([connection])
  api.accounts.mockResolvedValue([{ id: 2, name: 'Private account', platform: 'openai' }])
  api.save.mockResolvedValue(connection)
  api.sync.mockResolvedValue(undefined)
  api.source.mockResolvedValue({ id: 10, source_amount: '99.000000001', raw_source: { description: '<img src=x onerror=alert(1)>', cost: '99.000000001' } })
})

describe('UpstreamBillingView', () => {
  it('shows only own-key allocations to members and never fetches admin configuration', async () => {
    const wrapper = render()
    await flushPromises()
    expect(api.report).toHaveBeenCalledWith('2026-09', expect.any(AbortSignal))
    expect(api.connections).not.toHaveBeenCalled()
    expect(api.accounts).not.toHaveBeenCalled()
    expect(api.source).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('My virtual key')
    expect(wrapper.text()).toContain('USD 2.34')
    expect(wrapper.text()).toContain('CNY')
    expect(wrapper.text()).not.toContain('Private connection')
    expect(wrapper.text()).not.toContain('Private account')
    expect(wrapper.text()).not.toContain('private-resource')
    expect(wrapper.text()).not.toContain('Private bill')
    expect(wrapper.text()).not.toContain('upstreamBilling.official')
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('never prefills stored secrets and omits blank credentials when editing', async () => {
    state.isAdmin = true
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    const secret = wrapper.get<HTMLInputElement>('#secret-client_secret')
    expect(secret.element.value).toBe('')
    expect(secret.attributes('required')).toBeUndefined()
    await wrapper.get('#upstream-connection-form').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(expect.objectContaining({ secrets: {}, provider: 'azure' }), 1)
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('syncs the selected month and refreshes imported bills', async () => {
    state.isAdmin = true
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('upstreamBilling.sync'))!.trigger('click')
    await flushPromises()
    expect(api.sync).toHaveBeenCalledWith(1, '2026-09')
    expect(api.report).toHaveBeenCalledTimes(2)
    expect(showSuccess).toHaveBeenCalledWith('upstreamBilling.syncSuccess')
    wrapper.unmount()
  })

  it('separates raw bills from aligned bills and displays raw JSON as escaped text', async () => {
    state.isAdmin = true
    api.report.mockResolvedValueOnce({
      ...report,
      bills: [{ ...report.bills[0], source_id: 10, has_raw_source: true, source_amount: '99.000000001' }]
    })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('#billing-panel-aligned').exists()).toBe(true)
    expect(wrapper.find('#billing-panel-raw').exists()).toBe(false)
    await wrapper.get('#billing-tab-raw').trigger('click')
    expect(wrapper.find('#billing-panel-aligned').exists()).toBe(false)
    expect(wrapper.get('#billing-panel-raw').text()).toContain('USD 99.000000001')
    expect(api.source).not.toHaveBeenCalled()
    await wrapper.findAll('button').find(button => button.text() === 'upstreamBilling.viewRawData')!.trigger('click')
    await flushPromises()
    expect(api.source).toHaveBeenCalledWith(10, expect.any(AbortSignal))
    expect(wrapper.get('#billing-raw-json').text()).toContain('<img src=x onerror=alert(1)>')
    expect(wrapper.find('pre img').exists()).toBe(false)
    await wrapper.get('#billing-tab-evidence').trigger('click')
    expect(wrapper.get('#billing-panel-evidence').text()).toContain('upstreamBilling.tokenStatuses.unavailable')
    wrapper.unmount()
    expect((api.source.mock.calls[0][1] as AbortSignal).aborted).toBe(true)
  })

  it('shows zero differences explicitly and leaves unavailable comparison counts blank', async () => {
    state.isAdmin = true
    api.report.mockResolvedValueOnce({
      ...report,
      bills: [
        { ...report.bills[0], usage: { model: 'claude', token_type: 'output', tokens: 42 }, local_matched_tokens: 42, token_difference: 0, token_status: 'verified' },
        { ...report.bills[0], token_status: 'local_category_unverified' }
      ]
    })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#billing-tab-evidence').trigger('click')
    const rows = wrapper.findAll('#billing-panel-evidence tbody tr')
    expect(rows[0].findAll('td')[5].text()).toBe('0')
    expect(rows[1].findAll('td')[5].text()).toBe('—')
    expect(rows[1].text()).toContain('upstreamBilling.tokenStatuses.local_category_unverified')
    wrapper.unmount()
  })

  it('aborts an old report when the month changes and clears stale totals on failure', async () => {
    const wrapper = render()
    await flushPromises()
    const signal = api.report.mock.calls[0][1] as AbortSignal
    api.report.mockRejectedValueOnce(new Error('failed'))
    await wrapper.get('#billing-month').setValue('2026-08')
    await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(wrapper.text()).not.toContain('USD 2.34')
    expect(wrapper.get('[role="alert"]').text()).toContain('failed')
    wrapper.unmount()
  })

  it('preserves configured policy and lookback and validates the numeric window', async () => {
    state.isAdmin = true
    api.connections.mockResolvedValueOnce([{ ...connection, sync_lookback_months: 5, allocation_mode: 'official_only' }])
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    expect(wrapper.get<HTMLInputElement>('#sync-lookback').element.value).toBe('5')
    await wrapper.get('#sync-lookback').setValue('7')
    await wrapper.get('#upstream-connection-form').trigger('submit')
    expect(api.save).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('upstreamBilling.invalidSyncSettings')
    await wrapper.get('#sync-lookback').setValue('6')
    await wrapper.get('#upstream-connection-form').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(expect.objectContaining({ sync_lookback_months: 6, allocation_mode: 'official_only' }), 1)
    wrapper.unmount()
  })

  it.each(['aliyun', 'volcengine'])('accepts encrypted service credentials for %s without a model-key field', async provider => {
    state.isAdmin = true
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('upstreamBilling.addConnection'))!.trigger('click')
    const selector = wrapper.findAllComponents({ name: 'Select' }).find(component => component.attributes('id') === 'connection-provider')!
    expect(selector.props('options').map((option: { value: string }) => option.value)).not.toContain('deepseek')
    selector.vm.$emit('update:modelValue', provider)
    selector.vm.$emit('change', provider)
    await flushPromises()
    await wrapper.get('#connection-name').setValue('China production')
    await wrapper.get('#setting-account_id').setValue('12345678901234567890')
    await wrapper.get('#setting-product_code').setValue('official-product')
    expect(wrapper.get('#setting-account_id').attributes('readonly')).toBeUndefined()
    expect(wrapper.get('#setting-product_code').attributes('readonly')).toBeUndefined()
    await wrapper.get('#secret-access_key_id').setValue('billing-id')
    await wrapper.get('#secret-access_key_secret').setValue('billing-secret')
    expect(wrapper.get('#secret-access_key_id').attributes('type')).toBe('password')
    expect(wrapper.get('#secret-access_key_secret').attributes('type')).toBe('password')
    expect(wrapper.find('#secret-admin_api_key').exists()).toBe(false)
    await wrapper.get('#upstream-connection-form').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(expect.objectContaining({
      provider, settings: { account_id: '12345678901234567890', product_code: 'official-product' },
      secrets: { access_key_id: 'billing-id', access_key_secret: 'billing-secret' },
      sync_lookback_months: 2, allocation_mode: 'local_weighted'
    }), undefined)
    wrapper.unmount()
  })

  it.each(['aliyun', 'volcengine'])('locks the existing %s billing scope without locking credential rotation', async provider => {
    state.isAdmin = true
    api.connections.mockResolvedValueOnce([{ ...connection, provider, settings: { account_id: '123456', product_code: 'official-product' } }])
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    expect(wrapper.get('#setting-account_id').attributes('readonly')).toBeDefined()
    expect(wrapper.get('#setting-product_code').attributes('readonly')).toBeDefined()
    expect(wrapper.get('#secret-access_key_id').attributes('readonly')).toBeUndefined()
    expect(wrapper.get('#secret-access_key_secret').attributes('readonly')).toBeUndefined()
    expect(wrapper.get('#sync-lookback').attributes('readonly')).toBeUndefined()
    expect(wrapper.text()).toContain('upstreamBilling.immutableScopeHint')
    await wrapper.get('#secret-access_key_secret').setValue('rotated-secret')
    await wrapper.get('#upstream-connection-form').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(expect.objectContaining({
      settings: { account_id: '123456', product_code: 'official-product' },
      secrets: { access_key_secret: 'rotated-secret' }
    }), 1)
    wrapper.unmount()
  })

  it('filters raw and evidence rows consistently including tiny negative unmatched amounts', async () => {
    state.isAdmin = true
    api.report.mockResolvedValueOnce({
      ...report,
      bills: [
        { ...report.bills[0], resource_id: 'resource-A', allocated_cost: '99', unmatched_cost: '0', token_status: 'verified' },
        { ...report.bills[0], resource_id: 'resource-B', amount: '-0.000000000001', allocated_cost: '0', unmatched_cost: '-0.000000000001', token_status: 'not_checked' }
      ]
    })
    const wrapper = render()
    await flushPromises()
    const selector = wrapper.findAllComponents({ name: 'Select' }).find(component => component.attributes('id') === 'billing-filter-allocation')!
    selector.vm.$emit('update:modelValue', 'unmatched')
    await wrapper.get('#billing-tab-raw').trigger('click')
    expect(wrapper.findAll('#billing-panel-raw tbody tr')).toHaveLength(1)
    expect(wrapper.get('#billing-panel-raw tbody').text()).toContain('resource-B')
    expect(wrapper.get('#billing-panel-raw tbody').text()).toContain('-0.000000000001')
    expect(wrapper.get('#billing-panel-raw tbody').text()).not.toContain('resource-A')
    await wrapper.get('#billing-tab-evidence').trigger('click')
    expect(wrapper.findAll('#billing-panel-evidence tbody tr')).toHaveLength(1)
    expect(wrapper.get('#billing-panel-evidence tbody').text()).toContain('upstreamBilling.tokenStatuses.not_checked')
    await wrapper.get('#billing-filter-resource').setValue('missing')
    expect(wrapper.get('#billing-panel-evidence tbody').text()).toContain('upstreamBilling.noFilterResults')
    wrapper.unmount()
  })

  it('shows unmatched row amounts separately from allocated amounts', async () => {
    state.isAdmin = true
    api.report.mockResolvedValueOnce({ ...report, items: [{ ...allocation, method: 'unmatched', allocated_cost: '-1.23', tokens: 0 }] })
    const wrapper = render()
    await flushPromises()
    const cells = wrapper.findAll('#billing-panel-aligned tbody tr')[0].findAll('td')
    expect(cells[5].text()).toBe('—')
    expect(cells[6].text()).toBe('USD -1.23')
    expect(wrapper.text()).toContain('upstreamBilling.weightWarning')
    wrapper.unmount()
  })
})
