import { describe, it } from 'vitest'
import en from '../locales/en'
import vi from '../locales/vi'

function collectMessages(node: unknown, path = '', out: Record<string, string> = {}): Record<string, string> {
  if (typeof node === 'string') {
    out[path] = node
    return out
  }
  if (node && typeof node === 'object' && !Array.isArray(node)) {
    for (const [key, value] of Object.entries(node as Record<string, unknown>)) {
      collectMessages(value, path ? `${path}.${key}` : key, out)
    }
  }
  return out
}

describe('temporary missing i18n dump', () => {
  it('prints missing values', () => {
    const enMessages = collectMessages(en)
    const viMessages = collectMessages(vi)
    const missing = Object.keys(enMessages).filter((key) => !(key in viMessages))
    console.log(JSON.stringify({ count: missing.length, missing: missing.map((key) => ({ key, value: enMessages[key] })) }))
  })
})
