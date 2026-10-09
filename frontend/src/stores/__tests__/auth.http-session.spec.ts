import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import axios, { type InternalAxiosRequestConfig } from 'axios'
import { apiClient } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

vi.mock('@/i18n', () => ({ getLocale: () => 'en' }))
vi.mock('@/api', async () => {
  const auth = await import('@/api/auth')
  return { authAPI: auth, isTotp2FARequired: auth.isTotp2FARequired, passkeyAPI: {} }
})
const user = { id: 1, email: 'test@example.test', username: 'Test', role: 'user', balance: 0, status: 'active' }
const response = (config: InternalAxiosRequestConfig) => ({ data: { code: 0, data: user }, status: 200, statusText: 'OK', headers: {}, config })

describe('OAuth through real HTTP interceptors', () => {
  beforeEach(() => { localStorage.clear(); sessionStorage.clear(); setActivePinia(createPinia()); vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

  it('does not clear a later token-only session on a stale /me 401', async () => {
    let rejectOld!: (error: unknown) => void
    let oldConfig!: InternalAxiosRequestConfig
    apiClient.defaults.adapter = vi.fn(config => new Promise((_, reject) => { oldConfig = config; rejectOld = reject }))
    const store = useAuthStore()
    const pending = store.setToken('old-access', { expectedSessionVersion: store.authSessionVersion })
    const checked = expect(pending).rejects.toMatchObject({ code: 'AUTH_SESSION_CHANGED' })
    await vi.waitFor(() => expect(oldConfig).toBeDefined())
    apiClient.defaults.adapter = vi.fn(async config => response(config))
    await store.setToken('later-access', { expectedSessionVersion: store.authSessionVersion })
    store.setPendingAuthSession({ token: 'later-pending', token_field: 'pending_oauth_token', provider: 'oidc' })
    localStorage.setItem('oauth_aff_code', 'later-referral')
    const saved = { ...localStorage }
    rejectOld({ config: oldConfig, response: { status: 401, data: { code: 'INVALID_TOKEN' } } })
    await checked
    expect({ ...localStorage }).toEqual(saved)
    expect(store.token).toBe('later-access')
    expect(store.user?.id).toBe(1)
  })

  it('reports a current guarded token failure as its original authentication error', async () => {
    apiClient.defaults.adapter = vi.fn(async config => { throw { config, response: { status: 401, data: { code: 'INVALID_TOKEN' } } } })
    const store = useAuthStore()
    await expect(store.setToken('invalid-access', { expectedSessionVersion: store.authSessionVersion })).rejects.toMatchObject({ status: 401, code: 'INVALID_TOKEN' })
    expect(store.token).toBeNull()
  })

  it.each(['refreshUser', 'setToken'])('does not clear a rotated token when an old %s request returns 401', async action => {
    apiClient.defaults.adapter = vi.fn(async config => response(config))
    const store = useAuthStore()
    await store.setToken('first-access')
    let fail!: (error: unknown) => void
    let config!: InternalAxiosRequestConfig
    apiClient.defaults.adapter = vi.fn(c => new Promise((_, reject) => { config = c; fail = reject }))
    const result = action === 'setToken' ? store.setToken('first-access') : store.refreshUser()
    const checked = expect(result).rejects.toMatchObject({ code: 'AUTH_SESSION_CHANGED' })
    await vi.waitFor(() => expect(config).toBeDefined())
    localStorage.setItem('auth_token', 'rotated-access')
    fail({ config, response: { status: 401, data: { code: 'TOKEN_EXPIRED' } } })
    await checked
    expect(localStorage.getItem('auth_token')).toBe('rotated-access')
    if (action === 'refreshUser') expect(store.user?.id).toBe(1)
  })

  it('keeps single-argument OAuth token refresh and retry working', async () => {
    localStorage.setItem('refresh_token', 'initial-refresh')
    vi.spyOn(axios, 'post').mockResolvedValue({ data: { code: 0, data: { access_token: 'rotated-access', refresh_token: 'rotated-refresh', expires_in: 3600, token_type: 'Bearer' } } })
    apiClient.defaults.adapter = vi.fn(async config => {
      if (config.headers.Authorization === 'Bearer initial-access') throw { config, response: { status: 401, data: { code: 'TOKEN_EXPIRED' } } }
      return response(config)
    })
    const store = useAuthStore()
    store.setPendingAuthSession({ token: 'pending', token_field: 'pending_oauth_token', provider: 'oidc' })
    await expect(store.setToken('initial-access')).resolves.toMatchObject({ id: 1 })
    expect(store.user?.id).toBe(1)
    expect(localStorage.getItem('auth_token')).toBe('rotated-access')
    expect(localStorage.getItem('refresh_token')).toBe('rotated-refresh')
    expect(store.pendingAuthSession).toBeNull()
  })
})
