/**
 * Centralized platform color definitions.
 *
 * All components that need platform-specific styling should import from here
 * instead of defining their own color mappings.
 */

// TODO: This hardcoded union should eventually be removed; dynamic platforms
// are resolved via themeColorToTailwind() + PlatformDeclaration.theme_color
export type Platform = 'anthropic' | 'openai' | 'antigravity' | 'gemini'

// ── Badge (bg + text + border, for inline badges with border) ───────
const BADGE: Record<Platform, string> = {
  anthropic: 'bg-orange-500/10 text-orange-600 border-orange-500/30 dark:text-orange-400',
  openai: 'bg-green-500/10 text-green-600 border-green-500/30 dark:text-green-400',
  antigravity: 'bg-purple-500/10 text-purple-600 border-purple-500/30 dark:text-purple-400',
  gemini: 'bg-blue-500/10 text-blue-600 border-blue-500/30 dark:text-blue-400',
}
const BADGE_DEFAULT = 'bg-slate-500/10 text-slate-600 border-slate-500/30 dark:text-slate-400'

// ── Light badge (softer bg, no border) ──────────────────────────────
const BADGE_LIGHT: Record<Platform, string> = {
  anthropic: 'bg-orange-500/10 text-orange-600 dark:bg-orange-500/10 dark:text-orange-300',
  openai: 'bg-green-500/10 text-green-600 dark:bg-green-500/10 dark:text-green-300',
  antigravity: 'bg-purple-500/10 text-purple-600 dark:bg-purple-500/10 dark:text-purple-300',
  gemini: 'bg-blue-500/10 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300',
}

// ── Border ──────────────────────────────────────────────────────────
const BORDER: Record<Platform, string> = {
  anthropic: 'border-orange-500/20 dark:border-orange-500/20',
  openai: 'border-green-500/20 dark:border-green-500/20',
  antigravity: 'border-purple-500/20 dark:border-purple-500/20',
  gemini: 'border-blue-500/20 dark:border-blue-500/20',
}
const BORDER_DEFAULT = 'border-gray-200 dark:border-dark-700'

// ── Accent bar (gradient) ───────────────────────────────────────────
const ACCENT_BAR: Record<Platform, string> = {
  anthropic: 'bg-gradient-to-r from-orange-400 to-orange-500',
  openai: 'bg-gradient-to-r from-emerald-400 to-emerald-500',
  antigravity: 'bg-gradient-to-r from-purple-400 to-purple-500',
  gemini: 'bg-gradient-to-r from-blue-400 to-blue-500',
}
const ACCENT_BAR_DEFAULT = 'bg-gradient-to-r from-primary-400 to-primary-500'

// ── Text (price, icon) ─────────────────────────────────────────────
const TEXT: Record<Platform, string> = {
  anthropic: 'text-orange-600 dark:text-orange-400',
  openai: 'text-emerald-600 dark:text-emerald-400',
  antigravity: 'text-purple-600 dark:text-purple-400',
  gemini: 'text-blue-600 dark:text-blue-400',
}
const TEXT_DEFAULT = 'text-primary-600 dark:text-primary-400'

// ── Icon (check mark etc.) ──────────────────────────────────────────
const ICON: Record<Platform, string> = {
  anthropic: 'text-orange-500 dark:text-orange-400',
  openai: 'text-emerald-500 dark:text-emerald-400',
  antigravity: 'text-purple-500 dark:text-purple-400',
  gemini: 'text-blue-500 dark:text-blue-400',
}
const ICON_DEFAULT = 'text-primary-500 dark:text-primary-400'

// ── Button (solid bg) ───────────────────────────────────────────────
const BUTTON: Record<Platform, string> = {
  anthropic: 'bg-orange-500 text-white hover:bg-orange-600 active:bg-orange-700 dark:bg-orange-500/80 dark:hover:bg-orange-500',
  openai: 'bg-green-600 text-white hover:bg-green-700 active:bg-green-800 dark:bg-green-600/80 dark:hover:bg-green-600',
  antigravity: 'bg-purple-500 text-white hover:bg-purple-600 active:bg-purple-700 dark:bg-purple-500/80 dark:hover:bg-purple-500',
  gemini: 'bg-blue-500 text-white hover:bg-blue-600 active:bg-blue-700 dark:bg-blue-500/80 dark:hover:bg-blue-500',
}
const BUTTON_DEFAULT = 'bg-primary-500 text-white hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500'

// ── Discount badge ──────────────────────────────────────────────────
const DISCOUNT: Record<Platform, string> = {
  anthropic: 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300',
  openai: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  antigravity: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
  gemini: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
}
const DISCOUNT_DEFAULT = 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'

