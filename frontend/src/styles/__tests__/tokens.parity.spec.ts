import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const tokensCss = readFileSync(resolve(here, '../tokens.css'), 'utf8')
const chartThemeTs = readFileSync(resolve(here, '../../components/charts/chartTheme.ts'), 'utf8')

/**
 * Extract the custom properties declared in the FIRST occurrence of a selector
 * block. `tokens.css` declares `:root` twice on purpose — once for the color
 * family that flips, once for the non-color tokens that do not — so parity is
 * checked against the first (color) block only.
 */
function declaredIn(selector: string, source = tokensCss): Set<string> {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const block = source.match(new RegExp(`(^|\\n)${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  if (!block) throw new Error(`No \`${selector} { … }\` block found in tokens.css`)
  return new Set(Array.from(block[2].matchAll(/(--ds-[\w-]+)\s*:/g), (m) => m[1]))
}

describe('design tokens: light/dark parity', () => {
  /*
   * This is the guard that makes light/dark correctness structural rather than
   * a thing someone checks by eye. A token present under `:root` but missing
   * under `.dark` does not fail loudly at runtime — it silently inherits the
   * light value onto a near-black surface, which is exactly the class of bug
   * that is invisible until a user reports unreadable text.
   */
  it('declares every color token in both :root and .dark', () => {
    const light = declaredIn(':root')
    const dark = declaredIn('.dark')

    const missingInDark = [...light].filter((t) => !dark.has(t)).sort()
    const missingInLight = [...dark].filter((t) => !light.has(t)).sort()

    expect(missingInDark, 'declared under :root but not .dark').toEqual([])
    expect(missingInLight, 'declared under .dark but not :root').toEqual([])
  })

  it('stores colors as space-separated RGB channels, not hex', () => {
    /*
     * Channel form is load-bearing, not stylistic. Tailwind wraps these in
     * `rgb(var(--ds-x) / <alpha-value>)`, which is what keeps the existing
     * opacity modifiers working (`bg-white/80`, `border-line/50`). A hex value
     * here would break every one of them silently.
     */
    const colorBlock = tokensCss.match(/(^|\n):root\s*\{([\s\S]*?)\n\}/)![2]
    const hexValues = Array.from(
      colorBlock.matchAll(/(--ds-[\w-]+)\s*:\s*(#[0-9a-fA-F]{3,8})\s*;/g),
      (m) => `${m[1]}: ${m[2]}`
    )
    expect(hexValues, 'use "R G B" channels so opacity modifiers keep working').toEqual([])
  })

  it('caps the radius scale at 4px so nothing can reintroduce a pill', () => {
    // `--ds-radius-full` is the deliberate exception: status dots and avatars
    // are the only round things in this system.
    const radii = Array.from(
      tokensCss.matchAll(/--ds-radius(-(?!full)[a-z]+)?\s*:\s*(\d+)px/g),
      (m) => Number(m[2])
    )
    expect(radii.length).toBeGreaterThan(0)
    expect(Math.max(...radii)).toBeLessThanOrEqual(4)
  })
})

describe('design tokens: chart.js fallback map stays in sync', () => {
  /*
   * `chartTheme.ts` carries a literal copy of a subset of the tokens, because
   * jsdom does not compute custom properties from stylesheets — without the
   * fallback every charted color under Vitest would be `rgb()`. A copy can
   * drift, so assert both directions: the names must exist in tokens.css, and
   * the two fallback maps must cover the same keys as each other.
   */
  function fallbackKeys(mapName: string): Set<string> {
    const block = chartThemeTs.match(new RegExp(`const ${mapName}[^=]*=\\s*\\{([\\s\\S]*?)\\n\\}`))
    if (!block) throw new Error(`No \`${mapName}\` in chartTheme.ts`)
    return new Set(Array.from(block[1].matchAll(/^\s*'?([\w-]+)'?\s*:/gm), (m) => m[1]))
  }

  const light = fallbackKeys('FALLBACK_LIGHT')
  const dark = fallbackKeys('FALLBACK_DARK')

  it('covers the same keys in both themes', () => {
    expect([...light].sort()).toEqual([...dark].sort())
  })

  it('only names tokens that actually exist in tokens.css', () => {
    const declared = declaredIn(':root')
    const unknown = [...light].filter((k) => !declared.has(`--ds-${k}`)).sort()
    expect(unknown, 'fallback names a token tokens.css does not declare').toEqual([])
  })
})
