import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, put }
}))

import {
  getStepPlanUsageState,
  setStepPlanUsageAutoRefresh
} from '@/api/admin/cnProviders'

const state = {
  account_id: 7,
  eligible: true,
  auto_refresh_enabled: true,
  snapshot: {
    status: 'ok',
    credential_valid: true,
    plan_level: 'Plus',
    tiers: [{ window: 'monthly', used_percent: 30 }],
    last_attempt_at: '2026-10-21T03:00:00Z'
  }
}

describe('admin Step Plan usage API', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
  })

  it('reads persisted Step Plan usage state', async () => {
    get.mockResolvedValueOnce({ data: state })

    await expect(getStepPlanUsageState(7)).resolves.toEqual(state)
    expect(get).toHaveBeenCalledWith('/admin/cn-providers/accounts/7/quota/state')
  })

  it('persists the account-level auto-refresh toggle', async () => {
    put.mockResolvedValueOnce({ data: { ...state, auto_refresh_enabled: false } })

    await expect(setStepPlanUsageAutoRefresh(7, false)).resolves.toMatchObject({
      auto_refresh_enabled: false
    })
    expect(put).toHaveBeenCalledWith(
      '/admin/cn-providers/accounts/7/quota/auto-refresh',
      { enabled: false }
    )
  })
})
