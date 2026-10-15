import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, inject } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { accountWorkspaceKey } from '@/composables/useAccountWorkspace'
import RelayView from '../RelayView.vue'

const api = vi.hoisted(() => ({
  groups: vi.fn(), accounts: vi.fn(), earnings: vi.fn(), createGroup: vi.fn(), updateGroup: vi.fn(),
  deleteGroup: vi.fn(), deleteAccount: vi.fn(), showError: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('@/api/relay', () => ({ relayApi: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: api.showError, showSuccess: api.showSuccess }) }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))

const privateGroup = { id: 1, name: 'u7: My relay', platform: 'composite', owner_user_id: 7, status: 'active' }
const publicGroup = { id: 2, name: 'Public pool', platform: 'openai', supplier_rate: 0.9, status: 'active' }
const account = { id: 3, name: 'My Codex', platform: 'openai', type: 'oauth', concurrency: 3, status: 'active', group_ids: [1, 2], credentials: {} }
let wrapper: VueWrapper | undefined
const sharedForm = (name: string) => defineComponent({
  name,
  props: ['show', 'account', 'groups', 'initialGroupIds', 'proxies'],
  setup: () => ({ scope: inject(accountWorkspaceKey)?.basePath }),
  template: '<div v-if="show" :data-scope="scope"><slot /></div>'
})

function mountView() {
  wrapper = mount(RelayView, { global: { stubs: {
    AppLayout: { template: '<div><slot /></div>' },
    BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
    CreateAccountModal: sharedForm('CreateAccountModal'), EditAccountModal: sharedForm('EditAccountModal'), ReAuthAccountModal: sharedForm('ReAuthAccountModal'),
    AccountTestModal: sharedForm('AccountTestModal'), AccountStatsModal: sharedForm('AccountStatsModal'), AccountStatusIndicator: true,
    ConfirmDialog: true, RouterLink: { template: '<a><slot /></a>' }
  } } })
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  api.groups.mockResolvedValue({ private_groups: [privateGroup], public_groups: [publicGroup] })
  api.accounts.mockResolvedValue({ items: [account], total: 1 })
  api.earnings.mockResolvedValue({ items: [], total: 0, total_earned: 0, today_earned: 0, pending_earned: 0 })
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined })

async function click(view: VueWrapper, text: string) {
  await view.findAll('button').find(button => button.text() === text)!.trigger('click')
}

describe('My relay shared account flows', () => {
  it('uses the Admin creation component, with a user scope and only a private default group', async () => {
    const view = mountView(); await flushPromises()
    await click(view, 'relay.addAccount')
    const form = view.findComponent({ name: 'CreateAccountModal' })
    expect(form.props('show')).toBe(true)
    expect(form.props('initialGroupIds')).toEqual([1])
    expect(form.props('groups')).toEqual([privateGroup, publicGroup])
    expect(form.props('proxies')).toEqual([])
    expect(view.get('[data-scope]').attributes('data-scope')).toBe('/relay')
  })

  it('reuses editing and reauthorization without switching to an Admin scope', async () => {
    const view = mountView(); await flushPromises()
    await view.findAll('button').filter(button => button.text() === 'common.edit')[1].trigger('click')
    expect(view.findComponent({ name: 'EditAccountModal' }).props('account')).toEqual(account)
    await click(view, 'admin.accounts.reAuthorizeAccount')
    expect(view.findComponent({ name: 'ReAuthAccountModal' }).props('account')).toEqual(account)
    expect(view.findAll('[data-scope]').every(node => node.attributes('data-scope') === '/relay')).toBe(true)
  })

  it('opens shared testing and statistics for the selected owned account', async () => {
    const view = mountView(); await flushPromises()
    await click(view, 'admin.accounts.testConnection')
    expect(view.findComponent({ name: 'AccountTestModal' }).props('account')).toEqual(account)
    await click(view, 'admin.accounts.viewStats')
    expect(view.findComponent({ name: 'AccountStatsModal' }).props('account')).toEqual(account)
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
