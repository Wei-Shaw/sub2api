import fs from 'node:fs'
import path from 'node:path'

import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import ko from '../locales/ko'
import vi from '../locales/vi'
import zh from '../locales/zh'

type LocaleObject = Record<string, unknown>

const root = process.cwd()
const srcRoot = path.join(root, 'src')

const i18nReferencePatterns = [
  /\bt\(\s*['"]([^'"]+)['"]/g,
  /\$t\(\s*['"]([^'"]+)['"]/g,
  /i18n\.global\.t\(\s*['"]([^'"]+)['"]/g
]

function flattenLocale(obj: LocaleObject, prefix = '', out = new Set<string>()): Set<string> {
  for (const [key, value] of Object.entries(obj)) {
    const pathKey = prefix ? `${prefix}.${key}` : key
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      flattenLocale(value as LocaleObject, pathKey, out)
      continue
    }
    out.add(pathKey)
  }
  return out
}

function collectSourceFiles(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      if (['node_modules', 'dist', 'coverage'].includes(entry.name)) continue
      collectSourceFiles(fullPath, out)
      continue
    }
    if (!/\.(vue|ts)$/.test(entry.name)) continue
    if (fullPath.includes('/i18n/locales/')) continue
    if (fullPath.includes('/__tests__/') || /\.(spec|test)\.ts$/.test(fullPath)) continue
    out.push(fullPath)
  }
  return out
}

function isStaticLiteralKey(key: string): boolean {
  return key !== '' && !key.endsWith('.') && !key.includes('${') && !key.includes('`')
}

function collectStaticI18nReferences(): string[] {
  const keys = new Set<string>()
  for (const file of collectSourceFiles(srcRoot)) {
    const source = fs.readFileSync(file, 'utf8')
    for (const pattern of i18nReferencePatterns) {
      pattern.lastIndex = 0
      let match: RegExpExecArray | null
      while ((match = pattern.exec(source)) !== null) {
        const key = match[1]
        if (isStaticLiteralKey(key)) {
          keys.add(key)
        }
      }
    }
  }
  return [...keys].sort()
}

describe('static locale key coverage', () => {
  it('defines every static i18n key used by source in all supported locales', () => {
    const referencedKeys = collectStaticI18nReferences()
    const locales = {
      en: flattenLocale(en),
      zh: flattenLocale(zh),
      vi: flattenLocale(vi),
      ko: flattenLocale(ko)
    }

    const missing = Object.entries(locales).flatMap(([locale, keys]) =>
      referencedKeys
        .filter(key => !keys.has(key))
        .map(key => `${locale}:${key}`)
    )

    expect(missing).toEqual([])
  })
})
