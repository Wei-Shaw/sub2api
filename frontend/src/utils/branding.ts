import { buildApiUrl } from '@/api/url'
import { sanitizeUrl } from '@/utils/url'

// Hashed logos are served by the API. A root-relative path would be requested
// from the page origin, which 404s when VITE_API_BASE_URL points elsewhere.
const SITE_BRANDING_ASSET_PREFIX = '/api/v1/settings/public/logo-'

export function resolveSiteLogo(logoUrl: string): string {
  const sanitized = sanitizeUrl(logoUrl, {
    allowRelative: true,
    allowDataUrl: true,
  })
  if (!sanitized.startsWith(SITE_BRANDING_ASSET_PREFIX)) {
    return sanitized
  }
  return buildApiUrl(sanitized)
}

export function updateFavicon(logoUrl: string): void {
  const sanitizedLogoUrl = resolveSiteLogo(logoUrl)
  if (!sanitizedLogoUrl) {
    return
  }

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  link.type = faviconType(sanitizedLogoUrl)
  link.href = sanitizedLogoUrl
}

function faviconType(url: string): string {
  const dataType = /^data:(image\/[a-z0-9.+-]+)/i.exec(url)?.[1]?.toLowerCase()
  if (dataType === 'image/jpg') {
    return 'image/jpeg'
  }
  if (dataType) {
    return dataType
  }
  const path = url.split(/[?#]/, 1)[0].toLowerCase()
  if (path.endsWith('.svg')) return 'image/svg+xml'
  if (path.endsWith('.png')) return 'image/png'
  if (path.endsWith('.jpg') || path.endsWith('.jpeg')) return 'image/jpeg'
  if (path.endsWith('.gif')) return 'image/gif'
  if (path.endsWith('.webp')) return 'image/webp'
  return 'image/x-icon'
}
