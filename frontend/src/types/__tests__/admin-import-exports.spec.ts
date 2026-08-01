import { describe, expect, it } from 'vitest'
import type { AdminDataImportResult, AdminDataPayload } from '@/types'

describe('types index admin import exports', () => {
  it('exposes AdminDataPayload and AdminDataImportResult type aliases', () => {
    const payload: AdminDataPayload = {
      exported_at: '2026-04-01T00:00:00Z',
      proxies: [],
      accounts: [],
    }

    const result: AdminDataImportResult = {
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 0,
      account_failed: 0,
      errors: [],
    }

    expect(payload.proxies).toEqual([])
    expect(result.account_created).toBe(0)
  })
})
