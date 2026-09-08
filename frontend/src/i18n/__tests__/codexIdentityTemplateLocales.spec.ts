import { baseCompile } from '@intlify/message-compiler'
import { describe, expect, it } from 'vitest'
import en from '../locales/en/admin/settings'
import zh from '../locales/zh/admin/settings'

const flatten = (node: unknown, path = ''): Record<string, string> => {
  if (typeof node === 'string') return { [path]: node }
  if (!node || typeof node !== 'object') return {}
  return Object.entries(node as Record<string, unknown>).reduce<Record<string, string>>(
    (result, [key, value]) => ({
      ...result,
      ...flatten(value, path ? `${path}.${key}` : key),
    }),
    {},
  )
}

describe('Codex identity template locales', () => {
  const enMessages = flatten(en.settings.codexProfiles)
  const zhMessages = flatten(zh.settings.codexProfiles)

  it('keeps English and Chinese keys identical and non-empty', () => {
    expect(Object.keys(zhMessages).sort()).toEqual(Object.keys(enMessages).sort())
    expect(Object.values(enMessages).every((message) => message.trim().length > 0)).toBe(true)
    expect(Object.values(zhMessages).every((message) => message.trim().length > 0)).toBe(true)
  })

  it.each([
    ['en', enMessages],
    ['zh', zhMessages],
  ] as const)('%s messages compile', (_locale, messages) => {
    const errors: string[] = []
    for (const [path, message] of Object.entries(messages)) {
      baseCompile(message, { onError: (error) => errors.push(`${path}: ${error.message}`) })
    }
    expect(errors).toEqual([])
  })
})
