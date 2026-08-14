import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

/**
 * Every `t('literal')` in the source must resolve in *both* locales.
 *
 * `localeParity` guards the *difference* between en and zh; it is blind by
 * construction to a key that is missing from both. That is not hypothetical:
 * a call site can be written against a key that was never added, and the two
 * locales stay in perfect parity while the UI renders the dotted key path
 * where a sentence belongs. This spec closes that hole from the other side —
 * it starts from the call sites rather than from the message files.
 *
 * ## Known limitation: dynamic keys are invisible here
 *
 * Only string *literals* are resolvable statically. A composed key such as
 *
 *     // views/admin/ProxiesView.vue:266
 *     t('admin.accounts.status.' + value)
 *
 * depends on runtime data (there, every member of `Proxy['status']`), so this
 * scan skips it entirely — the prefix is not a key and the suffix is unknown.
 * That whole class of call site still needs review by eye: when you add or
 * widen a union that feeds a key suffix, enumerate the members and check each
 * one by hand. `admin.accounts.status.expired` was missing for exactly this
 * reason and this spec would not have caught it.
 *
 * Template literals are skipped for the same reason when they interpolate;
 * a plain backtick string with no `${}` is treated as a literal.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const SRC_DIR = resolve(HERE, '../..')
const LOCALES_DIR = resolve(HERE, '../locales')

type Json = Record<string, unknown>

/** Flatten to dotted leaf paths. Arrays are leaves — order/length is content. */
function flatten(value: unknown, prefix = '', out: Set<string> = new Set()): Set<string> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    out.add(prefix)
    return out
  }
  for (const [key, child] of Object.entries(value as Json)) {
    flatten(child, prefix ? `${prefix}.${key}` : key, out)
  }
  return out
}

/*
 * The message modules are enumerated from disk rather than listed here.
 * `localesNoKeyCollision` learned this the hard way: its hand-written module
 * list silently fell four modules behind, and a namespace missing from the
 * list is invisible to the check that is supposed to police it. A spec whose
 * inputs are typed by hand degrades quietly; one that reads the directory
 * cannot.
 */
function moduleNamesOnDisk(relativeDir: string): string[] {
  return readdirSync(resolve(LOCALES_DIR, relativeDir), { withFileTypes: true })
    .filter((e) => e.isFile() && e.name.endsWith('.ts') && e.name !== 'index.ts')
    .map((e) => e.name.replace(/\.ts$/, ''))
    .sort()
}

/** Rebuild a locale from the files on disk, the way `index.ts` assembles it. */
async function keysFromDisk(locale: string): Promise<Set<string>> {
  const out = new Set<string>()
  for (const name of moduleNamesOnDisk(locale)) {
    const mod = (await import(`../locales/${locale}/${name}.ts`)) as { default: Json }
    flatten(mod.default, '', out)
  }
  for (const name of moduleNamesOnDisk(`${locale}/admin`)) {
    const mod = (await import(`../locales/${locale}/admin/${name}.ts`)) as { default: Json }
    flatten(mod.default, 'admin', out)
  }
  return out
}

const enKeys = flatten(en)
const zhKeys = flatten(zh)

/*
 * `t(...)` / `$t(...)` with a single-quoted, double-quoted, or uninterpolated
 * backtick first argument. The lookbehind rejects identifiers that merely end
 * in `t` (`it(`, `expect(`, `import(`, `format(`) while still allowing a
 * receiver (`i18n.global.t(`). The key shape — leading letter, then word
 * characters and dots — keeps ordinary strings that happen to be passed to
 * some other `t` out of the result.
 */
const T_CALL = /(?<![\w$])\$?t\(\s*(['"`])([A-Za-z][\w-]*(?:\.[\w-]+)+)\1/g

/** Recursively collect scannable source files, skipping tests and messages. */
function sourceFiles(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === '__tests__' || entry.name === 'node_modules') continue
      if (full === LOCALES_DIR) continue
      sourceFiles(full, out)
      continue
    }
    if (!/\.(vue|ts)$/.test(entry.name)) continue
    if (/\.(spec|test)\.ts$/.test(entry.name)) continue
    out.push(full)
  }
  return out
}

interface Call {
  key: string
  where: string
}

function collectCalls(): Call[] {
  const calls: Call[] = []
  for (const file of sourceFiles(SRC_DIR)) {
    const text = readFileSync(file, 'utf8')
    const rel = relative(SRC_DIR, file).split(sep).join('/')
    for (const match of text.matchAll(T_CALL)) {
      const line = text.slice(0, match.index).split('\n').length
      calls.push({ key: match[2], where: `${rel}:${line}` })
    }
  }
  return calls
}

const calls = collectCalls()

/**
 * Call sites whose key resolves in neither locale and that are not fixed here.
 * This list may only shrink — see "keeps GRANDFATHERED honest" below. Prefer
 * adding the message over adding a line here.
 */
const GRANDFATHERED: string[] = []

const missing = [
  ...new Set(
    calls.filter((c) => !enKeys.has(c.key) || !zhKeys.has(c.key)).map((c) => `${c.key} (${c.where})`)
  )
].sort()

describe('i18n: every literal t() key exists', () => {
  it('scans a meaningful number of call sites', () => {
    expect(calls.length).toBeGreaterThan(1000)
    expect(enKeys.size).toBeGreaterThan(1000)
  })

  it('has no literal key missing from en or zh', () => {
    const allowed = new Set(GRANDFATHERED)
    expect(
      missing.filter((m) => !allowed.has(m)),
      'add the message to both locales — an unresolved key renders as its own dotted path'
    ).toEqual([])
  })

  it('keeps GRANDFATHERED honest', () => {
    const fixed = GRANDFATHERED.filter((m) => !missing.includes(m)).sort()
    expect(fixed, 'these resolve now — remove them from GRANDFATHERED').toEqual([])
  })

  it.each(['en', 'zh'])('%s bundle contains exactly the modules on disk', async (locale) => {
    const onDisk = await keysFromDisk(locale)
    const bundled = locale === 'en' ? enKeys : zhKeys
    expect([...onDisk].filter((k) => !bundled.has(k)).sort(), 'module on disk not spread into index.ts').toEqual([])
    expect([...bundled].filter((k) => !onDisk.has(k)).sort(), 'bundled key has no module on disk').toEqual([])
  })
})
