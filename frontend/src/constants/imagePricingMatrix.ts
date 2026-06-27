/**
 * \u56fe\u7247\u8ba1\u8d39\u77e9\u9635\u4ee3\u53f7\u4e0e\u9ed8\u8ba4\u4ef7\u8868\u3002
 *
 * - 6 \u4e2a\u5206\u8fa8\u7387\u6863\u4f4d\uff08\u4e0e\u540e\u7aef classifyImagePricingTier6 \u4e00\u81f4\uff09
 * - 3 \u4e2a quality \u6863\u4f4d\uff08\u4e0e\u540e\u7aef NormalizeImageQuality \u4e00\u81f4\uff1aauto/\"\" \u2192 high\uff09
 * - DEFAULT_IMAGE_PRICING_MATRIX \u662f OpenAI gpt-image \u516c\u793a\u4ef7\uff08\u5355\u4f4d\uff1a\u7f8e\u5143/\u5f20\uff09\uff0c
 *   \u7528\u4e8e\u201c\u586b\u5165\u5b98\u65b9\u9ed8\u8ba4\u8868\u201d\u4e00\u952e\u586b\u5145
 */

export const IMAGE_PRICING_TIER_KEYS = [
  '1024x768',
  '1024x1024',
  '1024x1536',
  '1920x1080',
  '2560x1440',
  '3840x2160',
] as const

export type ImagePricingTierKey = (typeof IMAGE_PRICING_TIER_KEYS)[number]

export const IMAGE_PRICING_QUALITY_KEYS = ['low', 'medium', 'high'] as const

export type ImagePricingQualityKey = (typeof IMAGE_PRICING_QUALITY_KEYS)[number]

export type ImagePricingMatrix = Record<string, Record<string, number>>

/**
 * OpenAI gpt-image \u516c\u793a\u4ef7\u8868\uff08\u5355\u4f4d\uff1a\u7f8e\u5143/\u5f20\uff09\u3002
 * \u4f9b\u5206\u7ec4\u8868\u5355\u201c\u586b\u5165\u5b98\u65b9\u9ed8\u8ba4\u8868\u201d\u6309\u94ae\u4f7f\u7528\u3002
 */
export const DEFAULT_IMAGE_PRICING_MATRIX: ImagePricingMatrix = {
  '1024x768': { low: 0.005, medium: 0.037, high: 0.145 },
  '1024x1024': { low: 0.006, medium: 0.053, high: 0.211 },
  '1024x1536': { low: 0.005, medium: 0.042, high: 0.165 },
  '1920x1080': { low: 0.005, medium: 0.040, high: 0.158 },
  '2560x1440': { low: 0.007, medium: 0.056, high: 0.222 },
  '3840x2160': { low: 0.012, medium: 0.101, high: 0.401 },
}

/**
 * \u6df1\u62f7\u9ed8\u8ba4\u77e9\u9635\uff0c\u907f\u514d\u8868\u5355\u8986\u76d6\u5e38\u91cf\u3002
 */
export function cloneDefaultImagePricingMatrix(): ImagePricingMatrix {
  const out: ImagePricingMatrix = {}
  for (const tier of IMAGE_PRICING_TIER_KEYS) {
    out[tier] = { ...DEFAULT_IMAGE_PRICING_MATRIX[tier] }
  }
  return out
}

/**
 * \u521b\u5efa\u4e00\u4e2a\u201c\u7a7a\u201d\u77e9\u9635\u9aa8\u67b6\uff086 \u884c \u00d7 3 \u5217\uff0c\u521d\u59cb\u503c null\uff09\uff0c
 * \u4f9b\u8868\u5355\u8868\u683c\u4f7f\u7528\uff1b\u5e8f\u5217\u5316\u65f6\u9700\u4ee5 toMatrixDTO \u8fc7\u6ee4\u6389 null\u3002
 */
export type EditableImagePricingMatrix = Record<string, Record<string, number | null>>

export function createEmptyImagePricingMatrix(): EditableImagePricingMatrix {
  const out: EditableImagePricingMatrix = {}
  for (const tier of IMAGE_PRICING_TIER_KEYS) {
    out[tier] = { low: null, medium: null, high: null }
  }
  return out
}

/**
 * \u4ece\u540e\u7aef\u8fd4\u56de\u7684\u77e9\u9635\u52a0\u8f7d\u5230\u8868\u5355\u53ef\u7f16\u8f91\u7ed3\u6784\u3002
 * \u672a\u586b\u7684\u683c\u4f4d\u8bbe\u4e3a null \u4ee5\u4f7f\u8f93\u5165\u6846\u4e3a\u7a7a\u3002
 */
export function loadEditableImagePricingMatrix(
  source: ImagePricingMatrix | null | undefined,
): EditableImagePricingMatrix {
  const out = createEmptyImagePricingMatrix()
  if (!source || typeof source !== 'object') {
    return out
  }
  for (const tier of IMAGE_PRICING_TIER_KEYS) {
    const row = source[tier]
    if (!row || typeof row !== 'object') {
      continue
    }
    for (const quality of IMAGE_PRICING_QUALITY_KEYS) {
      const v = row[quality]
      if (typeof v === 'number' && Number.isFinite(v) && v >= 0) {
        out[tier][quality] = v
      }
    }
  }
  return out
}

/**
 * \u8868\u5355 \u2192 API DTO\uff1a\u8fc7\u6ee4 null/\u8d1f\u6570/NaN\uff1b\u5168\u90e8\u4e3a\u7a7a\u65f6\u8fd4\u56de null
 * \uff08\u8868\u793a\u672a\u914d\u7f6e\u77e9\u9635\uff0c\u540e\u7aef\u4f1a\u56de\u9000\u5230 image_price_1k/2k/4k\uff09\u3002
 */
export function toMatrixDTO(
  editable: EditableImagePricingMatrix,
): ImagePricingMatrix | null {
  const out: ImagePricingMatrix = {}
  let any = false
  for (const tier of IMAGE_PRICING_TIER_KEYS) {
    const src = editable[tier] || {}
    const row: Record<string, number> = {}
    for (const quality of IMAGE_PRICING_QUALITY_KEYS) {
      const v = src[quality]
      if (v === null || v === undefined || Number.isNaN(v)) {
        continue
      }
      const n = Number(v)
      if (!Number.isFinite(n) || n < 0) {
        continue
      }
      row[quality] = n
      any = true
    }
    if (Object.keys(row).length > 0) {
      out[tier] = row
    }
  }
  return any ? out : null
}

/**
 * \u68c0\u67e5\u8868\u5355\u77e9\u9635\u662f\u5426\u6709\u975e\u6cd5\u8f93\u5165\uff08\u8d1f\u6570\u6216 NaN\uff09\u3002
 * \u8fd4\u56de\u9519\u8bef\u63cf\u8ff0\u6570\u7ec4\uff1b\u7a7a\u6570\u7ec4\u4ee3\u8868\u5168\u90e8\u5408\u6cd5\u3002
 */
export function validateEditableMatrix(
  editable: EditableImagePricingMatrix,
): string[] {
  const errors: string[] = []
  for (const tier of IMAGE_PRICING_TIER_KEYS) {
    const row = editable[tier] || {}
    for (const quality of IMAGE_PRICING_QUALITY_KEYS) {
      const v = row[quality]
      if (v === null || v === undefined) {
        continue
      }
      if (Number.isNaN(v) || !Number.isFinite(v) || (v as number) < 0) {
        errors.push(`${tier}/${quality}`)
      }
    }
  }
  return errors
}
