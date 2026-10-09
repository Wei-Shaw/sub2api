import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import RelayView from '../RelayView.vue'

const api = vi.hoisted(() => ({
  groups: vi.fn(), accounts: vi.fn(), earnings: vi.fn(), createAccount: vi.fn(), updateAccount: vi.fn(),
  createGroup: vi.fn(), updateGroup: vi.fn(), deleteGroup: vi.fn(), deleteAccount: vi.fn(),
  showError: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('@/api/relay', () => ({ relayApi: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: api.showError, showSuccess: api.showSuccess }) }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))

const privateGroup = { id: 1, name: 'u7: My relay', platform: 'composite', owner_user_id: 7, status: 'active' }
const publicGroup = { id: 2, name: 'Public pool', platform: 'openai', supplier_rate: 0.9, status: 'active' }
let wrapper: VueWrapper | undefined

const dialog = { props: ['show', 'title'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' }
function mountView() {
  wrapper = mount(RelayView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, BaseDialog: dialog, ConfirmDialog: true, RouterLink: { template: '<a><slot /></a>' } } } })
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  api.groups.mockResolvedValue({ private_groups: [privateGroup], public_groups: [publicGroup] })
  api.accounts.mockResolvedValue({ items: [], total: 0 })
  api.earnings.mockResolvedValue({ items: [], total: 0, total_earned: 0, today_earned: 0, pending_earned: 0 })
  api.createAccount.mockResolvedValue({ id: 3 })
  api.updateAccount.mockResolvedValue({ id: 3 })
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined })

describe('My relay', () => {
  it('connects an upstream privately and shares only groups explicitly selected', async () => {
    const view = mountView(); await flushPromises()
    await view.findAll('button').find(button => button.text() === 'relay.addAccount')!.trigger('click')
    await view.get('#relay-account-name').setValue('My upstream')
    await view.get('#relay-secret').setValue('test-secret')
    await view.get('#relay-base-url').setValue('https://upstream.example.com')
    await view.get('#relay-account-form').trigger('submit'); await flushPromises()
    expect(api.createAccount).toHaveBeenCalledWith(expect.objectContaining({
      name: 'My upstream', group_ids: [1], credentials: { base_url: 'https://upstream.example.com', api_key: 'test-secret' }
    }))
    expect(api.createAccount.mock.calls[0][0].group_ids).not.toContain(2)
  })

  it('keeps the existing credential when editing without a new secret and can stop sharing', async () => {
    api.accounts.mockResolvedValue({ items: [{ id: 3, name: 'Shared account', platform: 'openai', type: 'apikey', concurrency: 3, status: 'active', group_ids: [1, 2], credentials: { base_url: 'https://upstream.example.com' } }], total: 1 })
    const view = mountView(); await flushPromises()
    await view.findAll('button').filter(button => button.text() === 'common.edit')[1].trigger('click')
    const publicBox = view.findAll('#relay-account-form input[type="checkbox"]')[1]
    await publicBox.setValue(false)
    await view.get('#relay-account-form').trigger('submit'); await flushPromises()
    expect(api.updateAccount).toHaveBeenCalledWith(3, expect.objectContaining({ group_ids: [1], credentials: { base_url: 'https://upstream.example.com' } }))
    expect(api.updateAccount.mock.calls[0][1].credentials).not.toHaveProperty('api_key')
  })

  it('rejects invalid credential JSON before calling the API', async () => {
    const view = mountView(); await flushPromises()
    await view.findAll('button').find(button => button.text() === 'relay.addAccount')!.trigger('click')
    await view.get('#relay-credentials').setValue('[]')
    await view.get('#relay-account-form').trigger('submit'); await flushPromises()
    expect(api.showError).toHaveBeenCalledWith('relay.invalidJson')
    expect(api.createAccount).not.toHaveBeenCalled()
  })

  it('shows self-use income and pending earnings separately', async () => {
    api.earnings.mockResolvedValue({ total: 1, total_earned: 0.9, today_earned: 0.9, pending_earned: 1,
      items: [{ id: 1, account_id: 3, group_id: 2, charged_amount: 1, amount: 0.9, supplier_rate: 0.9, self_use: true, created_at: '2026-10-09T00:00:00Z', credited_at: null }] })
    const view = mountView(); await flushPromises()
    expect(view.text()).toContain('relay.selfUse')
    expect(view.text()).toContain('relay.pendingNotice')
    expect(view.text()).toContain('90%')
  })
})
