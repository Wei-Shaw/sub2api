import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * The theme owner is a module singleton with side effects at import time (a
 * MutationObserver and a matchMedia listener), so each case re-imports it
 * through `vi.resetModules()` rather than sharing one instance.
 */
async function loadTheme() {
  vi.resetModules()
  return await import('../useTheme')
}

function setPrefersDark(matches: boolean) {
  const listeners: Array<(e: MediaQueryListEvent) => void> = []
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches,
    media: query,
    addEventListener: (_: string, cb: (e: MediaQueryListEvent) => void) => listeners.push(cb),
    removeEventListener: () => {},
    addListener: (cb: (e: MediaQueryListEvent) => void) => listeners.push(cb),
    removeListener: () => {},
    dispatchEvent: () => false,
    onchange: null,
  }))
  return listeners
}

beforeEach(() => {
  localStorage.clear()
  document.documentElement.classList.remove('dark')
  setPrefersDark(false)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('useTheme storage contract', () => {
  /*
   * This contract is shared with `main.ts:25-31`, which applies the class
   * BEFORE mount to avoid a light-mode flash. If the stored representation ever
   * drifts from what main.ts reads, dark-mode users get a white flash on every
   * page load — a bug that is invisible in dev with a warm cache.
   */
  it('stores an explicit choice as "dark" / "light"', async () => {
    const { setTheme } = await loadTheme()

    setTheme('dark')
    expect(localStorage.getItem('theme')).toBe('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)

    setTheme('light')
    expect(localStorage.getItem('theme')).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('represents "follow the OS" as the ABSENCE of the key', async () => {
    const { setTheme } = await loadTheme()
    setTheme('dark')
    setTheme('system')
    // main.ts treats a missing key as "consult prefers-color-scheme". Writing
    // the literal string "system" here would make it fall through to light.
    expect(localStorage.getItem('theme')).toBeNull()
  })

  it('resolves "system" against the OS preference', async () => {
    setPrefersDark(true)
    const { setTheme, useTheme } = await loadTheme()
    setTheme('system')
    expect(useTheme().isDark.value).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('reads an existing preference at load', async () => {
    localStorage.setItem('theme', 'dark')
    const { useTheme } = await loadTheme()
    expect(useTheme().mode.value).toBe('dark')
  })
})

describe('useTheme reactivity', () => {
  it('toggles between explicit light and dark, never back to system', async () => {
    const { toggleTheme, useTheme } = await loadTheme()
    const { isDark, mode } = useTheme()

    toggleTheme()
    expect(isDark.value).toBe(true)
    expect(mode.value).toBe('dark')

    toggleTheme()
    expect(isDark.value).toBe(false)
    expect(mode.value).toBe('light')
  })

  it('picks up an out-of-band class flip via the MutationObserver', async () => {
    /*
     * This is the property that let the chart bridge land before AppSidebar was
     * migrated: the legacy toggles still write the class on <html> directly, and
     * `isDark` has to notice. Without it, charts would keep the stale palette
     * until every toggle had been rewritten.
     */
    const { useTheme } = await loadTheme()
    const { isDark } = useTheme()
    expect(isDark.value).toBe(false)

    document.documentElement.classList.add('dark')
    await new Promise((r) => setTimeout(r, 0))

    expect(isDark.value).toBe(true)
  })
})
