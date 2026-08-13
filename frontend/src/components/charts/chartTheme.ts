import { Chart as ChartJS } from 'chart.js'
import { computed, watch, type ComputedRef, type Ref } from 'vue'

import { useTheme } from '@/composables/useTheme'

/**
 * The bridge from CSS design tokens to chart.js, which needs concrete color
 * strings and cannot read a CSS custom property.
 *
 * THE BUG THIS REPLACES
 *
 * Ten components did this:
 *
 *     const isDark = computed(() => document.documentElement.classList.contains('dark'))
 *
 * That `computed` has no reactive dependencies, so Vue caches it forever. Every
 * chart in the app read the theme once at mount and never re-themed on toggle.
 * `useChartTheme()` below depends on the `isDark` ref from `useTheme`, so it
 * genuinely recomputes — which means charts will now repaint on toggle. Any
 * chart that builds its options object once and freezes it needs
 * `useThemedChart()` (bottom of this file) to actually redraw.
 */

/**
 * Fallback values, mirroring `src/styles/tokens.css`.
 *
 * Required, not defensive: jsdom does not compute custom properties from
 * stylesheets, so `getComputedStyle().getPropertyValue('--ds-…')` returns an
 * empty string under Vitest. Without these every charted color would collapse
 * to `rgb()` and the chart specs would assert on garbage.
 *
 * Keep in sync with tokens.css. `tokens.parity.spec.ts` checks that the key
 * sets match.
 */
const FALLBACK_LIGHT: Record<string, string> = {
  ink: '18 19 22',
  'ink-secondary': '92 96 104',
  'ink-tertiary': '112 116 124',
  surface: '255 255 255',
  'surface-raised': '255 255 255',
  'surface-sunken': '242 242 239',
  line: '216 216 211',
  'line-subtle': '226 226 222',
  'line-strong': '180 180 173',
  accent: '42 59 212',
  success: '15 123 63',
  warn: '149 97 10',
  danger: '180 35 24',
  info: '30 86 200',
}

const FALLBACK_DARK: Record<string, string> = {
  ink: '230 230 227',
  'ink-secondary': '146 152 162',
  'ink-tertiary': '123 129 139',
  surface: '16 17 19',
  'surface-raised': '23 24 27',
  'surface-sunken': '12 13 15',
  line: '42 44 49',
  'line-subtle': '35 36 40',
  'line-strong': '62 65 72',
  accent: '108 121 238',
  success: '63 207 110',
  warn: '240 180 41',
  danger: '255 122 112',
  info: '125 166 255',
}

/**
 * Categorical series palette. Ordered by perceptual distance, accent first.
 *
 * These are NOT the semantic colors. A series is not a status — if series 4
 * happened to be `--ds-danger` then every fourth line on every chart would read
 * as an error. Distinguishable in both themes and under deuteranopia.
 */
const SERIES_LIGHT = [
  '#2A3BD4',
  '#0F7B3F',
  '#95610A',
  '#B42318',
  '#6B4FA8',
  '#0E6E7A',
  '#B45BA0',
  '#4A5C1F',
]

const SERIES_DARK = [
  '#7D89F2',
  '#3FCF6E',
  '#F0B429',
  '#FF7A70',
  '#A794F0',
  '#4FC5D6',
  '#F08FD0',
  '#B8CC5A',
]

/**
 * Per-theme memo. `getComputedStyle` is microseconds, but it is called once
 * per token per chart-options rebuild, so the cache matters on the ops
 * dashboard where twenty charts rebuild together. Cleared on theme flip.
 */
const cache = new Map<string, string>()

function rawToken(name: string, dark: boolean): string {
  const key = `${dark ? 'd' : 'l'}:${name}`
  const hit = cache.get(key)
  if (hit !== undefined) return hit

  let value = ''
  if (typeof document !== 'undefined' && typeof getComputedStyle === 'function') {
    value = getComputedStyle(document.documentElement).getPropertyValue(`--ds-${name}`).trim()
  }
  if (!value) value = (dark ? FALLBACK_DARK : FALLBACK_LIGHT)[name] ?? '0 0 0'

  cache.set(key, value)
  return value
}

function isDarkNow(): boolean {
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
}

/** `token('accent')` → `'rgb(42 59 212)'` */
export function token(name: string): string {
  return `rgb(${rawToken(name, isDarkNow())})`
}

/** `tokenAlpha('accent', 0.12)` → `'rgb(42 59 212 / 0.12)'` */
export function tokenAlpha(name: string, alpha: number): string {
  return `rgb(${rawToken(name, isDarkNow())} / ${alpha})`
}

export function seriesPalette(dark = isDarkNow()): string[] {
  return dark ? SERIES_DARK : SERIES_LIGHT
}

/**
 * The residual-bucket color. "Others", "Unknown", "Uncategorized" and friends
 * are not a series — giving them the next categorical color implies they are
 * a peer of the ranked items. Neutral ink says "the rest" instead.
 */
export const OTHERS_COLOR = '#767A82'

/** Pick a series color by index, wrapping. */
export function seriesColor(index: number, dark = isDarkNow()): string {
  const palette = seriesPalette(dark)
  return palette[index % palette.length]
}

export interface ChartTheme {
  axis: string
  axisTitle: string
  grid: string
  gridZero: string
  tooltipBg: string
  tooltipFg: string
  tooltipMuted: string
  tooltipBorder: string
  series: string[]
  fontFamily: string
  fontSize: number
  success: string
  warn: string
  danger: string
  info: string
  accent: string
}

