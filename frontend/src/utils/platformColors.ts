/**
 * Centralized platform color definitions.
 *
 * All components that need platform-specific styling should import from here
 * instead of defining their own color mappings.
 */

export type Platform = 'anthropic' | 'openai' | 'antigravity' | 'gemini' | 'grok' | 'composite' | 'kiro' | 'deepseek' | 'glm' | 'zai' | 'minimax' | 'opencode'

// ── Badge (bg + text + border, for inline badges with border) ───────
const BADGE: Record<Platform, string> = {
  anthropic: 'bg-orange-500/10 text-orange-600 border-orange-500/30 dark:text-orange-400',
  openai: 'bg-green-500/10 text-green-600 border-green-500/30 dark:text-green-400',
  antigravity: 'bg-purple-500/10 text-purple-600 border-purple-500/30 dark:text-purple-400',
  gemini: 'bg-blue-500/10 text-blue-600 border-blue-500/30 dark:text-blue-400',
  grok: 'bg-zinc-800/10 text-zinc-800 border-zinc-800/30 dark:bg-zinc-500/10 dark:text-zinc-200 dark:border-zinc-500/30',
  composite: 'bg-cyan-500/10 text-cyan-700 border-cyan-500/30 dark:text-cyan-300',
  kiro: 'bg-orange-500/10 text-orange-600 border-orange-500/30 dark:text-orange-400',
  deepseek: 'bg-cyan-500/10 text-cyan-600 border-cyan-500/30 dark:text-cyan-300',
  glm: 'bg-fuchsia-500/10 text-fuchsia-600 border-fuchsia-500/30 dark:text-fuchsia-300',
  zai: 'bg-violet-500/10 text-violet-600 border-violet-500/30 dark:text-violet-300',
  minimax: 'bg-rose-500/10 text-rose-600 border-rose-500/30 dark:text-rose-300',
  opencode: 'bg-indigo-500/10 text-indigo-600 border-indigo-500/30 dark:text-indigo-300',
}
const BADGE_DEFAULT = 'bg-slate-500/10 text-slate-600 border-slate-500/30 dark:text-slate-400'

// ── Light badge (softer bg, no border) ──────────────────────────────
const BADGE_LIGHT: Record<Platform, string> = {
  anthropic: 'bg-orange-500/10 text-orange-600 dark:bg-orange-500/10 dark:text-orange-300',
  openai: 'bg-green-500/10 text-green-600 dark:bg-green-500/10 dark:text-green-300',
  antigravity: 'bg-purple-500/10 text-purple-600 dark:bg-purple-500/10 dark:text-purple-300',
  gemini: 'bg-blue-500/10 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300',
  grok: 'bg-zinc-800/10 text-zinc-800 dark:bg-zinc-500/10 dark:text-zinc-200',
  composite: 'bg-cyan-500/10 text-cyan-700 dark:bg-cyan-500/10 dark:text-cyan-300',
  kiro: 'bg-orange-500/10 text-orange-600 dark:bg-orange-500/10 dark:text-orange-300',
  deepseek: 'bg-cyan-500/10 text-cyan-600 dark:bg-cyan-500/10 dark:text-cyan-300',
  glm: 'bg-fuchsia-500/10 text-fuchsia-600 dark:bg-fuchsia-500/10 dark:text-fuchsia-300',
  zai: 'bg-violet-500/10 text-violet-600 dark:bg-violet-500/10 dark:text-violet-300',
  minimax: 'bg-rose-500/10 text-rose-600 dark:bg-rose-500/10 dark:text-rose-300',
  opencode: 'bg-indigo-500/10 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-300',
}

// ── Border ──────────────────────────────────────────────────────────
const BORDER: Record<Platform, string> = {
  anthropic: 'border-orange-500/20 dark:border-orange-500/20',
  openai: 'border-green-500/20 dark:border-green-500/20',
  antigravity: 'border-purple-500/20 dark:border-purple-500/20',
  gemini: 'border-blue-500/20 dark:border-blue-500/20',
  grok: 'border-zinc-800/20 dark:border-zinc-500/20',
  composite: 'border-cyan-500/20 dark:border-cyan-500/20',
  kiro: 'border-orange-500/20 dark:border-orange-500/20',
  deepseek: 'border-cyan-500/20 dark:border-cyan-500/20',
  glm: 'border-fuchsia-500/20 dark:border-fuchsia-500/20',
  zai: 'border-violet-500/20 dark:border-violet-500/20',
  minimax: 'border-rose-500/20 dark:border-rose-500/20',
  opencode: 'border-indigo-500/20 dark:border-indigo-500/20',
}
const BORDER_DEFAULT = 'border-gray-200 dark:border-dark-700'

