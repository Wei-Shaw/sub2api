/** Centralized platform color definitions (orange / emerald / blue / purple). */

export type Platform = 'anthropic' | 'openai' | 'antigravity' | 'gemini'

export function isPlatform(p: string): p is Platform {
  return p === 'anthropic' || p === 'openai' || p === 'antigravity' || p === 'gemini'
}

interface ColorTokens { base: string; text: string; textOnTint: string; border: string }

export const PLATFORM_TINT: Record<Platform, ColorTokens> = {
  anthropic: { base: 'orange', text: 'text-orange-700', textOnTint: 'text-orange-300', border: 'border-orange-500/30' },
  openai: { base: 'emerald', text: 'text-emerald-700', textOnTint: 'text-emerald-300', border: 'border-emerald-500/30' },
  gemini: { base: 'blue', text: 'text-blue-700', textOnTint: 'text-blue-300', border: 'border-blue-500/30' },
  antigravity: { base: 'purple', text: 'text-purple-700', textOnTint: 'text-purple-300', border: 'border-purple-500/30' },
}

const BADGE: Record<Platform, string> = {
  anthropic: 'bg-orange-500/10 text-orange-600 border-orange-500/30 dark:text-orange-400',
  openai: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/30 dark:text-emerald-400',
  gemini: 'bg-blue-500/10 text-blue-600 border-blue-500/30 dark:text-blue-400',
  antigravity: 'bg-purple-500/10 text-purple-600 border-purple-500/30 dark:text-purple-400',
}
const BADGE_DEFAULT = 'bg-slate-500/10 text-slate-600 border-slate-500/30 dark:text-slate-400'

const BADGE_LIGHT: Record<Platform, string> = {
  anthropic: 'bg-orange-500/10 text-orange-600 dark:bg-orange-500/10 dark:text-orange-300',
  openai: 'bg-emerald-500/10 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-300',
  gemini: 'bg-blue-500/10 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300',
  antigravity: 'bg-purple-500/10 text-purple-600 dark:bg-purple-500/10 dark:text-purple-300',
}

const BORDER: Record<Platform, string> = {
  anthropic: 'border-orange-500/20 dark:border-orange-500/20',
  openai: 'border-emerald-500/20 dark:border-emerald-500/20',
  gemini: 'border-blue-500/20 dark:border-blue-500/20',
  antigravity: 'border-purple-500/20 dark:border-purple-500/20',
}
const BORDER_DEFAULT = 'border-gray-200 dark:border-dark-700'

const TEXT: Record<Platform, string> = {
  anthropic: 'text-orange-600 dark:text-orange-400',
  openai: 'text-emerald-600 dark:text-emerald-400',
  gemini: 'text-blue-600 dark:text-blue-400',
  antigravity: 'text-purple-600 dark:text-purple-400',
}
const TEXT_DEFAULT = 'text-primary-600 dark:text-primary-400'

const TAG: Record<Platform, string> = {
  anthropic: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  openai: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  gemini: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  antigravity: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
}
const TAG_DEFAULT = 'bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-gray-300'

const PICKER_ACTIVE: Record<Platform, string> = {
  anthropic: 'border-orange-500 bg-orange-50 text-orange-700 dark:bg-orange-500/15 dark:text-orange-300 dark:border-orange-400',
  openai: 'border-emerald-500 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300 dark:border-emerald-400',
  gemini: 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300 dark:border-blue-400',
  antigravity: 'border-purple-500 bg-purple-50 text-purple-700 dark:bg-purple-500/15 dark:text-purple-300 dark:border-purple-400',
}
const PICKER_INACTIVE: Record<Platform, string> = {
  anthropic: 'border-gray-200 bg-white text-gray-600 hover:border-orange-300 hover:text-orange-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-orange-500/50',
  openai: 'border-gray-200 bg-white text-gray-600 hover:border-emerald-300 hover:text-emerald-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-emerald-500/50',
  gemini: 'border-gray-200 bg-white text-gray-600 hover:border-blue-300 hover:text-blue-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-blue-500/50',
  antigravity: 'border-gray-200 bg-white text-gray-600 hover:border-purple-300 hover:text-purple-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-purple-500/50',
}

const ACCENT_BAR: Record<Platform, string> = {
  anthropic: 'bg-gradient-to-r from-orange-400 to-orange-500',
  openai: 'bg-gradient-to-r from-emerald-400 to-emerald-500',
  gemini: 'bg-gradient-to-r from-blue-400 to-blue-500',
  antigravity: 'bg-gradient-to-r from-purple-400 to-purple-500',
}
const ACCENT_BAR_DEFAULT = 'bg-gradient-to-r from-primary-400 to-primary-500'

const ICON: Record<Platform, string> = {
  anthropic: 'text-orange-500 dark:text-orange-400',
  openai: 'text-emerald-500 dark:text-emerald-400',
  gemini: 'text-blue-500 dark:text-blue-400',
  antigravity: 'text-purple-500 dark:text-purple-400',
}
const ICON_DEFAULT = 'text-primary-500 dark:text-primary-400'

