import { describe, expect, it } from 'vitest'

import type { AdminGroup } from '@/types'
import {
  filterAutoRoutingCandidates,
  groupBillingTypeDisplay,
  groupPlatformConfiguration,
  groupRateMultiplierDisplay,
  groupTypeSelection
} from '../groupsAutoRouting'

const group = (overrides: Partial<AdminGroup>): AdminGroup =>
  ({
    id: 1,
    name: 'group',
    platform: 'openai',
    subscription_type: 'standard',
    status: 'active',
    routing_mode: 'fixed',
    auto_candidate_group_ids: [],
    ...overrides
  }) as AdminGroup

describe('filterAutoRoutingCandidates', () => {
  it('keeps active balance groups across platforms', () => {
    const candidates = filterAutoRoutingCandidates(
      [
        group({ id: 1 }),
        group({ id: 2, subscription_type: 'subscription' }),
        group({ id: 3, platform: 'anthropic' }),
        group({ id: 4, status: 'inactive' }),
        group({ id: 5, routing_mode: 'auto_lowest_cost' }),
        group({ id: 6, is_exclusive: true })
      ]
    )

    expect(candidates.map((candidate) => candidate.id)).toEqual([1, 3])
  })

  it('excludes the group currently being edited', () => {
    const candidates = filterAutoRoutingCandidates(
      [group({ id: 1 }), group({ id: 2 })],
      1
    )

    expect(candidates.map((candidate) => candidate.id)).toEqual([2])
  })
})

describe('automatic group display', () => {
  it('shows balance billing and auto rate multiplier for an automatic platform', () => {
    const automatic = group({
      platform: 'auto',
      routing_mode: 'auto_lowest_cost',
      subscription_type: 'standard',
      rate_multiplier: 1
    })

    expect(groupBillingTypeDisplay(automatic)).toBe('standard')
    expect(groupRateMultiplierDisplay(automatic)).toBe('auto')
  })

  it('keeps fixed group values unchanged', () => {
    const fixed = group({ subscription_type: 'standard', rate_multiplier: 0.75 })

    expect(groupBillingTypeDisplay(fixed)).toBe('standard')
    expect(groupRateMultiplierDisplay(fixed)).toBe('0.75x')
  })

  it('maps the auto platform to balance billing and automatic routing', () => {
    expect(groupTypeSelection(group({ platform: 'auto', routing_mode: 'auto_lowest_cost' }))).toBe(
      'standard'
    )
    expect(groupPlatformConfiguration('auto')).toEqual({
      subscriptionType: 'standard',
      routingMode: 'auto_lowest_cost'
    })
  })

  it('maps concrete platforms to fixed routing without changing balance billing', () => {
    expect(groupPlatformConfiguration('openai')).toEqual({
      subscriptionType: 'standard',
      routingMode: 'fixed'
    })
  })
})
