import type { PublicSettings } from '@/types'

export const PUBLIC_HOME_PATH = '/home'
export const INTERNAL_HOME_PATH = '/internal-home'
export const INTERNAL_MARKETING_ALLOWED_PATHS = [INTERNAL_HOME_PATH, '/docs', '/agents']

function normalizeHost(raw: string): string {
  let value = raw.trim().toLowerCase()
  if (!value) return ''

  const wildcard = value.startsWith('*.')
  if (wildcard) {
    value = value.slice(2)
  }

  let host = ''
  try {
    const url = value.includes('://')
      ? new URL(value)
      : new URL(`https://${value}`)
    host = url.hostname
  } catch {
    host = value.split('/')[0]?.split(':')[0] ?? ''
  }

  host = host.trim().toLowerCase().replace(/\.$/, '')
  if (!host || host.includes('*')) return ''
  return wildcard ? `*.${host}` : host
}

export function homeDomainMatchesHost(pattern: string, currentHost: string): boolean {
  const normalizedPattern = normalizeHost(pattern)
  const host = normalizeHost(currentHost)
  if (!normalizedPattern || !host) return false

  if (normalizedPattern.startsWith('*.')) {
    const suffix = normalizedPattern.slice(2)
    return host.endsWith(`.${suffix}`)
  }

  return host === normalizedPattern
}

export function isInternalHomeHost(
  settings: Pick<PublicSettings, 'internal_home_domains'> | null | undefined,
  currentHost = typeof window !== 'undefined' ? window.location.hostname : '',
): boolean {
  const domains = settings?.internal_home_domains
  if (!Array.isArray(domains) || domains.length === 0) {
    return false
  }
  return domains.some((domain) => homeDomainMatchesHost(domain, currentHost))
}

export function resolveHomePathForHost(
  settings: Pick<PublicSettings, 'internal_home_domains'> | null | undefined,
  currentHost = typeof window !== 'undefined' ? window.location.hostname : '',
): string {
  return isInternalHomeHost(settings, currentHost) ? INTERNAL_HOME_PATH : PUBLIC_HOME_PATH
}

export function isInternalMarketingPathAllowed(path: string): boolean {
  const normalizedPath = path.split(/[?#]/)[0] || '/'
  if (normalizedPath === '/') return true

  return INTERNAL_MARKETING_ALLOWED_PATHS.some((allowedPath) => (
    normalizedPath === allowedPath || normalizedPath.startsWith(`${allowedPath}/`)
  ))
}
