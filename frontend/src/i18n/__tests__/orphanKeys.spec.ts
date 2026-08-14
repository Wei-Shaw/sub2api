import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const SRC = resolve(dirname(fileURLToPath(import.meta.url)), '../..')

/**
 * The orphan-key ratchet.
 *
 * A message tree only ever grows on its own. A key whose call site was deleted
 * leaves no trace: nothing fails, nothing warns, the string just sits there and
 * gets translated forever. This spec makes that debt a number.
 *
 * How a key counts as REACHED:
 *   - its full dotted path appears as a string literal in any non-spec source
 *     file (broader than modelling `t(...)` call shapes, so it cannot miss a
 *     call site), or
 *   - it sits under a prefix that some file builds a key from dynamically —
 *     `'admin.groups.platforms.' + platform`, `` `payment.status.${s}` `` and
 *     friends. Those keys are unresolvable statically, so the whole subtree is
 *     treated as reached. Deleting one of them would render the key path into
 *     the UI, silently.
 *
 * The one shape a call-site scan genuinely cannot see is a key assembled inside
 * a helper, where neither half is written next to the other — the namespace
 * arrives as an argument and the code as data. The interior-node rule below
 * covers the case that occurs in this tree. A key whose namespace is ALSO
 * computed would still slip through, and nothing here would notice.
 *
 * ADDING A NUMBER HERE IS NOT A FIX. The only correct direction is down, and
 * the counts are exact on purpose: paying debt off means editing this file,
 * which puts it in the diff where a reviewer can see it.
 */
const ALLOWED_ORPHANS: Record<string, number> = {
  // All `wechatConnect`. NOT dead copy — see the note above the rule:
  // SecurityRegistrationSection hardcodes these strings through
  // `localText(zh, en)` instead of reading them, so the fix is to wire the
  // UI back to the keys, not to delete the translations.
  'admin.settings': 13,
}

function flatten(node: unknown, prefix: string, out: Set<string>, interior: Set<string>): void {
  if (node === null || typeof node !== 'object') return
  for (const [k, v] of Object.entries(node as Record<string, unknown>)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out.add(path)
    else if (typeof v === 'object' && v !== null) {
      interior.add(path)
      flatten(v, path, out, interior)
    }
  }
}

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry === 'assets' || entry === '__tests__') continue
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      // The message trees themselves are definitions, not call sites.
      if (relative(SRC, full).split(sep).join('/') === 'i18n/locales') continue
      walk(full, out)
    } else if (/\.(vue|ts)$/.test(entry) && !/\.spec\.ts$/.test(entry)) {
      out.push(full)
    }
  }
  return out
}

const keys = new Set<string>()
const interior = new Set<string>()
flatten(en, '', keys, interior)
flatten(zh, '', keys, interior)

const sources = walk(SRC).map((f) => readFileSync(f, 'utf8'))

const literals = new Set<string>()
const dynamicPrefixes = new Set<string>()
for (const source of sources) {
  for (const m of source.matchAll(/['"`]([A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)+)['"`]/g)) {
    literals.add(m[1])
  }
  // The static head can end mid-segment, not only at a dot: the payment dialog
  // writes `` t(`admin.settings.payment.field_${f.key}`) ``, and a prefix
  // pattern that insisted on a trailing dot reported all thirty of those keys
  // as dead. Capture whatever dotted head precedes the interpolation.
  for (const m of source.matchAll(/['"]((?:[A-Za-z0-9_]+\.)+[A-Za-z0-9_]*)['"]\s*\+/g)) {
    dynamicPrefixes.add(m[1])
  }
  for (const m of source.matchAll(/`((?:[A-Za-z0-9_]+\.)+[A-Za-z0-9_]*)\$\{/g)) {
    dynamicPrefixes.add(m[1])
  }
}

/**
 * A literal that names an INTERIOR node — a subtree, not a leaf — is a
 * namespace being handed to something that will append a code to it.
 * `extractI18nErrorMessage(err, t, 'payment.errors', fallback)` builds
 * `` `${namespace}.${code}` `` inside the helper, so the head never appears
 * next to the interpolation and no amount of pattern-matching at the call site
 * can see it. Thirty-nine backend error codes were reported dead this way.
 *
 * Writing a subtree's own path in code has no other purpose, so treating the
 * whole subtree as reached is both correct here and safe in general: it errs
 * toward keeping keys.
 */
for (const literal of literals) {
  if (interior.has(literal)) dynamicPrefixes.add(`${literal}.`)
}

const prefixes = [...dynamicPrefixes]
const orphans = [...keys]
  .filter((k) => !literals.has(k) && !prefixes.some((p) => k.startsWith(p)))
  .sort()

/** Two segments: fine enough to point at a feature, coarse enough to review. */
const namespaceOf = (key: string) => key.split('.').slice(0, 2).join('.')

describe('i18n orphan keys', () => {
  it('scans a plausible number of files and keys', () => {
    // Guards against the walker matching nothing, which would make every
    // assertion below vacuously pass.
    expect(sources.length).toBeGreaterThan(400)
    expect(keys.size).toBeGreaterThan(5000)
    expect(prefixes.length).toBeGreaterThan(30)
  })

  it('has no orphan outside a declared namespace', () => {
    const undeclared = [...new Set(orphans.map(namespaceOf))]
      .filter((ns) => !(ns in ALLOWED_ORPHANS))
      .sort()

    expect(
      undeclared,
      'A message tree grew a key nothing can reach. Delete the key, or — if a ' +
        'call site is coming — add the namespace to ALLOWED_ORPHANS as debt.'
    ).toEqual([])
  })

  it('keeps every declared count exact', () => {
    const actual = new Map<string, number>()
    for (const key of orphans) {
      const ns = namespaceOf(key)
      actual.set(ns, (actual.get(ns) ?? 0) + 1)
    }

    const drift = Object.entries(ALLOWED_ORPHANS)
      .map(([ns, allowed]) => ({ ns, allowed, found: actual.get(ns) ?? 0 }))
      .filter((row) => row.found !== row.allowed)
      .map((row) =>
        row.found > row.allowed
          ? `${row.ns}: ${row.found} orphans, ${row.allowed} declared — new dead keys`
          : `${row.ns}: ${row.found} orphans, ${row.allowed} declared — lower it to ${row.found}` +
            (row.found === 0 ? ' (or delete the entry)' : '')
      )
      .sort()

    expect(drift, 'ALLOWED_ORPHANS is out of date.').toEqual([])
  })
})
