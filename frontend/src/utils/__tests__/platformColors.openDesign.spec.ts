import { describe, expect, it } from 'vitest'

import {
  platformAccentColor,
  platformAccentBarClass,
  platformBadgeClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformBorderStrongClass,
  platformButtonClass,
  platformDiscountClass,
  platformGradientClass,
  platformGradientSubtextClass,
  platformGradientTextClass,
  platformIconClass,
  platformTextClass
} from '../platformColors'

// open_design 必须命中自己的 stone 调色板；注册漏了 isPlatform 会静默回落默认色，
// 这类回归只能靠逐表断言发现。
describe('platformColors open_design', () => {
  it('uses the stone palette in every table instead of the default', () => {
    expect(platformBadgeClass('open_design')).toContain('stone')
    expect(platformBadgeLightClass('open_design')).toContain('stone')
    expect(platformBorderClass('open_design')).toContain('stone')
    expect(platformBorderStrongClass('open_design')).toContain('stone')
    expect(platformAccentColor('open_design')).toBe('#78716c')
    expect(platformAccentBarClass('open_design')).toContain('stone')
    expect(platformTextClass('open_design')).toContain('stone')
    expect(platformIconClass('open_design')).toContain('stone')
    expect(platformButtonClass('open_design')).toContain('stone')
    expect(platformDiscountClass('open_design')).toContain('stone')
    expect(platformGradientClass('open_design')).toContain('stone')
    expect(platformGradientTextClass('open_design')).toContain('stone')
    expect(platformGradientSubtextClass('open_design')).toContain('stone')
  })

  it('keeps unknown platforms on the neutral default', () => {
    expect(platformAccentColor('bogus')).not.toBe('#78716c')
    expect(platformBadgeClass('bogus')).not.toContain('stone')
  })
})
