/**
 * Shared prop vocabulary for the primitive layer.
 *
 * The point of centralising these is that a `tone` means the same thing on a
 * Badge as it does on a Button as it does on a Meter. Component-local unions
 * drift, and drift is how you end up with three different ideas of "warning".
 */

/**
 * Three control heights. Not five.
 *
 * 24 / 28 / 32px. The old system had `.btn-sm|md|lg` where `lg` was
 * `px-6 py-3 text-base` (~48px) — a size that only existed for auth pages and
 * marketing CTAs. In a console, a 48px button next to a 32px row is noise.
 * Full-width `md` covers the same need without inventing a scale step.
 */
export type Size = 'xs' | 'sm' | 'md'

/**
 * Semantic tone.
 *
 * `accent` means INTERACTIVE OR SELECTED — never a status. The four status
 * tones mean status and nothing else. This separation is the whole reason the
 * accent is ultramarine and `--ds-info` is a deliberately different blue: if
 * accent could also signal "informational", a selected row and an info badge
 * would be indistinguishable.
 *
 * `info` is available but should be the last choice on a data surface. If
 * everything can be info, nothing is.
 */
export type Tone = 'neutral' | 'accent' | 'success' | 'warn' | 'danger' | 'info'

/** Fill treatment. `quiet` is text-only with no hover ground. */
export type Variant = 'solid' | 'outline' | 'ghost' | 'quiet'

/** Row density on data surfaces. 32px vs 40px. */
export type Density = 'compact' | 'default'

/** Control height in px, keyed by `Size`. */
export const SIZE_HEIGHT: Record<Size, number> = { xs: 24, sm: 28, md: 32 }

/**
 * Tailwind classes per tone for a *text* treatment — the default way to apply
 * a tone. Backgrounds are the exception, handled per component.
 */
export const TONE_TEXT: Record<Tone, string> = {
  neutral: 'text-ink-secondary',
  accent: 'text-accent',
  success: 'text-success',
  warn: 'text-warn',
  danger: 'text-danger',
  info: 'text-info',
}

/**
 * Tinted-with-border treatment, for badges and cell-level status.
 *
 * Every entry pairs a border with a tint. The border is not decoration: it is
 * what keeps the shape legible in a grayscale printout and for a reader who
 * cannot separate the tints by hue. Color is never the only channel.
 */
export const TONE_TINT: Record<Tone, string> = {
  neutral: 'border-line bg-surface-sunken text-ink-secondary',
  accent: 'border-accent-line bg-accent-tint text-accent',
  success: 'border-success/40 bg-success-tint text-success',
  warn: 'border-warn/40 bg-warn-tint text-warn',
  danger: 'border-danger/40 bg-danger-tint text-danger',
  info: 'border-info/40 bg-info-tint text-info',
}

/** Solid fill per tone. `accent` uses `accent-solid` — see tokens.css. */
export const TONE_SOLID: Record<Tone, string> = {
  neutral: 'border-line-strong bg-ink-secondary text-ink-inverse',
  accent: 'border-accent-solid bg-accent-solid text-accent-on',
  success: 'border-success bg-success text-white',
  warn: 'border-warn bg-warn text-white',
  danger: 'border-danger bg-danger text-white',
  info: 'border-info bg-info text-white',
}

/** Background fill only, for bars and dots. */
export const TONE_FILL: Record<Tone, string> = {
  neutral: 'bg-status-neutral',
  accent: 'bg-accent',
  success: 'bg-success',
  warn: 'bg-warn',
  danger: 'bg-danger',
  info: 'bg-info',
}
