export const grokVideoPriceResolutions = [
  { key: '480p', label: '480p' },
  { key: '720p', label: '720p' },
  { key: '1080p', label: '1080p' }
] as const

export const grokVideoPriceFamilies = [
  { key: 'grok-imagine-video', label: 'grok-imagine-video' },
  { key: 'grok-imagine-video-1.5', label: 'grok-imagine-video-1.5' }
] as const

export type VideoModelPrices = Record<string, Record<string, number>>
export type VideoModelPricesForm = Record<string, Record<string, number | string | null>>

function normalizeFamily(value: string): string {
  return value.trim().toLowerCase()
}

function normalizePrice(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const price = Number(value)
  return Number.isFinite(price) && price >= 0 ? price : null
}

function emptyTiers(): Record<string, number | string | null> {
  return Object.fromEntries(grokVideoPriceResolutions.map(({ key }) => [key, null]))
}

// Keep unknown families from an existing group so a future backend catalog is
// not silently discarded when an operator edits another group setting.
export function createVideoModelPricesForm(
  prices?: VideoModelPrices | null,
  platform = 'grok',
): VideoModelPricesForm {
  const form: VideoModelPricesForm = {}

  for (const [rawFamily, rawTiers] of Object.entries(prices ?? {})) {
    const family = normalizeFamily(rawFamily)
    if (!family || !rawTiers || typeof rawTiers !== 'object') continue
    form[family] = emptyTiers()
    for (const [rawResolution, rawPrice] of Object.entries(rawTiers)) {
      const price = normalizePrice(rawPrice)
      if (price !== null) form[family][rawResolution.trim().toLowerCase()] = price
    }
  }

  if (platform !== 'seedance') {
    for (const { key } of grokVideoPriceFamilies) {
      form[key] ??= emptyTiers()
    }
  }
  return form
}

export function serializeVideoModelPrices(
  form: VideoModelPricesForm,
  platform = 'grok',
): VideoModelPrices {
  const result: VideoModelPrices = {}
  for (const [rawFamily, tiers] of Object.entries(form)) {
    const family = normalizeFamily(rawFamily)
    if (!family || !tiers || typeof tiers !== 'object') continue
    // The Grok rows are a shared legacy form default. Never submit those
    // hidden rows when a Seedance group is being saved.
    if (platform === 'seedance' && grokVideoPriceFamilies.some(({ key }) => key === family)) {
      continue
    }

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

export function videoModelPriceFamilyRows(form: VideoModelPricesForm, platform = 'grok') {
  const known = new Set<string>(grokVideoPriceFamilies.map(({ key }) => key))
  const extra = Object.keys(form)
    .map(normalizeFamily)
    .filter((family) => family && !known.has(family))
    .sort()
    .map((key) => ({ key, label: key }))
  return platform === 'seedance' ? extra : [...grokVideoPriceFamilies, ...extra]
}

export function addVideoModelPriceFamily(form: VideoModelPricesForm): string {
  let index = 1
  while (form[`seedance-model-${index}`]) index += 1
  const key = `seedance-model-${index}`
  form[key] = emptyTiers()
  return key
}

export function renameVideoModelPriceFamily(
  form: VideoModelPricesForm,
  oldKey: string,
  nextKey: string,
): boolean {
  const normalized = normalizeFamily(nextKey)
  if (!normalized || normalized === oldKey || (form[normalized] && normalized !== oldKey)) return false
  const tiers = form[oldKey]
  if (!tiers) return false
  delete form[oldKey]
  form[normalized] = tiers
  return true
}

export function removeVideoModelPriceFamily(form: VideoModelPricesForm, key: string): void {
  delete form[key]
}
