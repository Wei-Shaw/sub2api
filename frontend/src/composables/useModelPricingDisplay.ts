import { computed, type Ref } from 'vue'
import type { ModelMarketplaceItem, ModelMarketplacePricePart } from '@/api/modelMarketplace'

export function formatModelPrice(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  if (value === 0) return '$0.0000'
  if (value < 0.0001) return `$${value.toExponential(2)}`
  if (value < 1) return `$${value.toFixed(6)}`
  return `$${value.toFixed(4)}`
}

export function formatRateMultiplier(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '1x'
  return `${value.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}x`
}

export function formatContextTokens(value: number | null | undefined): string {
  if (!value || value <= 0) return '—'
  if (value >= 1_000_000) {
    const millions = value / 1_000_000
    return `${Number.isInteger(millions) ? millions.toFixed(0) : millions.toFixed(1)}M`
  }
  if (value >= 1_000) return `${Math.round(value / 1_000)}K`
  return String(value)
}

export function pricePartVisible(part: ModelMarketplacePricePart | null | undefined): boolean {
  if (!part) return false
  return part.per_token > 0 || part.per_1m > 0 || part.display_usd > 0
}

export function useModelPricingSummary(items: Ref<ModelMarketplaceItem[]>) {
  const totalModels = computed(() => items.value.length)
  const tokenModels = computed(() => items.value.filter((item) => item.billing_mode === 'token').length)
  const imageModels = computed(() => items.value.filter((item) => item.billing_mode === 'image').length)
  const providerCount = computed(() => new Set(items.value.map((item) => item.provider)).size)

  return {
    totalModels,
    tokenModels,
    imageModels,
    providerCount,
  }
}
