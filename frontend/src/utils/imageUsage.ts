import type { UsageLog REDACTED from '@/types'

type Translate = (key: string) => string

// --- Image output token / cost helpers ---

type ImageOutputTokenRow = Pick<UsageLog, 'output_tokens' | 'image_output_tokens'>
type ImageOutputCostRow = Pick<UsageLog, 'image_output_cost'>

/** Whether the row contains any image-output tokens. */
export const hasImageOutputTokens = (row: ImageOutputTokenRow | null | undefined): boolean =>
  (row?.image_output_tokens ?? 0) > 0

/**
 * Text-only output tokens (total output minus image-output).
 * Returns 0 when no text tokens exist.
 */
export const textOutputTokens = (row: ImageOutputTokenRow | null | undefined): number =>
  Math.max(0, (row?.output_tokens ?? 0) - (row?.image_output_tokens ?? 0))

/** Whether the row has a non-zero image-output cost. */
export const hasImageOutputCost = (row: ImageOutputCostRow | null | undefined): boolean =>
  (row?.image_output_cost ?? 0) > 0

// --- Image size / billing helpers ---

const knownImageSizeSources = new Set(['output', 'input', 'default', 'legacy'])
const knownImageBillingSizes = new Set(['1K', '2K', '4K', 'mixed'])

type ImageUsageRow = Pick<
  UsageLog,
  'image_size' | 'image_input_size' | 'image_output_size' | 'image_size_source' | 'image_size_breakdown'
>

const trimmed = (value: string | null | undefined): string => value?.trim() ?? ''

export const formatImageBillingSize = (row: ImageUsageRow | null | undefined, t: Translate): string => {
  const size = trimmed(row?.image_size)
  if (!size) {
    return t('usage.imageSizeNotRecorded')
  REDACTED
  if (knownImageBillingSizes.has(size)) {
    return size
  REDACTED
  return `${t('usage.imageSizeLegacyUnstandardized')REDACTED: ${sizeREDACTED`
REDACTED

export const formatImageInputSize = (row: ImageUsageRow | null | undefined, t: Translate): string => {
  const size = trimmed(row?.image_input_size)
  return size || t('usage.imageSizeUnknown')
REDACTED

export const formatImageOutputSize = (row: ImageUsageRow | null | undefined, t: Translate): string => {
  const size = trimmed(row?.image_output_size)
  return size || t('usage.imageSizeUnknown')
REDACTED

export const formatImageSizeSource = (row: ImageUsageRow | null | undefined, t: Translate): string => {
  const source = trimmed(row?.image_size_source).toLowerCase()
  if (knownImageSizeSources.has(source)) {
    return t(`usage.imageSizeSource${source.charAt(0).toUpperCase()REDACTED${source.slice(1)REDACTED`)
  REDACTED
  if (trimmed(row?.image_size)) {
    return t('usage.imageSizeSourceLegacy')
  REDACTED
  return t('usage.imageSizeSourceMissing')
REDACTED

export const formatImageSizeBreakdown = (row: ImageUsageRow | null | undefined): string => {
  const breakdown = row?.image_size_breakdown
  if (!breakdown || Object.keys(breakdown).length === 0) {
    return ''
  REDACTED
  return ['1K', '2K', '4K']
    .filter((tier) => (breakdown[tier] ?? 0) > 0)
    .map((tier) => `${tierREDACTED x ${breakdown[tier]REDACTED`)
    .join(', ')
REDACTED
