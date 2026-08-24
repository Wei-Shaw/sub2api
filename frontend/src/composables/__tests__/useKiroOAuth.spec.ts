import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  generateAuthUrl: vi.fn(),
  exchangeCode: vi.fn(),
  refreshToken: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    kiro: {
      generateAuthUrl: mocks.generateAuthUrl,
      generateIDCAuthUrl: vi.fn(),
      exchangeCode: mocks.exchangeCode,
      refreshToken: mocks.refreshToken,
      importToken: vi.fn()
    }
  }
}))

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

import { useKiroOAuth } from '../useKiroOAuth'

describe('useKiroOAuth', () => {
  beforeEach(() => {
    mocks.generateAuthUrl.mockReset()
    mocks.exchangeCode.mockReset()
    mocks.refreshToken.mockReset()
  })

  it('requests an explicit External IdP authorization session', async () => {
    mocks.generateAuthUrl.mockResolvedValueOnce({
      auth_url: 'https://app.kiro.dev/signin?state=state-1',
      session_id: 'session-1',
      state: 'state-1'
    })

    const kiroOAuth = useKiroOAuth()
    const result = await kiroOAuth.generateAuthUrl(9, 'ExternalIdp')

    expect(result).toBe(true)
    expect(mocks.generateAuthUrl).toHaveBeenCalledWith({
      proxy_id: 9,
      provider: 'ExternalIdp'
    })
    expect(kiroOAuth.authUrl.value).toBe('https://app.kiro.dev/signin?state=state-1')
    expect(kiroOAuth.sessionId.value).toBe('session-1')
    expect(kiroOAuth.state.value).toBe('state-1')
  })

  it('submits Kiro External IdP descriptors as raw callback URLs', async () => {
    const descriptor = 'http://localhost:49153/signin/callback?login_option=external_idp&issuer_url=https%3A%2F%2Flogin.microsoftonline.com%2Ftenant%2Fv2.0&client_id=client-1&scopes=openid+profile'
    mocks.exchangeCode.mockResolvedValueOnce({
      auth_url: 'https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize',
      session_id: 'session-1',
      state: 'state-2'
    })

    const kiroOAuth = useKiroOAuth()
    const result = await kiroOAuth.exchangeAuthCode({
      code: descriptor,
      sessionId: 'session-1',
      state: 'state-1',
      proxyId: 9
    })

    expect(result).toBeNull()
    expect(mocks.exchangeCode).toHaveBeenCalledWith({
      session_id: 'session-1',
      state: 'state-1',
      code: descriptor,
      callback_path: undefined,
      login_option: undefined,
      proxy_id: 9
    })
    expect(kiroOAuth.authUrl.value).toBe('https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize')
    expect(kiroOAuth.sessionId.value).toBe('session-1')
    expect(kiroOAuth.state.value).toBe('state-2')
  })

  it('updates authorization state when Kiro exchange returns an explicit External IdP authorization URL', async () => {
    mocks.exchangeCode.mockResolvedValueOnce({
      auth_url: 'https://idp.example.com/authorize',
      session_id: 'session-1',
      state: 'state-2'
    })

    const kiroOAuth = useKiroOAuth()
    const result = await kiroOAuth.exchangeAuthCode({
      code: 'https://localhost:49153/callback?issuer_url=https%3A%2F%2Fidp.example.com&client_id=client-1',
      sessionId: 'session-1',
      state: 'state-1',
      proxyId: 9
    })

    expect(result).toBeNull()
    expect(kiroOAuth.authUrl.value).toBe('https://idp.example.com/authorize')
    expect(kiroOAuth.sessionId.value).toBe('session-1')
    expect(kiroOAuth.state.value).toBe('state-2')
  })

  it('passes external IdP refresh metadata and persists it into credentials', async () => {
    mocks.refreshToken.mockResolvedValueOnce({
      access_token: 'access-2',
      refresh_token: 'refresh-2',
      auth_method: 'external_idp',
      provider: 'ExternalIdp',
      client_id: 'client-1',
      token_endpoint: 'https://idp.example.com/token',
      issuer_url: 'https://idp.example.com',
      scopes: 'openid profile email'
    })

    const kiroOAuth = useKiroOAuth()
    const tokenInfo = await kiroOAuth.validateRefreshToken({
      refreshToken: 'refresh-1',
      authMethod: 'external_idp',
      provider: 'ExternalIdp',
      clientId: 'client-1',
      tokenEndpoint: 'https://idp.example.com/token',
      issuerUrl: 'https://idp.example.com',
      scopes: 'openid profile email',
      proxyId: 9
    })

    expect(mocks.refreshToken).toHaveBeenCalledWith({
      refresh_token: 'refresh-1',
      auth_method: 'external_idp',
      provider: 'ExternalIdp',
      client_id: 'client-1',
      client_secret: undefined,
      start_url: undefined,
      region: undefined,
      profile_arn: undefined,
      token_endpoint: 'https://idp.example.com/token',
      issuer_url: 'https://idp.example.com',
      scopes: 'openid profile email',
      proxy_id: 9
    })
    expect(kiroOAuth.buildCredentials(tokenInfo!)).toMatchObject({
      auth_method: 'external_idp',
      provider: 'ExternalIdp',
      client_id: 'client-1',
      token_endpoint: 'https://idp.example.com/token',
      issuer_url: 'https://idp.example.com',
      scopes: 'openid profile email'
    })
  })
})