// ── Header gradient (subscription confirm) ─────────────────────────
const GRADIENT: Record<Platform, string> = {
  anthropic: 'from-orange-500 to-orange-600',
  openai: 'from-emerald-500 to-emerald-600',
  antigravity: 'from-purple-500 to-purple-600',
  gemini: 'from-blue-500 to-blue-600',
}
const GRADIENT_DEFAULT = 'from-primary-500 to-primary-600'

// ── Header text (light text on gradient bg) ────────────────────────
const GRADIENT_TEXT: Record<Platform, string> = {
  anthropic: 'text-orange-100',
  openai: 'text-emerald-100',
  antigravity: 'text-purple-100',
  gemini: 'text-blue-100',
}
const GRADIENT_TEXT_DEFAULT = 'text-primary-100'

const GRADIENT_SUBTEXT: Record<Platform, string> = {
  anthropic: 'text-orange-200',
  openai: 'text-emerald-200',
  antigravity: 'text-purple-200',
  gemini: 'text-blue-200',
}
const GRADIENT_SUBTEXT_DEFAULT = 'text-primary-200'

// ── Dot (small accent indicator) ────────────────────────────────────
const DOT: Record<Platform, string> = {
  anthropic: 'bg-orange-500',
  openai: 'bg-emerald-500',
  antigravity: 'bg-purple-500',
  gemini: 'bg-blue-500',
}

// ── Public API ──────────────────────────────────────────────────────

function isPlatform(p: string): p is Platform {
  return p === 'anthropic' || p === 'openai' || p === 'antigravity' || p === 'gemini'
}

export function platformBadgeClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return BADGE[p]
  if (themeColor) return dynamicBadgeClass(themeColor)
  return BADGE_DEFAULT
}

export function platformBadgeLightClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return BADGE_LIGHT[p]
  if (themeColor) return dynamicBadgeClass(themeColor)
  return BADGE_DEFAULT
}

export function platformBorderClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return BORDER[p]
  if (themeColor) {
    const tw = themeColorToTailwind(themeColor)
    return `border-${tw}-500/20 dark:border-${tw}-500/20`
  }
  return BORDER_DEFAULT
}

export function platformAccentBarClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return ACCENT_BAR[p]
  if (themeColor) {
    const tw = themeColorToTailwind(themeColor)
    return `bg-gradient-to-r from-${tw}-400 to-${tw}-500`
  }
  return ACCENT_BAR_DEFAULT
}

export function platformTextClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return TEXT[p]
  if (themeColor) return dynamicTextClass(themeColor)
  return TEXT_DEFAULT
}

export function platformIconClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return ICON[p]
  if (themeColor) {
    const tw = themeColorToTailwind(themeColor)
    return `text-${tw}-500 dark:text-${tw}-400`
  }
  return ICON_DEFAULT
}

export function platformDotClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return DOT[p]
  if (themeColor) {
    const tw = themeColorToTailwind(themeColor)
    return `bg-${tw}-500`
  }
  return 'bg-gray-400'
}

export function platformButtonClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return BUTTON[p]
  if (themeColor) {
    const tw = themeColorToTailwind(themeColor)
    return `bg-${tw}-500 text-white hover:bg-${tw}-600 active:bg-${tw}-700 dark:bg-${tw}-500/80 dark:hover:bg-${tw}-500`
  }
  return BUTTON_DEFAULT
}

export function platformDiscountClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return DISCOUNT[p]
  if (themeColor) {
    const tw = themeColorToTailwind(themeColor)
    return `bg-${tw}-100 text-${tw}-700 dark:bg-${tw}-900/40 dark:text-${tw}-300`
  }
  return DISCOUNT_DEFAULT
}

export function platformGradientClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return GRADIENT[p]
  if (themeColor) {
    const tw = themeColorToTailwind(themeColor)
    return `from-${tw}-500 to-${tw}-600`
  }
  return GRADIENT_DEFAULT
}

export function platformGradientTextClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return GRADIENT_TEXT[p]
  if (themeColor) {
    const tw = themeColorToTailwind(themeColor)
    return `text-${tw}-100`
  }
  return GRADIENT_TEXT_DEFAULT
}

export function platformGradientSubtextClass(p: string, themeColor?: string): string {
  if (isPlatform(p)) return GRADIENT_SUBTEXT[p]
  if (themeColor) {
    const tw = themeColorToTailwind(themeColor)
    return `text-${tw}-200`
  }
  return GRADIENT_SUBTEXT_DEFAULT
}

