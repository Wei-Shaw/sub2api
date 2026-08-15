import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { post },
}))

import {
  authorizePassword,
  checkAccountsRisk,
  checkSSOState,
  createFromSSO,
  getGrokRiskCheckTimeout,
  getGrokSSOImportTimeout,
  GROK_RISK_MAX_BATCH_IDS,
} from '@/api/admin/grok'

describe('admin Grok SSO import API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: { created: [], failed: [] } })
  })

  it.each([
    [1, 180_000],
    [3, 180_000],
    [4, 270_000],
    [7, 360_000],
  ])('uses a timeout sized for %i keys', async (keyCount, expectedTimeout) => {
    expect(getGrokSSOImportTimeout(keyCount)).toBe(expectedTimeout)

    await createFromSSO({
      sso_tokens: Array.from({ length: keyCount }, (_, index) => `sso-${index + 1}`),
    })

    expect(post).toHaveBeenCalledWith(
      '/admin/grok/sso-to-oauth',
      expect.objectContaining({ sso_tokens: expect.any(Array) }),
      { timeout: expectedTimeout },
    )
  })

  it('posts SSO risk checks without requiring import', async () => {
    post.mockResolvedValueOnce({
      data: { total: 1, flagged: 0, clean: 1, unknown: 0, error: 0, items: [] }
    })

    await checkSSOState({ sso_tokens: ['sso-1', 'sso-2'], proxy_id: 3 })

    expect(post).toHaveBeenCalledWith(
      '/admin/grok/sso-check-state',
      { sso_tokens: ['sso-1', 'sso-2'], proxy_id: 3 },
      { timeout: getGrokSSOImportTimeout(2) },
    )
  })

  it('chunks risk checks and sizes the timeout per batch', async () => {
    const firstChunk = Array.from({ length: GROK_RISK_MAX_BATCH_IDS }, (_, index) => index + 1)
    const ids = [...firstChunk, GROK_RISK_MAX_BATCH_IDS + 1]
    post
      .mockResolvedValueOnce({
        data: { total: GROK_RISK_MAX_BATCH_IDS, flagged: 1, clean: GROK_RISK_MAX_BATCH_IDS - 1, error: 0, skipped: 0, items: [] },
      })
      .mockResolvedValueOnce({
        data: { total: 1, flagged: 0, clean: 1, error: 0, skipped: 0, items: [] },
      })

    const result = await checkAccountsRisk(ids)

    expect(post).toHaveBeenNthCalledWith(
      1,
      '/admin/grok/accounts/check-risk',
      { account_ids: firstChunk },
      { timeout: getGrokRiskCheckTimeout(GROK_RISK_MAX_BATCH_IDS) },
    )
    expect(post).toHaveBeenNthCalledWith(
      2,
      '/admin/grok/accounts/check-risk',
      { account_ids: [GROK_RISK_MAX_BATCH_IDS + 1] },
      { timeout: getGrokRiskCheckTimeout(1) },
    )
    expect(result.flagged).toBe(1)
    expect(result.clean).toBe(GROK_RISK_MAX_BATCH_IDS)
  })

  it('preserves password whitespace and applies the authorization timeout', async () => {
    post.mockResolvedValueOnce({ data: { access_token: 'access-token' } })

    await authorizePassword(' user@example.com ----  password with spaces  ', 7)

    expect(post).toHaveBeenCalledWith(
      '/admin/grok/oauth/password',
      {
        email: 'user@example.com',
        password: '  password with spaces  ',
        proxy_id: 7,
      },
      { timeout: 120_000 },
    )
  })
})
