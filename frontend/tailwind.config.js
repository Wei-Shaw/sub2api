/** @type {import('tailwindcss').Config} */

/*
 * ─────────────────────────────────────────────────────────────────────────────
 * READ THIS BEFORE ADDING A COLOR UTILITY.
 *
 * There are two color families here and they behave differently.
 *
 *   Family B — semantic, defined in src/styles/tokens.css, FLIPS with `.dark`.
 *              `surface` `line` `ink` `accent` `success` `warn` `danger` `info`
 *              `canvas` `focus` `overlay`.
 *              New code uses ONLY these, and NEVER writes a `dark:` counterpart:
 *                  ok:    class="bg-surface text-ink border-line"
 *                  smell: class="bg-surface dark:bg-surface"
 *                  bug:   class="bg-surface dark:bg-dark-800"
 *
 *   Family A — legacy ramps, plain static hex, DO NOT flip.
 *              `gray-*` `dark-*` `primary-*` `red-*` `blue-*` …
 *              ~14,500 existing utilities reference these through explicit
 *              `dark:` pairs. They are re-tinted onto the new palette so the
 *              unmigrated app renders correctly, and they are burned down by
 *              src/__tests__/designSystem.legacy.spec.ts. Do not add new uses.
 *
 * FOOTGUN: `dark` is a COLOR NAME here, not the dark-mode variant.
 *   `dark:bg-dark-800` is correct.  `bg-dark-800` alone breaks light mode.
 *   2,972 sites use it, so it cannot be renamed. The audit spec flags bare uses.
 * ─────────────────────────────────────────────────────────────────────────────
 */

/** Family B accessor: `rgb(var(--ds-x) / <alpha>)`, so opacity modifiers work. */
const ds =
  (name) =>
  ({ opacityValue } = {}) =>
    opacityValue === undefined
      ? `rgb(var(--ds-${name}))`
      : `rgb(var(--ds-${name}) / ${opacityValue})`

/*
 * Family A — warm neutral. Backs `gray` and the four aliases nobody should
 * have used (`slate` `zinc` `neutral` `stone`, ~170 sites between them).
 *
 * Index 400 is a deliberate compromise and the only value in this file that
 * does not cleanly hit AA. It carries two incompatible roles:
 *     813 sites  `text-gray-400`        → light-mode tertiary text on #FFFFFF
 *   1,451 sites  `dark:text-gray-400`   → dark-mode secondary text on #101113
 * No single hex reaches 4.5:1 against both grounds (the ceiling for one is
 * below the floor for the other). #767A82 is the value that maximizes the
 * worse of the two: 4.31:1 light / 4.39:1 dark. The old value was #9CA3AF at
 * 2.55:1 in light mode, so this is a large improvement at every one of those
 * sites — but it is not a pass. Migrated code uses the single-role Family B
 * tokens instead: `--ds-ink-tertiary` (4.6:1) and `--ds-ink-secondary` (6.3:1).
 */
const neutralRamp = {
  50: '#FAFAF9',
  100: '#F2F2EF',
  200: '#E2E2DE',
  300: '#C9C9C4',
  400: '#767A82',
  500: '#5C6068',
  600: '#4A4E55',
  700: '#3A3D43',
  800: '#2A2C31',
  900: '#17181B',
  950: '#08090A',
}

/*
 * Family A — the `dark-*` ramp. Indices are chosen to match how the app
 * actually uses them, not to be a uniform lightness ladder:
 *   700 (993 sites) is overwhelmingly `dark:border-dark-700`  → hairline
 *   800 (411 sites) is `dark:bg-dark-800`                     → panel surface
 *   900 (203 sites) is `dark:bg-dark-900`                     → sidebar/deeper
 *   400 (296 sites) is `dark:text-dark-400`                   → muted text
 */
const nightRamp = {
  50: '#F2F2F0',
  100: '#E6E6E3',
  200: '#C4C8CF',
  300: '#A8ADB6',
  400: '#7B818B',
  500: '#55585F',
  600: '#3E4148',
  700: '#2A2C31',
  800: '#101113',
  900: '#0C0D0F',
  950: '#08090A',
}

