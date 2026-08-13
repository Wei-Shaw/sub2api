import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const SRC = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const stepsSource = readFileSync(resolve(SRC, 'components/Guide/steps.ts'), 'utf8')

/**
 * Every driver.js onboarding step targets a DOM element by selector. Those
 * selectors live in `steps.ts`; the elements they point at live in twenty-odd
 * views. Nothing connects the two, so renaming or restructuring markup breaks
 * the tour silently — `useOnboardingTour` just waits out `ELEMENT_TIMEOUT_MS`
 * (8s) and gives up on that step.
 *
 * That is not hypothetical. `accounts-create-btn` was already broken before
 * this spec existed: the selector is in `steps.ts` but the attribute is in no
 * file at all, so the admin tour dead-ends there for eight seconds. See the
 * `KNOWN_BROKEN` entry below.
 *
 * This matters most during a design-system migration, where the whole point is
 * that markup gets rewritten. Tiers 1, 3a, 4 and 6 all touch files holding
 * anchors, so this spec is the thing that stops the tour rotting one tier at a
 * time.
 */

/**
 * Anchors known to be broken BEFORE this spec was written. Pre-existing debt,
 * not a regression, so it does not fail the build — but it is enumerated so it
 * cannot be mistaken for "fine". Fix the anchor, delete the entry.
 *
 * `accounts-create-btn` → add `data-tour="accounts-create-btn"` to the create
 * button in `views/admin/AccountsView.vue`, then remove it from this list.
 */
const KNOWN_BROKEN = new Set(['[data-tour="accounts-create-btn"]'])

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry === '__tests__') continue
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) walk(full, out)
    else if (/\.(vue|ts)$/.test(entry) && !/\.spec\.ts$/.test(entry)) out.push(full)
  }
  return out
}

const allSource = walk(SRC)
  .filter((f) => !f.endsWith(join('components', 'Guide', 'steps.ts')))
  .map((f) => readFileSync(f, 'utf8'))
  .join('\n')

/** Selectors declared in steps.ts, e.g. `#sidebar-wallet`, `[data-tour="x"]`. */
const selectors = Array.from(
  new Set(Array.from(stepsSource.matchAll(/element:\s*'([^']+)'/g), (m) => m[1]))
)

/**
 * Does anything in the tree produce this selector?
 *
 * Deliberately a source-text search rather than a mount: the anchors are spread
 * across ~20 views behind auth, feature flags and simple-mode filtering, so
 * rendering them all would be a far more fragile test than checking that the
 * attribute is written somewhere. It cannot prove the element is *reachable*,
 * only that it exists — which is exactly the failure mode we keep hitting.
 */
function isProduced(selector: string): boolean {
  const attr = selector.match(/^\[data-tour="(.+)"\]$/)
  if (attr) return allSource.includes(`data-tour="${attr[1]}"`)

  const id = selector.match(/^#([\w-]+)$/)
  if (id) {
    // Either a literal `id="x"`, or the value appearing inside a ternary that
    // feeds `:id` — AppSidebar assigns three of these conditionally.
    return allSource.includes(`id="${id[1]}"`) || allSource.includes(`'${id[1]}'`)
  }

  return false
}

describe('onboarding tour anchors', () => {
  it('finds the step selectors in steps.ts', () => {
    expect(selectors.length).toBeGreaterThanOrEqual(20)
  })

  it('every anchor exists somewhere in the tree', () => {
    const missing = selectors.filter((s) => !KNOWN_BROKEN.has(s) && !isProduced(s)).sort()
    expect(
      missing,
      'a tour step points at an element no file produces; the tour will stall for 8s on it'
    ).toEqual([])
  })

  it('keeps KNOWN_BROKEN honest', () => {
    // If someone fixes an anchor, this fails and forces the list to shrink.
    const fixed = [...KNOWN_BROKEN].filter((s) => isProduced(s)).sort()
    expect(fixed, 'anchor now exists — remove it from KNOWN_BROKEN').toEqual([])
  })
})