// TODO: Callers should migrate to dynamicPlatformLabel() which reads from plugin API
export function platformLabel(p: string): string {
  switch (p) {
    case 'anthropic': return 'Anthropic'
    case 'openai': return 'OpenAI'
    case 'antigravity': return 'Antigravity'
    case 'gemini': return 'Gemini'
    default: return p || 'API'
  }
}

// ── Dynamic platform support ─────────────────────────────────────────
// Plugin-declared platforms provide a theme_color (CSS color string like
// "#7c3aed" or "purple"). These helpers generate Tailwind-compatible
// classes or fall back to the default slate palette.

const CSS_COLOR_TO_TAILWIND: Record<string, string> = {
  '#ea580c': 'orange', '#f97316': 'orange',
  '#16a34a': 'green', '#22c55e': 'green', '#10b981': 'emerald',
  '#7c3aed': 'purple', '#a855f7': 'purple',
  '#2563eb': 'blue', '#3b82f6': 'blue',
  '#dc2626': 'red', '#ef4444': 'red',
  '#0891b2': 'cyan', '#06b6d4': 'cyan',
  '#d97706': 'amber', '#f59e0b': 'amber',
  '#ec4899': 'pink', '#db2777': 'pink',
  '#059669': 'emerald',
  '#4f46e5': 'indigo', '#6366f1': 'indigo',
  '#0d9488': 'teal', '#14b8a6': 'teal',
}

function themeColorToTailwind(themeColor: string): string {
  if (!themeColor) return 'slate'
  const lower = themeColor.toLowerCase()
  // Direct color name
  if (['orange', 'green', 'emerald', 'purple', 'blue', 'red', 'cyan', 'amber', 'pink', 'indigo', 'teal', 'slate'].includes(lower)) {
    return lower
  }
  return CSS_COLOR_TO_TAILWIND[lower] || 'slate'
}

export function dynamicBadgeClass(themeColor: string): string {
  const tw = themeColorToTailwind(themeColor)
  return `bg-${tw}-500/10 text-${tw}-600 border-${tw}-500/30 dark:text-${tw}-400`
}

export function dynamicTextClass(themeColor: string): string {
  const tw = themeColorToTailwind(themeColor)
  return `text-${tw}-600 dark:text-${tw}-400`
}

// ── Dynamic helpers for PlatformTypeBadge / GroupBadge ────────────────

/** Platform badge (PlatformTypeBadge platformClass) */
export function dynamicPlatformBadgeClass(themeColor: string): string {
  const tw = themeColorToTailwind(themeColor)
  return `bg-${tw}-100 text-${tw}-700 dark:bg-${tw}-900/30 dark:text-${tw}-400`
}

/** Type badge — slightly lighter text (PlatformTypeBadge typeClass) */
export function dynamicTypeBadgeClass(themeColor: string): string {
  const tw = themeColorToTailwind(themeColor)
  return `bg-${tw}-100 text-${tw}-600 dark:bg-${tw}-900/30 dark:text-${tw}-400`
}

/** Group subscription badge (GroupBadge badgeClass subscription variant) */
export function dynamicGroupSubBadgeClass(themeColor: string): string {
  const tw = themeColorToTailwind(themeColor)
  return `bg-${tw}-100 text-${tw}-700 dark:bg-${tw}-900/30 dark:text-${tw}-400`
}

/** Group standard badge (GroupBadge badgeClass standard variant) */
export function dynamicGroupStdBadgeClass(themeColor: string): string {
  const tw = themeColorToTailwind(themeColor)
  return `bg-${tw}-50 text-${tw}-700 dark:bg-${tw}-900/20 dark:text-${tw}-400`
}

/** Subscription label (GroupBadge labelClass normal state) */
export function dynamicSubLabelClass(themeColor: string): string {
  const tw = themeColorToTailwind(themeColor)
  return `bg-${tw}-200/60 text-${tw}-800 dark:bg-${tw}-800/40 dark:text-${tw}-300`
}

// Registry for dynamic platform labels (from API)
const dynamicLabels = new Map<string, string>()

export function registerPlatformLabel(platform: string, label: string) {
  dynamicLabels.set(platform, label)
}

export function registerPlatformLabels(platforms: Array<{ platform: string; display_name: string }>) {
  for (const p of platforms) {
    dynamicLabels.set(p.platform, p.display_name)
  }
}

// Override platformLabel to check dynamic registry first
const _originalPlatformLabel = platformLabel
export { _originalPlatformLabel }

// Re-export with dynamic support
export function dynamicPlatformLabel(p: string): string {
  const dynamic = dynamicLabels.get(p)
  if (dynamic) return dynamic
  return platformLabel(p)
}
