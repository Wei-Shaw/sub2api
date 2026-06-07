import { describe, expect, it } from 'vitest'
import type { AdminDataImportRequest, AdminDataImportResult, AdminDataPayload } from '@/types'

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
    const request: AdminDataImportRequest = {
      data: payload,
      group_id: 7,
      skip_default_group_bind: true,
      proxy_assignment: {
        mode: 'default_live',
        default_proxy_id: 9,
      },
    }

    expect(payload.proxies).toEqual([])
    expect(result.account_created).toBe(0)
    expect(request.proxy_assignment?.mode).toBe('default_live')
  })
})
