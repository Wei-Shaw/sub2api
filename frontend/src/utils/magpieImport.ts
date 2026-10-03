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
  siteName: string
  /** The API key's id on this site: it makes the provider id, so each key is its own provider. */
  keyId: number
  /** The API key's name, shown after the site name. */
  keyName: string
  apiKey: string
  /** The site's homepage. Magpie keeps it only when it is https. */
  website?: string
  /** The page where users mint keys. Magpie keeps it only when it is https. */
  keysUrl?: string
}

/** Magpie cuts a longer name at this many bytes, which can split a multi-byte character. */
export const MAGPIE_NAME_MAX_BYTES = 80

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

function truncateUtf8(value: string, maxBytes: number): string {
  const encoder = new TextEncoder()
  let out = ''
  let used = 0
  for (const char of value) {
    const size = encoder.encode(char).length
    if (used + size > maxBytes) {
      break
    }
    out += char
    used += size
  }
  return out
}

/**
 * Magpie offers to replace the provider whose id an import link repeats, so
 * the id carries the key's id: another key of the site becomes a provider of
 * its own, and importing the same key again updates its provider. The prefix
 * is the site name's slug, or the API host's when the name has no ASCII
 * letters or digits.
 */
export function resolveMagpieProviderId(siteName: string, baseUrl: string, keyId: number): string {
  let prefix = magpieSlug(siteName)
  if (!prefix) {
    try {
      prefix = magpieSlug(new URL(baseUrl).hostname)
    } catch {
      prefix = ''
    }
  }
  return `${prefix || 'sub2api'}-${keyId}`
}

/** "Site-key name", within the bytes Magpie keeps of a name. */
export function resolveMagpieProviderName(siteName: string, keyName: string): string {
  const site = siteName.trim()
  const key = keyName.trim()
  return truncateUtf8(key ? `${site}-${key}` : site, MAGPIE_NAME_MAX_BYTES)
}

export function buildMagpieImportLink(input: MagpieImportLinkInput): string {
  const endpoints = resolveMagpieImportEndpoints(input.platform, input.baseUrl)
  const params = new URLSearchParams()

  params.set('name', resolveMagpieProviderName(input.siteName, input.keyName))
  params.set('id', resolveMagpieProviderId(input.siteName, input.baseUrl, input.keyId))
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
