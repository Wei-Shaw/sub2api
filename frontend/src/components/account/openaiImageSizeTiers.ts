export const OPENAI_IMAGE_SIZE_TIERS = ['1K', '2K', '4K'] as const

export type OpenAIImageSizeTier = (typeof OPENAI_IMAGE_SIZE_TIERS)[number]

export const OPENAI_IMAGE_SIZE_TIERS_EXTRA_KEY = 'openai_image_size_tiers'

// 返回账号允许的档位（按 1K/2K/4K 排序）；未配置、空列表或无有效档位视为三档全允许。
export function readOpenAIImageSizeTiers(extra: unknown): OpenAIImageSizeTier[] {
  const raw = (extra as Record<string, unknown> | null | undefined)?.[OPENAI_IMAGE_SIZE_TIERS_EXTRA_KEY]
  const values = Array.isArray(raw)
    ? raw.filter((item): item is string => typeof item === 'string').map(item => item.trim().toUpperCase())
    : []
  const tiers = OPENAI_IMAGE_SIZE_TIERS.filter(tier => values.includes(tier))
  return tiers.length > 0 ? tiers : [...OPENAI_IMAGE_SIZE_TIERS]
}

export function normalizeOpenAIImageSizeTiers(tiers: readonly string[]): OpenAIImageSizeTier[] {
  return OPENAI_IMAGE_SIZE_TIERS.filter(tier => tiers.includes(tier))
}

// 三档全选即不限：返回 null，表示 extra 中不写该键（批量编辑以空列表清除）。
export function openAIImageSizeTiersPayload(tiers: readonly string[]): OpenAIImageSizeTier[] | null {
  const normalized = normalizeOpenAIImageSizeTiers(tiers)
  if (normalized.length === 0 || normalized.length === OPENAI_IMAGE_SIZE_TIERS.length) {
    return null
  }
  return normalized
}

export function applyOpenAIImageSizeTiersToExtra(extra: Record<string, unknown>, tiers: readonly string[]): void {
  const payload = openAIImageSizeTiersPayload(tiers)
  if (payload) {
    extra[OPENAI_IMAGE_SIZE_TIERS_EXTRA_KEY] = payload
  } else {
    delete extra[OPENAI_IMAGE_SIZE_TIERS_EXTRA_KEY]
  }
}
