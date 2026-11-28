import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { post },
}))

import { authorizePassword, createFromSSO, getGrokSSOImportTimeout, queryUsageResetCards, redeemUsageResetCard } from '@/api/admin/grok'

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

  it('sends reset credentials only in POST bodies to the admin endpoints', async () => {
    post.mockResolvedValue({ data: { cards: [] } })
    await queryUsageResetCards(12, 'web-session-secret')
    expect(post).toHaveBeenLastCalledWith('/admin/grok/accounts/12/reset-cards/query', { sso_token: 'web-session-secret' })
    await redeemUsageResetCard(12, 'web-session-secret', 'card-one')
    expect(post).toHaveBeenLastCalledWith('/admin/grok/accounts/12/reset-cards/redeem', {
      sso_token: 'web-session-secret', token_id: 'card-one'
    })
  })
})
