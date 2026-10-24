import { describe, expect, it } from 'vitest'

import { providerGradient } from '../useChannelMonitorFormat'

describe('channel monitor StepFun presentation', () => {
  it('uses a StepFun emerald icon gradient instead of the neutral fallback', () => {
    const gradient = providerGradient('stepfun')

    expect(gradient).toContain('emerald')
    expect(gradient).not.toContain('gray')
  })
})
