/** Product naming rules for the Bestloong delivery. */
export const BESTLOONG_UI_NAME = 'Bestloongai'
export const BESTLOONG_TUTORIAL_NAME = 'Bestloong'
export const BESTLOONG_API_NAME = 'Bestloong API'
export const BESTLOONG_BASE_URL = 'https://token.bestloongai.com/v1'

export function resolveDefaultBrandLogo(): string {
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
    ? '/bestloongai-logo-dark.svg'
    : '/bestloongai-logo-light.svg'
}

/**
 * Prevent the legacy default name from leaking into the branded UI while
 * preserving an explicitly configured custom site name.
 */
export function resolveUiSiteName(value?: string | null): string {
  const name = value?.trim()
  return !name || name === 'Sub2API' ? BESTLOONG_UI_NAME : name
}
