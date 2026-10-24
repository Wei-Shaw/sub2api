import { describe, expect, it } from 'vitest'

import {
  SCHEDULING_THRESHOLD_PLATFORMS,
  normalizeAccountSchedulingThresholdsMap,
} from '../admin/settings'

describe('account scheduling threshold platforms', () => {
  it('includes StepFun and preserves its threshold when normalizing', () => {
    expect(SCHEDULING_THRESHOLD_PLATFORMS).toContain('stepfun')
    expect(normalizeAccountSchedulingThresholdsMap({ stepfun: 37 })).toMatchObject({
      stepfun: 37,
    })
  })
})
