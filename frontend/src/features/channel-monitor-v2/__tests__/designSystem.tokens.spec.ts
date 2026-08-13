/**
 * Tier 3b design-system contract for the channel-monitor surface.
 *
 * WHAT THIS REPLACES, AND WHY
 *
 * `designSystem.structure.spec.ts` used exactly this technique — read the `.vue`
 * source, assert on strings, never mount — and that technique is right: these
 * are markup-level facts (which utility classes a template writes) that a
 * mounted render would only obscure, and a source assertion cannot be satisfied
 * by accident.
 *
 * What was wrong was the contract it enforced. It pinned the OLD visual
 * language: `rounded-3xl`, `ring-1 ring-gray-900/5`, `card-header`, `stat-card`,
 * `min-h-[360px]`. So the one spec in this feature's critical-CI list was
 * actively holding the surface on the system the rewrite is removing — passing
 * it and migrating were mutually exclusive.
 *
 * This file keeps the technique and inverts the contract. Three claims per
 * migrated file:
 *
 *   1. it imports the new primitives BY DIRECT PATH (never through
 *      `components/common/index.ts` — that barrel pulls `createI18n` into the
 *      module graph and breaks specs that mock `vue-i18n`, which has already
 *      broken specs in this repo);
 *   2. it uses the system's layout / data-surface classes;
 *   3. it contains ZERO legacy tokens.
 *
 * Comments are stripped before matching, so the prose above can name a banned
 * token without tripping its own check.
 */
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const root = resolve(__dirname, '../../..')

/**
 * Per-line heuristics are not enough: an HTML comment body continues onto lines
 * that begin with ordinary text. Same approach as `designSystem.legacy.spec.ts`.
 */
function stripComments(source: string): string {
  return source
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/.*$/gm, '$1')
}

function read(rel: string): string {
  return stripComments(readFileSync(resolve(root, rel), 'utf8'))
}

/** Every file this tier owns. Adding a file here is the only way in. */
const SURFACE = [
  'views/user/ChannelStatusView.vue',
  'views/user/ChannelStatusV1View.vue',
  'views/user/ChannelStatusV2View.vue',
  'features/channel-monitor-v2/FilterMultiSelect.vue',
  'features/channel-monitor-v2/MetricCell.vue',
  'features/channel-monitor-v2/MonitorRankBadge.vue',
  'features/channel-monitor-v2/MonitorSettingsPanel.vue',
  'features/channel-monitor-v2/MonitorTrendChart.vue',
  'features/channel-monitor-v2/RelayPulseMatrix.vue',
  'components/user/monitor/MonitorAvailabilityRow.vue',
  'components/user/monitor/MonitorCard.vue',
  'components/user/monitor/MonitorCardGrid.vue',
  'components/user/monitor/MonitorHero.vue',
  'components/user/monitor/MonitorMetricPair.vue',
  'components/user/monitor/MonitorTimeline.vue',
  'components/user/monitor/ProviderIcon.vue',
] as const

/**
 * Tokens that must not reappear anywhere on this surface. Each one is a
 * *rendered* fact that no config change can reach — a class written into a
 * template still emits, or (in the `dark:` case) still double-applies.
 */