// ── Border strong (higher-contrast platform tint, e.g. plaza group cards) ──
const BORDER_STRONG: Record<Platform, string> = {
  anthropic: 'border-orange-500/35 dark:border-orange-500/30',
  openai: 'border-green-500/35 dark:border-green-500/30',
  antigravity: 'border-purple-500/35 dark:border-purple-500/30',
  gemini: 'border-blue-500/35 dark:border-blue-500/30',
  grok: 'border-zinc-800/35 dark:border-zinc-500/35',
  composite: 'border-cyan-500/35 dark:border-cyan-500/30',
  kiro: 'border-orange-500/35 dark:border-orange-500/30',
  deepseek: 'border-cyan-500/35 dark:border-cyan-500/30',
  glm: 'border-fuchsia-500/35 dark:border-fuchsia-500/30',
  zai: 'border-violet-500/35 dark:border-violet-500/30',
  minimax: 'border-rose-500/35 dark:border-rose-500/30',
  opencode: 'border-indigo-500/35 dark:border-indigo-500/30',
}
const BORDER_STRONG_DEFAULT = 'border-gray-300 dark:border-dark-600'

// ── Accent (single raw color per platform; consumers derive washes/tints
//    from it via CSS color-mix, e.g. plaza paid-price zone) ──
const ACCENT: Record<Platform, string> = {
  anthropic: '#f97316', // orange-500
  openai: '#22c55e', // green-500
  antigravity: '#a855f7', // purple-500
  gemini: '#3b82f6', // blue-500
  grok: '#71717a', // zinc-500
  composite: '#06b6d4', // cyan-500
  kiro: '#f97316', // orange-500
  deepseek: '#06b6d4', // cyan-500
  glm: '#d946ef', // fuchsia-500
  zai: '#8b5cf6', // violet-500
  minimax: '#f43f5e', // rose-500
  opencode: '#6366f1', // indigo-500
}
const ACCENT_DEFAULT = '#14b8a6' // primary-500 (teal)

// ── Accent bar (gradient) ───────────────────────────────────────────
const ACCENT_BAR: Record<Platform, string> = {
  anthropic: 'bg-gradient-to-r from-orange-400 to-orange-500',
  openai: 'bg-gradient-to-r from-emerald-400 to-emerald-500',
  antigravity: 'bg-gradient-to-r from-purple-400 to-purple-500',
  gemini: 'bg-gradient-to-r from-blue-400 to-blue-500',
  grok: 'bg-gradient-to-r from-zinc-700 to-zinc-900',
  composite: 'bg-gradient-to-r from-slate-500 to-cyan-500',
  kiro: 'bg-gradient-to-r from-orange-400 to-orange-500',
  deepseek: 'bg-gradient-to-r from-cyan-400 to-cyan-500',
  glm: 'bg-gradient-to-r from-fuchsia-400 to-fuchsia-500',
  zai: 'bg-gradient-to-r from-violet-400 to-violet-500',
  minimax: 'bg-gradient-to-r from-rose-400 to-rose-500',
  opencode: 'bg-gradient-to-r from-indigo-400 to-indigo-500',
}
const ACCENT_BAR_DEFAULT = 'bg-gradient-to-r from-primary-400 to-primary-500'

// ── Text (price, icon) ─────────────────────────────────────────────
const TEXT: Record<Platform, string> = {
  anthropic: 'text-orange-600 dark:text-orange-400',
  openai: 'text-emerald-600 dark:text-emerald-400',
  antigravity: 'text-purple-600 dark:text-purple-400',
  gemini: 'text-blue-600 dark:text-blue-400',
  grok: 'text-zinc-800 dark:text-zinc-200',
  composite: 'text-cyan-700 dark:text-cyan-300',
  kiro: 'text-orange-600 dark:text-orange-400',
  deepseek: 'text-cyan-600 dark:text-cyan-300',
  glm: 'text-fuchsia-600 dark:text-fuchsia-300',
  zai: 'text-violet-600 dark:text-violet-300',
  minimax: 'text-rose-600 dark:text-rose-300',
  opencode: 'text-indigo-600 dark:text-indigo-300',
}
const TEXT_DEFAULT = 'text-primary-600 dark:text-primary-400'

