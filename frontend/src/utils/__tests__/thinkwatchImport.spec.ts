import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  buildThinkWatchImportLink,
  normalizeThinkWatchBaseUrl,
  openThinkWatchImportLink
} from '@/utils/thinkwatchImport'
import type { GroupPlatform } from '@/types'

function paramsFromLink(link: string): URLSearchParams {
  return new URLSearchParams(link.split('?')[1] || '')
}

describe('thinkwatchImport utils', () => {
  it.each([
    ['https://api.example.com', 'https://api.example.com'],
    ['https://api.example.com/', 'https://api.example.com'],
    ['https://api.example.com///', 'https://api.example.com'],
    ['https://api.example.com/v1', 'https://api.example.com'],
    ['https://api.example.com/v1/', 'https://api.example.com'],
    [' https://api.example.com/v1 ', 'https://api.example.com'],
    ['https://api.example.com/sub2api/v1', 'https://api.example.com/sub2api'],
    ['https://api.example.com/v1beta', 'https://api.example.com/v1beta']
  ])('normalizes base URL %s to %s', (baseUrl, expected) => {
    expect(normalizeThinkWatchBaseUrl(baseUrl)).toBe(expected)
  })

  it('builds a sub2api import link with the site root and the key', () => {
    expect(
      buildThinkWatchImportLink({
        baseUrl: 'https://api.example.com/v1/',
        platform: 'anthropic',
        apiKey: 'sk-test'
      })
    ).toBe('thinkwatch://import?service=sub2api&url=https%3A%2F%2Fapi.example.com&key=sk-test')
  })

  it.each(['anthropic', 'openai', 'gemini', 'grok', undefined, null] as Array<GroupPlatform | undefined | null>)(
    'uses the site root without a protocol for platform %s',
    (platform) => {
      const params = paramsFromLink(
        buildThinkWatchImportLink({ baseUrl: 'https://api.example.com', platform, apiKey: 'sk-test' })
      )

      expect([...params.keys()]).toEqual(['service', 'url', 'key'])
      expect(params.get('service')).toBe('sub2api')
      expect(params.get('url')).toBe('https://api.example.com')
      expect(params.has('name')).toBe(false)
    }
  )

  it.each([
    ['https://api.example.com', 'https://api.example.com/antigravity'],
    ['https://api.example.com/v1/', 'https://api.example.com/antigravity'],
    ['https://api.example.com/sub2api/', 'https://api.example.com/sub2api/antigravity']
  ])('points Antigravity keys at the Anthropic endpoint under %s', (baseUrl, url) => {
    const params = paramsFromLink(
      buildThinkWatchImportLink({ baseUrl, platform: 'antigravity', apiKey: 'sk-test' })
    )

    expect([...params.keys()]).toEqual(['service', 'url', 'protocol', 'key'])
    expect(params.get('service')).toBe('sub2api')
    expect(params.get('url')).toBe(url)
    expect(params.get('protocol')).toBe('anthropic')
    expect(params.get('key')).toBe('sk-test')
  })

  it('percent-encodes every value so a plus sign in the key is not read as a space', () => {
    const link = buildThinkWatchImportLink({
      baseUrl: 'https://api.example.com',
      apiKey: 'sk-a+b/c=d'
    })

    expect(link).toContain('&key=sk-a%2Bb%2Fc%3Dd')
    expect(paramsFromLink(link).get('key')).toBe('sk-a+b/c=d')
  })
})

describe('openThinkWatchImportLink', () => {
  const link = 'thinkwatch://import?service=sub2api&url=https%3A%2F%2Fapi.example.com&key=sk-test'

  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('opens the link in the current tab and reports when the page kept focus', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    const onNotOpened = vi.fn()

    openThinkWatchImportLink(link, onNotOpened, 1000)

    expect(open).toHaveBeenCalledWith(link, '_self')
    vi.advanceTimersByTime(999)
    expect(onNotOpened).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(onNotOpened).toHaveBeenCalledTimes(1)
  })

  it('stays quiet when the window lost focus to a browser prompt or the app', () => {
    vi.spyOn(window, 'open').mockReturnValue(null)
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    const onNotOpened = vi.fn()

    openThinkWatchImportLink(link, onNotOpened, 1000)
    window.dispatchEvent(new Event('blur'))
    vi.advanceTimersByTime(1000)

    expect(onNotOpened).not.toHaveBeenCalled()
  })

  it('stays quiet when the page no longer has focus', () => {
    vi.spyOn(window, 'open').mockReturnValue(null)
    vi.spyOn(document, 'hasFocus').mockReturnValue(false)
    const onNotOpened = vi.fn()

    openThinkWatchImportLink(link, onNotOpened, 1000)
    vi.advanceTimersByTime(1000)

    expect(onNotOpened).not.toHaveBeenCalled()
  })

  it('reports right away when the browser refuses to open the link', () => {
    vi.spyOn(window, 'open').mockImplementation(() => {
      throw new Error('blocked')
    })
    const onNotOpened = vi.fn()

    openThinkWatchImportLink(link, onNotOpened, 1000)

    expect(onNotOpened).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(1000)
    expect(onNotOpened).toHaveBeenCalledTimes(1)
  })
})
