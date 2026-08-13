/**
 * `applyChartDefaults()` in `chartTheme.ts` is the single owner of line
 * smoothing, point radius, hit radius and corner radius. Its decisions are
 * about data integrity, not taste:
 *
 *   - bezier smoothing draws a curve through values that were never measured,
 *     and flattens the single-bucket spike a trend chart exists to expose;
 *   - dots on a dense time series are noise, so the radius is 0 and `hitRadius`
 *     keeps the samples hoverable anyway;
 *   - bars have square corners, like every other corner in the system.
 *
 * A per-dataset `tension: 0.3` silently opts back out of all of that, and an
 * audit found 22 of them across 9 files. This spec is the regression guard: it
 * reads the source of every chart component and fails if any of those keys
 * comes back. No canvas, no mounting — the claim is about configuration, which
 * is text.
 *
 * Comments are stripped before matching so the prose above (and the
 * explanations inside the components) can name a banned key without tripping
 * its own check.
 */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative, resolve, sep } from 'node:path'
import { describe, expect, it } from 'vitest'

const srcRoot = resolve(__dirname, '../../..')

/**
 * `features/channel-monitor-v2` holds the same contract in its own
 * `designSystem.tokens.spec.ts`; asserting it twice would just mean two files
 * to update when that surface moves.
 */
const EXCLUDED = ['features']

function collectChartComponents(dir: string, found: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      if (EXCLUDED.includes(entry) || entry === '__tests__') continue
      collectChartComponents(full, found)
      continue
    }
    if (!entry.endsWith('.vue')) continue
    if (readFileSync(full, 'utf8').includes("from 'vue-chartjs'")) found.push(full)
  }
  return found
}

function stripComments(source: string): string {
  return source
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:'"`\\])\/\/.*$/gm, '$1')
}

/** Every key here is already set globally, and set deliberately. */
const BANNED: Array<{ id: string; pattern: RegExp; why: string }> = [
  {
    id: 'tension',
    pattern: /\btension\s*:/,
    why: 'line.tension = 0 — smoothing invents values between samples',
  },
  { id: 'lineTension', pattern: /\blineTension\b/, why: 'chart.js v2 alias for tension' },
  {
    id: 'cubicInterpolationMode',
    pattern: /\bcubicInterpolationMode\b/,
    why: 'monotone interpolation invents values too',
  },
  { id: 'pointRadius', pattern: /\bpointRadius\s*:/, why: 'point.radius = 0' },
  { id: 'pointHitRadius', pattern: /\bpointHitRadius\s*:/, why: 'point.hitRadius = 8' },
  { id: 'borderRadius', pattern: /\bborderRadius\s*:/, why: 'bar.borderRadius = 0' },
  {
    id: 'borderWidth',
    pattern: /\bborderWidth\s*:/,
    why:
      'arc.borderWidth = 1 with borderColor = surface-raised — a ground-coloured hairline ' +
      'between slices, which is how two neighbouring slices of similar hue stay tellable apart',
  },
]

/**
 * Files that deviate on purpose, per key. Deviating is allowed; deviating
 * silently is not — every entry here must carry its reason in the component
 * itself, the way `pointHoverRadius: 5` does in `DailyRevenueChart`.
 *
 * ADDING A FILE HERE IS NOT A FIX unless the comment beside the override
 * explains why that chart is the exception.
 */
const ALLOWED: Record<string, string[]> = {
  /*
   * Both of these are unbounded doughnuts: one row per distinct model, with no
   * server-side LIMIT and no "Others" bucket to collapse the tail into. The
   * separator costs ~1px of arc per boundary, so on a sub-1% slice it is the
   * whole slice — it would delete data instead of clarifying it. Every other
   * doughnut in the app is bounded (top-N + Others, a closed endpoint set, or
   * four fixed error buckets) and takes the global hairline.
   */
  borderWidth: [
    'components/charts/ModelDistributionChart.vue',
    'components/user/dashboard/UserDashboardCharts.vue',
  ],
}

describe('chart components do not re-declare global chart.js defaults', () => {
  const components = collectChartComponents(srcRoot)
  const named = components.map((file) => ({
    name: relative(srcRoot, file).split(sep).join('/'),
    source: stripComments(readFileSync(file, 'utf8')),
  }))

  it('finds the chart components to check', () => {
    // A refactor that renames or relocates the charts must not silently turn
    // this whole spec into a no-op.
    expect(components.length).toBeGreaterThanOrEqual(10)
  })

  for (const { name, source } of named) {
    it(`${name} inherits smoothing and point geometry from applyChartDefaults()`, () => {
      for (const { id, pattern, why } of BANNED) {
        if ((ALLOWED[id] ?? []).includes(name)) continue
        expect(source, `${name} re-declares a global default (${why})`).not.toMatch(pattern)
      }
    })
  }

  for (const { id } of BANNED) {
    const allowed = ALLOWED[id]
    if (!allowed?.length) continue

    it(`keeps ALLOWED['${id}'] honest`, () => {
      // A stale allowlist is worse than none: it hides that the debt is already
      // paid, and it lets the override come back unnoticed under cover.
      const { pattern } = BANNED.find((entry) => entry.id === id)!
      const known = new Map(named.map(({ name, source }) => [name, source]))
      const stale = allowed
        .filter((name) => {
          const source = known.get(name)
          return source === undefined || !pattern.test(source)
        })
        .sort()

      expect(stale, `no longer overrides ${id} — remove from ALLOWED['${id}']`).toEqual([])
    })
  }
})

describe('the doughnut slice separator is a global, not a per-chart choice', () => {
  it('applyChartDefaults sets a 1px arc border in the raised-surface colour', async () => {
    // The ban above only means something if the global it defends is real. No
    // canvas and no mounting: registering ArcElement is what makes chart.js
    // create `defaults.elements.arc` at all, and the rest is configuration.
    const { Chart, ArcElement } = await import('chart.js')
    const { applyChartDefaults, token } = await import('../chartTheme')

    Chart.register(ArcElement)
    applyChartDefaults()

    expect(Chart.defaults.elements.arc.borderWidth).toBe(1)
    // Ground-coloured, so the separator reads as a cut rather than as a stroke.
    expect(Chart.defaults.elements.arc.borderColor).toBe(token('surface-raised'))
  })
})