// ── Icon (check mark etc.) ──────────────────────────────────────────
const ICON: Record<Platform, string> = {
  anthropic: 'text-orange-500 dark:text-orange-400',
  openai: 'text-emerald-500 dark:text-emerald-400',
  antigravity: 'text-purple-500 dark:text-purple-400',
  gemini: 'text-blue-500 dark:text-blue-400',
  grok: 'text-zinc-800 dark:text-zinc-200',
  composite: 'text-cyan-600 dark:text-cyan-300',
  kiro: 'text-orange-500 dark:text-orange-400',
  deepseek: 'text-cyan-500 dark:text-cyan-300',
  glm: 'text-fuchsia-500 dark:text-fuchsia-300',
  zai: 'text-violet-500 dark:text-violet-300',
  minimax: 'text-rose-500 dark:text-rose-300',
  opencode: 'text-indigo-500 dark:text-indigo-300',
}
const ICON_DEFAULT = 'text-primary-500 dark:text-primary-400'

// ── Button (solid bg) ───────────────────────────────────────────────
const BUTTON: Record<Platform, string> = {
  anthropic: 'bg-orange-500 text-white hover:bg-orange-600 active:bg-orange-700 dark:bg-orange-500/80 dark:hover:bg-orange-500',
  openai: 'bg-green-600 text-white hover:bg-green-700 active:bg-green-800 dark:bg-green-600/80 dark:hover:bg-green-600',
  antigravity: 'bg-purple-500 text-white hover:bg-purple-600 active:bg-purple-700 dark:bg-purple-500/80 dark:hover:bg-purple-500',
  gemini: 'bg-blue-500 text-white hover:bg-blue-600 active:bg-blue-700 dark:bg-blue-500/80 dark:hover:bg-blue-500',
  grok: 'bg-zinc-800 text-white hover:bg-zinc-900 active:bg-black dark:bg-zinc-700 dark:hover:bg-zinc-600',
  composite: 'bg-cyan-700 text-white hover:bg-cyan-800 active:bg-cyan-900 dark:bg-cyan-600 dark:hover:bg-cyan-500',
  kiro: 'bg-orange-500 text-white hover:bg-orange-600 active:bg-orange-700 dark:bg-orange-500/80 dark:hover:bg-orange-500',
  deepseek: 'bg-cyan-600 text-white hover:bg-cyan-700 active:bg-cyan-800 dark:bg-cyan-600/80 dark:hover:bg-cyan-600',
  glm: 'bg-fuchsia-600 text-white hover:bg-fuchsia-700 active:bg-fuchsia-800 dark:bg-fuchsia-600/80 dark:hover:bg-fuchsia-600',
  zai: 'bg-violet-600 text-white hover:bg-violet-700 active:bg-violet-800 dark:bg-violet-600/80 dark:hover:bg-violet-600',
  minimax: 'bg-rose-600 text-white hover:bg-rose-700 active:bg-rose-800 dark:bg-rose-600/80 dark:hover:bg-rose-600',
  opencode: 'bg-indigo-600 text-white hover:bg-indigo-700 active:bg-indigo-800 dark:bg-indigo-600/80 dark:hover:bg-indigo-600',
}
const BUTTON_DEFAULT = 'bg-primary-500 text-white hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500'

// ── Discount badge ──────────────────────────────────────────────────
const DISCOUNT: Record<Platform, string> = {
  anthropic: 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300',
  openai: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  antigravity: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
  gemini: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  grok: 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-200',
  composite: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/40 dark:text-cyan-300',
  kiro: 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300',
  deepseek: 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/40 dark:text-cyan-300',
  glm: 'bg-fuchsia-100 text-fuchsia-700 dark:bg-fuchsia-900/40 dark:text-fuchsia-300',
  zai: 'bg-violet-100 text-violet-700 dark:bg-violet-900/40 dark:text-violet-300',
  minimax: 'bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300',
  opencode: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300',
}
const DISCOUNT_DEFAULT = 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'

