import { describe, expect, it, vi, beforeEach } from 'vitest'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      const messages: Record<string, string> = {
        'admin.accounts.oauth.grok.failedToExchangeCode': 'Grok 授权完成失败',
        'admin.accounts.oauth.grok.failedToPollDevice': '轮询 Grok 设备授权状态失败',
        'admin.accounts.oauth.grok.errors.GROK_OAUTH_INVALID_STATE':
          'Grok OAuth state 与当前会话不匹配。请重新生成授权链接。',
        'admin.accounts.oauth.grok.errors.GROK_OAUTH_AUTHORIZATION_PENDING':
          '用户尚未在浏览器中完成设备授权，请稍后再试。'
      }
      return messages[key] ?? key
    }
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    grok: {
      generateAuthUrl: vi.fn(),
      exchangeCode: vi.fn(),
      pollDeviceLogin: vi.fn(),
      refreshGrokToken: vi.fn()
    }
  }
}))

import { useGrokOAuth } from '@/composables/useGrokOAuth'
import { adminAPI } from '@/api/admin'

describe('useGrokOAuth.generateAuthUrl', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('starts device polling after generating a device auth URL', async () => {
    vi.mocked(adminAPI.grok.generateAuthUrl).mockResolvedValueOnce({
      auth_url: 'https://auth.x.ai/oauth2/device?user_code=ABCD',
      session_id: 'session-1',
      state: 'state-1',
      flow: 'device',
      user_code: 'ABCD',
      interval: 5,
      expires_in: 600
    })
    vi.mocked(adminAPI.grok.pollDeviceLogin).mockResolvedValueOnce({
      status: 'pending',
      interval: 5
    })

    const oauth = useGrokOAuth()
    const ok = await oauth.generateAuthUrl(null)
    expect(ok).toBe(true)
    expect(oauth.authUrl.value).toContain('user_code=ABCD')
    expect(oauth.userCode.value).toBe('ABCD')
    expect(oauth.flow.value).toBe('device')
    expect(oauth.polling.value).toBe(true)

    oauth.stopPolling()
  })
})

describe('useGrokOAuth.exchangeAuthCode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows a structured backend recovery hint', async () => {
    vi.mocked(adminAPI.grok.exchangeCode).mockRejectedValueOnce({
      status: 400,
      reason: 'GROK_OAUTH_INVALID_STATE',
      message: 'invalid oauth state'
    })
    const oauth = useGrokOAuth()

    const tokenInfo = await oauth.exchangeAuthCode({
      sessionId: 'session-id',
      state: 'wrong-state'
    })

    expect(tokenInfo).toBeNull()
    expect(oauth.error.value).toBe(
      'Grok OAuth state 与当前会话不匹配。请重新生成授权链接。'
    )
  })

  it('returns an already authorized token without calling exchange', async () => {
    const oauth = useGrokOAuth()
    oauth.authorizedToken.value = {
      access_token: 'ready-access',
      refresh_token: 'ready-refresh'
    }

    const tokenInfo = await oauth.exchangeAuthCode({ sessionId: 'session-id' })
    expect(tokenInfo?.access_token).toBe('ready-access')
    expect(adminAPI.grok.exchangeCode).not.toHaveBeenCalled()
  })
})

describe('useGrokOAuth.buildCredentials', () => {
  it('persists the Grok CLI subscription proxy for OAuth inference', () => {
    const oauth = useGrokOAuth()

    const credentials = oauth.buildCredentials({
      access_token: 'access-token',
      token_type: 'Bearer',
      expires_at: 1_900_000_000,
      client_id: 'client-id',
      scope: 'openid grok-cli:access',
      email: 'grok@example.com'
    })

    expect(credentials.base_url).toBe('https://cli-chat-proxy.grok.com/v1')
  })
})