/** Family A — ultramarine. Replaces the teal `primary`; also backs `indigo`. */
const ultramarineRamp = {
  50: '#F0F2FE',
  100: '#DFE3FD',
  200: '#C2C9FB',
  300: '#9AA5F7',
  400: '#6C79EE',
  500: '#4A56E2',
  600: '#2A3BD4',
  700: '#232FAE',
  800: '#1E288B',
  900: '#1C246E',
  950: '#131743',
}

/*
 * Family A — muted violet. Backs `purple` `violet` `fuchsia` `pink` (~150
 * sites), which this app uses as CATEGORY colors on badges, not as status.
 * Deliberately desaturated so it reads as a category next to ultramarine
 * rather than competing with it for "this is interactive".
 */
const violetRamp = {
  50: '#F6F2FA',
  100: '#EBE3F5',
  200: '#DACCEC',
  300: '#C0ABDD',
  400: '#A78BCB',
  500: '#8A66B4',
  600: '#6F4E96',
  700: '#5A3E79',
  800: '#45305C',
  900: '#2E2040',
  950: '#1B1329',
}

/** Family A — info blue. Backs `blue` `sky` `cyan`. Distinct in lightness from accent. */
const infoRamp = {
  50: '#EAF0FC',
  100: '#D5E1F9',
  200: '#B6CBF3',
  300: '#8FAFEA',
  400: '#7DA6FF',
  500: '#2C66D8',
  600: '#1E56C8',
  700: '#1846A3',
  800: '#143A85',
  900: '#0C2352',
  950: '#071634',
}

/** Family A — success green. Backs `emerald` `green` `lime` `teal`. */
const successRamp = {
  50: '#E9F6EE',
  100: '#D2EDDD',
  200: '#A9DCBF',
  300: '#74C797',
  400: '#3FCF6E',
  500: '#15943F',
  600: '#0F7B3F',
  700: '#0C6333',
  800: '#094D28',
  900: '#06331B',
  950: '#041F11',
}

/** Family A — warn amber. Backs `amber` `yellow`. */
const warnRamp = {
  50: '#FCF4E4',
  100: '#F8E8C4',
  200: '#F0D492',
  300: '#E4BC5E',
  400: '#F0B429',
  500: '#B27A0C',
  600: '#95610A',
  700: '#7A4F08',
  800: '#5E3D06',
  900: '#3D2704',
  950: '#241703',
}

/*
 * Family A — burnt orange. Kept as its OWN ramp rather than folded into
 * `warn`: ~110 sites use orange alongside amber, and collapsing both would
 * erase a distinction the badges are currently making.
 */
const orangeRamp = {
  50: '#FDF0E8',
  100: '#FADDCB',
  200: '#F5BF9C',
  300: '#EC9A64',
  400: '#E07B3C',
  500: '#C25A19',
  600: '#A24812',
  700: '#833A0F',
  800: '#632B0B',
  900: '#3F1C07',
  950: '#261104',
}

/** Family A — danger red. Backs `red` `rose`. */
const dangerRamp = {
  50: '#FDECEA',
  100: '#FBD9D5',
  200: '#F5B5AE',
  300: '#EC8B80',
  400: '#FF7A70',
  500: '#D6392B',
  600: '#B42318',
  700: '#8F1C13',
  800: '#6C150E',
  900: '#4C0F0A',
  950: '#2E0906',
}

