import {
  grayAccentBarClass,
  grayBadgeClass,
  grayBadgeLightClass,
  grayBorderClass,
  grayButtonClass,
  grayDiscountClass,
  grayGradientClass,
  grayGradientSubtextClass,
  grayGradientTextClass,
  grayIconClass,
  grayTextClass,
} from '@/utils/grayTheme'

export type Platform = 'anthropic' | 'openai' | 'antigravity' | 'gemini'

export function platformBadgeClass(_p: string): string {
  return grayBadgeClass()
}

export function platformBadgeLightClass(_p: string): string {
  return grayBadgeLightClass()
}

export function platformBorderClass(_p: string): string {
  return grayBorderClass()
}

export function platformAccentBarClass(_p: string): string {
  return grayAccentBarClass()
}

export function platformTextClass(_p: string): string {
  return grayTextClass()
}

export function platformIconClass(_p: string): string {
  return grayIconClass()
}

export function platformButtonClass(_p: string): string {
  return grayButtonClass()
}

export function platformDiscountClass(_p: string): string {
  return grayDiscountClass()
}

export function platformGradientClass(_p: string): string {
  return grayGradientClass()
}

export function platformGradientTextClass(_p: string): string {
  return grayGradientTextClass()
}

export function platformGradientSubtextClass(_p: string): string {
  return grayGradientSubtextClass()
}

export function platformLabel(p: string): string {
  switch (p) {
    case 'anthropic': return 'Anthropic'
    case 'openai': return 'OpenAI'
    case 'antigravity': return 'Antigravity'
    case 'gemini': return 'Gemini'
    default: return p || 'API'
  }
}
