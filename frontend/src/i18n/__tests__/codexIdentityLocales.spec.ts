import { baseCompile } from '@intlify/message-compiler'
import { describe, expect, it } from 'vitest'
import en from '../locales/en/admin/accounts'
import zh from '../locales/zh/admin/accounts'
import { codexIdentityValidationMessageKey } from '@/utils/codexIdentityValidation'

const validationCodes = [
  'PROXY_INVALID',
  'PROXY_NOT_FOUND',
  'PROXY_MODE_INVALID',
  'PROXY_REQUIRED',
  'PROXY_NOT_ALLOWED',
  'SURFACE_NOT_ALLOWED',
  'ARCHITECTURE_NOT_ALLOWED',
  'SLOT_COUNT_OUT_OF_RANGE',
  'SLOT_OVERRIDE_OUT_OF_RANGE',
  'DUPLICATE_SLOT_OVERRIDE',
  'BINDING_SCOPE_INVALID',
  'UNSUPPORTED_POLICY_INVALID',
  'OFF_MODE_HAS_PROFILES',
  'PROFILE_REQUIRED',
  'DUPLICATE_PROFILE',
  'AFFINITY_TTL_OUT_OF_RANGE',
  'SESSION_SLOT_COUNT_OUT_OF_RANGE',
  'SESSION_SLOT_COUNT_NOT_APPLICABLE',
  'DEVICE_SHARED_RESTRICTIONS_INVALID',
] as const

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

describe('Codex identity locales', () => {
  const enMessages = flatten(en.accounts.codexIdentity)
  const zhMessages = flatten(zh.accounts.codexIdentity)

  it('keeps English and Chinese keys identical and non-empty', () => {
    expect(Object.keys(zhMessages).sort()).toEqual(Object.keys(enMessages).sort())
    expect(Object.values(enMessages).every((message) => message.trim().length > 0)).toBe(true)
    expect(Object.values(zhMessages).every((message) => message.trim().length > 0)).toBe(true)
  })

  it('covers every validation code with localized text', () => {
    for (const code of validationCodes) {
      const fullKey = codexIdentityValidationMessageKey(code)
      expect(fullKey).not.toBeNull()
      const relativeKey = fullKey?.replace('admin.accounts.codexIdentity.', '') ?? ''
      expect(enMessages[relativeKey], code).toBeTruthy()
      expect(zhMessages[relativeKey], code).toBeTruthy()
    }
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
