import fs from 'node:fs'
import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { PLATFORM_QUOTA_PLATFORMS } from '@/api/admin/users'
import { ACCOUNT_API_KEY_HINT_PLATFORMS, getAccountApiKeyHintKey } from '@/utils/accountPlatformHints'
import { ADMIN_PLATFORM_VALUES } from '@/utils/platformOptions'
import en from '../locales/en'
import ko from '../locales/ko'
import vi from '../locales/vi'
import zh from '../locales/zh'

type LocaleObject = Record<string, unknown>

const frontendRoot = process.cwd()
const repoRoot = path.resolve(frontendRoot, '..')
const backendRoot = path.join(repoRoot, 'backend')

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

function readBackend(relativePath: string): string {
  return fs.readFileSync(path.join(backendRoot, relativePath), 'utf8')
}

function parseDomainPlatformConstMap(): Record<string, string> {
  const source = readBackend('internal/domain/constants.go')
  const block = source.match(/\/\/ Platform constants[\s\S]*?const\s*\(([\s\S]*?)\)/)?.[1]
  if (!block) throw new Error('Could not find backend platform constants block')

  const values: Record<string, string> = {}
  const constPattern = /\b(Platform\w+)\s*=\s*"([^"]+)"/g
  let match: RegExpExecArray | null
  while ((match = constPattern.exec(block)) !== null) {
    values[match[1]] = match[2]
  }
  return values
}

function parseBackendAdminGroupPlatforms(): string[] {
  const source = readBackend('internal/handler/admin/group_handler.go')
  const matches = source
    .split('\n')
    .map((line) => line.match(/\bPlatform\s+[^`]+`[^`]*binding:"[^"]*oneof=([^"]+)"/))
    .filter((match): match is RegExpMatchArray => Boolean(match))
  if (matches.length === 0) throw new Error('Could not find admin group platform oneof bindings')

  const seen = new Set<string>()
  for (const match of matches) {
    for (const platform of match[1].trim().split(/\s+/)) {
      seen.add(platform)
    }
  }
  const result: string[] = []
  seen.forEach((platform) => result.push(platform))
  return result
}

function parseBackendAllowedQuotaPlatforms(): string[] {
  const constMap = parseDomainPlatformConstMap()
  const source = readBackend('internal/service/domain_constants.go')
  const block = source.match(/AllowedQuotaPlatforms\s*=\s*\[\]string\s*\{([\s\S]*?)\n\}/)?.[1]
  if (!block) throw new Error('Could not find backend AllowedQuotaPlatforms block')

  const result: string[] = []
  const platformPattern = /\b(Platform\w+)\b/g
  let match: RegExpExecArray | null
  while ((match = platformPattern.exec(block)) !== null) {
    const value = constMap[match[1]]
    if (!value) throw new Error(`Unknown backend platform constant ${match[1]}`)
    result.push(value)
  }
  return result
}

describe('admin platform i18n coverage', () => {
  it('keeps frontend admin platform option values aligned with backend admin group platforms', () => {
    expect(Array.from(ADMIN_PLATFORM_VALUES)).toEqual(parseBackendAdminGroupPlatforms())
  })

  it('keeps frontend platform quota values aligned with backend allowed quota platforms', () => {
    expect(Array.from(PLATFORM_QUOTA_PLATFORMS)).toEqual(parseBackendAllowedQuotaPlatforms())
  })

  it('defines admin account/group labels for every backend admin platform in every locale', () => {
    const platforms = parseBackendAdminGroupPlatforms()
    const locales = {
      en: flattenLocale(en),
      zh: flattenLocale(zh),
      vi: flattenLocale(vi),
      ko: flattenLocale(ko)
    }

    const namespaces = ['admin.groups.platforms', 'admin.accounts.platforms'] as const
    const missing = Object.entries(locales).flatMap(([locale, keys]) =>
      namespaces.flatMap((namespace) =>
        platforms
          .filter((platform) => !keys.has(`${namespace}.${platform}`))
          .map((platform) => `${locale}:${namespace}.${platform}`)
      )
    )

    expect(missing).toEqual([])
  })

  it('defines API-key helper copy for every API-key compatible account platform in every locale', () => {
    const locales = {
      en: flattenLocale(en),
      zh: flattenLocale(zh),
      vi: flattenLocale(vi),
      ko: flattenLocale(ko)
    }

    const missing = Object.entries(locales).flatMap(([locale, keys]) =>
      ACCOUNT_API_KEY_HINT_PLATFORMS.flatMap((platform) =>
        (['baseUrl', 'apiKey'] as const)
          .map((kind) => getAccountApiKeyHintKey(platform, kind))
          .filter((key) => !keys.has(key))
          .map((key) => `${locale}:${key}`)
      )
    )

    expect(missing).toEqual([])
  })
})
