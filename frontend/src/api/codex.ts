export interface CodexModelsManifestResult {
  content: string
  modelCount: number
}

export type CodexModelsManifestSelection =
  | { mode: 'auto' }
  | { mode: 'compatible'; clientVersion: string }

function normalizeGatewayBaseUrl(baseUrl: string): string {
  const fallback = typeof window !== 'undefined' ? window.location.origin : ''
  const value = (baseUrl || fallback).trim().replace(/\/+$/, '')
  if (!value) return ''
  return value.replace(/\/(?:v1|backend-api\/codex(?:\/models)?)$/i, '')
}

function normalizeCodexBaseUrl(baseUrl: string): string {
  const value = normalizeGatewayBaseUrl(baseUrl)
  if (!value) return '/v1'
  return /\/v1$/i.test(value) ? value : `${value}/v1`
}

function buildAutomaticCodexModelsManifestUrl(baseUrl: string): string {
  const value = normalizeGatewayBaseUrl(baseUrl)
  if (!value) return '/backend-api/codex/models'
  return `${value}/backend-api/codex/models`
}

const codexClientVersionPattern = /^[0-9]+(\.[0-9]+){1,3}(-[0-9A-Za-z.]+)?$/

export function buildCodexModelsManifestUrl(
  baseUrl: string,
  selection: CodexModelsManifestSelection = { mode: 'auto' }
): string {
  if (selection.mode === 'auto') return buildAutomaticCodexModelsManifestUrl(baseUrl)
  const clientVersion = selection.clientVersion.trim()
  if (!codexClientVersionPattern.test(clientVersion) || clientVersion.length > 64) {
    throw new Error('Codex compatibility version is invalid')
  }
  const url = normalizeCodexBaseUrl(baseUrl)
  const params = new URLSearchParams({ client_version: clientVersion })
  return `${url}/models?${params.toString()}`
}

function isCodexModelsManifest(value: unknown): value is { models: unknown[] } {
  return typeof value === 'object' && value !== null && Array.isArray((value as { models?: unknown }).models)
}

export async function fetchCodexModelsManifest(
  baseUrl: string,
  apiKey: string,
  options: { selection?: CodexModelsManifestSelection; signal?: AbortSignal } = {}
): Promise<CodexModelsManifestResult> {
  const response = await fetch(buildCodexModelsManifestUrl(baseUrl, options.selection), {
    method: 'GET',
    headers: {
      Accept: 'application/json',
      Authorization: `Bearer ${apiKey}`
    },
    cache: 'no-store',
    signal: options.signal
  })

  if (!response.ok) {
    throw new Error(`Codex models request failed with status ${response.status}`)
  }

  const payload: unknown = await response.json()
  if (!isCodexModelsManifest(payload)) {
    throw new Error('Codex models response is not a valid manifest')
  }

  return {
    content: JSON.stringify(payload, null, 2),
    modelCount: payload.models.length
  }
}
