/**
 * Plugin-local platform color helpers.
 * V5 W9 inlined from host. T10 consolidates four parallel switch-case
 * implementations under ChannelsView, types.ts, useChannelMonitorFormat
 * and MonitorCard.
 *
 * All helpers derive from the single PLATFORM_TINT table.
 *
 * Color picks (per task T10):
 *   - openai      = emerald   (matches host design; replaces monitor green)
 *   - anthropic   = orange    (matches host / channel form / model tag)
 *   - gemini      = blue      (matches host / channel form; replaces monitor sky)
 *   - antigravity = purple    (matches host / channel form; absent in monitor)
 */

export type Platform = 'anthropic' | 'openai' | 'antigravity' | 'gemini'

interface ColorTokens {
  base: string
  text: string
  textOnTint: string
  border: string
}

export const PLATFORM_TINT: Record<Platform, ColorTokens> = {
  anthropic: {
    base: 'orange',
    text: 'text-orange-700',
    textOnTint: 'text-orange-300',
    border: 'border-orange-500/30',
  },
  openai: {
    base: 'emerald',
    text: 'text-emerald-700',
    textOnTint: 'text-emerald-300',
    border: 'border-emerald-500/30',
  },
  gemini: {
    base: 'blue',
    text: 'text-blue-700',
    textOnTint: 'text-blue-300',
    border: 'border-blue-500/30',
  },
  antigravity: {
    base: 'purple',
    text: 'text-purple-700',
    textOnTint: 'text-purple-300',
    border: 'border-purple-500/30',
  },
}

const NEUTRAL_BADGE = 'bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-gray-300'
const NEUTRAL_BORDER = 'border-gray-200 dark:border-dark-700'
const NEUTRAL_TEXT = 'text-gray-600 dark:text-gray-400'

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

const TEXT: Record<Platform, string> = {
  anthropic: 'text-orange-600 dark:text-orange-400',
  openai: 'text-emerald-600 dark:text-emerald-400',
  gemini: 'text-blue-600 dark:text-blue-400',
  antigravity: 'text-purple-600 dark:text-purple-400',
}

const TAG: Record<Platform, string> = {
  anthropic: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  openai: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  gemini: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  antigravity: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
}

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

const GRADIENT: Record<Platform, string> = {
  anthropic: 'bg-gradient-to-br from-orange-50 to-amber-100 dark:from-orange-500/10 dark:to-amber-500/20',
  openai: 'bg-gradient-to-br from-emerald-50 to-emerald-100 dark:from-emerald-500/10 dark:to-emerald-500/20',
  gemini: 'bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-blue-500/10 dark:to-indigo-500/20',
  antigravity: 'bg-gradient-to-br from-purple-50 to-fuchsia-100 dark:from-purple-500/10 dark:to-fuchsia-500/20',
}

function isPlatform(p: string): p is Platform {
  return p === 'anthropic' || p === 'openai' || p === 'antigravity' || p === 'gemini'
}

export function platformBadgeClass(p: string): string {
  return isPlatform(p) ? BADGE[p] : BADGE_DEFAULT
}

export function platformBadgeLightClass(p: string): string {
  return isPlatform(p) ? BADGE_LIGHT[p] : 'bg-slate-500/10 text-slate-600 dark:text-slate-300'
}

export function platformBorderClass(p: string): string {
  return isPlatform(p) ? BORDER[p] : NEUTRAL_BORDER
}

export function platformTextClass(p: string): string {
  return isPlatform(p) ? TEXT[p] : NEUTRAL_TEXT
}

export function platformTagClass(p: string): string {
  return isPlatform(p) ? TAG[p] : NEUTRAL_BADGE
}

export function platformPickerClass(p: string, active: boolean): string {
  if (!isPlatform(p)) {
    return active
      ? 'border-gray-400 bg-gray-50 text-gray-700 dark:border-dark-500 dark:bg-dark-700 dark:text-gray-200'
      : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400'
  }
  return active ? PICKER_ACTIVE[p] : PICKER_INACTIVE[p]
}

export function platformGradientClass(p: string): string {
  if (isPlatform(p)) return GRADIENT[p]
  return 'bg-gradient-to-br from-gray-100 to-gray-200 dark:from-dark-700 dark:to-dark-600'
}

export function platformTintTextClass(p: string): string {
  return isPlatform(p) ? TEXT[p] : 'text-gray-500 dark:text-gray-300'
}

export function platformLabel(p: string): string {
  switch (p) {
    case 'anthropic':
      return 'Anthropic'
    case 'openai':
      return 'OpenAI'
    case 'antigravity':
      return 'Antigravity'
    case 'gemini':
      return 'Gemini'
    default:
      return p || 'API'
  }
}
