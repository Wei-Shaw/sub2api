import { describe, expect, it } from 'vitest'
import {
  platformAccentColor,
  platformBadgeClass,
  platformGradientClass,
  platformLabel,
  platformTextClass
} from '../platformColors'

describe('platformColors', () => {
  it('Kiro 平台使用独立紫色主题，不复用 Anthropic 橙色', () => {
    expect(platformBadgeClass('kiro')).toContain('violet')
    expect(platformTextClass('kiro')).toContain('violet')
    expect(platformGradientClass('kiro')).toContain('from-violet-500')
    expect(platformGradientClass('kiro')).toContain('to-fuchsia-500')
    expect(platformBadgeClass('kiro')).not.toContain('orange')
  })

  it('Adobe 平台使用 Logo 品牌红，而非通用 red 或兜底主色', () => {
    expect(platformAccentColor('adobe')).toBe('#fa0f00')
    expect(platformBadgeClass('adobe')).toContain('adobe-500')
    expect(platformTextClass('adobe')).toContain('adobe-600')
    expect(platformGradientClass('adobe')).toBe('from-adobe-500 to-adobe-600')
    expect(platformBadgeClass('adobe')).not.toContain('red-')
    expect(platformBadgeClass('adobe')).not.toContain('slate')
    expect(platformLabel('adobe')).toBe('Adobe')
  })
})
