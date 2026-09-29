import { beforeEach, describe, expect, it, vi } from 'vitest'

import { resolveSiteLogo, updateFavicon } from '@/utils/branding'

const hashedLogo = '/api/v1/settings/public/logo-' + 'a'.repeat(64) + '.png'

describe('resolveSiteLogo', () => {
  it('keeps a same-origin hashed logo on the API path', () => {
    expect(resolveSiteLogo(hashedLogo)).toBe(hashedLogo)
  })

  it('points a hashed logo at the configured API origin', async () => {
    vi.resetModules()
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api/v1')
    const mod = await import('@/utils/branding')

    expect(mod.resolveSiteLogo(hashedLogo)).toBe(`https://api.example.com${hashedLogo}`)
    vi.unstubAllEnvs()
  })

  it('leaves external, static, data and empty logos unchanged', () => {
    expect(resolveSiteLogo('https://cdn.example.com/logo.png')).toBe('https://cdn.example.com/logo.png')
    expect(resolveSiteLogo('/logo.svg')).toBe('/logo.svg')
    expect(resolveSiteLogo('data:image/png;base64,abc')).toBe('data:image/png;base64,abc')
    expect(resolveSiteLogo('')).toBe('')
  })

  it('rejects unsafe logo URLs', () => {
    expect(resolveSiteLogo('javascript:alert(1)')).toBe('')
    expect(resolveSiteLogo('//evil.example/logo.png')).toBe('')
  })
})

describe('updateFavicon', () => {
  beforeEach(() => {
    document.head.innerHTML = '<link rel="icon" href="/logo.svg">'
  })

  it('replaces the default favicon with the configured logo', () => {
    updateFavicon('https://example.com/custom-logo.png')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.href).toBe('https://example.com/custom-logo.png')
    expect(link?.type).toBe('image/png')
  })

  it('uses the API origin for a hashed logo', async () => {
    vi.resetModules()
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api/v1')
    const mod = await import('@/utils/branding')

    mod.updateFavicon(hashedLogo)

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.href).toBe(`https://api.example.com${hashedLogo}`)
    expect(link?.type).toBe('image/png')
    vi.unstubAllEnvs()
  })

  it('ignores unsafe logo URLs', () => {
    updateFavicon('javascript:alert(1)')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('href')).toBe('/logo.svg')
  })
})