const BUTTON: Record<Platform, string> = {
  anthropic: 'bg-orange-500 text-white hover:bg-orange-600 active:bg-orange-700 dark:bg-orange-500/80 dark:hover:bg-orange-500',
  openai: 'bg-emerald-600 text-white hover:bg-emerald-700 active:bg-emerald-800 dark:bg-emerald-600/80 dark:hover:bg-emerald-600',
  gemini: 'bg-blue-500 text-white hover:bg-blue-600 active:bg-blue-700 dark:bg-blue-500/80 dark:hover:bg-blue-500',
  antigravity: 'bg-purple-500 text-white hover:bg-purple-600 active:bg-purple-700 dark:bg-purple-500/80 dark:hover:bg-purple-500',
}
const BUTTON_DEFAULT = 'bg-primary-500 text-white hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500'

const DISCOUNT: Record<Platform, string> = {
  anthropic: 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300',
  openai: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  gemini: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  antigravity: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
}
const DISCOUNT_DEFAULT = 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'

const CARD_GRADIENT: Record<Platform, string> = {
  anthropic: 'bg-gradient-to-br from-orange-50 to-amber-100 dark:from-orange-500/10 dark:to-amber-500/20',
  openai: 'bg-gradient-to-br from-emerald-50 to-emerald-100 dark:from-emerald-500/10 dark:to-emerald-500/20',
  gemini: 'bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-blue-500/10 dark:to-indigo-500/20',
  antigravity: 'bg-gradient-to-br from-purple-50 to-fuchsia-100 dark:from-purple-500/10 dark:to-fuchsia-500/20',
}

const HEADER_GRADIENT: Record<Platform, string> = {
  anthropic: 'from-orange-500 to-orange-600',
  openai: 'from-emerald-500 to-emerald-600',
  gemini: 'from-blue-500 to-blue-600',
  antigravity: 'from-purple-500 to-purple-600',
}
const HEADER_GRADIENT_DEFAULT = 'from-primary-500 to-primary-600'

const GRADIENT_TEXT: Record<Platform, string> = {
  anthropic: 'text-orange-100',
  openai: 'text-emerald-100',
  gemini: 'text-blue-100',
  antigravity: 'text-purple-100',
}
const GRADIENT_TEXT_DEFAULT = 'text-primary-100'

const GRADIENT_SUBTEXT: Record<Platform, string> = {
  anthropic: 'text-orange-200',
  openai: 'text-emerald-200',
  gemini: 'text-blue-200',
  antigravity: 'text-purple-200',
}
const GRADIENT_SUBTEXT_DEFAULT = 'text-primary-200'

export function platformBadgeClass(p: string): string {
  return isPlatform(p) ? BADGE[p] : BADGE_DEFAULT
}
export function platformBadgeLightClass(p: string): string {
  return isPlatform(p) ? BADGE_LIGHT[p] : BADGE_DEFAULT
}
export function platformBorderClass(p: string): string {
  return isPlatform(p) ? BORDER[p] : BORDER_DEFAULT
}
export function platformTextClass(p: string): string {
  return isPlatform(p) ? TEXT[p] : TEXT_DEFAULT
}
export function platformTagClass(p: string): string {
  return isPlatform(p) ? TAG[p] : TAG_DEFAULT
}
export function platformTintTextClass(p: string): string {
  return isPlatform(p) ? TEXT[p] : 'text-gray-500 dark:text-gray-300'
}
export function platformPickerClass(p: string, active: boolean): string {
  if (!isPlatform(p)) {
    return active
      ? 'border-gray-400 bg-gray-50 text-gray-700 dark:border-dark-500 dark:bg-dark-700 dark:text-gray-200'
      : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400'
  }
  return active ? PICKER_ACTIVE[p] : PICKER_INACTIVE[p]
}
export function platformAccentBarClass(p: string): string {
  return isPlatform(p) ? ACCENT_BAR[p] : ACCENT_BAR_DEFAULT
}
export function platformIconClass(p: string): string {
  return isPlatform(p) ? ICON[p] : ICON_DEFAULT
}
export function platformButtonClass(p: string): string {
  return isPlatform(p) ? BUTTON[p] : BUTTON_DEFAULT
}
export function platformDiscountClass(p: string): string {
  return isPlatform(p) ? DISCOUNT[p] : DISCOUNT_DEFAULT
}
export function platformGradientClass(p: string): string {
  if (isPlatform(p)) return CARD_GRADIENT[p]
  return 'bg-gradient-to-br from-gray-100 to-gray-200 dark:from-dark-700 dark:to-dark-600'
}
export function platformHeaderGradientClass(p: string): string {
  return isPlatform(p) ? HEADER_GRADIENT[p] : HEADER_GRADIENT_DEFAULT
}
export function platformGradientTextClass(p: string): string {
  return isPlatform(p) ? GRADIENT_TEXT[p] : GRADIENT_TEXT_DEFAULT
}
export function platformGradientSubtextClass(p: string): string {
  return isPlatform(p) ? GRADIENT_SUBTEXT[p] : GRADIENT_SUBTEXT_DEFAULT
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
