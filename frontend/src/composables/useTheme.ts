import { readonly, ref } from 'vue'

/**
 * The single reactive owner of light/dark.
 *
 * WHY THIS EXISTS
 *
 * Before this module there was no reactive theme state anywhere. Three views
 * each kept a private `isDark` ref and wrote `localStorage` themselves —
 * `AppSidebar.vue:251/842`, `HomeView.vue:505/527`, `KeyUsageView.vue:441/446` —
 * so toggling the theme in one of them left the other two stale.
 *
 * Worse, ten components derived the theme like this:
 *
 *     const isDark = computed(() => document.documentElement.classList.contains('dark'))
 *
 * A `computed` with no reactive dependencies caches forever. Every chart in
 * the app therefore picked up the theme once, at mount, and never re-themed
 * on toggle. That is the bug `chartTheme.ts` is built on top of this to fix.
 *
 * THE MUTATION OBSERVER IS NOT DEFENSIVE PROGRAMMING
 *
 * It is what lets this land without migrating anything else first. The three
 * legacy toggles still flip the class on `<html>` directly; the observer sees
 * that and updates `isDark`, so charts start re-theming correctly before
 * `AppSidebar` (1,084 lines, the highest-risk file in the tree) is touched.
 * Once every toggle routes through `setTheme`, the observer becomes redundant
 * — but it costs nothing, so keep it as the backstop for anything that flips
 * the class out of band.
 *
 * Storage contract is unchanged and must stay that way: `localStorage.theme`
 * is `'dark' | 'light'`, and its ABSENCE means "follow the OS". `main.ts:25-31`
 * applies the class from this key before mount, which is what prevents a
 * light-mode flash on first paint.
 */

export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'theme'
const DARK_CLASS = 'dark'

const hasDom = typeof document !== 'undefined' && typeof window !== 'undefined'

function prefersDark(): boolean {
  if (!hasDom || typeof window.matchMedia !== 'function') return false
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function readStoredMode(): ThemeMode {
  if (!hasDom) return 'system'
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    return saved === 'dark' || saved === 'light' ? saved : 'system'
  } catch {
    // Private-mode Safari and some embedded webviews throw on localStorage.
    return 'system'
  }
}

function classIsDark(): boolean {
  return hasDom && document.documentElement.classList.contains(DARK_CLASS)
}

const isDark = ref(classIsDark())
const mode = ref<ThemeMode>(readStoredMode())

/** Resolve a mode to a concrete boolean, consulting the OS for `'system'`. */
function resolve(next: ThemeMode): boolean {
  return next === 'system' ? prefersDark() : next === 'dark'
}

/**
 * Set the theme and persist it. `'system'` clears the stored preference so the
 * app follows the OS from then on, matching what `main.ts` expects to find.
 */
export function setTheme(next: ThemeMode): void {
  mode.value = next
  const dark = resolve(next)
  isDark.value = dark

  if (!hasDom) return
  document.documentElement.classList.toggle(DARK_CLASS, dark)

  try {
    if (next === 'system') localStorage.removeItem(STORAGE_KEY)
    else localStorage.setItem(STORAGE_KEY, next)
  } catch {
    // Ignore — the class is already applied, only persistence is lost.
  }
}

/** Flip between explicit light and dark. Never lands back on `'system'`. */
export function toggleTheme(): void {
  setTheme(isDark.value ? 'light' : 'dark')
}

if (hasDom) {
  // Backstop for out-of-band class flips (see the note above).
  new MutationObserver(() => {
    const dark = classIsDark()
    if (dark !== isDark.value) isDark.value = dark
  }).observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })

  // Follow the OS, but only while the user has expressed no preference.
  if (typeof window.matchMedia === 'function') {
    const query = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (event: MediaQueryListEvent) => {
      if (mode.value !== 'system') return
      isDark.value = event.matches
      document.documentElement.classList.toggle(DARK_CLASS, event.matches)
    }
    // Safari < 14 has no addEventListener on MediaQueryList.
    if (typeof query.addEventListener === 'function') query.addEventListener('change', onChange)
    else if (typeof query.addListener === 'function') query.addListener(onChange)
  }
}

export function useTheme() {
  return {
    isDark: readonly(isDark),
    mode: readonly(mode),
    setTheme,
    toggleTheme,
  }
}
