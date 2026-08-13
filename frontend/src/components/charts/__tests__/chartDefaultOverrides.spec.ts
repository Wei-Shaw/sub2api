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
const BANNED: Array<{ pattern: RegExp; why: string }> = [
  { pattern: /\btension\s*:/, why: 'line.tension = 0 — smoothing invents values between samples' },
  { pattern: /\blineTension\b/, why: 'chart.js v2 alias for tension' },
  { pattern: /\bcubicInterpolationMode\b/, why: 'monotone interpolation invents values too' },
  { pattern: /\bpointRadius\s*:/, why: 'point.radius = 0' },
  { pattern: /\bpointHitRadius\s*:/, why: 'point.hitRadius = 8' },
  { pattern: /\bborderRadius\s*:/, why: 'bar.borderRadius = 0' },
]

describe('chart components do not re-declare global chart.js defaults', () => {
  const components = collectChartComponents(srcRoot)

  it('finds the chart components to check', () => {
    // A refactor that renames or relocates the charts must not silently turn
    // this whole spec into a no-op.
    expect(components.length).toBeGreaterThanOrEqual(10)
  })

  for (const file of components) {
    const name = relative(srcRoot, file).split(sep).join('/')

    it(`${name} inherits smoothing and point geometry from applyChartDefaults()`, () => {
      const source = stripComments(readFileSync(file, 'utf8'))

      for (const { pattern, why } of BANNED) {
        expect(source, `${name} re-declares a global default (${why})`).not.toMatch(pattern)
      }
    })
  }
})
