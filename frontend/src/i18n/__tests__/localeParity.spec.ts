import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

/**
 * en/zh key parity.
 *
 * The repo already guards two i18n properties — top-level collisions
 * (`localesNoKeyCollision`) and message compilability (`localesMessageCompile`)
 * — but nothing checked that the two locales describe the same key set. A key
 * present in `en` and absent in `zh` does not throw: vue-i18n renders the key
 * path itself, so a Chinese user sees `admin.accounts.form.priorityHint` where
 * a sentence should be.
 *
 * This matters specifically because of the design-system migration. Every
 * rewritten view is an opportunity to add copy, and "zh to follow" is how a
 * translation backlog of several thousand keys gets created one PR at a time.
 * With this spec in place, adding an `en` key without its `zh` counterpart
 * fails immediately, in the PR that did it.
 *
 * Any existing asymmetry is grandfathered rather than fixed here: it predates
 * this work, some of it may be intentional (locale-specific legal copy), and
 * bundling a translation pass into a styling change would make both harder to
 * review. What the allowlist guarantees is that the number only goes down.
 */

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

const enKeys = flatten(en)
const zhKeys = flatten(zh)

const onlyEn = [...enKeys].filter((k) => !zhKeys.has(k)).sort()
const onlyZh = [...zhKeys].filter((k) => !enKeys.has(k)).sort()

/**
 * Pre-existing asymmetry, measured at the end of Tier 0. Shrink this; never
 * grow it. Each entry is a key that exists in exactly one locale.
 */
const GRANDFATHERED_ONLY_EN: string[] = []
const GRANDFATHERED_ONLY_ZH: string[] = []

describe('i18n: en/zh parity', () => {
  it('loads both locales', () => {
    expect(enKeys.size).toBeGreaterThan(1000)
    expect(zhKeys.size).toBeGreaterThan(1000)
  })

  it('has no en key missing from zh', () => {
    const allowed = new Set(GRANDFATHERED_ONLY_EN)
    expect(
      onlyEn.filter((k) => !allowed.has(k)),
      'add the zh translation in the same commit — a missing key renders as its own path'
    ).toEqual([])
  })

  it('has no zh key missing from en', () => {
    const allowed = new Set(GRANDFATHERED_ONLY_ZH)
    expect(onlyZh.filter((k) => !allowed.has(k)), 'add the en translation').toEqual([])
  })

  it('keeps the grandfathered lists honest', () => {
    const enFixed = GRANDFATHERED_ONLY_EN.filter((k) => zhKeys.has(k) || !enKeys.has(k)).sort()
    const zhFixed = GRANDFATHERED_ONLY_ZH.filter((k) => enKeys.has(k) || !zhKeys.has(k)).sort()
    expect(enFixed, 'no longer asymmetric — remove from GRANDFATHERED_ONLY_EN').toEqual([])
    expect(zhFixed, 'no longer asymmetric — remove from GRANDFATHERED_ONLY_ZH').toEqual([])
  })
})
