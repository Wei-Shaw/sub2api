export interface CodexModelsManifestResult {
  content: string
  modelCount: number
  responseBytes: number
}

function normalizeCodexBaseUrl(baseUrl: string): string {
  const fallback = typeof window !== 'undefined' ? window.location.origin : ''
  const value = (baseUrl || fallback).trim().replace(/\/+$/, '')
  if (!value) return '/v1'
  return /\/v1$/i.test(value) ? value : `${value}/v1`
}

export function buildCodexModelsManifestUrl(
  baseUrl: string,
  clientVersion?: string
): string {
  // Downloads use the gateway's automatically maintained identity. Only real
  // clients that need version-specific negotiation should supply a version.
  const params = new URLSearchParams(clientVersion
    ? { client_version: clientVersion }
    : { format: 'codex' })
  return `${buildCodexModelCatalogUrl(baseUrl)}?${params.toString()}`
}

export function buildCodexModelCatalogUrl(baseUrl: string): string {
  return `${normalizeCodexBaseUrl(baseUrl)}/models`
}

function isCodexModelsManifest(value: unknown): value is { models: unknown[] } {
  return typeof value === 'object' && value !== null && Array.isArray((value as { models?: unknown }).models)
}

export async function fetchCodexModelsManifest(
  baseUrl: string,
  apiKey: string,
  signal?: AbortSignal
): Promise<CodexModelsManifestResult> {
  const response = await fetch(buildCodexModelsManifestUrl(baseUrl), {
    method: 'GET',
    headers: {
      Accept: 'application/json',
      Authorization: `Bearer ${apiKey}`
    },
    cache: 'no-store',
    signal
  })

  if (!response.ok) {
    throw new Error(`Codex models request failed with status ${response.status}`)
  }

  const text = await response.text()
  const payload: unknown = JSON.parse(text)
  if (!isCodexModelsManifest(payload)) {
    throw new Error('Codex models response is not a valid manifest')
  }

  return {
    content: JSON.stringify(payload, null, 2),
    modelCount: payload.models.length,
    responseBytes: new TextEncoder().encode(text).byteLength
  }
}
