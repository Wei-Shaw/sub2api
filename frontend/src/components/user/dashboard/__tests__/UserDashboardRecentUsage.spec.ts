import { describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import UserDashboardRecentUsage from '../UserDashboardRecentUsage.vue'
import type { UsageLog } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('UserDashboardRecentUsage', () => {
  it('费用和 token 字段缺失时使用安全兜底值', () => {
    const wrapper = mount(UserDashboardRecentUsage, {
      props: {
        data: [
          {
            id: 1,
            model: 'gpt-5.5',
            created_at: '2026-06-20T12:00:00Z',
          } as UsageLog,
        ],
        loading: false,
      },
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
          Icon: {
            template: '<span />',
          },
        },
      },
    })

    expect(wrapper.text()).toContain('$0.0000 / $0.0000')
    expect(wrapper.text()).toContain('0 tokens')
  })
})
