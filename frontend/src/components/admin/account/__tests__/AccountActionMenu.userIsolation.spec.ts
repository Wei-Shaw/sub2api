import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountActionMenu from '../AccountActionMenu.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

function account(enabled: boolean): Account {
  return {
    id: 7,
    name: 'risk-account',
    platform: 'openai',
    type: 'apikey',
    status: 'active',
    schedulable: true,
    credentials: {},
    extra: enabled ? { user_isolation_enabled: true } : {},
    concurrency: 1,
    priority: 0,
    created_at: '2026-08-22T00:00:00Z',
    updated_at: '2026-08-22T00:00:00Z'
  } as Account
}

const RouterLinkStub = {
  props: ['to'],
  emits: ['click'],
  template: '<a href="#" @click.prevent="$emit(\'click\')"><slot /></a>'
}

describe('AccountActionMenu user isolation shortcut', () => {
  it('links enabled accounts to the risk user lookup page', async () => {
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account: account(true), position: { top: 10, left: 10 } },
      global: {
        stubs: {
          Teleport: { template: '<div><slot /></div>' },
          RouterLink: RouterLinkStub,
          Icon: true
        }
      }
    })

    const link = wrapper.findComponent(RouterLinkStub)
    expect(link.exists()).toBe(true)
    expect(link.props('to')).toEqual({ path: '/admin/user-isolation', query: { account_id: '7' } })
    await link.trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('hides the shortcut when user isolation is disabled', () => {
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account: account(false), position: { top: 10, left: 10 } },
      global: {
        stubs: {
          Teleport: { template: '<div><slot /></div>' },
          RouterLink: RouterLinkStub,
          Icon: true
        }
      }
    })

    expect(wrapper.findComponent(RouterLinkStub).exists()).toBe(false)
  })
})
