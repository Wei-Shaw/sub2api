import { afterEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from '../client'
import { initializeNotificationEmailLocale, updateNotificationEmailLocale } from '../user'

describe('updateNotificationEmailLocale', () => {
  afterEach(() => {
    apiClient.defaults.adapter = undefined
  })

  it('sends the explicit locale to the authenticated user endpoint', async () => {
    const adapter = vi.fn().mockResolvedValue({
      status: 200,
      data: { code: 0, data: { locale: 'zh' } },
      headers: {},
      config: {},
      statusText: 'OK',
    })
    apiClient.defaults.adapter = adapter

    await updateNotificationEmailLocale('zh')

    expect(adapter).toHaveBeenCalledOnce()
    const config = adapter.mock.calls[0][0]
    expect(config.method).toBe('put')
    expect(config.url).toBe('/user/notification-email-locale')
    expect(JSON.parse(config.data)).toEqual({ locale: 'zh' })
  })

  it('sends the fallback locale to the non-overwriting initialization endpoint', async () => {
    const adapter = vi.fn().mockResolvedValue({
      status: 200,
      data: { code: 0, data: { initialized: true } },
      headers: {},
      config: {},
      statusText: 'OK',
    })
    apiClient.defaults.adapter = adapter

    await initializeNotificationEmailLocale('zh')

    expect(adapter).toHaveBeenCalledOnce()
    const config = adapter.mock.calls[0][0]
    expect(config.method).toBe('put')
    expect(config.url).toBe('/user/notification-email-locale/initialize')
    expect(JSON.parse(config.data)).toEqual({ locale: 'zh' })
  })
})
