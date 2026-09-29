import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import axios from 'axios'
import type { AxiosInstance } from 'axios'

vi.mock('@/i18n', () => ({
  getLocale: () => 'zh-CN',
  i18n: { global: { t: (key: string) => key } },
}))

const f212OriginalLocation = window.location

function f212SignIn(withRefreshToken = true) {
  localStorage.setItem('auth_token', 'access-token')
  if (withRefreshToken) {
    localStorage.setItem('refresh_token', 'refresh-token')
  }
  localStorage.setItem('auth_user', JSON.stringify({ id: 7 }))
  localStorage.setItem('token_expires_at', String(Date.now() + 3600_000))
}

function f212MockRefresh() {
  return vi.spyOn(axios, 'post').mockResolvedValue({
    data: {
      code: 0,
      message: 'ok',
      data: { access_token: 'new-token', refresh_token: 'new-refresh-token', expires_in: 3600, token_type: 'Bearer' },
    },
  })
}

describe('API Client business 401 pass-through [F2-12]', () => {
  let apiClient: AxiosInstance
  let location: { pathname: string; search: string; href: string }

  beforeEach(async () => {
    localStorage.clear()
    sessionStorage.clear()
    location = { pathname: '/profile', search: '', href: '/profile' }
    Object.defineProperty(window, 'location', { value: location, writable: true })
    vi.resetModules()
    apiClient = (await import('@/api/client')).apiClient
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', { value: f212OriginalLocation, writable: true })
    vi.restoreAllMocks()
  })

  it('Passkey 验证失败的 401 不刷新令牌、不重放请求，直接返回原始错误', async () => {
    f212SignIn()
    const refreshPost = f212MockRefresh()
    const adapter = vi.fn()
      .mockRejectedValueOnce({
        response: {
          status: 401,
          data: { code: 401, message: 'passkey verification failed', reason: 'PASSKEY_VERIFICATION_FAILED' },
        },
        config: { url: '/user/passkeys/register/finish', headers: { Authorization: 'Bearer access-token' } },
        code: 'ERR_BAD_REQUEST',
      })
      // 一次性 ceremony 会话已被首次请求消费，重放只会得到误导性的会话失效错误
      .mockRejectedValueOnce({
        response: {
          status: 400,
          data: { code: 400, message: 'passkey session is invalid or expired', reason: 'PASSKEY_SESSION_INVALID' },
        },
        config: { url: '/user/passkeys/register/finish', headers: { Authorization: 'Bearer new-token' } },
        code: 'ERR_BAD_REQUEST',
      })
    apiClient.defaults.adapter = adapter

    await expect(apiClient.post('/user/passkeys/register/finish', {})).rejects.toMatchObject({
      status: 401,
      reason: 'PASSKEY_VERIFICATION_FAILED',
      message: 'passkey verification failed',
    })
    expect(adapter).toHaveBeenCalledTimes(1)
    expect(refreshPost).not.toHaveBeenCalled()
    expect(localStorage.getItem('auth_token')).toBe('access-token')
    expect(localStorage.getItem('refresh_token')).toBe('refresh-token')
    expect(sessionStorage.getItem('auth_expired')).toBeNull()
    expect(location.href).toBe('/profile')
  })

  it('无 refresh_token 时业务 401 也不清除登录态', async () => {
    f212SignIn(false)
    apiClient.defaults.adapter = vi.fn().mockRejectedValueOnce({
      response: {
        status: 401,
        data: { code: 401, message: 'passkey verification failed', reason: 'PASSKEY_VERIFICATION_FAILED' },
      },
      config: { url: '/user/passkeys/register/finish', headers: { Authorization: 'Bearer access-token' } },
      code: 'ERR_BAD_REQUEST',
    })

    await expect(apiClient.post('/user/passkeys/register/finish', {})).rejects.toMatchObject({
      status: 401,
      reason: 'PASSKEY_VERIFICATION_FAILED',
    })
    expect(localStorage.getItem('auth_token')).toBe('access-token')
    expect(localStorage.getItem('auth_user')).toBe(JSON.stringify({ id: 7 }))
    expect(sessionStorage.getItem('auth_expired')).toBeNull()
    expect(location.href).toBe('/profile')
  })

  it.each([
    ['无 reason 的 401', { code: 401, message: 'User not authenticated' }],
    ['信封格式的 TOKEN_REVOKED', { code: 401, message: 'token has been revoked', reason: 'TOKEN_REVOKED' }],
    ['中间件格式的 SESSION_BINDING_MISMATCH', { code: 'SESSION_BINDING_MISMATCH', message: 'changed' }],
  ])('令牌类 401（%s）仍刷新并重放', async (_name, body) => {
    f212SignIn()
    const refreshPost = f212MockRefresh()
    const adapter = vi.fn()
      .mockRejectedValueOnce({
        response: { status: 401, data: body },
        config: { url: '/user/profile', headers: { Authorization: 'Bearer access-token' } },
        code: 'ERR_BAD_REQUEST',
      })
      .mockResolvedValueOnce({ status: 200, data: { code: 0, data: { ok: true } }, headers: {}, config: {}, statusText: 'OK' })
    apiClient.defaults.adapter = adapter

    await expect(apiClient.get('/user/profile')).resolves.toMatchObject({ data: { ok: true } })
    expect(refreshPost).toHaveBeenCalledTimes(1)
    expect(adapter).toHaveBeenCalledTimes(2)
    expect(adapter.mock.calls[1][0].headers.Authorization).toBe('Bearer new-token')
  })
})
