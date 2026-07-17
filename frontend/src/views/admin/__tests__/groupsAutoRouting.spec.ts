import { describe, expect, it } from 'vitest'

import type { AdminGroup } from '@/types'
import {
  filterAutoRoutingCandidates,
  groupBillingTypeDisplay,
  groupRateMultiplierDisplay,
  groupTypeConfiguration,
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
  it('keeps only active balance groups on the same platform', () => {
    const candidates = filterAutoRoutingCandidates(
      [
        group({ id: 1 }),
        group({ id: 2, subscription_type: 'subscription' }),
        group({ id: 3, platform: 'anthropic' }),
        group({ id: 4, status: 'inactive' }),
        group({ id: 5, routing_mode: 'auto_lowest_cost' })
      ],
      'openai'
    )

    expect(candidates.map((candidate) => candidate.id)).toEqual([1])
  })

  it('excludes the group currently being edited', () => {
    const candidates = filterAutoRoutingCandidates(
      [group({ id: 1 }), group({ id: 2 })],
      'openai',
      1
    )

    expect(candidates.map((candidate) => candidate.id)).toEqual([2])
  })
})

describe('automatic group display', () => {
  it('shows auto for both billing type and rate multiplier', () => {
    const automatic = group({
      routing_mode: 'auto_lowest_cost',
      subscription_type: 'standard',
      rate_multiplier: 1
    })

    expect(groupBillingTypeDisplay(automatic)).toBe('auto')
    expect(groupRateMultiplierDisplay(automatic)).toBe('auto')
  })

  it('keeps fixed group values unchanged', () => {
    const fixed = group({ subscription_type: 'standard', rate_multiplier: 0.75 })

    expect(groupBillingTypeDisplay(fixed)).toBe('standard')
    expect(groupRateMultiplierDisplay(fixed)).toBe('0.75x')
  })

  it('maps automatic routing to the auto form type without changing the stored subscription type', () => {
    expect(groupTypeSelection(group({ routing_mode: 'auto_lowest_cost' }))).toBe('auto')
    expect(groupTypeConfiguration('auto')).toEqual({
      subscriptionType: 'standard',
      routingMode: 'auto_lowest_cost'
    })
  })
})
