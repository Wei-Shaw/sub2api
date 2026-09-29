import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { AxiosInstance } from 'axios'

vi.mock('@/i18n', () => ({
  getLocale: () => 'zh-CN',
  i18n: { global: { t: (key: string) => key } },
}))

type F201Location = { pathname: string; search: string; href: string }

const f201OriginalLocation = window.location

function f201SetLocation(pathname: string, search = ''): F201Location {
  const location = { pathname, search, href: pathname + search }
  Object.defineProperty(window, 'location', { value: location, writable: true })
  return location
}

describe('API Client 401 on signed-out public pages [F2-01]', () => {
  let apiClient: AxiosInstance

  beforeEach(async () => {
    localStorage.clear()
    sessionStorage.clear()
    vi.resetModules()
    apiClient = (await import('@/api/client')).apiClient
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', { value: f201OriginalLocation, writable: true })
    vi.restoreAllMocks()
  })

  it('OAuth 回调页绑定登录密码错误时不跳转登录页，并返回结构化错误', async () => {
    const location = f201SetLocation('/auth/linuxdo/callback')
    apiClient.defaults.adapter = vi.fn().mockRejectedValueOnce({
      response: {
        status: 401,
        data: { code: 401, message: 'invalid email or password', reason: 'INVALID_CREDENTIALS' },
      },
      config: { url: '/auth/oauth/pending/bind-login', headers: {} },
      code: 'ERR_BAD_REQUEST',
    })

    await expect(
      apiClient.post('/auth/oauth/pending/bind-login', { email: 'a@b.c', password: 'x' })
    ).rejects.toMatchObject({ status: 401, reason: 'INVALID_CREDENTIALS', message: 'invalid email or password' })
    expect(location.href).toBe('/auth/linuxdo/callback')
    expect(sessionStorage.getItem('auth_expired')).toBeNull()
  })

  it('支付结果页未登录访问需登录接口时不跳转，留给页面回退到公开接口', async () => {
    const location = f201SetLocation('/payment/result', '?order_id=1&out_trade_no=T1')
    apiClient.defaults.adapter = vi.fn().mockRejectedValueOnce({
      response: { status: 401, data: { code: 'UNAUTHORIZED', message: 'Authorization header is required' } },
      config: { url: '/payment/orders/1', headers: {} },
      code: 'ERR_BAD_REQUEST',
    })

    await expect(apiClient.get('/payment/orders/1')).rejects.toMatchObject({
      status: 401,
      code: 'UNAUTHORIZED',
    })
    expect(location.href).toBe('/payment/result?order_id=1&out_trade_no=T1')
  })

  it('受保护页面即使请求未携带凭证（其他标签页已登出）仍跳转登录页', async () => {
    const location = f201SetLocation('/dashboard')
    apiClient.defaults.adapter = vi.fn().mockRejectedValueOnce({
      response: { status: 401, data: { code: 'UNAUTHORIZED', message: 'Authorization header is required' } },
      config: { url: '/user/profile', headers: {} },
      code: 'ERR_BAD_REQUEST',
    })

    await expect(apiClient.get('/user/profile')).rejects.toMatchObject({ status: 401, code: 'UNAUTHORIZED' })
    expect(location.href).toBe('/login?redirect=%2Fdashboard')
  })

  it('公开流程页上携带过期凭证的请求仍清理会话并跳转登录页', async () => {
    localStorage.setItem('auth_token', 'expired-token')
    const location = f201SetLocation('/payment/result', '?order_id=1')
    apiClient.defaults.adapter = vi.fn().mockRejectedValueOnce({
      response: { status: 401, data: { code: 'TOKEN_EXPIRED', message: 'Token has expired' } },
      config: { url: '/payment/orders/1', headers: { Authorization: 'Bearer expired-token' } },
      code: 'ERR_BAD_REQUEST',
    })

    await expect(apiClient.get('/payment/orders/1')).rejects.toMatchObject({ status: 401, code: 'TOKEN_EXPIRED' })
    expect(localStorage.getItem('auth_token')).toBeNull()
    expect(sessionStorage.getItem('auth_expired')).toBe('1')
    expect(location.href).toBe('/login?redirect=%2Fpayment%2Fresult%3Forder_id%3D1')
  })
})
