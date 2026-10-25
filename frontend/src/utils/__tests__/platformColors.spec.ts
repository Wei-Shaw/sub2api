import { describe, expect, it } from 'vitest'

import {
  platformAccentColor,
  platformBadgeClass,
  platformTextClass,
} from '../platformColors'

describe('StepFun platform colors', () => {
  it('uses the StepFun accent instead of the default platform colors', () => {
    expect(platformBadgeClass('stepfun')).toContain('emerald')
    expect(platformBadgeClass('stepfun')).not.toBe(platformBadgeClass('not-a-platform'))
    expect(platformAccentColor('stepfun')).toBe('#10b981')
    expect(platformTextClass('stepfun')).toContain('text-emerald')
  })
})
