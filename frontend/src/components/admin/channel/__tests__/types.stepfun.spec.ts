import { describe, expect, it } from 'vitest'

import { getPlatformTagClass, getPlatformTextClass } from '../types'

describe('channel monitor StepFun platform colors', () => {
  it('uses emerald tag and text colors for StepFun models', () => {
    expect(getPlatformTagClass('stepfun')).toContain('emerald')
    expect(getPlatformTextClass('stepfun')).toContain('emerald')
  })
})
