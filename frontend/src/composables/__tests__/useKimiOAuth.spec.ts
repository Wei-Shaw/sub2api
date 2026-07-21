import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

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
    kimi: {
      startDeviceAuth: vi.fn(),
      pollDeviceToken: vi.fn(),
      refreshKimiToken: vi.fn(),
      refreshAccountToken: vi.fn(),
      createAccountFromOAuth: vi.fn()
    }
  }
}))

import {
  buildKimiCredentials,
  buildKimiExtraInfo,
  useKimiOAuth,
  KIMI_DEFAULT_BASE_URL,
  KIMI_OAUTH_REASON_PENDING,
  KIMI_OAUTH_REASON_SLOW_DOWN,
  KIMI_OAUTH_REASON_EXPIRED,
  KIMI_OAUTH_REASON_ACCESS_DENIED
} from '@/composables/useKimiOAuth'
import { adminAPI } from '@/api/admin'

const deviceAuthResponse = {
  session_id: 'session-abc',
  user_code: 'ABCD-EFGH',
  verification_uri: 'https://www.kimi.com/coding/device',
  verification_uri_complete: 'https://www.kimi.com/coding/device?user_code=ABCD-EFGH',
  interval: 5,
  expires_in: 600
}

const tokenInfo = {
  access_token: 'kimi-at',
  refresh_token: 'kimi-rt',
  token_type: 'Bearer',
  expires_at: 1700000000,
  device_id: 'device-123',
  scope: 'coding'
}

describe('useKimiOAuth device flow', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockReset()
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('fills device auth fields and starts polling after startDeviceAuth', async () => {
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockResolvedValueOnce(deviceAuthResponse)
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockRejectedValue({
      reason: KIMI_OAUTH_REASON_PENDING,
      metadata: {}
    })
    const oauth = useKimiOAuth()

    const ok = await oauth.startDeviceAuth(null)

    expect(ok).toBe(true)
    expect(oauth.status.value).toBe('awaiting_user')
    expect(oauth.userCode.value).toBe('ABCD-EFGH')
    expect(oauth.sessionId.value).toBe('session-abc')
    expect(oauth.verificationUriComplete.value).toBe(
      'https://www.kimi.com/coding/device?user_code=ABCD-EFGH'
    )
    expect(oauth.pollInterval.value).toBe(5)
    expect(oauth.polling.value).toBe(true)

    await vi.advanceTimersByTimeAsync(5000)
    expect(adminAPI.kimi.pollDeviceToken).toHaveBeenCalledTimes(1)
    expect(adminAPI.kimi.pollDeviceToken).toHaveBeenCalledWith({ session_id: 'session-abc' })
  })

  it('passes proxy_id to both start and poll requests', async () => {
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockResolvedValueOnce(deviceAuthResponse)
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockRejectedValue({
      reason: KIMI_OAUTH_REASON_PENDING,
      metadata: {}
    })
    const oauth = useKimiOAuth()

    await oauth.startDeviceAuth(7)
    expect(adminAPI.kimi.startDeviceAuth).toHaveBeenCalledWith({ proxy_id: 7 })

    await vi.advanceTimersByTimeAsync(5000)
    expect(adminAPI.kimi.pollDeviceToken).toHaveBeenCalledWith({
      session_id: 'session-abc',
      proxy_id: 7
    })
  })

  it('keeps polling on authorization_pending and honors metadata.interval', async () => {
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockResolvedValueOnce(deviceAuthResponse)
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockRejectedValue({
      reason: KIMI_OAUTH_REASON_PENDING,
      metadata: { interval: '3' }
    })
    const oauth = useKimiOAuth()

    await oauth.startDeviceAuth(null)
    await vi.advanceTimersByTimeAsync(5000)

    expect(adminAPI.kimi.pollDeviceToken).toHaveBeenCalledTimes(1)
    expect(oauth.status.value).toBe('awaiting_user')
    expect(oauth.pollInterval.value).toBe(3)
    expect(oauth.polling.value).toBe(true)

    await vi.advanceTimersByTimeAsync(3000)
    expect(adminAPI.kimi.pollDeviceToken).toHaveBeenCalledTimes(2)
  })

  it('slows down polling on slow_down using metadata.suggested_interval', async () => {
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockResolvedValueOnce(deviceAuthResponse)
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockRejectedValueOnce({
      reason: KIMI_OAUTH_REASON_SLOW_DOWN,
      metadata: { suggested_interval: '10' }
    })
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockRejectedValue({
      reason: KIMI_OAUTH_REASON_PENDING,
      metadata: {}
    })
    const oauth = useKimiOAuth()

    await oauth.startDeviceAuth(null)
    await vi.advanceTimersByTimeAsync(5000)

    expect(oauth.pollInterval.value).toBe(10)
    expect(oauth.status.value).toBe('awaiting_user')

    // Next poll should be scheduled 10s later, not 5s.
    await vi.advanceTimersByTimeAsync(5000)
    expect(adminAPI.kimi.pollDeviceToken).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(5000)
    expect(adminAPI.kimi.pollDeviceToken).toHaveBeenCalledTimes(2)
  })

  it('emits onAuthorized and stops polling when the user confirms', async () => {
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockResolvedValueOnce(deviceAuthResponse)
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockResolvedValueOnce(tokenInfo)
    const onAuthorized = vi.fn()
    const oauth = useKimiOAuth({ onAuthorized })

    await oauth.startDeviceAuth(null)
    await vi.advanceTimersByTimeAsync(5000)

    expect(oauth.status.value).toBe('success')
    expect(oauth.polling.value).toBe(false)
    expect(onAuthorized).toHaveBeenCalledTimes(1)
    expect(onAuthorized).toHaveBeenCalledWith(tokenInfo)

    await vi.advanceTimersByTimeAsync(30000)
    expect(adminAPI.kimi.pollDeviceToken).toHaveBeenCalledTimes(1)
  })

  it('stops polling with expired status when the device code expires', async () => {
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockResolvedValueOnce(deviceAuthResponse)
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockRejectedValueOnce({
      reason: KIMI_OAUTH_REASON_EXPIRED,
      message: 'expired'
    })
    const oauth = useKimiOAuth()

    await oauth.startDeviceAuth(null)
    await vi.advanceTimersByTimeAsync(5000)

    expect(oauth.status.value).toBe('expired')
    expect(oauth.polling.value).toBe(false)
    expect(oauth.error.value).toBe('admin.accounts.oauth.kimi.expired')

    await vi.advanceTimersByTimeAsync(30000)
    expect(adminAPI.kimi.pollDeviceToken).toHaveBeenCalledTimes(1)
  })

  it('stops polling with denied status when access is denied', async () => {
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockResolvedValueOnce(deviceAuthResponse)
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockRejectedValueOnce({
      reason: KIMI_OAUTH_REASON_ACCESS_DENIED,
      message: 'denied'
    })
    const oauth = useKimiOAuth()

    await oauth.startDeviceAuth(null)
    await vi.advanceTimersByTimeAsync(5000)

    expect(oauth.status.value).toBe('denied')
    expect(oauth.polling.value).toBe(false)
    expect(oauth.error.value).toBe('admin.accounts.oauth.kimi.denied')
  })

  it('marks status as expired when the client-side expiry guard trips', async () => {
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockResolvedValueOnce({
      ...deviceAuthResponse,
      expires_in: 8
    })
    vi.mocked(adminAPI.kimi.pollDeviceToken).mockRejectedValue({
      reason: KIMI_OAUTH_REASON_PENDING,
      metadata: {}
    })
    const oauth = useKimiOAuth()

    await oauth.startDeviceAuth(null)
    // First poll at t=5s is still valid (pending); the next one at t=10s
    // trips the client-side expiry guard (expires_in = 8s).
    await vi.advanceTimersByTimeAsync(10000)

    expect(oauth.status.value).toBe('expired')
    expect(oauth.polling.value).toBe(false)
  })

  it('sets error status when startDeviceAuth fails', async () => {
    vi.mocked(adminAPI.kimi.startDeviceAuth).mockRejectedValueOnce({
      message: 'network down'
    })
    const oauth = useKimiOAuth()

    const ok = await oauth.startDeviceAuth(null)

    expect(ok).toBe(false)
    expect(oauth.status.value).toBe('error')
    expect(oauth.error.value).toBe('network down')
    expect(oauth.polling.value).toBe(false)
  })
})

