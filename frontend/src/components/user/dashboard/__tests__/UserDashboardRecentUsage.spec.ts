import { describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import UserDashboardRecentUsage from '../UserDashboardRecentUsage.vue'

describe('UserDashboardRecentUsage', () => {
  it('links the empty state to API keys so a new user knows where to start', () => {
    const wrapper = mount(UserDashboardRecentUsage, {
      props: { data: [], loading: false },
      global: { stubs: { RouterLink: RouterLinkStub, Icon: true } },
    })

    expect(wrapper.text()).toContain('dashboard.noUsageRecords')
    const cta = wrapper.findAllComponents(RouterLinkStub).find(link => link.props('to') === '/keys')
    expect(cta?.text()).toBe('dashboard.createApiKey')
  })
})
