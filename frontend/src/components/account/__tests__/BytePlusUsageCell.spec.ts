import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BytePlusUsageCell from '../BytePlusUsageCell.vue'
import * as api from '@/api/admin/byteplusPrepaid'
import type { BillingStatus, PrepaidStatus } from '@/api/admin/byteplusPrepaid'
import english from '@/i18n/locales/en/admin/byteplusPrepaid'
vi.mock('@/api/admin/byteplusPrepaid', () => ({ getStatus: vi.fn(), providerStatus: vi.fn(), queryProvider: vi.fn() }))
const model = 'dreamina-seedance-2-5-260628'
const local = (): PrepaidStatus => ({ observed_at: '2030-01-01T00:00:00Z', pool: { id: 'pool', name: 'Shared fixture pool', api_base: 'https://ark.ap-southeast.bytepluses.com/api/v3', enforced: true, frozen: false, safety_lock: false, reason: '', reconcile_after: null }, policies: [{ model, name: 'Seedance fixture', minimum_validity_seconds: 4200 }], credential_matches: true, grants: [{ id: 'fixture-pack', model, capacity_tokens: '2000000', spent_tokens: '0', reserved_tokens: '1.015625', available_tokens: '1999998.984375', expires_at: '2030-01-02T00:00:00Z', verified_at: '2029-12-31T00:00:00Z', eligible: true, eligibility_reason: 'eligible', needs_verification: false }], jobs: [], open_jobs: 1, bound_accounts: 2, credentials: 1 })
const billing = (): BillingStatus => ({ configured: false, setup: 'missing_credentials', balance: { state: 'never_queried', observed_at: null, attempted_at: null, data: null }, packages: { state: 'never_queried', observed_at: null, attempted_at: null, data: null }, coupons: { state: 'never_queried', observed_at: null, attempted_at: null, data: null } })
function view() { return mount(BytePlusUsageCell, { props: { accountId: 11 }, global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: { admin: english } } })] } }) }
beforeEach(() => { vi.resetAllMocks(); vi.mocked(api.getStatus).mockResolvedValue(local()); vi.mocked(api.providerStatus).mockResolvedValue(billing()); vi.mocked(api.queryProvider).mockResolvedValue(billing()) })
afterEach(() => { vi.useRealTimers() })
const button = (wrapper: ReturnType<typeof view>, text: string) => wrapper.findAll('button').find(b => b.text() === text)!
describe('BytePlus local funding and provider query', () => {
  it('shows exact local status, refresh and missing-credential guidance without automatic provider queries', async () => {
    const w = view(); await flushPromises()
    expect(w.text()).toContain('1999998.984375'); expect(w.text()).toContain('1.015625')
    expect(w.get('[data-testid="byteplus-setup"]').text()).toContain('not configured')
    expect(api.queryProvider).not.toHaveBeenCalled()
    await button(w, 'Query BytePlus').trigger('click'); await flushPromises()
    expect(api.queryProvider).toHaveBeenCalledWith(11)
    expect(w.text()).toContain('BYTEPLUS_BILLING_CONFIG_FILE'); expect(w.text()).toContain('inference API key cannot query billing')
    await button(w, 'Refresh local status').trigger('click'); await flushPromises()
    expect(api.getStatus).toHaveBeenCalledTimes(2); expect(api.queryProvider).toHaveBeenCalledTimes(1); w.unmount()
  })
  it('retains dated local data after refresh failure and never changes it into zero', async () => {
    const w = view(); await flushPromises(); vi.mocked(api.getStatus).mockRejectedValue(new Error('fixture failure'))
    await button(w, 'Refresh local status').trigger('click'); await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('stale'); expect(w.text()).toContain('1999998.984375'); w.unmount()
  })
  it('shows provider observations and restrictions separately without altering local funding', async () => {
    const b = billing(); b.configured = true; b.setup = 'ready'; b.balance = { state: 'ok', observed_at: '2030-01-01T00:00:00Z', attempted_at: '2030-01-01T00:00:00Z', data: { AvailableBalance: '12.50', Currency: 'USD' } }; b.packages = { ...b.balance, state: 'forbidden', data: [{ InstanceNo: 'provider-pack', AvailableAmount: '4.015625', Unit: 'Token', ExpiryTime: '2030-01-02T00:00:00Z' }] }; b.coupons = { ...b.balance, data: [{ CouponID: 'coupon', RemainingAmount: '3.00', ExpiredTime: '2030-01-02T00:00:00Z', ProductLimitList: [{ ProductCode: 'ark', BillingMethod: 'postpaid' }] }] }
    vi.mocked(api.queryProvider).mockResolvedValue(b)
    const w = view(); await flushPromises(); await button(w, 'Query BytePlus').trigger('click'); await flushPromises()
    expect(w.text()).toContain('12.50 USD'); expect(w.text()).toContain('provider-pack'); expect(w.text()).toContain('permission denied'); expect(w.text()).toContain('postpaid'); expect(w.text()).toContain('1999998.984375'); w.unmount()
  })
  it('excludes expired and paused grants with positive remainder', async () => {
    const s = local(); s.grants[0]!.eligible = false; s.grants[0]!.eligibility_reason = 'expired'; vi.mocked(api.getStatus).mockResolvedValue(s)
    const w = view(); await flushPromises(); expect(w.get('[data-testid="byteplus-model"]').text()).toContain('Spendable tokens: 0'); expect(w.text()).toContain('1.015625'); w.unmount()
  })
  it('invalidates spendable funding at the validity boundary without a network request', async () => {
    vi.useFakeTimers(); const s = local(); s.grants[0]!.expires_at = '2030-01-01T01:10:01Z'; vi.mocked(api.getStatus).mockResolvedValue(s)
    const w = view(); await flushPromises(); expect(w.text()).toContain('1999998.984375'); await vi.advanceTimersByTimeAsync(1002)
    expect(w.get('[data-testid="byteplus-model"]').text()).toContain('Spendable tokens: 0'); expect(api.getStatus).toHaveBeenCalledTimes(1); w.unmount()
  })
  it('ignores a provider result after switching accounts', async () => {
    let resolve!: (b: BillingStatus) => void; vi.mocked(api.queryProvider).mockImplementation(() => new Promise(r => { resolve = r }))
    const w = view(); await flushPromises(); await button(w, 'Query BytePlus').trigger('click'); await w.setProps({ accountId: 12 }); await flushPromises()
    const b = billing(); b.setup = 'invalid_configuration'; resolve(b); await flushPromises(); expect(w.text()).not.toContain('permissions are too broad'); expect(api.getStatus).toHaveBeenLastCalledWith(12); w.unmount()
  })
})
