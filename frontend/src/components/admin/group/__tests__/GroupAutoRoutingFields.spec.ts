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
  it('shows only eligible balance candidates', () => {
    const wrapper = mount(GroupAutoRoutingFields, {
      props: {
        routingMode: 'auto_lowest_cost',
        candidateGroupIds: [],
        platform: 'openai',
        groups: [
          group({ id: 1, name: 'eligible' }),
          group({ id: 2, name: 'subscription', subscription_type: 'subscription' }),
          group({ id: 3, name: 'other-platform', platform: 'anthropic' })
        ]
      }
    })

    expect(wrapper.text()).toContain('eligible')
    expect(wrapper.text()).not.toContain('subscription')
    expect(wrapper.text()).not.toContain('other-platform')
  })

  it('clears selected candidates when automatic routing is disabled', async () => {
    const wrapper = mount(GroupAutoRoutingFields, {
      props: {
        routingMode: 'auto_lowest_cost',
        candidateGroupIds: [1],
        platform: 'openai',
        groups: [group({ id: 1 })]
      }
    })

    await wrapper.get('button').trigger('click')

    expect(wrapper.emitted('update:routingMode')).toEqual([['fixed']])
    expect(wrapper.emitted('update:candidateGroupIds')).toEqual([[[]]])
  })
})
