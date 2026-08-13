import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const SRC = resolve(dirname(fileURLToPath(import.meta.url)), '..')

/**
 * The migration ratchet.
 *
 * The Swiss/editorial rewrite lands its palette, type, radius and elevation
 * globally in one commit (tailwind.config.js + style.css), so the whole app
 * changes appearance without any view being touched. What that global lever
 * CANNOT reach is markup: a `bg-gradient-to-r` or a `backdrop-blur-xl` written
 * directly into a component still renders a gradient or a blur.
 *
 * This spec is how those get burned down instead of forgotten. Every banned
 * pattern below is listed with the files that still contain it. Each tier's PR
 * deletes entries from `ALLOWED`, and the spec fails if a new file appears —
 * so migration progress is a number in a test file rather than a vibe, and
 * regressions are impossible to land quietly.
 *
 * ADDING A FILE TO `ALLOWED` IS NOT A FIX. It is a debt entry. The only correct
 * direction for these lists is shorter.
 */

const BANNED: Array<{ id: string; pattern: RegExp; why: string }> = [
  {
    id: 'gradient-fill',
    pattern: /\bbg-gradient-to-[trbl]{1,2}\b/,
    why: 'No gradient fills. A gradient implies a value ramp the data does not have.',
  },
  {
    id: 'backdrop-blur',
    pattern: /\bbackdrop-blur(-\w+)?\b/,
    why: 'No glassmorphism. The blur scale is zeroed in the config, so these are already inert — delete them.',
  },
  {
    id: 'named-gradient',
    pattern:
      /\b(bg-mesh-gradient|bg-gradient-primary|bg-gradient-dark|bg-gradient-glass|bg-gradient-radial)\b/,
    why: 'Deleted from the theme; these emit nothing.',
  },
  {
    id: 'glow-shadow',
    pattern: /\bshadow-(glow|glow-lg|glass|glass-sm|card|card-hover|inner-glow)\b/,
    why: 'Deleted from the theme; elevation is popover/modal or a 1px rule.',
  },
  {
    id: 'glass-class',
    pattern: /\bclass="[^"]*\b(glass|glass-card|card-glass)\b/,
    why: 'Neutralized to a flat surface — use `bg-surface border border-line` directly.',
  },
  {
    id: 'text-gradient',
    pattern: /\btext-gradient\b/,
    why: 'Clip-text gradients are gone.',
  },
  {
    id: 'hover-lift',
    pattern: /hover:-translate-y-|active:scale-\[/,
    why: 'Nothing lifts on hover and nothing shrinks on press in this system.',
  },
  {
    id: 'raw-teal',
    // The old brand hue, in every form it appeared in. These live in scoped
    // `<style>` blocks and JS colour arrays, which no config change can reach —
    // they survived the global palette swap and had to be hunted individually.
    pattern: /#14b8a6|#0d9488|#2dd4bf|#5eead4|rgba?\(\s*20\s*,\s*184\s*,\s*166/i,
    why: 'The accent is ultramarine. Use a token or the chartTheme series palette.',
  },
  {
    id: 'bare-dark-color',
    // `dark` is a COLOR NAME in this config, not the dark-mode variant. A bare
    // `bg-dark-800` (no `dark:` prefix) paints a near-black surface in LIGHT
    // mode. This is the single most likely way to introduce a theme bug here.
    pattern: /(^|[\s"'`])(bg|text|border|ring|divide|placeholder|from|to|via)-dark-\d/,
    why: '`dark-*` is a static color, not a variant. Prefix with `dark:` or use a Family B token.',
  },
]

/**
 * Files still carrying a banned pattern.
 *
 * EMPTY, as of the end of the migration — every rule is now a plain ban rather
 * than a burndown. The mechanism stays because the ratchet is what stops the
 * old visual language from coming back: a new gradient or a new blur fails the
 * relevant test the moment it lands, and the only way to make it pass is to
 * write an explicit debt entry here, which is visible in review.
 */
const ALLOWED: Record<string, string[]> = {
  'gradient-fill': [],
  'backdrop-blur': [],
  'named-gradient': [],
  'glow-shadow': [],
  'glass-class': [],
  'text-gradient': [],
  'raw-teal': [],
  'hover-lift': [],
  'bare-dark-color': [],
}

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry === '__tests__' || entry === 'assets') continue
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) walk(full, out)
    else if (/\.(vue|ts)$/.test(entry) && !/\.spec\.ts$/.test(entry)) out.push(full)
  }
  return out
}

/**
 * Strip comments before matching, so a rule can be *described* in prose without
 * tripping its own check. Per-line heuristics are not enough: an HTML comment
 * body continues onto lines that begin with ordinary text.
 */
function stripComments(source: string): string {
  return source
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/.*$/gm, '$1')
}

/** Matching lines, comments removed. */
function lineMatches(source: string, pattern: RegExp): string[] {
  return stripComments(source)
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => pattern.test(line))
}

const files = walk(SRC).map((f) => ({
  path: relative(SRC, f).replace(/\\/g, '/'),
  source: readFileSync(f, 'utf8'),
}))

describe('design system: legacy visual language burndown', () => {
  it('scans a plausible number of files', () => {
    // Guards against the walker silently matching nothing, which would make
    // every assertion below vacuously pass.
    expect(files.length).toBeGreaterThan(400)
  })

  for (const { id, pattern, why } of BANNED) {
    it(`has no unlisted use of ${id}`, () => {
      const allowed = new Set(ALLOWED[id] ?? [])
      const offenders = files
        .filter(({ path, source }) => !allowed.has(path) && lineMatches(source, pattern).length > 0)
        .map(({ path, source }) => `${path} → ${lineMatches(source, pattern)[0]}`)
        .sort()

      expect(
        offenders,
        `${why}\nIf this is intentional debt, add the file to ALLOWED['${id}'].`
      ).toEqual([])
    })

    it(`keeps ALLOWED['${id}'] honest`, () => {
      // A stale allowlist is worse than none: it hides the fact that the debt
      // is already paid, and it lets the pattern come back unnoticed.
      const allowed = ALLOWED[id] ?? []
      const known = new Map(files.map(({ path, source }) => [path, source]))
      const stale = allowed
        .filter((path) => {
          const source = known.get(path)
          return source === undefined || lineMatches(source, pattern).length === 0
        })
        .sort()

      expect(stale, `no longer needed — remove from ALLOWED['${id}']`).toEqual([])
    })
  }
})
