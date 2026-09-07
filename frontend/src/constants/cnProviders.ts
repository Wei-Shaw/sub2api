/**
 * Canonical names used by the frontend for Chinese API providers.
 *
 * `qwen` is the canonical platform value shared with the backend.
 */
export type CanonicalCnProviderPlatform = 'kimi' | 'zhipu' | 'deepseek' | 'qwen'

export function normalizeCnProviderPlatform(
  platform: string | null | undefined
): CanonicalCnProviderPlatform | null {
  switch (platform?.trim().toLowerCase()) {
    case 'kimi':
      return 'kimi'
    case 'zhipu':
      return 'zhipu'
    case 'deepseek':
      return 'deepseek'
    case 'qwen':
      return 'qwen'
    default:
      return null
  }
}

export function isCnProviderPlatform(platform: string | null | undefined): boolean {
  return normalizeCnProviderPlatform(platform) !== null
}

export function isQwenPlatform(platform: string | null | undefined): boolean {
  return normalizeCnProviderPlatform(platform) === 'qwen'
}

export function isCnCodingPlanPlatform(platform: string | null | undefined): boolean {
  const normalized = normalizeCnProviderPlatform(platform)
  return normalized === 'kimi' || normalized === 'zhipu'
}
