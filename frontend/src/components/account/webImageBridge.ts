import type { Account } from '@/types'

export function isFreeWebImageAccount(account: Account | null | undefined): boolean {
  return account?.platform === 'openai' && account.type === 'oauth' &&
    account.parent_account_id == null &&
    String(account.credentials?.plan_type ?? '').trim().toLowerCase() === 'free'
}

export function hasWebImageConfiguration(extra?: Record<string, unknown>): boolean {
  return typeof extra?.chatgpt_web_image_enabled === 'boolean'
}

export function readWebImageSettings(extra?: Record<string, unknown>) {
  return {
    enabled: extra?.chatgpt_web_image_enabled === true,
    regularChat: extra?.chatgpt_web_image_regular_chat === true,
    markdownPreview: extra?.chatgpt_web_image_markdown_preview !== false
  }
}

// Prefer exact mappings, then the longest trailing-star match, matching
// the account resolver for ordinary tests and custom image aliases.
function resolveConfiguredModel(mapping: unknown, publicID: string): string | null {
  if (!mapping || typeof mapping !== 'object' || Array.isArray(mapping)) return null
  const entries = Object.entries(mapping).filter((entry): entry is [string, string] => typeof entry[1] === 'string')
  const exact = entries.find(([name]) => name === publicID)
  const found = exact ?? entries
    .filter(([name]) => name.endsWith('*') && publicID.startsWith(name.slice(0, -1)))
    .sort(([left], [right]) => right.length - left.length)[0]
  return found ? found[1].trim() : null
}

// Free accounts use every gpt-image-* name as a web compatibility entry,
// regardless of legacy mappings. Other accounts still test the mapped target.
export function isOpenAIImageTestModel(mapping: unknown, publicID: string, freeWebImage = false): boolean {
  const isImageName = (name: string) => name.trim().toLowerCase().startsWith('gpt-image-')
  if (freeWebImage && isImageName(publicID)) return true
  return isImageName(resolveConfiguredModel(mapping, publicID) ?? publicID)
}