export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    /*
     * TOP-LEVEL, not `extend` — this REPLACES Tailwind's default palette.
     * Anything not listed here emits no CSS at all. That is intentional: it
     * means a stray `bg-lavender-300` is inert rather than off-system.
     */
    colors: {
      inherit: 'inherit',
      current: 'currentColor',
      transparent: 'transparent',
      black: '#000000',
      white: '#ffffff',

      // ── Family B ──────────────────────────────────────────────────────
      canvas: ds('canvas'),
      overlay: ds('overlay'),
      focus: ds('focus'),
      'focus-contrast': ds('focus-contrast'),
      surface: {
        DEFAULT: ds('surface'),
        sunken: ds('surface-sunken'),
        raised: ds('surface-raised'),
        hover: ds('surface-hover'),
        active: ds('surface-active'),
      },
      line: {
        DEFAULT: ds('line'),
        subtle: ds('line-subtle'),
        strong: ds('line-strong'),
        emphasis: ds('line-emphasis'),
      },
      ink: {
        DEFAULT: ds('ink'),
        secondary: ds('ink-secondary'),
        tertiary: ds('ink-tertiary'),
        disabled: ds('ink-disabled'),
        inverse: ds('ink-inverse'),
      },
      accent: {
        DEFAULT: ds('accent'),
        hover: ds('accent-hover'),
        active: ds('accent-active'),
        // `accent-solid` is the only accent safe to put white text on in dark
        // mode; in light mode it is identical to `accent`. Fills use it.
        solid: ds('accent-solid'),
        on: ds('accent-on'),
        tint: ds('accent-tint'),
        'tint-strong': ds('accent-tint-strong'),
        line: ds('accent-line'),
      },
      success: { DEFAULT: ds('success'), tint: ds('success-tint') },
      warn: { DEFAULT: ds('warn'), tint: ds('warn-tint') },
      danger: { DEFAULT: ds('danger'), tint: ds('danger-tint') },
      info: { DEFAULT: ds('info'), tint: ds('info-tint') },
      status: { neutral: ds('neutral-status'), 'neutral-tint': ds('neutral-tint') },

      // ── Family A: legacy names, static hex, re-tinted. Compat only. ────
      gray: neutralRamp,
      slate: neutralRamp,
      zinc: neutralRamp,
      neutral: neutralRamp,
      stone: neutralRamp,
      dark: nightRamp,
      primary: ultramarineRamp,
      indigo: ultramarineRamp,
      purple: violetRamp,
      violet: violetRamp,
      fuchsia: violetRamp,
      pink: violetRamp,
      blue: infoRamp,
      sky: infoRamp,
      cyan: infoRamp,
      emerald: successRamp,
      green: successRamp,
      lime: successRamp,
      teal: successRamp,
      amber: warnRamp,
      yellow: warnRamp,
      orange: orangeRamp,
      red: dangerRamp,
      rose: dangerRamp,
    },

    /*
     * TOP-LEVEL override. This is the single highest-leverage line in the
     * file: it snaps all 1,910 `rounded-*` utilities in the app to the new
     * scale without touching a view. `full` survives because status dots and
     * avatars need it — the badge pills are handled by codemod, not here.
     */
    borderRadius: {
      none: '0px',
      sm: '1px',
      DEFAULT: '2px',
      md: '2px',
      lg: '2px',
      xl: '2px',
      '2xl': '4px',
      '3xl': '4px',
      full: '9999px',
    },

    /*
     * TOP-LEVEL override. Every blur collapses to either a hairline rule or
     * one of the two real elevations. Legacy names are kept as aliases so the
     * ~270 existing `shadow-*` sites degrade to the right thing.
     */
    boxShadow: {
      none: 'none',
      sticky: 'var(--ds-shadow-sticky)',
      popover: 'var(--ds-shadow-popover)',
      modal: 'var(--ds-shadow-modal)',
      sm: 'var(--ds-shadow-sticky)',
      DEFAULT: 'var(--ds-shadow-popover)',
      md: 'var(--ds-shadow-popover)',
      lg: 'var(--ds-shadow-popover)',
      xl: 'var(--ds-shadow-modal)',
      '2xl': 'var(--ds-shadow-modal)',
      inner: 'inset 0 1px 0 0 rgb(var(--ds-line-subtle))',
    },

    /*
     * TOP-LEVEL override, every value zeroed. There is no glass in this
     * system, and 31 view sites still write `backdrop-blur-*` directly where
     * `@apply`-level neutralization cannot reach them. Zeroing the scale makes
     * every one of those utilities inert in a single line; the audit spec
     * still reports them so they get deleted rather than left as dead classes.
     */
    backdropBlur: {
      none: '0',
      sm: '0',
      DEFAULT: '0',
      md: '0',
      lg: '0',
      xl: '0',
      '2xl': '0',
      '3xl': '0',
    },

    extend: {
      fontFamily: {
        sans: 'var(--ds-font-sans)',
        mono: 'var(--ds-font-mono)',
      },

      fontSize: {
        '2xs': [
          'var(--ds-text-2xs)',
          { lineHeight: 'var(--ds-lh-2xs)', letterSpacing: 'var(--ds-tr-2xs)' },
        ],
        xs: [
          'var(--ds-text-xs)',
          { lineHeight: 'var(--ds-lh-xs)', letterSpacing: 'var(--ds-tr-xs)' },
        ],
        sm: ['var(--ds-text-sm)', { lineHeight: 'var(--ds-lh-sm)' }],
        base: ['var(--ds-text-base)', { lineHeight: 'var(--ds-lh-base)' }],
        md: [
          'var(--ds-text-md)',
          { lineHeight: 'var(--ds-lh-md)', letterSpacing: 'var(--ds-tr-md)' },
        ],
        lg: [
          'var(--ds-text-lg)',
          { lineHeight: 'var(--ds-lh-lg)', letterSpacing: 'var(--ds-tr-lg)' },
        ],
        xl: [
          'var(--ds-text-xl)',
          { lineHeight: 'var(--ds-lh-xl)', letterSpacing: 'var(--ds-tr-xl)' },
        ],
        '2xl': [
          'var(--ds-text-2xl)',
          { lineHeight: 'var(--ds-lh-2xl)', letterSpacing: 'var(--ds-tr-2xl)' },
        ],
        '3xl': [
          'var(--ds-text-3xl)',
          { lineHeight: 'var(--ds-lh-3xl)', letterSpacing: 'var(--ds-tr-3xl)' },
        ],
        // Editorial ceiling: nothing in an ops console needs to be bigger.
        '4xl': [
          'var(--ds-text-3xl)',
          { lineHeight: 'var(--ds-lh-3xl)', letterSpacing: 'var(--ds-tr-3xl)' },
        ],
        '5xl': [
          'var(--ds-text-3xl)',
          { lineHeight: 'var(--ds-lh-3xl)', letterSpacing: 'var(--ds-tr-3xl)' },
        ],
      },

      /*
       * Remapping the whole duration scale onto the motion tokens is what
       * gives every `duration-*` utility in all 297 files free
       * `prefers-reduced-motion` support — the media query in tokens.css
       * collapses the three variables to 1ms and everything follows.
       */
      transitionDuration: {
        DEFAULT: 'var(--ds-dur-base)',
        fast: 'var(--ds-dur-fast)',
        base: 'var(--ds-dur-base)',
        slow: 'var(--ds-dur-slow)',
        0: '0ms',
        75: 'var(--ds-dur-fast)',
        100: 'var(--ds-dur-fast)',
        150: 'var(--ds-dur-base)',
        200: 'var(--ds-dur-base)',
        300: 'var(--ds-dur-slow)',
        500: 'var(--ds-dur-slow)',
        700: 'var(--ds-dur-slow)',
        1000: 'var(--ds-dur-slow)',
      },
      transitionTimingFunction: {
        DEFAULT: 'var(--ds-ease-std)',
        out: 'var(--ds-ease-out)',
        in: 'var(--ds-ease-in)',
        'in-out': 'var(--ds-ease-std)',
      },

      // Focus is an `outline` in this system (see style.css). These exist so
      // the ~151 legacy `focus:ring-primary-500` sites land on the right hue
      // from day one rather than staying teal until they are migrated.
      ringColor: { DEFAULT: ds('focus') },
      ringWidth: { DEFAULT: '2px' },
      ringOffsetWidth: { DEFAULT: '2px' },
      ringOffsetColor: { DEFAULT: ds('surface') },

      spacing: {
        row: 'var(--ds-row-h)',
        'row-comfy': 'var(--ds-row-h-comfy)',
        'row-touch': 'var(--ds-row-h-touch)',
        hdr: 'var(--ds-header-h)',
        'app-hdr': 'var(--ds-app-header-h)',
        page: 'var(--ds-page-pad)',
        sidebar: 'var(--ds-sidebar-w)',
        'sidebar-collapsed': 'var(--ds-sidebar-w-collapsed)',
      },

      /*
       * Only two animations survive, and both are load-bearing:
       *   spin  — 197 sites, all loading spinners
       *   pulse — 92 sites, all skeletons
       * The reduced-motion carve-outs for them live in style.css: spin slows
       * to 1.2s rather than freezing (a frozen spinner reads as a hung app),
       * pulse stops entirely in favour of a static dim.
       * fade-in / slide-* / scale-in / shimmer / glow are gone.
       */
      animation: {
        spin: 'spin 0.7s linear infinite',
        pulse: 'ds-pulse 1.6s var(--ds-ease-std) infinite',
      },
      keyframes: {
        'ds-pulse': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.45' },
        },
      },
    },
  },
  plugins: [],
}
