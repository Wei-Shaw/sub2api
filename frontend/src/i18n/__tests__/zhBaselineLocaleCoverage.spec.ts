import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import ko from '../locales/ko'
import vi from '../locales/vi'
import zh from '../locales/zh'

type LocaleObject = Record<string, unknown>

function flattenLocale(obj: LocaleObject, prefix = '', out = new Set<string>()): Set<string> {
  for (const [key, value] of Object.entries(obj)) {
    const pathKey = prefix ? `${prefix}.${key}` : key
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      flattenLocale(value as LocaleObject, pathKey, out)
    } else {
      out.add(pathKey)
    }
  }
  return out
}

describe('locale coverage against zh baseline', () => {
  it('defines every zh dotted key in every supported non-zh locale', () => {
    const baseline = flattenLocale(zh)
    const locales = {
      en: flattenLocale(en),
      vi: flattenLocale(vi),
      ko: flattenLocale(ko)
    }

    const missing = Object.entries(locales).flatMap(([locale, keys]) =>
      [...baseline]
        .filter(key => !keys.has(key))
        .map(key => `${locale}:${key}`)
    )

    expect(missing).toEqual([])
  })
})
