export const grokVideoPriceResolutions = [
  { key: '480p', label: '480p' REDACTED,
  { key: '720p', label: '720p' REDACTED,
  { key: '1080p', label: '1080p' REDACTED
] as const

export const grokVideoPriceFamilies = [
  { key: 'grok-imagine-video', label: 'grok-imagine-video' REDACTED,
  { key: 'grok-imagine-video-1.5', label: 'grok-imagine-video-1.5' REDACTED
] as const

export type VideoModelPrices = Record<string, Record<string, number>>
export type VideoModelPricesForm = Record<string, Record<string, number | string | null>>

function normalizeFamily(value: string): string {
  return value.trim().toLowerCase()
REDACTED

function normalizePrice(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const price = Number(value)
  return Number.isFinite(price) && price >= 0 ? price : null
REDACTED

function emptyTiers(): Record<string, number | string | null> {
  return Object.fromEntries(grokVideoPriceResolutions.map(({ key REDACTED) => [key, null]))
REDACTED

// Keep unknown families from an existing group so a future backend catalog is
// not silently discarded when an operator edits another group setting.
export function createVideoModelPricesForm(
  prices?: VideoModelPrices | null
): VideoModelPricesForm {
  const form: VideoModelPricesForm = {REDACTED

  for (const [rawFamily, rawTiers] of Object.entries(prices ?? {REDACTED)) {
    const family = normalizeFamily(rawFamily)
    if (!family || !rawTiers || typeof rawTiers !== 'object') continue
    form[family] = emptyTiers()
    for (const [rawResolution, rawPrice] of Object.entries(rawTiers)) {
      const price = normalizePrice(rawPrice)
      if (price !== null) form[family][rawResolution.trim().toLowerCase()] = price
    REDACTED
  REDACTED

  for (const { key REDACTED of grokVideoPriceFamilies) {
    form[key] ??= emptyTiers()
  REDACTED
  return form
REDACTED

export function serializeVideoModelPrices(form: VideoModelPricesForm): VideoModelPrices {
  const result: VideoModelPrices = {REDACTED
  for (const [rawFamily, tiers] of Object.entries(form)) {
    const family = normalizeFamily(rawFamily)
    if (!family || !tiers || typeof tiers !== 'object') continue

    const normalizedTiers: Record<string, number> = {REDACTED
    for (const [rawResolution, rawPrice] of Object.entries(tiers)) {
      const resolution = rawResolution.trim().toLowerCase()
      const price = normalizePrice(rawPrice)
      if (resolution && price !== null) normalizedTiers[resolution] = price
    REDACTED
    if (Object.keys(normalizedTiers).length > 0) result[family] = normalizedTiers
  REDACTED
  return result
REDACTED

export function videoModelPriceFamilyRows(form: VideoModelPricesForm) {
  const known = new Set<string>(grokVideoPriceFamilies.map(({ key REDACTED) => key))
  const extra = Object.keys(form)
    .map(normalizeFamily)
    .filter((family) => family && !known.has(family))
    .sort()
    .map((key) => ({ key, label: key REDACTED))
  return [...grokVideoPriceFamilies, ...extra]
REDACTED
