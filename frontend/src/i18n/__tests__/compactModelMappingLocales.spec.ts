import { describe, expect, it } from 'vitest'

import en from '../locales/en/admin/accounts'
import zh from '../locales/zh/admin/accounts'

describe('Compact model mapping locales', () => {
  it('provides translated placeholders for both mapping fields', () => {
    expect(zh.accounts.fromModel).toBe('源模型')
    expect(zh.accounts.toModel).toBe('目标模型')
    expect(en.accounts.fromModel).toBe('Source model')
    expect(en.accounts.toModel).toBe('Target model')
  })
})
