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
          props: ['modelValue', 'options', 'disabled'],
          template: '<button type="button" :disabled="disabled">{{ modelValue }}:{{ options.length }}</button>'
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

  it('looks up across all enabled accounts when no account is selected', async () => {
    routeQuery.account_id = undefined
    const wrapper = mountView()
    await flushPromises()

    const isolationID = `u1_${'B'.repeat(43)}`
    await wrapper.get('[data-test="isolation-id"]').setValue(isolationID)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(getAccount).not.toHaveBeenCalled()
    expect(listAccounts).toHaveBeenCalled()
    expect(lookup).toHaveBeenCalledWith({ isolation_id: isolationID })
  })

  it('continues through account pages until it finds enabled accounts', async () => {
    routeQuery.account_id = undefined
    const disabledAccounts = Array.from({ length: 100 }, (_, index) => ({
      ...account,
      id: index + 1,
      extra: {}
    }))
    listAccounts
      .mockResolvedValueOnce({ items: disabledAccounts, total: 101, pages: 2 })
      .mockResolvedValueOnce({ items: [{ ...account, id: 101 }], total: 101, pages: 2 })

    const wrapper = mountView()
    await flushPromises()

    expect(listAccounts).toHaveBeenNthCalledWith(1, 1, 100, { search: undefined, lite: 'true' })
    expect(listAccounts).toHaveBeenNthCalledWith(2, 2, 100, { search: undefined, lite: 'true' })
    expect(wrapper.get('[data-test="account-select"]').text()).toBe(':1')
  })

  it('locks lookup inputs until the request finishes', async () => {
    let resolveLookup!: (value: Awaited<ReturnType<typeof lookup>>) => void
    lookup.mockReturnValueOnce(new Promise(resolve => {
      resolveLookup = resolve
    }))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="isolation-id"]').setValue(`u1_${'A'.repeat(43)}`)

    await wrapper.get('form').trigger('submit')
    expect(wrapper.get('[data-test="account-select"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="isolation-id"]').attributes('disabled')).toBeDefined()

    resolveLookup({
      account: { id: 7, name: 'risk-account', platform: 'openai', type: 'apikey' },
      user: { id: 42, email: 'risk@example.com', username: 'risk', status: 'active' }
    })
    await flushPromises()

    expect(wrapper.get('[data-test="account-select"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-test="isolation-id"]').attributes('disabled')).toBeUndefined()
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
