import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'

export type PlazaFilter = {
  platform?: string
  groupId?: number
  rate?: number
  query?: string
}

export function effectiveRate(group: ModelPlazaGroup): number {
  return group.user_rate_multiplier ?? group.rate_multiplier
}

export function groupBillingLabel(group: ModelPlazaGroup): string {
  return group.subscription_type === 'subscription' ? '订阅 1:10' : '余额 1:1'
}

export function isImageModel(model: PlazaModel): boolean {
  return model.pricing?.billing_mode === 'image'
}

export function filterPlazaGroups(groups: ModelPlazaGroup[], filter: PlazaFilter): ModelPlazaGroup[] {
  const query = filter.query?.trim().toLowerCase()
  return groups.filter((group) => {
    if (filter.platform && filter.platform !== 'all' && group.platform !== filter.platform) return false
    if (filter.groupId !== undefined && group.id !== filter.groupId) return false
    if (filter.rate !== undefined && effectiveRate(group) !== filter.rate) return false
    if (!query) return true
    return group.models.some((model) =>
      [model.name, model.platform, group.name, group.description]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .includes(query)
    )
  }).map((group) => {
    if (!query) return group
    return {
      ...group,
      models: group.models.filter((model) =>
        [model.name, model.platform, group.name, group.description]
          .filter(Boolean)
          .join(' ')
          .toLowerCase()
          .includes(query)
      )
    }
  })
}

export function sortPlazaModels(models: PlazaModel[]): PlazaModel[] {
  return [...models].sort((a, b) => {
    const aPrice = a.official_pricing?.output_price ?? null
    const bPrice = b.official_pricing?.output_price ?? null
    if (aPrice !== null && bPrice !== null && aPrice !== bPrice) return bPrice - aPrice
    if (aPrice !== null && bPrice === null) return -1
    if (aPrice === null && bPrice !== null) return 1
    return a.name.localeCompare(b.name)
  })
}

export function formatCatalogPrice(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '—'
  const absolute = Math.abs(value)
  const digits = absolute >= 1 ? 2 : absolute >= 0.01 ? 4 : 6
  return value.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')
}

// Keep the public comparison consistent with the legacy catalog: channel prices
// are shown as the configured CNY-equivalent price, while official prices are
// shown in the provider's USD per-million-token unit.
export const legacyOfficialExchangeRate = 6.777525

export function formatChannelPricePerMillion(value: number | null | undefined, rate = 1): string {
  if (value == null || !Number.isFinite(value)) return '—'
  return `¥${formatCatalogPrice(value * 1_000_000 * rate)}`
}

export function formatOfficialPricePerMillion(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '—'
  return `$${formatCatalogPrice(value * 1_000_000)}`
}

export function calculateDiscountPercent(
  channelValue: number | null | undefined,
  officialValue: number | null | undefined,
  rate = 1
): number | null {
  if (channelValue == null || officialValue == null || !Number.isFinite(channelValue) || !Number.isFinite(officialValue) || officialValue <= 0) return null
  const channelPerMillion = channelValue * 1_000_000 * rate
  const officialPerMillion = officialValue * 1_000_000 * legacyOfficialExchangeRate
  return (1 - channelPerMillion / officialPerMillion) * 100
}
