const LEGACY_THEME_STORAGE_KEY = 'theme'

export function disableLegacyThemePreference(): void {
  document.documentElement.classList.remove('dark')
  localStorage.removeItem(LEGACY_THEME_STORAGE_KEY)
}
