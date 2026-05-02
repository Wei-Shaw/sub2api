import { describe, expect, it } from 'vitest'
import { detectMobileDevice } from '../device'

describe('detectMobileDevice', () => {
  it('prefers userAgentData.mobile when available', () => {
    expect(detectMobileDevice({
      navigator: {
        userAgent: 'Mozilla/5.0',
        userAgentData: { mobile: true },
      },
    })).toBe(true)
  })

  it('recognizes handheld browsers from the mobile UA token', () => {
    expect(detectMobileDevice({
      navigator: {
        userAgent: 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/136.0 Mobile Safari/537.36',
        maxTouchPoints: 5,
      },
    })).toBe(true)
  })

  it('recognizes iPadOS desktop mode via touch capability', () => {
    expect(detectMobileDevice({
      navigator: {
        userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15',
        platform: 'MacIntel',
        maxTouchPoints: 5,
      },
    })).toBe(true)
  })

  it('falls back to input capability detection for touch-first devices', () => {
    expect(detectMobileDevice({
      navigator: {
        userAgent: 'Mozilla/5.0',
        maxTouchPoints: 10,
      },
      matchMedia: (query) => ({
        matches: query === '(pointer: coarse)' || query === '(hover: none)',
      }),
    })).toBe(true)
  })

  it('keeps desktop environments as non-mobile', () => {
    expect(detectMobileDevice({
      navigator: {
        userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/136.0 Safari/537.36',
        platform: 'MacIntel',
        maxTouchPoints: 0,
      },
      matchMedia: () => ({ matches: false }),
    })).toBe(false)
  })
})
