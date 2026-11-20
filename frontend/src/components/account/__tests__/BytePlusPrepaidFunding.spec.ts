import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import { createPinia } from 'pinia'
import BytePlusPrepaidFunding from '../BytePlusPrepaidFunding.vue'
import * as api from '@/api/admin/byteplusPrepaid'
import type { PrepaidStatus } from '@/api/admin/byteplusPrepaid'
import english from '@/i18n/locales/en/admin/byteplusPrepaid'

vi.mock('@/api/admin/byteplusPrepaid', () => ({ getStatus: vi.fn(), listPools: vi.fn(), update: vi.fn(), reconcile: vi.fn() }))
const model = 'dreamina-seedance-2-5-260628'
const empty = (): PrepaidStatus => ({ pool: null, policies: [{ model, name: 'Seedance 2.5', minimum_validity_seconds: 4200 }], credential_matches: true, grants: [], jobs: [], open_jobs: 0, bound_accounts: 0, credentials: 0 })
const paused = (): PrepaidStatus => ({ ...empty(), pool: { id: 'fixture-pool', name: 'Fixture funding', api_base: 'https://ark.ap-southeast.bytepluses.com/api/v3', enforced: true, frozen: true, safety_lock: false, reason: 'Initial setup', reconcile_after: null }, bound_accounts: 2, credentials: 1 })
function view() {
  return mount(BytePlusPrepaidFunding, { props: { accountId: 11, accountName: 'Fixture Ark' }, global: { plugins: [createPinia(), createI18n({ legacy: false, locale: 'en', messages: { en: { admin: english } } })] } })
}
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(api.getStatus).mockResolvedValue(empty())
  vi.mocked(api.listPools).mockResolvedValue([])
})

describe('BytePlus prepaid funding controls', () => {
  it('is opt-in and creates a paused pool only after explicit setup confirmation', async () => {
    vi.mocked(api.update).mockResolvedValue(paused())
    const wrapper = view()
    await flushPromises()
    expect(api.update).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="prepaid-mode"]').text()).toContain('not enrolled')
    expect(wrapper.get('[data-testid="prepaid-enable"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="prepaid-confirmed"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith(11, 'enable', { confirmed: true, pool_id: '', name: 'Fixture Ark' })
    expect(wrapper.get('[data-testid="prepaid-mode"]').text()).toContain('paused')
    expect(wrapper.get('[data-testid="prepaid-resume"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
  it('posts exact remaining tokens and keeps a committed grant visible without a second read', async () => {
    vi.mocked(api.getStatus).mockResolvedValue(paused())
    const credited = paused()
    credited.grants = [{ id: 'fixture-pack', model, capacity_tokens: '1234567.015625', spent_tokens: '0', reserved_tokens: '0', available_tokens: '1234567.015625', expires_at: '2030-01-01T23:59:59Z', eligible: true, needs_verification: false }]
    vi.mocked(api.update).mockResolvedValue(credited)
    vi.mocked(api.listPools).mockResolvedValueOnce([]).mockRejectedValue(new Error('later list read failed'))
    const wrapper = view()
    await flushPromises()
    await wrapper.get('[data-testid="prepaid-pack-id"]').setValue('fixture-pack')
    await wrapper.get('[data-testid="prepaid-model"]').setValue(model)
    await wrapper.get('[data-testid="prepaid-balance"]').setValue('1234.567015625')
    await wrapper.get('[data-testid="prepaid-unit"]').setValue('3')
    await wrapper.get('[data-testid="prepaid-expiry"]').setValue('2030-01-01T23:59:59')
    await wrapper.get('[data-testid="prepaid-confirmed"]').setValue(true)
    await wrapper.get('[data-testid="prepaid-grant-form"]').trigger('submit')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith(11, 'grant', expect.objectContaining({ pack_id: 'fixture-pack', model, remaining_tokens: '1234567.015625', expires_at: new Date('2030-01-01T23:59:59').toISOString() }))
    expect(wrapper.get('[data-testid="prepaid-grants"]').text()).toContain('1234567.015625')
    expect(wrapper.get('[data-testid="prepaid-grants"]').text()).toMatch(/\d{1,2}:\d{2}:\d{2}/)
    expect((wrapper.get('[data-testid="prepaid-pack-id"]').element as HTMLInputElement).value).toBe('')
    expect(api.listPools).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('retains past-deadline holds and never enables PAYG while a job is unsettled', async () => {
    const pending = paused()
    pending.open_jobs = 1
    pending.jobs = [{ id: 'fixture-reservation', model, state: 'pending', task_id: 'cgt-fixture', reserved_tokens: '470214.84375', ceiling_tokens: '470214.84375', actual_tokens: null, created_at: '2000-01-01T00:00:00Z', valid_until: '2000-01-01T01:10:00Z' }]
    vi.mocked(api.getStatus).mockResolvedValue(pending)
    const settled = paused()
    settled.jobs = [{ ...pending.jobs[0]!, state: 'succeeded', reserved_tokens: '0', actual_tokens: '66692.765625' }]
    vi.mocked(api.reconcile).mockResolvedValue(settled)
    const wrapper = view()
    await flushPromises()
    expect(api.reconcile).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="prepaid-jobs"]').text()).toContain('470214.84375')
    await wrapper.get('[data-testid="prepaid-allow-payg"]').setValue(true)
    expect(wrapper.get('[data-testid="prepaid-disable"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="prepaid-reconcile-fixture-reservation"]').trigger('click')
    await flushPromises()
    expect(api.reconcile).toHaveBeenCalledWith(11, 'fixture-reservation')
    expect(wrapper.get('[data-testid="prepaid-jobs"]').text()).toContain('66692.765625')
    expect(api.update).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('requires balance re-verification after a PAYG period and retains safety locks', async () => {
    const data = paused()
    data.pool!.reconcile_after = '2026-01-01T00:00:00Z'
    data.grants = [{ id: 'stale-pack', model, capacity_tokens: '1000000', spent_tokens: '100', reserved_tokens: '0', available_tokens: '999900', expires_at: '2030-01-01T00:00:00Z', eligible: true, needs_verification: true }]
    vi.mocked(api.getStatus).mockResolvedValue(data)
    const wrapper = view()
    await flushPromises()
    await wrapper.get('[data-testid="prepaid-confirmed"]').setValue(true)
    expect(wrapper.get('[data-testid="prepaid-resume"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('Recheck every unexpired pack')
    expect(api.update).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('ignores stale account responses when the editor switches accounts', async () => {
    let complete!: (value: PrepaidStatus) => void
    vi.mocked(api.getStatus).mockImplementationOnce(() => new Promise(resolve => { complete = resolve })).mockResolvedValueOnce({ ...paused(), pool: { ...paused().pool!, name: 'Second account' } })
    const wrapper = view()
    await wrapper.setProps({ accountId: 12 })
    await flushPromises()
    complete({ ...paused(), pool: { ...paused().pool!, name: 'Stale first account' } })
    await flushPromises()
    expect(wrapper.text()).toContain('Second account')
    expect(wrapper.text()).not.toContain('Stale first account')
    wrapper.unmount()
  })
})
