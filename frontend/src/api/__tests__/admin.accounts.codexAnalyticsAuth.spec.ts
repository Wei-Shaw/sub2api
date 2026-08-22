import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { InternalAxiosRequestConfig } from 'axios'
import { apiClient } from '@/api/client'
import { getCodexAnalytics } from '@/api/admin/accounts'

const { refreshAuthTokens } = vi.hoisted(() => ({
  refreshAuthTokens: vi.fn()
}))

vi.mock('@/api/tokenRefresh', () => ({ refreshAuthTokens }))

describe('admin account Codex analytics authentication handling', () => {
  beforeEach(() => {
    localStorage.clear()
    refreshAuthTokens.mockReset()
    window.history.replaceState({}, '', '/admin/accounts/42')
  })

  it('returns an upstream 401 without refreshing or clearing the admin session', async () => {
    localStorage.setItem('auth_token', 'admin-access')
    localStorage.setItem('refresh_token', 'admin-refresh')
    localStorage.setItem('auth_user', JSON.stringify({ id: 42 }))
    localStorage.setItem('token_expires_at', '4102444800000')

    const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.reject({
      response: {
        status: 401,
        data: {
          code: 'UPSTREAM_UNAUTHORIZED',
          reason: 'OPENAI_QUOTA_UPSTREAM_ERROR',
          message: 'OpenAI account authorization failed'
        },
        headers: {},
        config,
        statusText: 'Unauthorized'
      },
      config,
      code: 'ERR_BAD_REQUEST',
      message: 'Request failed with status code 401'
    }))
    apiClient.defaults.adapter = adapter

    await expect(getCodexAnalytics(42)).rejects.toMatchObject({
      status: 401,
      code: 'UPSTREAM_UNAUTHORIZED',
      reason: 'OPENAI_QUOTA_UPSTREAM_ERROR'
    })

    expect(adapter).toHaveBeenCalledTimes(1)
    expect(adapter.mock.calls[0][0]).toMatchObject({
      skipAuthRefreshReasons: [
        'OPENAI_QUOTA_UPSTREAM_ERROR',
        'OPENAI_CODEX_PROFILE_UNAUTHORIZED'
      ]
    })
    expect(refreshAuthTokens).not.toHaveBeenCalled()
    expect(localStorage.getItem('auth_token')).toBe('admin-access')
    expect(localStorage.getItem('refresh_token')).toBe('admin-refresh')
    expect(localStorage.getItem('auth_user')).toBe(JSON.stringify({ id: 42 }))
    expect(localStorage.getItem('token_expires_at')).toBe('4102444800000')
    expect(window.location.pathname).toBe('/admin/accounts/42')
  })

  it('refreshes and retries when the same endpoint returns a panel-auth 401', async () => {
    localStorage.setItem('auth_token', 'expired-admin-access')
    localStorage.setItem('refresh_token', 'admin-refresh')
    localStorage.setItem('auth_user', JSON.stringify({ id: 42 }))
    localStorage.setItem('token_expires_at', '0')
    refreshAuthTokens.mockImplementationOnce(async () => {
      localStorage.setItem('auth_token', 'renewed-admin-access')
      localStorage.setItem('refresh_token', 'renewed-admin-refresh')
      localStorage.setItem('token_expires_at', String(Date.now() + 3_600_000))
      return {
        access_token: 'renewed-admin-access',
        refresh_token: 'renewed-admin-refresh',
        expires_in: 3600,
        token_type: 'Bearer'
      }
    })

    const adapter = vi.fn()
    adapter.mockImplementationOnce((config: InternalAxiosRequestConfig) => Promise.reject({
      response: {
        status: 401,
        data: {
          code: 'TOKEN_EXPIRED',
          reason: 'TOKEN_EXPIRED',
          message: 'Token expired'
        },
        headers: {},
        config,
        statusText: 'Unauthorized'
      },
      config,
      code: 'ERR_BAD_REQUEST',
      message: 'Request failed with status code 401'
    }))
    adapter.mockResolvedValueOnce({
      status: 200,
      data: { code: 0, data: { ok: true } },
      headers: {},
      config: {},
      statusText: 'OK'
    })
    apiClient.defaults.adapter = adapter

    await expect(getCodexAnalytics(42)).resolves.toEqual({ ok: true })

    expect(refreshAuthTokens).toHaveBeenCalledTimes(1)
    expect(refreshAuthTokens).toHaveBeenCalledWith({ failedAccessToken: 'expired-admin-access' })
    expect(adapter).toHaveBeenCalledTimes(2)
    expect(adapter.mock.calls[1][0].headers.get('Authorization')).toBe('Bearer renewed-admin-access')
  })
})