const BANNED: Array<{ id: string; pattern: RegExp; why: string }> = [
  {
    id: 'pill-radius',
    pattern: /\brounded-(2xl|3xl)\b/,
    why: 'Radius ceiling is 4px; `full` is status dots and avatars only.',
  },
  {
    id: 'ring',
    // eslint-disable-next-line no-useless-escape
    pattern: /(^|[\s"'`:[])ring-/,
    why: 'Focus is the global `outline` in style.css — a box-shadow ring is clipped by `overflow: hidden` and needs a per-surface offset color.',
  },
  {
    id: 'gradient',
    pattern: /\bbg-gradient-to-[trbl]{1,2}\b|linear-gradient\(/,
    why: 'A gradient implies a value ramp the data does not have.',
  },
  {
    id: 'backdrop-blur',
    pattern: /\bbackdrop-blur(-\w+)?\b/,
    why: 'There is no glass in this system.',
  },
  {
    id: 'glow-shadow',
    pattern: /\bshadow-(glow|glow-lg|glass|glass-sm|card|card-hover|inner-glow)\b/,
    why: 'Elevation is popover/modal or a 1px rule.',
  },
  {
    id: 'hover-lift',
    pattern: /hover:-translate-y-|active:scale-\[/,
    why: 'Nothing lifts on hover and nothing shrinks on press.',
  },
  {
    id: 'dark-variant',
    pattern: /\bdark:/,
    why: 'Family B tokens flip on their own; a `dark:` pair beside one double-applies. New code writes ZERO of these.',
  },
  {
    id: 'bare-dark-color',
    pattern: /(^|[\s"'`])(bg|text|border|ring|divide|placeholder|from|to|via)-dark-\d/,
    why: '`dark-*` is a static near-black color, not a variant — it paints light mode black.',
  },
]

describe('channel-monitor surface carries no legacy design tokens', () => {
  for (const rel of SURFACE) {
    it(`${rel} is clean`, () => {
      const src = read(rel)
      for (const rule of BANNED) {
        const hit = src
          .split('\n')
          .map((line) => line.trim())
          .filter((line) => rule.pattern.test(line))
        expect(hit, `${rel} — ${rule.id}: ${rule.why}`).toEqual([])
      }
    })
  }
})

describe('channel-monitor surface consumes the primitive layer', () => {
  it('the V2 shell imports primitives by direct path and uses system layout classes', () => {
    const src = read('views/user/ChannelStatusV2View.vue')

    // Primitives, by direct path — never through the barrel.
    expect(src).toContain("from '@/components/common/NumCell.vue'")
    expect(src).toContain("from '@/components/common/StatusDot.vue'")
    expect(src).toContain("from '@/components/common/Meter.vue'")
    expect(src).toContain("from '@/components/common/Button.vue'")
    expect(src).toContain("from '@/components/common/Badge.vue'")
    expect(src).toContain("from '@/components/common/primitives'")
    expect(src).not.toMatch(/from '@\/components\/common'/)
    expect(src).not.toMatch(/from '@\/components\/common\/index'/)

    // Layout / data-surface classes.
    expect(src).toContain('page-title')
    expect(src).toContain('rounded border border-line bg-surface')
    expect(src).toContain('monitor-toolbar')
    expect(src).toContain('divide-line-subtle')
    // Tables are the shared `.table` surface, with numeric columns declared.
    expect(src).toContain('class="table')
    expect(src).toContain('is-numeric')
    // Selection, not status: the accent marks the current user's row.
    expect(src).toContain('is-selected')
    // Overview-first: the KPI strip precedes the primary viz.
    expect(src.indexOf('summaryAria')).toBeLessThan(src.indexOf('MonitorTrendChart'))
    // Dense tables still scroll internally rather than growing the page.
    expect(src).toMatch(/max-h-\[min\(52vh/)
    expect(src).toContain('overflow-auto')
    // Behaviour that must survive the visual pass.
    expect(src).toContain('trendView')
    expect(src).toContain("'platform_group'")
    expect(src).toContain('healthModeOptions')
    expect(src).toContain('clearFilters')
    expect(src).toContain("'cache'")
    expect(src).toContain('MonitorTrendChart')
    expect(src).toContain('RelayPulseMatrix')
    // No page-level fixed min-width that forces viewport horizontal scroll.
    expect(src).not.toMatch(/min-width:\s*980px/)
    expect(src).not.toMatch(/min-w-\[980px\]/)
  })

  it('every quantity on the V2 shell is rendered through NumCell', () => {
    const src = read('views/user/ChannelStatusV2View.vue')
    // Latency, success, cache, throughput and error counts all go through it.
    expect(src.match(/<NumCell/g)?.length ?? 0).toBeGreaterThanOrEqual(10)
    /*
     * "Not measured" and "measured zero" are different facts on a monitor, and
     * the getters below are what keeps them apart — each returns null rather
     * than coercing an unreported channel to 0.
     */
    expect(src).toContain('function measured(')
    expect(src).toMatch(/function successPct\([\s\S]*?: number \| null/)
    expect(src).toMatch(/function ttftMs\([\s\S]*?: number \| null/)
    expect(src).toMatch(/function cachePct\([\s\S]*?: number \| null/)
  })

  it('the pulse matrix owns the health scale and renders cells through NumCell', () => {
    const src = read('features/channel-monitor-v2/RelayPulseMatrix.vue')

    expect(src).toContain("from '@/components/common/NumCell.vue'")
    expect(src).toContain("from '@/components/common/Badge.vue'")
    expect(src).toContain("from '@/components/common/Button.vue'")
    expect(src).toContain('rounded border border-line bg-surface')
    expect(src).toContain('matrix-scroll')
    expect(src).toContain('overflow-auto')
    expect(src).toMatch(/max-h-\[min\(42vh/)
    // Hover tooltips, never a click modal.
    expect(src).toContain('pulse-tooltip')
    expect(src).not.toContain('modal-overlay')
    expect(src).not.toContain('modal-content')

    // The band ramp lives here and ONLY here — see the ChannelStatusV2View
    // assertion below for the other half of that contract.
    expect(src).toContain('.health-score10')
    expect(src).toContain('.health-unknown')
    // ...and it is expressed in tokens, not hex.
    expect(src).toContain('var(--ds-warn)')
    expect(src).toContain('var(--ds-danger)')
    expect(src).not.toMatch(/#[0-9a-fA-F]{3,8}\b/)
    // Row geometry comes from the data-surface tokens.
    expect(src).toContain('var(--ds-row-h)')
    expect(src).toContain('var(--ds-header-h)')
    expect(src).toContain('border-line-subtle')
    expect(src).toContain('bg-surface-sunken')
    /*
     * `--color-primary-500` was never declared anywhere in the repo (the token
     * family is `--ds-*`), so the focus outline always fell through to a
     * hardcoded indigo and could not follow the accent.
     */
    expect(src).not.toContain('--color-primary-500')
    expect(src).toContain('var(--ds-shadow-popover)')
  })

  it('the V2 shell no longer duplicates the health-band scale', () => {
    const src = read('views/user/ChannelStatusV2View.vue')
    // 16 near-identical declarations used to live in both files, so every band
    // change had to be made twice and noticed twice.
    expect(src).not.toContain('health-score')
    expect(src).not.toContain('<style')
    expect(src).toContain('StatusDot')
  })

  it('the trend chart draws measured values in themed series colors', () => {
    const src = read('features/channel-monitor-v2/MonitorTrendChart.vue')

    expect(src).toContain("from '@/components/charts/chartTheme'")
    expect(src).toContain('useThemedChart')
    expect(src).toContain('chartTheme.value.series')
    expect(src).toContain("from '@/components/common/Badge.vue'")
    expect(src).toContain("from '@/components/common/Button.vue'")
    expect(src).toContain('rounded border border-line bg-surface')
    expect(src).toContain('EmptyState')

    /*
     * No invented data. Bezier smoothing (`tension`, `cubicInterpolationMode`)
     * fabricates values between samples; the 3-point moving average that used
     * to feed these series fabricated the samples themselves, halving exactly
     * the single-bucket spikes this page exists to surface.
     */
    expect(src).not.toContain('tension')
    expect(src).not.toContain('cubicInterpolationMode')
    expect(src).not.toContain('smoothTrend')
    // No hardcoded palette, and no theme branch reimplementing chartTheme.
    expect(src).not.toMatch(/#[0-9a-fA-F]{3,8}\b/)
    expect(src).not.toContain('isDark')
    // Defaults set globally by applyChartDefaults() must not be re-declared.
    expect(src).not.toContain('pointRadius')
    expect(src).not.toContain('borderWidth')
  })

  it('MetricCell is a type-led cell, not a stat card', () => {
    const src = read('features/channel-monitor-v2/MetricCell.vue')
    expect(src).toContain("from '@/components/common/StatusDot.vue'")
    expect(src).toContain("from '@/components/common/primitives'")
    // No box of its own: a row of these lives inside ONE bordered panel.
    expect(src).not.toContain('stat-card')
    expect(src).not.toContain('border border-line')
    expect(src).toContain('text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary')
    expect(src).toContain('font-mono')
    expect(src).toContain('tabular-nums')
    // Colour marks a crossed threshold and nothing else — `healthy` is ink.
    expect(src).toContain("'text-ink'")
    expect(src).toContain("'text-warn'")
    expect(src).toContain("'text-danger'")
    // The status channel is a dot AND a word; the label is a required pairing.
    expect(src).toContain('stateLabel')
  })

  it('the V1 card surface uses primitives for state and quantities', () => {
    const card = read('components/user/monitor/MonitorCard.vue')
    expect(card).toContain("from '@/components/common/StatusDot.vue'")
    expect(card).toContain("from '@/components/common/Badge.vue'")
    expect(card).toContain('rounded border border-line bg-surface')
    // Hover moves the ground only.
    expect(card).toContain('hover:bg-surface-hover')
    // `providerGradient()` still returns `bg-gradient-to-br` ramps for the
    // unmigrated admin surfaces; this card must not call it.
    expect(card).not.toContain('providerGradient')
    expect(card).not.toContain('statusBadgeClass')

    const pair = read('components/user/monitor/MonitorMetricPair.vue')
    expect(pair).toContain("from '@/components/common/NumCell.vue'")
    expect(pair).toContain('number | null')

    const availability = read('components/user/monitor/MonitorAvailabilityRow.vue')
    expect(availability).toContain("from '@/components/common/Meter.vue'")
    expect(availability).toContain("from '@/components/common/NumCell.vue'")
    // `hslForPct()` painted a continuous rainbow, so the hue was the whole signal.
    expect(availability).not.toContain('hslForPct')

    const hero = read('components/user/monitor/MonitorHero.vue')
    expect(hero).toContain("from '@/components/common/StatusDot.vue'")
    expect(hero).toContain("from '@/components/common/Button.vue'")
    // A permanent animation on a permanent state.
    expect(hero).not.toContain('animate-pulse')

    const timeline = read('components/user/monitor/MonitorTimeline.vue')
    // Operational is a neutral hairline, not sixty bars of emerald.
    expect(timeline).toContain("operational: 'bg-line-strong'")
    expect(timeline).toContain("degraded: 'bg-warn'")
    expect(timeline).toContain("failed: 'bg-danger'")
  })

  it('the settings panel and filter menu use system chrome', () => {
    const settings = read('features/channel-monitor-v2/MonitorSettingsPanel.vue')
    expect(settings).toContain("from '@/components/common/Button.vue'")
    expect(settings).toContain("from '@/components/common/Badge.vue'")
    expect(settings).toContain('page-header')
    expect(settings).toContain('page-title')
    expect(settings).toContain('rounded border border-line bg-surface')
    expect(settings).toContain('tab-active')
    expect(settings).toMatch(/max-h-\[min\(40vh/)
    expect(settings).toContain('accent-color: rgb(var(--ds-accent-solid))')

    const filter = read('features/channel-monitor-v2/FilterMultiSelect.vue')
    expect(filter).toContain("from '@/components/common/NumCell.vue'")
    expect(filter).toContain('dropdown')
    expect(filter).toContain('dropdown-item')
    // `.dropdown` in style.css already supplies surface, hairline and elevation.
    expect(filter).not.toContain('shadow-lg')
    expect(filter).not.toContain('transition-all')
  })

  it('the V1/V2 dispatcher and the V1 shell stay on the feature flag', () => {
    const dispatcher = read('views/user/ChannelStatusView.vue')
    expect(dispatcher).toContain('isChannelMonitorV1Mode')
    expect(dispatcher).toContain('ChannelStatusV1View')
    expect(dispatcher).toContain('ChannelStatusV2View')

    const v1 = read('views/user/ChannelStatusV1View.vue')
    expect(v1).toContain('page-title')
    expect(v1).toContain('MonitorCardGrid')
    expect(v1).toContain('MonitorDetailDialog')
    expect(v1).toContain('useAutoRefresh')
  })
})
