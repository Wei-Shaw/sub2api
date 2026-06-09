import fs from 'node:fs'
import path from 'node:path'

import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import ko from '../locales/ko'
import vi from '../locales/vi'
import zh from '../locales/zh'

type LocaleObject = Record<string, unknown>

const userSourceDirs = [
  'src/views/user',
  'src/components/user'
]

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
    } else {
      out.add(pathKey)
    }
  }
  return out
}

function collectSourceFiles(dir: string): string[] {
  if (!fs.existsSync(dir)) {
    return []
  }

  const files: string[] = []
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      files.push(...collectSourceFiles(fullPath))
      continue
    }
    if (/\.(vue|ts)$/.test(entry.name)) {
      files.push(fullPath)
    }
  }
  return files
}

function isUserLocaleKey(key: string): boolean {
  return key.startsWith('usage.') || key.startsWith('dashboard.') || key.startsWith('profile.')
}

function isStaticLiteralKey(key: string): boolean {
  return key !== '' && !key.endsWith('.') && !key.includes('${') && !key.includes('`')
}

function collectUserLocaleReferences(): string[] {
  const keys = new Set<string>()
  for (const sourceDir of userSourceDirs) {
    for (const file of collectSourceFiles(sourceDir)) {
      const source = fs.readFileSync(file, 'utf8')
      for (const pattern of i18nReferencePatterns) {
        pattern.lastIndex = 0
        let match: RegExpExecArray | null
        while ((match = pattern.exec(source)) !== null) {
          const key = match[1]
          if (isUserLocaleKey(key) && isStaticLiteralKey(key)) {
            keys.add(key)
          }
        }
      }
    }
  }
  return [...keys].sort()
}

describe('user locale coverage', () => {
  it('defines every static user-facing i18n key used by user source in all supported locales', () => {
    const referencedKeys = collectUserLocaleReferences()
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
