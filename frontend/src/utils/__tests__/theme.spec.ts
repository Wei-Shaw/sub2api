import { beforeEach, describe, expect, it } from 'vitest'
import { disableLegacyThemePreference } from '@/utils/theme'

const storage = new Map<string, string>()

Object.defineProperty(window, 'localStorage', {
  configurable: true,
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
  },
})

describe('disableLegacyThemePreference', () => {
  beforeEach(() => {
    storage.clear()
    document.documentElement.classList.remove('dark')
  })

  it('clears the old saved dark preference and keeps the document in light mode', () => {
    localStorage.setItem('theme', 'dark')
    document.documentElement.classList.add('dark')

    disableLegacyThemePreference()

    expect(localStorage.getItem('theme')).toBeNull()
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })
})
