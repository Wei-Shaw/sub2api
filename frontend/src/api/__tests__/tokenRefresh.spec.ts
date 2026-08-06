import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import axios from 'axios'

vi.mock('axios', () => ({
  default: {
    post: vi.fn()
  }
}))

const mockedPost = vi.mocked(axios.post)

function seedSession(overrides: Partial<Record<string, string>> = {}): void {
  localStorage.setItem('auth_token', overrides.auth_token || 'old-access')
  localStorage.setItem('refresh_token', overrides.refresh_token || 'old-refresh')
  localStorage.setItem('token_expires_at', overrides.token_expires_at || String(Date.now() - 1))
  localStorage.setItem('auth_user', JSON.stringify({ id: 7, email: 'admin@example.com' }))
}

function refreshedResponse() {
  return {
    data: {
      code: 0,
      message: 'ok',
      data: {
        access_token: 'new-access',
        refresh_token: 'new-refresh',
        expires_in: 3600,
        token_type: 'Bearer'
      }
    }
  }
}

describe('refreshAuthTokens', () => {
  beforeEach(() => {
    localStorage.clear()
    mockedPost.mockReset()
    vi.resetModules()
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: undefined
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shares one refresh request between concurrent callers in the same document', async () => {
    seedSession()
    let resolveRequest!: (value: ReturnType<typeof refreshedResponse>) => void
    mockedPost.mockImplementationOnce(
      () => new Promise((resolve) => {
        resolveRequest = resolve
      })
    )
    const { refreshAuthTokens } = await import('@/api/tokenRefresh')

    const first = refreshAuthTokens({ failedAccessToken: 'old-access' })
    const second = refreshAuthTokens({ failedAccessToken: 'old-access' })

    expect(mockedPost).toHaveBeenCalledTimes(1)
    resolveRequest(refreshedResponse())

    await expect(first).resolves.toMatchObject({ access_token: 'new-access' })
    await expect(second).resolves.toMatchObject({ refresh_token: 'new-refresh' })
    expect(localStorage.getItem('refresh_token')).toBe('new-refresh')
  })

  it('adopts tokens refreshed by another tab after acquiring the Web Lock', async () => {
    seedSession()
    const request = vi.fn(async (_name: string, callback: () => Promise<unknown>) => {
      localStorage.setItem('auth_token', 'peer-access')
      localStorage.setItem('token_expires_at', String(Date.now() + 3600_000))
      localStorage.setItem('refresh_token', 'peer-refresh')
      return callback()
    })
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: { request }
    })
    const { refreshAuthTokens } = await import('@/api/tokenRefresh')

    const result = await refreshAuthTokens({ failedAccessToken: 'old-access' })

    expect(request).toHaveBeenCalledTimes(1)
    expect(mockedPost).not.toHaveBeenCalled()
    expect(result).toMatchObject({
      access_token: 'peer-access',
      refresh_token: 'peer-refresh'
    })
  })

  it('recovers when a peer publishes the rotated token just after this request fails', async () => {
    seedSession()
    mockedPost.mockRejectedValueOnce(new Error('refresh token already used'))
    const { refreshAuthTokens } = await import('@/api/tokenRefresh')

    window.setTimeout(() => {
      localStorage.setItem('auth_token', 'peer-access')
      localStorage.setItem('token_expires_at', String(Date.now() + 3600_000))
      localStorage.setItem('refresh_token', 'peer-refresh')
    }, 10)

    await expect(
      refreshAuthTokens({ failedAccessToken: 'old-access' })
    ).resolves.toMatchObject({
      access_token: 'peer-access',
      refresh_token: 'peer-refresh'
    })
  })

  it('waits for a slow peer after losing a refresh-token race without Web Locks', async () => {
    vi.useFakeTimers()
    seedSession()
    let resolveWinningRequest!: (value: ReturnType<typeof refreshedResponse>) => void
    mockedPost.mockImplementationOnce(
      () => new Promise((resolve) => {
        resolveWinningRequest = resolve
      })
    )
    const firstTab = await import('@/api/tokenRefresh')
    vi.resetModules()
    const secondTab = await import('@/api/tokenRefresh')

    const winner = firstTab.refreshAuthTokens({ failedAccessToken: 'old-access' })
    mockedPost.mockRejectedValueOnce({ response: { status: 401 } })
    const loser = secondTab.refreshAuthTokens({ failedAccessToken: 'old-access' })

    window.setTimeout(() => resolveWinningRequest(refreshedResponse()), 1_500)
    await vi.advanceTimersByTimeAsync(1_600)

    await expect(winner).resolves.toMatchObject({ access_token: 'new-access' })
    await expect(loser).resolves.toMatchObject({ refresh_token: 'new-refresh' })
    expect(mockedPost).toHaveBeenCalledTimes(2)
    expect(localStorage.getItem('refresh_token')).toBe('new-refresh')
  })

  it('does not adopt a token from a different signed-in user', async () => {
    vi.useFakeTimers()
    seedSession()
    mockedPost.mockRejectedValueOnce(new Error('refresh token already used'))
    const { refreshAuthTokens } = await import('@/api/tokenRefresh')

    window.setTimeout(() => {
      localStorage.setItem('auth_user', JSON.stringify({ id: 8, email: 'other@example.com' }))
      localStorage.setItem('auth_token', 'other-access')
      localStorage.setItem('token_expires_at', String(Date.now() + 3600_000))
      localStorage.setItem('refresh_token', 'other-refresh')
    }, 10)

    const rejection = expect(
      refreshAuthTokens({ failedAccessToken: 'old-access' })
    ).rejects.toThrow('refresh token already used')
    await vi.advanceTimersByTimeAsync(1_100)
    await rejection
  })

  it('does not restore a session that was logged out while refresh was in flight', async () => {
    vi.useFakeTimers()
    seedSession()
    let resolveRequest!: (value: ReturnType<typeof refreshedResponse>) => void
    mockedPost.mockImplementationOnce(
      () => new Promise((resolve) => {
        resolveRequest = resolve
      })
    )
    const { refreshAuthTokens } = await import('@/api/tokenRefresh')

    const pending = refreshAuthTokens({ failedAccessToken: 'old-access' })
    localStorage.clear()
    resolveRequest(refreshedResponse())

    const rejection = expect(pending).rejects.toThrow('Session changed during token refresh')
    await vi.advanceTimersByTimeAsync(1_100)
    await rejection
    expect(localStorage.getItem('auth_token')).toBeNull()
    expect(localStorage.getItem('refresh_token')).toBeNull()
  })
})