/*
 * Everything here resolves against the document, including `series`.
 *
 * This used to take a `dark` flag, which read as though it could build a theme
 * for either mode. It could not: `rawToken` uses that flag only as a cache key
 * and as the fallback table to reach for, while the value itself always comes
 * from `getComputedStyle(document.documentElement)`. So `buildTheme(true)` on a
 * light document returned light axis, grid and tooltip colours next to a dark
 * series palette — a half-flipped theme, and no type would have caught it.
 *
 * One source of truth is the honest shape: CSS custom properties live on the
 * document, so the document decides. `useChartTheme` still depends on the
 * `isDark` ref for reactivity, and `setTheme` writes the ref and the class
 * together, so the two never disagree.
 */
function buildTheme(): ChartTheme {
  return {
    axis: token('ink-tertiary'),
    axisTitle: token('ink-secondary'),
    grid: tokenAlpha('line-subtle', 1),
    gridZero: token('line-strong'),
    tooltipBg: token('surface-raised'),
    tooltipFg: token('ink'),
    tooltipMuted: token('ink-secondary'),
    tooltipBorder: token('line'),
    series: seriesPalette(),
    fontFamily: '"IBM Plex Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
    fontSize: 11,
    success: token('success'),
    warn: token('warn'),
    danger: token('danger'),
    info: token('info'),
    accent: token('accent'),
  }
}

/**
 * Copy `values` onto `target`, tolerating a target chart.js never created.
 * `target` is typed `unknown` because chart.js's own option interfaces have no
 * index signature, so a narrower type would reject every call site.
 */
function assign(target: unknown, values: Record<string, unknown>) {
  if (!target || typeof target !== 'object') return
  Object.assign(target, values)
}

function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

/**
 * Global chart.js defaults. Call once at app boot; `useChartTheme` re-applies
 * them on every theme flip so charts constructed later inherit the right ones.
 *
 * The element defaults encode design decisions, not preferences:
 *   `line.tension = 0`  — bezier smoothing invents values between samples. On
 *                          a latency or error-rate chart that is a lie.
 *   `point.radius = 0`  — dots on a dense time series are noise; `hitRadius`
 *                          keeps them hoverable anyway.
 *   `bar.borderRadius`  — 0, same reason every other radius is ~0.
 */
export function applyChartDefaults(theme: ChartTheme = buildTheme()): void {
  ChartJS.defaults.font.family = theme.fontFamily
  ChartJS.defaults.font.size = theme.fontSize
  ChartJS.defaults.color = theme.axis
  ChartJS.defaults.borderColor = theme.grid

  ChartJS.defaults.animation = prefersReducedMotion() ? false : { duration: 150 }

  /*
   * chart.js only creates `defaults.elements.<type>` for element types that
   * have actually been `register()`ed. A doughnut chart registers `ArcElement`
   * alone, so `defaults.elements.line` and `.bar` are genuinely undefined
   * there — writing to them throws. Assign only into what exists.
   */
  assign(ChartJS.defaults.elements?.line, {
    // Bezier smoothing invents values between samples. On a latency or
    // error-rate series that is a lie about the data.
    tension: 0,
    borderWidth: 1.5,
  })
  assign(ChartJS.defaults.elements?.point, {
    // Dots on a dense time series are noise; hitRadius keeps them hoverable.
    radius: 0,
    hitRadius: 8,
  })
  assign(ChartJS.defaults.elements?.bar, { borderRadius: 0 })
  assign(ChartJS.defaults.elements?.arc, { borderWidth: 1, borderColor: theme.tooltipBg })

  assign(ChartJS.defaults.plugins?.legend?.labels, {
    boxWidth: 8,
    boxHeight: 8,
    usePointStyle: false,
  })

  assign(ChartJS.defaults.plugins?.tooltip, {
    backgroundColor: theme.tooltipBg,
    titleColor: theme.tooltipFg,
    bodyColor: theme.tooltipMuted,
    borderColor: theme.tooltipBorder,
    borderWidth: 1,
    cornerRadius: 2,
    displayColors: true,
    boxWidth: 8,
    boxHeight: 8,
  })
}

/**
 * The composable every chart imports. Unlike the `computed` it replaces, this
 * one has a real reactive dependency and therefore actually recomputes.
 */
export function useChartTheme(): ComputedRef<ChartTheme> {
  const { isDark } = useTheme()

  return computed(() => {
    // Touch the ref so this actually recomputes, and drop the memo so the new
    // theme's values are read rather than the previous theme's cached ones. The
    // value itself comes from the document — see `buildTheme`.
    void isDark.value
    cache.clear()
    const theme = buildTheme()
    applyChartDefaults(theme)
    return theme
  })
}

type ChartLike = { chart?: { update: (mode?: string) => void } } | null | undefined

/**
 * Redraw on theme flip. chart.js reads colors when the options object is
 * constructed, so a chart whose options are built once will keep the old
 * palette until something forces an update.
 *
 * `'none'` skips the transition — animating a theme change looks like a glitch.
 */
export function useThemedChart(chartRef: Ref<ChartLike>): ComputedRef<ChartTheme> {
  const theme = useChartTheme()
  watch(theme, () => chartRef.value?.chart?.update('none'))
  return theme
}
