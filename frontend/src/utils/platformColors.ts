/**
 * Centralized platform color definitions.
 *
 * All components that need platform-specific styling should import from here
 * instead of defining their own color mappings.
 */

export type Platform =
  | 'anthropic'
  | 'openai'
  | 'antigravity'
  | 'gemini'
  | 'grok'
  | 'kimi'
  | 'zhipu'
  | 'deepseek'
  | 'minimax'
  | 'opencode_go'
  | 'typesafe'
  | 'composite'

// ── Badge (bg + text + border, for inline badges with border) ───────
const BADGE: Record<Platform, string> = {
  anthropic: 'bg-surface-sunken text-fg border-border-strong',
  openai: 'bg-surface-sunken text-fg border-border-strong',
  antigravity: 'bg-surface-sunken text-fg border-border-strong',
  gemini: 'bg-surface-sunken text-fg border-border-strong',
  grok: 'bg-surface-sunken text-fg border-border-strong',
  kimi: 'bg-surface-sunken text-fg border-border-strong',
  zhipu: 'bg-surface-sunken text-fg border-border-strong',
  deepseek: 'bg-surface-sunken text-fg border-border-strong',
  minimax: 'bg-surface-sunken text-fg border-border-strong',
  opencode_go: 'bg-surface-sunken text-fg border-border-strong',
  typesafe: 'bg-surface-sunken text-fg border-border-strong',
  composite: 'bg-surface-sunken text-fg border-border-strong',
}
const BADGE_DEFAULT = 'bg-surface-sunken text-fg border-border-strong'

// ── Light badge (softer bg, no border) ──────────────────────────────
const BADGE_LIGHT: Record<Platform, string> = {
  anthropic: 'bg-surface-sunken text-fg',
  openai: 'bg-surface-sunken text-fg',
  antigravity: 'bg-surface-sunken text-fg',
  gemini: 'bg-surface-sunken text-fg',
  grok: 'bg-surface-sunken text-fg',
  kimi: 'bg-surface-sunken text-fg',
  zhipu: 'bg-surface-sunken text-fg',
  deepseek: 'bg-surface-sunken text-fg',
  minimax: 'bg-surface-sunken text-fg',
  opencode_go: 'bg-surface-sunken text-fg',
  typesafe: 'bg-surface-sunken text-fg',
  composite: 'bg-surface-sunken text-fg',
}

// ── Border ──────────────────────────────────────────────────────────
const BORDER: Record<Platform, string> = {
  anthropic: 'border-border',
  openai: 'border-border',
  antigravity: 'border-border',
  gemini: 'border-border',
  grok: 'border-border',
  kimi: 'border-border',
  zhipu: 'border-border',
  deepseek: 'border-border',
  minimax: 'border-border',
  opencode_go: 'border-border',
  typesafe: 'border-border',
  composite: 'border-border',
}
const BORDER_DEFAULT = 'border-border'

// ── Border strong (higher-contrast platform tint, e.g. plaza group cards) ──
const BORDER_STRONG: Record<Platform, string> = {
  anthropic: 'border-border-strong',
  openai: 'border-border-strong',
  antigravity: 'border-border-strong',
  gemini: 'border-border-strong',
  grok: 'border-border-strong',
  kimi: 'border-border-strong',
  zhipu: 'border-border-strong',
  deepseek: 'border-border-strong',
  minimax: 'border-border-strong',
  opencode_go: 'border-border-strong',
  typesafe: 'border-border-strong',
  composite: 'border-border-strong',
}
const BORDER_STRONG_DEFAULT = 'border-border-strong'

// ── Accent (single raw color per platform; consumers derive washes/tints
//    from it via CSS color-mix, e.g. plaza paid-price zone) ──
const ACCENT: Record<Platform, string> = {
  anthropic: 'rgb(var(--accent))',
  openai: 'rgb(var(--accent))',
  antigravity: 'rgb(var(--accent))',
  gemini: 'rgb(var(--accent))',
  grok: 'rgb(var(--accent))',
  kimi: 'rgb(var(--accent))',
  zhipu: 'rgb(var(--accent))',
  deepseek: 'rgb(var(--accent))',
  minimax: 'rgb(var(--accent))',
  opencode_go: 'rgb(var(--accent))',
  typesafe: 'rgb(var(--accent))',
  composite: 'rgb(var(--accent))',
}
const ACCENT_DEFAULT = 'rgb(var(--accent))'

// ── Accent bar (gradient) ───────────────────────────────────────────
const ACCENT_BAR: Record<Platform, string> = {
  anthropic: ' bg-accent',
  openai: ' bg-accent',
  antigravity: ' bg-accent',
  gemini: ' bg-accent',
  grok: ' bg-accent',
  kimi: ' bg-accent',
  zhipu: ' bg-accent',
  deepseek: ' bg-accent',
  minimax: ' bg-accent',
  opencode_go: ' bg-accent',
  typesafe: ' bg-accent',
  composite: ' bg-accent',
}
const ACCENT_BAR_DEFAULT = ' bg-accent'

