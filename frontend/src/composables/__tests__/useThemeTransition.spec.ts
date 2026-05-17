import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useThemeTransition } from '@/composables/useThemeTransition'

const storage = new Map<string, string>()
const matchMediaMock = vi.fn()

Object.defineProperty(window, 'localStorage', {
  configurable: true,
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
  },
})

Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  value: matchMediaMock,
})

describe('useThemeTransition', () => {
  beforeEach(() => {
    storage.clear()
    vi.clearAllMocks()
    vi.useFakeTimers()
    document.documentElement.classList.remove('dark')
    document.body.innerHTML = ''
    delete (document as Document & { startViewTransition?: unknown }).startViewTransition
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1000 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
    document.documentElement.animate = vi.fn()
    matchMediaMock.mockImplementation((query: string) => ({
      matches: query.includes('prefers-reduced-motion') ? false : false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('toggles the dark class and persists the chosen theme without view transitions', async () => {
    const { isDark, toggleTheme } = useThemeTransition()

    await toggleTheme()

    expect(isDark.value).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
    expect(document.documentElement.animate).not.toHaveBeenCalled()
  })

  it('uses a circular view transition from the click point when supported', async () => {
    const ready = Promise.resolve()
    const startViewTransition = vi.fn((callback: () => void) => {
      callback()
      return { ready }
    })
    ;(document as Document & { startViewTransition?: unknown }).startViewTransition = startViewTransition

    const { toggleTheme } = useThemeTransition()
    await toggleTheme(new MouseEvent('click', { clientX: 100, clientY: 200 }))
    await ready

    expect(startViewTransition).toHaveBeenCalledTimes(1)
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(document.documentElement.animate).toHaveBeenCalledWith(
      [
        { clipPath: 'circle(20px at 100px 200px)', opacity: 0 },
        { clipPath: 'circle(180px at 100px 200px)', opacity: 0.35 },
        { clipPath: expect.stringMatching(/^circle\(.+px at 100px 200px\)$/), opacity: 0.82 },
        { clipPath: expect.stringMatching(/^circle\(.+px at 100px 200px\)$/), opacity: 1 },
      ],
      expect.objectContaining({
        duration: 1200,
        easing: 'cubic-bezier(0.19, 1, 0.22, 1)',
        pseudoElement: '::view-transition-new(root)',
      })
    )
    expect(document.documentElement.animate).toHaveBeenCalledWith(
      { opacity: [1, 0.94] },
      expect.objectContaining({
        duration: 1200,
        easing: 'cubic-bezier(0.19, 1, 0.22, 1)',
        pseudoElement: '::view-transition-old(root)',
      })
    )
    const overlay = document.querySelector('.theme-ripple-overlay')
    expect(overlay).not.toBeNull()
    expect(overlay?.querySelectorAll('.theme-ripple-ring')).toHaveLength(3)
    expect((overlay as HTMLElement).style.getPropertyValue('--theme-ripple-x')).toBe('100px')
    expect((overlay as HTMLElement).style.getPropertyValue('--theme-ripple-y')).toBe('200px')

    vi.advanceTimersByTime(1200)

    expect(document.querySelector('.theme-ripple-overlay')).toBeNull()
  })

  it('skips the ripple when the user prefers reduced motion', async () => {
    matchMediaMock.mockImplementation((query: string) => ({
      matches: query.includes('prefers-reduced-motion'),
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }))
    ;(document as Document & { startViewTransition?: unknown }).startViewTransition = vi.fn()

    const { toggleTheme } = useThemeTransition()
    await toggleTheme(new MouseEvent('click', { clientX: 100, clientY: 200 }))

    expect((document as Document & { startViewTransition?: unknown }).startViewTransition).not.toHaveBeenCalled()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(document.documentElement.animate).not.toHaveBeenCalled()
  })
})
