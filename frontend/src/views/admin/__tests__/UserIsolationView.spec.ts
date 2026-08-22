import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UserIsolationView from '../UserIsolationView.vue'

const { getAccount, listAccounts, lookup, push, routeQuery } = vi.hoisted(() => ({
  getAccount: vi.fn(),
  listAccounts: vi.fn(),
  lookup: vi.fn(),
  push: vi.fn(),
  routeQuery: {} as Record<string, string | undefined>
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { getById: getAccount, list: listAccounts },
    userIsolation: { lookup }
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: routeQuery }),
  useRouter: () => ({ push })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const account = {
  id: 7,
  name: 'risk-account',
  platform: 'openai',
  type: 'apikey',
  status: 'active',
  schedulable: true,
  credentials: {},
  extra: { user_isolation_enabled: true },
  concurrency: 1,
  priority: 0,
  created_at: '2026-08-22T00:00:00Z',
  updated_at: '2026-08-22T00:00:00Z'
}

function mountView() {
  return mount(UserIsolationView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Select: {
          props: ['modelValue', 'options'],
          template: '<div data-test="account-select-stub">{{ modelValue }}</div>'
        },
        Icon: true
      }
    }
  })
}

describe('UserIsolationView', () => {
  beforeEach(() => {
    routeQuery.account_id = '7'
    getAccount.mockReset().mockResolvedValue(account)
    listAccounts.mockReset().mockResolvedValue({ items: [], total: 0, pages: 0 })
    lookup.mockReset().mockResolvedValue({
      account: { id: 7, name: 'risk-account', platform: 'openai', type: 'apikey' },
      user: { id: 42, email: 'risk@example.com', username: 'risk', status: 'active' }
    })
    push.mockReset()
  })

  it('prefills the routed account and renders an exact user match', async () => {
    const wrapper = mountView()
    await flushPromises()

    const isolationID = `u1_${'A'.repeat(43)}`
    await wrapper.get('[data-test="isolation-id"]').setValue(isolationID)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(getAccount).toHaveBeenCalledWith(7)
    expect(lookup).toHaveBeenCalledWith({ account_id: 7, isolation_id: isolationID })
    expect(wrapper.get('[data-test="result"]').text()).toContain('risk@example.com')
  })

  it('links the matched user to management and usage records', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="isolation-id"]').setValue(`u1_${'A'.repeat(43)}`)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    await wrapper.get('[data-test="view-user"]').trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/admin/users', query: { search: 'risk@example.com' } })

    await wrapper.get('[data-test="view-usage"]').trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/admin/usage', query: { user_id: '42' } })
  })
})
