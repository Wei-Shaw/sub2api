import type { GroupPlatform } from '@/types'

/**
 * Magpie import links, per https://usemagpie.ai/docs/import.
 *
 * The web form keeps every parameter in the URL fragment, so the API key never
 * reaches usemagpie.ai: the page turns the fragment into magpie://import?… on the
 * user's machine, and offers the download when Magpie is not installed yet.
 */
export const MAGPIE_IMPORT_URL = 'https://usemagpie.ai/import'

export interface MagpieImportEndpoints {
  /** Anthropic Messages base URL: the root, without /v1. */
  anthropic?: string
  /** OpenAI Chat Completions base URL, ending in /v1. */
  chat?: string
  /** OpenAI Responses base URL, ending in /v1. */
  responses?: string
  /** models.dev provider id Magpie uses for display names, context sizes and reasoning levels. */
  catalog?: string
}

export interface MagpieImportLinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  providerName: string
  apiKey: string
  /** The site's homepage. Magpie keeps it only when it is https. */
  website?: string
  /** The page where users mint keys. Magpie keeps it only when it is https. */
  keysUrl?: string
}

const MAGPIE_CATALOG_BY_PLATFORM: Partial<Record<GroupPlatform, string>> = {
  anthropic: 'anthropic',
  openai: 'openai',
  gemini: 'google',
  grok: 'xai',
  kimi: 'moonshotai',
  zhipu: 'zhipuai',
  deepseek: 'deepseek',
  minimax: 'minimax',
  opencode_go: 'opencode-go'
}

/** Mirrors Magpie's Slug(): lowercase, runs of anything but [a-z0-9] become "-", dashes trimmed. */
export function magpieSlug(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function normalizeRootUrl(baseUrl: string): string {
  return baseUrl.trim().replace(/\/+$/, '').replace(/\/v1$/, '')
}

export function resolveMagpieImportEndpoints(
  platform: GroupPlatform | undefined | null,
  baseUrl: string
): MagpieImportEndpoints {
  const root = normalizeRootUrl(baseUrl)
  const resolvedPlatform: GroupPlatform = platform || 'anthropic'

  switch (resolvedPlatform) {
    case 'antigravity':
      // The dedicated /antigravity routes only serve the Anthropic Messages API.
      return { anthropic: `${root}/antigravity` }
    case 'gemini':
      // /v1/messages and /v1/chat/completions bridge to Gemini accounts; /v1/responses does not.
      return {
        anthropic: root,
        chat: `${root}/v1`,
        catalog: MAGPIE_CATALOG_BY_PLATFORM.gemini
      }
    default:
      // Every other platform auto-routes all three APIs on the gateway.
      return {
        anthropic: root,
        chat: `${root}/v1`,
        responses: `${root}/v1`,
        catalog: MAGPIE_CATALOG_BY_PLATFORM[resolvedPlatform]
      }
  }
}

/**
 * Magpie derives the provider id from the name and rejects a link whose name
 * has no ASCII letters or digits (a Chinese-only site name, for example).
 * Return an explicit id from the API host in that case; otherwise let Magpie
 * derive it so the id matches what a hand-typed name would get.
 */
export function resolveMagpieProviderId(providerName: string, baseUrl: string): string | undefined {
  if (magpieSlug(providerName)) {
    return undefined
  }

  let host = ''
  try {
    host = new URL(baseUrl).hostname
  } catch {
    host = ''
  }
  return magpieSlug(host) || 'sub2api'
}

export function buildMagpieImportLink(input: MagpieImportLinkInput): string {
  const endpoints = resolveMagpieImportEndpoints(input.platform, input.baseUrl)
  const params = new URLSearchParams()

  params.set('name', input.providerName)
  const id = resolveMagpieProviderId(input.providerName, input.baseUrl)
  if (id) {
    params.set('id', id)
  }
  if (endpoints.chat) {
    params.set('chat', endpoints.chat)
  }
  if (endpoints.responses) {
    params.set('responses', endpoints.responses)
  }
  if (endpoints.anthropic) {
    params.set('anthropic', endpoints.anthropic)
  }
  params.set('key', input.apiKey)
  if (endpoints.catalog) {
    params.set('catalog', endpoints.catalog)
  }
  if (input.website) {
    params.set('website', input.website)
  }
  if (input.keysUrl) {
    params.set('keys', input.keysUrl)
  }

  return `${MAGPIE_IMPORT_URL}#${params.toString()}`
}