describe('buildKimiCredentials', () => {
  it('converts numeric expires_at to ISO string and keeps device_id/base_url', () => {
    const creds = buildKimiCredentials(tokenInfo)

    expect(creds.access_token).toBe('kimi-at')
    expect(creds.refresh_token).toBe('kimi-rt')
    expect(creds.expires_at).toBe(new Date(1700000000 * 1000).toISOString())
    expect(creds.device_id).toBe('device-123')
    expect(creds.scope).toBe('coding')
    expect(creds.base_url).toBe(KIMI_DEFAULT_BASE_URL)
  })

  it('keeps string expires_at as-is and filters empty values', () => {
    const creds = buildKimiCredentials({
      access_token: 'at',
      refresh_token: 'rt',
      expires_at: '2026-01-01T00:00:00Z',
      client_id: '',
      device_id: undefined
    })

    expect(creds.expires_at).toBe('2026-01-01T00:00:00Z')
    expect(Object.prototype.hasOwnProperty.call(creds, 'client_id')).toBe(false)
    expect(Object.prototype.hasOwnProperty.call(creds, 'device_id')).toBe(false)
    expect(Object.prototype.hasOwnProperty.call(creds, 'token_type')).toBe(false)
    expect(creds.base_url).toBe(KIMI_DEFAULT_BASE_URL)
  })
})

describe('buildKimiExtraInfo', () => {
  it('returns an empty object', () => {
    expect(buildKimiExtraInfo(tokenInfo)).toEqual({})
  })
})
