import { describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    antigravity: {
      generateAuthUrl: vi.fn(),
      exchangeCode: vi.fn(),
      refreshAntigravityToken: vi.fn()
    }
  }
}))

import { useAntigravityOAuth } from '@/composables/useAntigravityOAuth'

describe('useAntigravityOAuth.buildCredentials', () => {
  it('falls back to the submitted refresh token when the response omits it', () => {
    const oauth = useAntigravityOAuth()

    const credentials = oauth.buildCredentials(
      {
        access_token: 'access-token',
        expires_at: 1_900_000_000
      },
      'submitted-refresh-token'
    )

    expect(credentials.refresh_token).toBe('submitted-refresh-token')
  })

  it('prefers a new refresh token returned by the response', () => {
    const oauth = useAntigravityOAuth()

    const credentials = oauth.buildCredentials(
      {
        access_token: 'access-token',
        refresh_token: 'rotated-refresh-token',
        expires_at: 1_900_000_000
      },
      'submitted-refresh-token'
    )

    expect(credentials.refresh_token).toBe('rotated-refresh-token')
  })
})
