export const DEFAULT_SITE_NAME = 'gptplusch'
export const LEGACY_SITE_NAME = 'Sub2API'
export const LEGACY_DEFAULT_LOGO = '/logo.png'

export function resolveSiteName(siteName?: string | null): string {
  const normalized = typeof siteName === 'string' ? siteName.trim() : ''
  if (!normalized || normalized === LEGACY_SITE_NAME) {
    return DEFAULT_SITE_NAME
  }
  return normalized
}

export function resolveSiteLogo(siteLogo?: string | null): string {
  const normalized = typeof siteLogo === 'string' ? siteLogo.trim() : ''
  if (!normalized || normalized === LEGACY_DEFAULT_LOGO) {
    return ''
  }
  return normalized
}
