export function buildStoreUrl(baseUrl: string, email?: string): string {
  const url = new URL(baseUrl)
  const value = email?.trim()

  if (!value) {
    return url.toString()
  }

  const hash = new URLSearchParams(url.hash.slice(1))
  hash.set('email', value)
  url.hash = hash.toString()
  return url.toString()
}
