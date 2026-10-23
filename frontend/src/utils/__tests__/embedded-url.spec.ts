import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { buildEmbeddedUrl, detectTheme } from '../embedded-url'

describe('embedded-url', () => {
  const originalLocation = window.location

  beforeEach(() => {
    Object.defineProperty(window, 'location', {
      value: {
        origin: 'https://app.example.com',
        href: 'https://app.example.com/user/purchase',
      },
      writable: true,
      configurable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
    document.documentElement.classList.remove('dark')
    vi.restoreAllMocks()
  })

  it('adds embedded query parameters including locale and source context', () => {
    const result = buildEmbeddedUrl(
      'https://pay.example.com/checkout?plan=pro',
      42,
      'token-123',
      'dark',
      'zh-CN',
      { passUserId: true, passAuthToken: true },
    )

    const url = new URL(result)
    expect(url.searchParams.get('plan')).toBe('pro')
    expect(url.searchParams.get('user_id')).toBe('42')
    expect(url.searchParams.get('token')).toBe('token-123')
    expect(url.searchParams.get('theme')).toBe('dark')
    expect(url.searchParams.get('lang')).toBe('zh-CN')
    expect(url.searchParams.get('ui_mode')).toBe('embedded')
    expect(url.searchParams.get('src_host')).toBe('https://app.example.com')
    expect(url.searchParams.get('src_url')).toBe('https://app.example.com/user/purchase')
  })

  it.each(['https://pay.example.com/checkout', 'https://app.example.com/docs'])(
    'does not forward authentication context by default to %s', (baseUrl) => {
      const url = new URL(buildEmbeddedUrl(baseUrl, 42, 'token-123', 'dark', 'en'))
      expect(url.searchParams.has('user_id')).toBe(false)
      expect(url.searchParams.has('token')).toBe(false)
      expect(url.searchParams.get('theme')).toBe('dark')
      expect(url.searchParams.get('lang')).toBe('en')
    },
  )

  it.each([
    { passUserId: false, passAuthToken: false },
    { passUserId: true, passAuthToken: false },
    { passUserId: false, passAuthToken: true },
    { passUserId: true, passAuthToken: true },
  ])('independently honors explicit authentication options %j', (options) => {
    const url = new URL(buildEmbeddedUrl(
      'https://pay.example.com/checkout?plan=pro#details', 42, 'token-123', 'light', 'en', options,
    ))
    expect(url.searchParams.get('user_id')).toBe(options.passUserId ? '42' : null)
    expect(url.searchParams.get('token')).toBe(options.passAuthToken ? 'token-123' : null)
    expect(url.searchParams.get('plan')).toBe('pro')
    expect(url.hash).toBe('#details')
  })

  it('does not leak authentication context indirectly through the source URL', () => {
    window.location.href = 'https://app.example.com/custom/docs?token=source-token&user_id=42#access_token=fragment-token'
    const url = new URL(buildEmbeddedUrl('https://example.com/docs', 42, 'token-123'))
    expect(url.searchParams.get('src_url')).toBe('https://app.example.com/custom/docs')
    expect(url.toString()).not.toContain('source-token')
    expect(url.toString()).not.toContain('fragment-token')
    expect(url.searchParams.has('user_id')).toBe(false)
    expect(url.searchParams.has('token')).toBe(false)
  })

  it('preserves destination-specific credentials when forwarding is disabled', () => {
    const url = new URL(buildEmbeddedUrl(
      'https://pay.example.com/checkout?token=destination-token&user_id=external-user', 42, 'token-123',
    ))
    expect(url.searchParams.get('token')).toBe('destination-token')
    expect(url.searchParams.get('user_id')).toBe('external-user')
  })

  it('does not append credentials to unsupported URL schemes', () => {
    const baseUrl = 'javascript:alert(1)'
    expect(buildEmbeddedUrl(baseUrl, 42, 'token-123', 'light', 'en', {
      passUserId: true, passAuthToken: true,
    })).toBe(baseUrl)
  })

  it('omits optional params when they are empty', () => {
    const result = buildEmbeddedUrl('https://pay.example.com/checkout', undefined, '', 'light', undefined, {
      passUserId: true, passAuthToken: true,
    })

    const url = new URL(result)
    expect(url.searchParams.get('theme')).toBe('light')
    expect(url.searchParams.get('ui_mode')).toBe('embedded')
    expect(url.searchParams.has('user_id')).toBe(false)
    expect(url.searchParams.has('token')).toBe(false)
    expect(url.searchParams.has('lang')).toBe(false)
  })

  it('returns original string for invalid url input', () => {
    expect(buildEmbeddedUrl('not a url', 1, 'token')).toBe('not a url')
  })

  it('detects dark mode from document root class', () => {
    document.documentElement.classList.add('dark')
    expect(detectTheme()).toBe('dark')
  })
})
