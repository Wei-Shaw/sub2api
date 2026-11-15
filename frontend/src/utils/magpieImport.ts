import type { GroupPlatform } from '@/types'

export interface MagpieImportLinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  providerName: string
  apiKey: string
}

export interface MagpieImportEndpoints {
  chat?: string
  responses?: string
  anthropic?: string
  gemini?: string
}

function normalizeBaseUrl(baseUrl: string): string {
  return baseUrl.trim().replace(/\/+$/, '').replace(/\/v1(?:beta)?$/i, '')
}

export function resolveMagpieImportEndpoints(
  platform: GroupPlatform | undefined | null,
  baseUrl: string
): MagpieImportEndpoints {
  const root = normalizeBaseUrl(baseUrl)

  switch (platform || 'anthropic') {
    case 'openai':
      return {
        chat: `${root}/v1`,
        responses: `${root}/v1`
      }
    case 'gemini':
      return { gemini: `${root}/v1beta` }
    case 'antigravity':
      return {
        anthropic: `${root}/antigravity`,
        gemini: `${root}/antigravity/v1beta`
      }
    default:
      return { anthropic: root }
  }
}

export function buildMagpieImportLink(input: MagpieImportLinkInput): string {
  const endpoints = resolveMagpieImportEndpoints(input.platform, input.baseUrl)
  const params = new URLSearchParams({
    name: input.providerName.trim() || 'Sub2API',
    key: input.apiKey
  })

  for (const [name, value] of Object.entries(endpoints)) {
    if (value) {
      params.set(name, value)
    }
  }

  return `https://usemagpie.ai/import#${params.toString()}`
}
