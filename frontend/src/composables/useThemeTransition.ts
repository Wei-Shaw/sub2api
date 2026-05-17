import { ref } from 'vue'

const THEME_STORAGE_KEY = 'theme'
const RIPPLE_DURATION_MS = 1200
const RIPPLE_EASING = 'cubic-bezier(0.19, 1, 0.22, 1)'
const RIPPLE_START_RADIUS_PX = 20
const RIPPLE_SOFT_RADIUS_PX = 180
const RIPPLE_RING_COUNT = 3

interface ViewTransitionLike {
  ready: Promise<void>
}

type DocumentWithViewTransition = Document & {
  startViewTransition?: (callback: () => void) => ViewTransitionLike
}

function prefersReducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

function getStoredOrSystemDark(): boolean {
  const savedTheme = localStorage.getItem(THEME_STORAGE_KEY)
  return (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  )
}

function persistTheme(nextIsDark: boolean): void {
  document.documentElement.classList.toggle('dark', nextIsDark)
  localStorage.setItem(THEME_STORAGE_KEY, nextIsDark ? 'dark' : 'light')
}

function getTransitionPoint(event?: MouseEvent): { x: number; y: number } {
  if (event && Number.isFinite(event.clientX) && Number.isFinite(event.clientY)) {
    return { x: event.clientX, y: event.clientY }
  }
  return { x: window.innerWidth / 2, y: window.innerHeight / 2 }
}

function getMaxRadius(x: number, y: number): number {
  return Math.hypot(
    Math.max(x, window.innerWidth - x),
    Math.max(y, window.innerHeight - y)
  )
}

function createRippleOverlay(x: number, y: number): void {
  const existingOverlay = document.querySelector('.theme-ripple-overlay')
  existingOverlay?.remove()

  const overlay = document.createElement('div')
  overlay.className = 'theme-ripple-overlay'
  overlay.style.setProperty('--theme-ripple-x', `${x}px`)
  overlay.style.setProperty('--theme-ripple-y', `${y}px`)

  for (let index = 0; index < RIPPLE_RING_COUNT; index += 1) {
    const ring = document.createElement('span')
    ring.className = 'theme-ripple-ring'
    ring.style.setProperty('--theme-ripple-delay', `${index * 120}ms`)
    ring.style.setProperty('--theme-ripple-alpha', `${0.28 - index * 0.06}`)
    overlay.appendChild(ring)
  }

  document.body.appendChild(overlay)

  window.setTimeout(() => {
    overlay.remove()
  }, RIPPLE_DURATION_MS)
}

export function useThemeTransition() {
  const isDark = ref(document.documentElement.classList.contains('dark'))

  function initTheme(): void {
    const nextIsDark = getStoredOrSystemDark()
    isDark.value = nextIsDark
    document.documentElement.classList.toggle('dark', nextIsDark)
  }

  async function toggleTheme(event?: MouseEvent): Promise<void> {
    const nextIsDark = !isDark.value
    const documentWithTransition = document as DocumentWithViewTransition
    const canUseViewTransition =
      typeof documentWithTransition.startViewTransition === 'function' &&
      !prefersReducedMotion()

    if (!canUseViewTransition) {
      isDark.value = nextIsDark
      persistTheme(nextIsDark)
      return
    }

    const { x, y } = getTransitionPoint(event)
    const maxRadius = getMaxRadius(x, y)
    const transition = documentWithTransition.startViewTransition?.(() => {
      isDark.value = nextIsDark
      persistTheme(nextIsDark)
    })

    await transition?.ready
    createRippleOverlay(x, y)

    document.documentElement.animate(
      [
        {
          clipPath: `circle(${RIPPLE_START_RADIUS_PX}px at ${x}px ${y}px)`,
          opacity: 0,
        },
        {
          clipPath: `circle(${RIPPLE_SOFT_RADIUS_PX}px at ${x}px ${y}px)`,
          opacity: 0.35,
        },
        {
          clipPath: `circle(${maxRadius * 0.92}px at ${x}px ${y}px)`,
          opacity: 0.82,
        },
        {
          clipPath: `circle(${maxRadius}px at ${x}px ${y}px)`,
          opacity: 1,
        },
      ],
      {
        duration: RIPPLE_DURATION_MS,
        easing: RIPPLE_EASING,
        pseudoElement: '::view-transition-new(root)',
      }
    )

    document.documentElement.animate(
      { opacity: [1, 0.94] },
      {
        duration: RIPPLE_DURATION_MS,
        easing: RIPPLE_EASING,
        pseudoElement: '::view-transition-old(root)',
      }
    )
  }

  return {
    isDark,
    initTheme,
    toggleTheme,
  }
}
