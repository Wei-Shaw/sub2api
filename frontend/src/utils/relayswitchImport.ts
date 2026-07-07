export interface RelaySwitchProviderImportInput {
  name: string
  baseUrl: string
  apiKey: string
}

function toBase64UrlUtf8(value: unknown): string {
  const bytes = new TextEncoder().encode(JSON.stringify(value))
  let binary = ''

  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }

  return btoa(binary)
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/g, '')
}

export function buildRelaySwitchProviderImportDeeplink(
  input: RelaySwitchProviderImportInput
): string {
  const payload = toBase64UrlUtf8({
    name: input.name,
    baseUrl: input.baseUrl,
    apiKey: input.apiKey
  })

  return `relay-switch://v1/import?resource=provider&payload=${payload}`
}
