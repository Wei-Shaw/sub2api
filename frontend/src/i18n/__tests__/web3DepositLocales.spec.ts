import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

function messageAt(messages: Record<string, unknown>, path: string): unknown {
  return path.split('.').reduce<unknown>((value, key) => {
    if (!value || typeof value !== 'object') return undefined
    return (value as Record<string, unknown>)[key]
  }, messages)
}

describe('Web3 deposit unavailable messages', () => {
  const keys = [
    'web3Deposit.unavailable.title',
    'web3Deposit.unavailable.featureDisabled',
    'web3Deposit.unavailable.userEntryDisabled',
    'web3Deposit.unavailable.runtimeUnhealthy',
    'web3Deposit.unavailable.default',
  ]

  it.each(keys)('provides English and Chinese messages for %s', (key) => {
    expect(messageAt(en, key)).toEqual(expect.any(String))
    expect(messageAt(zh, key)).toEqual(expect.any(String))
  })
})
