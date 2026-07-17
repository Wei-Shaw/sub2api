import { describe, expect, it } from 'vitest'

import { platformBadgeClass, platformLabel } from '../platformColors'

describe('platformColors auto platform', () => {
  it('uses a dedicated Auto label and cyan badge instead of the fallback style', () => {
    expect(platformLabel('auto')).toBe('Auto')
    expect(platformBadgeClass('auto')).toContain('cyan')
    expect(platformBadgeClass('auto')).not.toContain('slate')
  })
})
