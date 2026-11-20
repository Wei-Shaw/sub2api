import { describe, expect, it } from 'vitest'

import {
  platformAccentBarClass,
  platformAccentColor,
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

// 逐表固定取值：只断言 "包含 stone" 会放过把 badge 字符串抄进 border/text/button
// 的那类错误（每张表的契约不同）。
describe('platformColors open_design', () => {
  it('matches the stone palette in every table', () => {
    expect(platformBadgeClass('open_design')).toBe(
      'bg-stone-500/10 text-stone-700 border-stone-500/30 dark:text-stone-300'
    )
    expect(platformBadgeLightClass('open_design')).toBe(
      'bg-stone-500/10 text-stone-700 dark:bg-stone-500/10 dark:text-stone-300'
    )
    expect(platformBorderClass('open_design')).toBe('border-stone-500/20 dark:border-stone-500/20')
    expect(platformBorderStrongClass('open_design')).toBe('border-stone-500/35 dark:border-stone-500/30')
    expect(platformAccentColor('open_design')).toBe('#78716c')
    expect(platformAccentBarClass('open_design')).toBe('bg-gradient-to-r from-stone-400 to-stone-500')
    expect(platformTextClass('open_design')).toBe('text-stone-600 dark:text-stone-300')
    expect(platformIconClass('open_design')).toBe('text-stone-500 dark:text-stone-400')
    expect(platformButtonClass('open_design')).toBe(
      'bg-stone-500 text-white hover:bg-stone-600 active:bg-stone-700 dark:bg-stone-500/80 dark:hover:bg-stone-500'
    )
    expect(platformDiscountClass('open_design')).toBe(
      'bg-stone-100 text-stone-700 dark:bg-stone-900/40 dark:text-stone-300'
    )
    expect(platformGradientClass('open_design')).toBe('from-stone-500 to-stone-600')
    expect(platformGradientTextClass('open_design')).toBe('text-stone-100')
    expect(platformGradientSubtextClass('open_design')).toBe('text-stone-200')
  })

  it('keeps unknown platforms on the neutral default', () => {
    expect(platformAccentColor('bogus')).not.toBe('#78716c')
    expect(platformBadgeClass('bogus')).not.toContain('stone')
  })
})