// ── Header gradient (subscription confirm) ─────────────────────────
const GRADIENT: Record<Platform, string> = {
  anthropic: 'from-orange-500 to-orange-600',
  openai: 'from-emerald-500 to-emerald-600',
  antigravity: 'from-purple-500 to-purple-600',
  gemini: 'from-blue-500 to-blue-600',
  grok: 'from-zinc-700 to-zinc-900',
  composite: 'from-slate-600 to-cyan-600',
  kiro: 'from-orange-500 to-orange-600',
  deepseek: 'from-cyan-500 to-cyan-600',
  glm: 'from-fuchsia-500 to-fuchsia-600',
  zai: 'from-violet-500 to-violet-600',
  minimax: 'from-rose-500 to-rose-600',
  opencode: 'from-indigo-500 to-indigo-600',
}
const GRADIENT_DEFAULT = 'from-primary-500 to-primary-600'

// ── Header text (light text on gradient bg) ────────────────────────
const GRADIENT_TEXT: Record<Platform, string> = {
  anthropic: 'text-orange-100',
  openai: 'text-emerald-100',
  antigravity: 'text-purple-100',
  gemini: 'text-blue-100',
  grok: 'text-zinc-100',
  composite: 'text-cyan-100',
  kiro: 'text-orange-100',
  deepseek: 'text-cyan-100',
  glm: 'text-fuchsia-100',
  zai: 'text-violet-100',
  minimax: 'text-rose-100',
  opencode: 'text-indigo-100',
}
const GRADIENT_TEXT_DEFAULT = 'text-primary-100'

const GRADIENT_SUBTEXT: Record<Platform, string> = {
  anthropic: 'text-orange-200',
  openai: 'text-emerald-200',
  antigravity: 'text-purple-200',
  gemini: 'text-blue-200',
  grok: 'text-zinc-300',
  composite: 'text-cyan-200',
  kiro: 'text-orange-200',
  deepseek: 'text-cyan-200',
  glm: 'text-fuchsia-200',
  zai: 'text-violet-200',
  minimax: 'text-rose-200',
  opencode: 'text-indigo-200',
}
const GRADIENT_SUBTEXT_DEFAULT = 'text-primary-200'

// ── Public API ──────────────────────────────────────────────────────

function isPlatform(p: string): p is Platform {
  return p === 'anthropic' || p === 'openai' || p === 'antigravity' || p === 'gemini' || p === 'grok' || p === 'composite' || p === 'kiro' || p === 'deepseek' || p === 'glm' || p === 'zai' || p === 'minimax' || p === 'opencode'
}

export function platformBadgeClass(p: string): string {
  return isPlatform(p) ? BADGE[p] : BADGE_DEFAULT
}

export function platformBadgeLightClass(p: string): string {
  return isPlatform(p) ? BADGE_LIGHT[p] : BADGE_DEFAULT
}

export function platformBorderClass(p: string): string {
  return isPlatform(p) ? BORDER[p] : BORDER_DEFAULT
}

export function platformBorderStrongClass(p: string): string {
  return isPlatform(p) ? BORDER_STRONG[p] : BORDER_STRONG_DEFAULT
}

export function platformAccentColor(p: string): string {
  return isPlatform(p) ? ACCENT[p] : ACCENT_DEFAULT
}

export function platformAccentBarClass(p: string): string {
  return isPlatform(p) ? ACCENT_BAR[p] : ACCENT_BAR_DEFAULT
}

export function platformTextClass(p: string): string {
  return isPlatform(p) ? TEXT[p] : TEXT_DEFAULT
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
  return isPlatform(p) ? GRADIENT[p] : GRADIENT_DEFAULT
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
    case 'grok': return 'Grok'
    case 'composite': return 'Composite'
    case 'kiro': return 'Kiro'
    case 'deepseek': return 'DeepSeek'
    case 'glm': return 'GLM'
    case 'zai': return 'Z.ai'
    case 'minimax': return 'MiniMax'
    case 'opencode': return 'OpenCode'
    default: return p || 'API'
  }
}
