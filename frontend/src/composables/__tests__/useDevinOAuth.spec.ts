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
    devin: {
      generateAuthUrl: vi.fn(),
      exchangeCode: vi.fn()
    }
  }
}))

import { useDevinOAuth } from '@/composables/useDevinOAuth'
import { adminAPI } from '@/api/admin'

describe('useDevinOAuth.generateAuthUrl', () => {
  it('stores auth url + session fields on success', async () => {
    vi.mocked(adminAPI.devin.generateAuthUrl).mockResolvedValueOnce({
      auth_url: 'https://app.devin.ai/authorize?x=1',
      session_id: 'sess-1',
      state: 'state-1'
    } as any)
    const oauth = useDevinOAuth()

    expect(await oauth.generateAuthUrl(3)).toBe(true)
    expect(adminAPI.devin.generateAuthUrl).toHaveBeenCalledWith({ proxy_id: 3 })
    expect(oauth.authUrl.value).toContain('app.devin.ai')
    expect(oauth.sessionId.value).toBe('sess-1')
    expect(oauth.state.value).toBe('state-1')
    expect(oauth.error.value).toBe('')
  })

  it('reports failure and clears loading', async () => {
    vi.mocked(adminAPI.devin.generateAuthUrl).mockRejectedValueOnce({ message: 'boom' })
    const oauth = useDevinOAuth()

    expect(await oauth.generateAuthUrl(null)).toBe(false)
    expect(oauth.loading.value).toBe(false)
    expect(oauth.error.value).not.toBe('')
  })
})

describe('useDevinOAuth.exchangeAuthCode', () => {
  it('requires session/state for PKCE code exchange', async () => {
    const oauth = useDevinOAuth()
    const result = await oauth.exchangeAuthCode({ code: 'pkce-code' })
    expect(result).toBeNull()
    expect(oauth.error.value).toBe('admin.accounts.oauth.devin.missingExchangeParams')
    expect(adminAPI.devin.exchangeCode).not.toHaveBeenCalled()
  })

  it('pasted session token skips the session requirement', async () => {
    vi.mocked(adminAPI.devin.exchangeCode).mockResolvedValueOnce({
      access_token: 'devin-session-token$abc',
      api_server_url: 'https://server.codeium.example'
    } as any)
    const oauth = useDevinOAuth()

    const info = await oauth.exchangeAuthCode({ code: ' devin-session-token$abc ' })
    expect(adminAPI.devin.exchangeCode).toHaveBeenCalledWith({ code: 'devin-session-token$abc' })
    expect(info?.access_token).toBe('devin-session-token$abc')
  })

  it('forwards session fields and proxy for PKCE exchange', async () => {
    vi.mocked(adminAPI.devin.exchangeCode).mockResolvedValueOnce({ access_token: 'tok' } as any)
    const oauth = useDevinOAuth()

    await oauth.exchangeAuthCode({ code: 'c', sessionId: 's', state: 'st', proxyId: 9 })
    expect(adminAPI.devin.exchangeCode).toHaveBeenCalledWith({
      code: 'c', session_id: 's', state: 'st', proxy_id: 9
    })
  })
})

describe('useDevinOAuth credential builders', () => {
  it('buildCredentials drops empty optional fields', () => {
    const oauth = useDevinOAuth()
    const creds = oauth.buildCredentials({
      access_token: 'tok',
      api_server_url: '',
      client_version: undefined
    } as any)
    expect(creds).toEqual({ access_token: 'tok' })
  })

  it('buildExtraInfo only includes present identity fields', () => {
    const oauth = useDevinOAuth()
    expect(oauth.buildExtraInfo({ email: 'a@b.c' } as any)).toEqual({ email: 'a@b.c' })
    expect(oauth.buildExtraInfo({} as any)).toEqual({})
  })

  it('resetState clears everything', async () => {
    vi.mocked(adminAPI.devin.generateAuthUrl).mockResolvedValueOnce({
      auth_url: 'u', session_id: 's', state: 'st'
    } as any)
    const oauth = useDevinOAuth()
    await oauth.generateAuthUrl(null)
    oauth.resetState()
    expect(oauth.authUrl.value).toBe('')
    expect(oauth.sessionId.value).toBe('')
  })
})
