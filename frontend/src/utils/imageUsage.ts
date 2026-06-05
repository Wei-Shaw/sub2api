import type { UsageLog } from '@/types'

type Translate = (key: string) => string

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
  }
  if (knownImageBillingSizes.has(size)) {
    return size
  }
  return `${t('usage.imageSizeLegacyUnstandardized')}: ${size}`
}

export const formatImageInputSize = (row: ImageUsageRow | null | undefined, t: Translate): string => {
  const size = trimmed(row?.image_input_size)
  return size || t('usage.imageSizeUnknown')
}

export const formatImageOutputSize = (row: ImageUsageRow | null | undefined, t: Translate): string => {
  const size = trimmed(row?.image_output_size)
  return size || t('usage.imageSizeUnknown')
}

export const formatImageSizeSource = (row: ImageUsageRow | null | undefined, t: Translate): string => {
  const source = trimmed(row?.image_size_source).toLowerCase()
  if (knownImageSizeSources.has(source)) {
    return t(`usage.imageSizeSource${source.charAt(0).toUpperCase()}${source.slice(1)}`)
  }
  if (trimmed(row?.image_size)) {
    return t('usage.imageSizeSourceLegacy')
  }
  return t('usage.imageSizeSourceMissing')
}

export const formatImageSizeBreakdown = (row: ImageUsageRow | null | undefined): string => {
  const breakdown = row?.image_size_breakdown
  if (!breakdown || Object.keys(breakdown).length === 0) {
    return ''
  }
  return ['1K', '2K', '4K']
    .filter((tier) => (breakdown[tier] ?? 0) > 0)
    .map((tier) => `${tier} x ${breakdown[tier]}`)
    .join(', ')
}
