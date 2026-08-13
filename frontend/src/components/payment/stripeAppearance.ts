/**
 * Stripe Elements theming — the one place the design system can reach into an
 * iframe it does not own.
 *
 * WHY THIS FILE EXISTS
 *
 * Stripe renders the Payment Element inside a cross-origin iframe. No
 * stylesheet of ours applies to it: `bg-surface` stops at the frame boundary.
 * The only lever is the Appearance API, which takes a flat bag of colours,
 * one font stack and one radius, and repaints Stripe's own chrome with them.
 *
 * Two rules follow from that, and both used to be broken here:
 *
 *   1. THE VALUES COME FROM THE TOKENS, NOT FROM A HEX LITERAL.
 *      The old call sites passed `{ borderRadius: '8px' }` and nothing else —
 *      an 8px radius in a system whose ceiling is 4px, so the payment form was
 *      visibly rounder than every control around it. Reading the live custom
 *      properties means the iframe follows `tokens.css` for free, including the
 *      `prefers-contrast: more` promotions.
 *
 *   2. IT HAS TO BE RE-SENT WHEN THE THEME FLIPS.
 *      Appearance is resolved once, at `elements()` time. Toggling to dark left
 *      the Element sitting on a white ground in the middle of a near-black
 *      page — the single most obvious "this widget is not part of the app"
 *      artefact in the product. `elements.update({ appearance })` fixes it;
 *      see the `watch(isDark, …)` in the consuming views.
 *
 * THE SEAM WE DO NOT PRETEND AWAY
 *
 * `fontFamily` names IBM Plex Sans, but Stripe only loads fonts passed through
 * its own `fonts` option (a stylesheet URL the iframe can fetch). We do not
 * ship one, so inside the frame the stack falls through to `system-ui`. That is
 * a deliberate, declared mismatch: a half-matched face would read as a bug,
 * whereas the system fallback reads as "this is the payment provider's field".
 * The colours, radius and size DO match, which is what carries the join.
 */

import type { Appearance } from '@stripe/stripe-js'

/**
 * Tokens are stored as space-separated RGB channels (`42 59 212`) so Tailwind's
 * `rgb(var(--ds-x) / <alpha-value>)` keeps opacity modifiers working. Stripe
 * wants a finished CSS colour, so every channel triple has to be wrapped before
 * it crosses the boundary — handing `42 59 212` to Stripe silently yields no
 * colour at all.
 */
function wrapChannels(channels: string): string {
  const parts = channels.trim().split(/[\s,]+/).filter(Boolean)
  if (parts.length < 3) return ''
  return `rgb(${parts[0]}, ${parts[1]}, ${parts[2]})`
}

/**
 * Last-resort literals, used only when there is no DOM to read (unit tests,
 * SSR) or when a token is missing. They mirror `:root` / `.dark` in tokens.css;
 * if they drift, the iframe is wrong only in an environment that has no visible
 * iframe, which is why duplicating them here is acceptable.
 */
const FALLBACK_LIGHT: Record<string, string> = {
  '--ds-accent-solid': '42 59 212',
  '--ds-surface': '255 255 255',
  '--ds-surface-sunken': '242 242 239',
  '--ds-ink': '18 19 22',
  '--ds-ink-secondary': '92 96 104',
  '--ds-ink-disabled': '154 160 170',
  '--ds-danger': '180 35 24',
  '--ds-line': '216 216 211',
  '--ds-focus': '42 59 212',
}

const FALLBACK_DARK: Record<string, string> = {
  '--ds-accent-solid': '68 83 221',
  '--ds-surface': '16 17 19',
  '--ds-surface-sunken': '12 13 15',
  '--ds-ink': '230 230 227',
  '--ds-ink-secondary': '146 152 162',
  '--ds-ink-disabled': '97 102 110',
  '--ds-danger': '255 122 112',
  '--ds-line': '42 44 49',
  '--ds-focus': '142 153 245',
}

const FALLBACK_SANS =
  "'IBM Plex Sans', system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"

interface TokenReader {
  color(name: string): string
  raw(name: string, fallback: string): string
}

function createTokenReader(isDark: boolean): TokenReader {
  const fallbacks = isDark ? FALLBACK_DARK : FALLBACK_LIGHT
  const computed =
    typeof document !== 'undefined' && typeof window !== 'undefined'
      ? window.getComputedStyle(document.documentElement)
      : null

  const read = (name: string): string => (computed?.getPropertyValue(name) ?? '').trim()

  return {
    color(name: string): string {
      return wrapChannels(read(name) || fallbacks[name] || '')
    },
    raw(name: string, fallback: string): string {
      return read(name) || fallback
    },
  }
}

/**
 * Build the Appearance for the current theme.
 *
 * `theme` still switches between Stripe's `stripe` and `night` bases rather
 * than relying on our variables alone: the base decides the parts of Stripe's
 * chrome that have no variable at all (icon fills, the wallet buttons, the
 * spinner), and a light base under dark variables leaves those artefacts
 * glowing white.
 */
export function buildStripeAppearance(isDark: boolean): Appearance {
  const token = createTokenReader(isDark)

  const surface = token.color('--ds-surface')
  const ink = token.color('--ds-ink')
  const line = token.color('--ds-line')
  const focus = token.color('--ds-focus')

  return {
    theme: isDark ? 'night' : 'stripe',
    variables: {
      colorPrimary: token.color('--ds-accent-solid'),
      colorBackground: surface,
      colorText: ink,
      colorTextSecondary: token.color('--ds-ink-secondary'),
      colorTextPlaceholder: token.color('--ds-ink-disabled'),
      colorDanger: token.color('--ds-danger'),
      fontFamily: token.raw('--ds-font-sans', FALLBACK_SANS),
      fontSizeBase: token.raw('--ds-text-base', '14px'),
      // 2px, from `--ds-radius`. Never the 8px this used to hardcode.
      borderRadius: token.raw('--ds-radius', '2px'),
      spacingUnit: token.raw('--ds-space-2', '4px'),
    },
    /*
     * Only the four rules that carry the design: a 1px hairline instead of
     * Stripe's default shadowed input, a focus ring that matches the global
     * `outline` treatment, and a tab strip that reads as a strip rather than a
     * row of floating pills. Stripe ignores properties it does not support, so
     * this list stays deliberately short — every extra rule is a guess about
     * another team's internal class names.
     */
    rules: {
      '.Input': {
        border: `1px solid ${line}`,
        boxShadow: 'none',
      },
      '.Input:focus': {
        border: `1px solid ${focus}`,
        boxShadow: `0 0 0 1px ${focus}`,
      },
      '.Tab': {
        border: `1px solid ${line}`,
        boxShadow: 'none',
      },
      '.Tab--selected': {
        border: `1px solid ${focus}`,
        boxShadow: 'none',
      },
    },
  }
}
