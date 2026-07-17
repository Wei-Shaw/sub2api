import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import type { AdminGroup } from '@/types'
import GroupAutoRoutingFields from '../GroupAutoRoutingFields.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const group = (overrides: Partial<AdminGroup>): AdminGroup =>
  ({
    id: 1,
    name: 'balance-group',
    platform: 'openai',
    subscription_type: 'standard',
    status: 'active',
    rate_multiplier: 1,
    routing_mode: 'fixed',
    auto_candidate_group_ids: [],
    ...overrides
  }) as AdminGroup

describe('GroupAutoRoutingFields', () => {
  it('shows eligible balance candidates across platforms', () => {
    const wrapper = mount(GroupAutoRoutingFields, {
      props: {
        candidateGroupIds: [],
        groups: [
          group({ id: 1, name: 'eligible' }),
          group({ id: 2, name: 'subscription', subscription_type: 'subscription' }),
          group({ id: 3, name: 'other-platform', platform: 'anthropic' }),
          group({ id: 4, name: 'exclusive', is_exclusive: true })
        ]
      }
    })

    expect(wrapper.text()).toContain('eligible')
    expect(wrapper.text()).not.toContain('subscription')
    expect(wrapper.text()).toContain('other-platform')
    expect(wrapper.text()).not.toContain('exclusive')
  })

  it('updates selected candidates', async () => {
    const wrapper = mount(GroupAutoRoutingFields, {
      props: {
        candidateGroupIds: [1],
        groups: [group({ id: 1 })]
      }
    })

    await wrapper.get('input[type="checkbox"]').setValue(false)

    expect(wrapper.emitted('update:candidateGroupIds')).toEqual([[[]]])
  })
})
