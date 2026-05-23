export type GeoFetch = typeof fetch

interface GeoProvider {
  url: string
  countryCodeKeys: string[]
}

const GEO_PROVIDERS: GeoProvider[] = [
  { url: 'https://ipapi.co/json/', countryCodeKeys: ['country_code', 'country'] },
  { url: 'https://ipwho.is/?fields=success,country_code,country_code2', countryCodeKeys: ['country_code', 'country_code2'] },
]

export function isMainlandChinaCountryCode(countryCode: string | null | undefined): boolean {
  return (countryCode || '').trim().toUpperCase() === 'CN'
}

function readCountryCode(payload: unknown, keys: string[]): string {
  if (!payload || typeof payload !== 'object') return ''
  const record = payload as Record<string, unknown>
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }
  return ''
}

async function fetchWithTimeout(fetcher: GeoFetch, url: string, timeoutMs: number): Promise<Response> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), timeoutMs)
  try {
    return await fetcher(url, {
      cache: 'no-store',
      signal: controller.signal,
    })
  } finally {
    window.clearTimeout(timer)
  }
}

export async function detectMainlandChinaAccess(
  fetcher: GeoFetch = fetch,
  timeoutMs = 3500,
): Promise<boolean> {
  for (const provider of GEO_PROVIDERS) {
    try {
      const response = await fetchWithTimeout(fetcher, provider.url, timeoutMs)
      if (!response.ok) continue

      const countryCode = readCountryCode(await response.json(), provider.countryCodeKeys)
      if (!countryCode) continue

      return isMainlandChinaCountryCode(countryCode)
    } catch {
      continue
    }
  }

  return false
}
