import { describe, expect, it } from 'vitest'
import {
  composeMonitorEndpoint,
  isMonitorGroupCompatible,
  splitMonitorEndpoint,
} from '@/utils/channelMonitorRoute'

describe('channelMonitorRoute', () => {
  it('allows Antigravity groups for Gemini and Anthropic monitoring only', () => {
    expect(isMonitorGroupCompatible('gemini', 'gemini')).toBe(true)
    expect(isMonitorGroupCompatible('gemini', 'antigravity')).toBe(true)
    expect(isMonitorGroupCompatible('anthropic', 'anthropic')).toBe(true)
    expect(isMonitorGroupCompatible('anthropic', 'antigravity')).toBe(true)
    expect(isMonitorGroupCompatible('openai', 'antigravity')).toBe(false)
    expect(isMonitorGroupCompatible('grok', 'antigravity')).toBe(false)
  })

  it('composes the fixed Antigravity route without duplicate slashes', () => {
    expect(composeMonitorEndpoint('https://example.com/', true)).toBe(
      'https://example.com/antigravity',
    )
    expect(composeMonitorEndpoint('https://example.com/', false)).toBe('https://example.com')
    expect(composeMonitorEndpoint('https://example.com/antigravity/', true)).toBe(
      'https://example.com/antigravity',
    )
  })

  it('restores the route state while editing an existing monitor', () => {
    expect(splitMonitorEndpoint('https://example.com/antigravity/')).toEqual({
      origin: 'https://example.com',
      useAntigravityRoute: true,
    })
    expect(splitMonitorEndpoint('https://example.com')).toEqual({
      origin: 'https://example.com',
      useAntigravityRoute: false,
    })
  })
})
