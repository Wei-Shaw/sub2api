import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, del } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put, delete: del }
}))

import {
  deleteOllamaCloudUsageSession,
  getOllamaCloudUsage,
  getOllamaCloudUsageSettings,
  refreshOllamaCloudUsage,
  saveOllamaCloudUsageSession,
  setOllamaCloudUsageAutoRefresh,
  updateOllamaCloudUsageSettings
} from '@/api/admin/accounts'

const state = {
  account_id: 7,
  eligible: true,
  configured: true,
  auto_refresh_enabled: false,
  encryption_key_configured: true
}

describe('admin Ollama Cloud usage API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    del.mockReset()
  })

  it('uses dedicated global settings endpoints', async () => {
    const settings = { enabled: false, interval_minutes: 60 }
    get.mockResolvedValueOnce({ data: settings })
    put.mockResolvedValueOnce({ data: settings })

    await expect(getOllamaCloudUsageSettings()).resolves.toEqual(settings)
    await expect(updateOllamaCloudUsageSettings(settings)).resolves.toEqual(settings)
    expect(get).toHaveBeenCalledWith('/admin/accounts/ollama-cloud-usage/settings')
    expect(put).toHaveBeenCalledWith('/admin/accounts/ollama-cloud-usage/settings', settings)
  })

  it('keeps session configuration write-only and separate from account updates', async () => {
    get.mockResolvedValueOnce({ data: state })
    put.mockResolvedValueOnce({ data: state }).mockResolvedValueOnce({ data: state })
    del.mockResolvedValueOnce({ data: { ...state, configured: false } })
    post.mockResolvedValueOnce({ data: state })

    await expect(getOllamaCloudUsage(7)).resolves.toEqual(state)
    await expect(saveOllamaCloudUsageSession(7, 'wos-session=secret')).resolves.toEqual(state)
    await expect(setOllamaCloudUsageAutoRefresh(7, true)).resolves.toEqual(state)
    await expect(refreshOllamaCloudUsage(7)).resolves.toEqual(state)
    await expect(deleteOllamaCloudUsageSession(7)).resolves.toMatchObject({ configured: false })

    expect(put).toHaveBeenNthCalledWith(1, '/admin/accounts/7/ollama-cloud-usage/session', { session: 'wos-session=secret' })
    expect(put).toHaveBeenNthCalledWith(2, '/admin/accounts/7/ollama-cloud-usage/auto-refresh', { enabled: true })
    expect(post).toHaveBeenCalledWith('/admin/accounts/7/ollama-cloud-usage/refresh')
    expect(del).toHaveBeenCalledWith('/admin/accounts/7/ollama-cloud-usage/session')
  })
})
