import { describe, expect, it } from 'vitest'

import type { AdminGroup } from '@/types'
import { filterAutoRoutingCandidates } from '../groupsAutoRouting'

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