// ── Text (price, icon) ─────────────────────────────────────────────
const TEXT: Record<Platform, string> = {
  anthropic: 'text-fg',
  openai: 'text-fg',
  antigravity: 'text-fg',
  gemini: 'text-fg',
  grok: 'text-fg',
  kimi: 'text-fg',
  zhipu: 'text-fg',
  deepseek: 'text-fg',
  minimax: 'text-fg',
  opencode_go: 'text-fg',
  typesafe: 'text-fg',
  composite: 'text-fg',
}
const TEXT_DEFAULT = 'text-fg'

// ── Icon (check mark etc.) ──────────────────────────────────────────
const ICON: Record<Platform, string> = {
  anthropic: 'text-accent',
  openai: 'text-accent',
  antigravity: 'text-accent',
  gemini: 'text-accent',
  grok: 'text-accent',
  kimi: 'text-accent',
  zhipu: 'text-accent',
  deepseek: 'text-accent',
  minimax: 'text-accent',
  opencode_go: 'text-accent',
  typesafe: 'text-accent',
  composite: 'text-accent',
}
const ICON_DEFAULT = 'text-accent'

// ── Button (solid bg) ───────────────────────────────────────────────
const BUTTON: Record<Platform, string> = {
  anthropic: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  openai: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  antigravity: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  gemini: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  grok: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  kimi: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  zhipu: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  deepseek: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  minimax: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  opencode_go: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  typesafe: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
  composite: 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken',
}
const BUTTON_DEFAULT = 'bg-accent text-white hover:bg-accent-strong dark:text-surface-sunken'

// ── Discount badge ──────────────────────────────────────────────────
const DISCOUNT: Record<Platform, string> = {
  anthropic: 'bg-danger-weak text-danger-strong',
  openai: 'bg-danger-weak text-danger-strong',
  antigravity: 'bg-danger-weak text-danger-strong',
  gemini: 'bg-danger-weak text-danger-strong',
  grok: 'bg-danger-weak text-danger-strong',
  kimi: 'bg-danger-weak text-danger-strong',
  zhipu: 'bg-danger-weak text-danger-strong',
  deepseek: 'bg-danger-weak text-danger-strong',
  minimax: 'bg-danger-weak text-danger-strong',
  opencode_go: 'bg-danger-weak text-danger-strong',
  typesafe: 'bg-danger-weak text-danger-strong',
  composite: 'bg-danger-weak text-danger-strong',
}
const DISCOUNT_DEFAULT = 'bg-danger-weak text-danger-strong'

// ── Header gradient (subscription confirm) ─────────────────────────
const GRADIENT: Record<Platform, string> = {
  anthropic: 'bg-accent',
  openai: 'bg-accent',
  antigravity: 'bg-accent',
  gemini: 'bg-accent',
  grok: 'bg-accent',
  kimi: 'bg-accent',
  zhipu: 'bg-accent',
  deepseek: 'bg-accent',
  minimax: 'bg-accent',
  opencode_go: 'bg-accent',
  typesafe: 'bg-accent',
  composite: 'bg-accent',
}
const GRADIENT_DEFAULT = 'bg-accent'

// ── Header text (light text on gradient bg) ────────────────────────
const GRADIENT_TEXT: Record<Platform, string> = {
  anthropic: 'text-white',
  openai: 'text-white',
  antigravity: 'text-white',
  gemini: 'text-white',
  grok: 'text-white',
  kimi: 'text-white',
  zhipu: 'text-white',
  deepseek: 'text-white',
  minimax: 'text-white',
  opencode_go: 'text-white',
  typesafe: 'text-white',
  composite: 'text-white',
}
const GRADIENT_TEXT_DEFAULT = 'text-white'

const GRADIENT_SUBTEXT: Record<Platform, string> = {
  anthropic: 'text-white/80',
  openai: 'text-white/80',
  antigravity: 'text-white/80',
  gemini: 'text-white/80',
  grok: 'text-white/80',
  kimi: 'text-white/80',
  zhipu: 'text-white/80',
  deepseek: 'text-white/80',
  minimax: 'text-white/80',
  opencode_go: 'text-white/80',
  typesafe: 'text-white/80',
  composite: 'text-white/80',
}
const GRADIENT_SUBTEXT_DEFAULT = 'text-white/80'

// ── Public API ──────────────────────────────────────────────────────

function isPlatform(p: string): p is Platform {
  return (
    p === 'anthropic' ||
    p === 'openai' ||
    p === 'antigravity' ||
    p === 'gemini' ||
    p === 'grok' ||
    p === 'kimi' ||
    p === 'zhipu' ||
    p === 'deepseek' ||
    p === 'minimax' ||
    p === 'opencode_go' ||
    p === 'typesafe' ||
    p === 'composite'
  )
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
    case 'kimi': return 'Kimi'
    case 'zhipu': return 'Zhipu GLM'
    case 'deepseek': return 'DeepSeek'
    case 'minimax': return 'MiniMax'
    case 'opencode_go': return 'OpenCode'
    case 'typesafe': return 'TypeSafe / Jev'
    case 'composite': return 'Composite'
    default: return p || 'API'
  }
}
