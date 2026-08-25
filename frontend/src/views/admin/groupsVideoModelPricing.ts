export const videoPriceResolutions = [
  { key: '480p', label: '480p' },
  { key: '720p', label: '720p' },
  { key: '1080p', label: '1080p' }
] as const

export type VideoModelPrices = Record<string, Record<string, number>>
export type VideoModelPricesForm = Record<string, Record<string, number | string | null>>

export const temporaryVideoModelKeyPrefix = '__new_video_model__'

export function isTemporaryVideoModelKey(key: string): boolean {
  return key.startsWith(temporaryVideoModelKeyPrefix)
}

function normalizeFamily(value: string): string {
  return value.trim().toLowerCase()
}

function normalizePrice(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const price = Number(value)
  return Number.isFinite(price) && price >= 0 ? price : null
}

export function createEmptyVideoPriceTiers(): Record<string, number | string | null> {
  return Object.fromEntries(videoPriceResolutions.map(({ key }) => [key, null]))
}

// Keep unknown families from an existing group so a future backend catalog is
// not silently discarded when an operator edits another group setting.
export function createVideoModelPricesForm(
  prices?: VideoModelPrices | null
): VideoModelPricesForm {
  const form: VideoModelPricesForm = {}

  for (const [rawFamily, rawTiers] of Object.entries(prices ?? {})) {
    const family = normalizeFamily(rawFamily)
    if (!family || !rawTiers || typeof rawTiers !== 'object') continue
    form[family] = createEmptyVideoPriceTiers()
    for (const [rawResolution, rawPrice] of Object.entries(rawTiers)) {
      const price = normalizePrice(rawPrice)
      if (price !== null) form[family][rawResolution.trim().toLowerCase()] = price
    }
  }
  return form
}

export function serializeVideoModelPrices(form: VideoModelPricesForm): VideoModelPrices {
  const result: VideoModelPrices = {}
  for (const [rawFamily, tiers] of Object.entries(form)) {
    const family = normalizeFamily(rawFamily)
    if (!family || isTemporaryVideoModelKey(family) || !tiers || typeof tiers !== 'object') continue

    const normalizedTiers: Record<string, number> = {}
    for (const [rawResolution, rawPrice] of Object.entries(tiers)) {
      const resolution = rawResolution.trim().toLowerCase()
      const price = normalizePrice(rawPrice)
      if (resolution && price !== null) normalizedTiers[resolution] = price
    }
    if (Object.keys(normalizedTiers).length > 0) result[family] = normalizedTiers
  }
  return result
}

export function videoModelPriceFamilyRows(form: VideoModelPricesForm) {
  return Object.keys(form)
    .map(normalizeFamily)
    .filter(Boolean)
    .sort()
    .map((key) => ({ key, label: isTemporaryVideoModelKey(key) ? '' : key }))
}
