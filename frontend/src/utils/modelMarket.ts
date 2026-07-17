import type {
  UserAvailableChannel,
  UserAvailableGroup,
  UserSupportedModel
} from '@/api/channels'

export type ModelMarketPricingFilter = 'all' | 'with' | 'without'

export interface ModelMarketChannelRef {
  name: string
  description: string
}

export interface ModelMarketItem {
  group: UserAvailableGroup
  platform: string
  channels: ModelMarketChannelRef[]
  models: ModelMarketModel[]
  channel_count: number
  model_count: number
  has_pricing: boolean
}

export interface ModelMarketModel extends UserSupportedModel {
  channels: ModelMarketChannelRef[]
  has_pricing: boolean
  pricing_conflict: boolean
}

export interface ModelMarketFilters {
  search: string
  platform: string
  channel: string
  pricing: ModelMarketPricingFilter
}

export interface ModelMarketFilterOptions {
  platforms: string[]
  channels: string[]
}

interface ModelAccumulator extends ModelMarketItem {
  modelPricingKeys: Map<string, string | null>
}

export function buildModelMarketItems(channels: UserAvailableChannel[]): ModelMarketItem[] {
  const byGroup = new Map<number, ModelAccumulator>()

  for (const channel of channels) {
    for (const section of channel.platforms) {
      for (const group of section.groups) {
        const item = getOrCreateGroupItem(byGroup, group, section.platform)
        addChannel(item, channel)
        addModels(item, section.supported_models, section.platform, channel)
      }
    }
  }

  return Array.from(byGroup.values())
    .map(finalizeGroupItem)
    .filter((item) => item.model_count > 0)
    .sort(compareModelMarketItems)
}

export function filterModelMarketItems(items: ModelMarketItem[], filters: ModelMarketFilters): ModelMarketItem[] {
  const search = filters.search.trim().toLowerCase()
  return items
    .map((item) => filterGroupItem(item, filters, search))
    .filter((item): item is ModelMarketItem => item !== null)
}

export function getModelMarketFilterOptions(items: ModelMarketItem[]): ModelMarketFilterOptions {
  return {
    platforms: sortStrings(unique(items.map((item) => item.platform).filter(Boolean))),
    channels: sortStrings(unique(items.flatMap((item) => item.channels.map((channel) => channel.name))))
  }
}

export function countModelMarketModels(items: ModelMarketItem[]): number {
  return items.reduce((count, item) => count + item.models.length, 0)
}

function getOrCreateGroupItem(
  byGroup: Map<number, ModelAccumulator>,
  group: UserAvailableGroup,
  platform: string
): ModelAccumulator {
  let item = byGroup.get(group.id)
  if (!item) {
    item = {
      group,
      platform: group.platform || platform,
      channels: [],
      models: [],
      channel_count: 0,
      model_count: 0,
      has_pricing: false,
      modelPricingKeys: new Map()
    }
    byGroup.set(group.id, item)
  }
  return item
}

function addChannel(item: ModelMarketItem, channel: UserAvailableChannel) {
  if (item.channels.some((existing) => existing.name === channel.name)) return
  item.channels.push({
    name: channel.name,
    description: channel.description || ''
  })
}

function addModels(
  item: ModelAccumulator,
  models: UserSupportedModel[],
  platform: string,
  channel: UserAvailableChannel
) {
  for (const model of models) {
    const normalizedModel = {
      ...model,
      platform: model.platform || platform,
      channels: [createChannelRef(channel)],
      pricing_conflict: false,
      has_pricing: model.pricing != null
    }
    const key = modelKey(normalizedModel.platform, normalizedModel.name)
    const existingIndex = item.models.findIndex((existing) => modelKey(existing.platform, existing.name) === key)
    if (existingIndex === -1) {
      item.models.push(normalizedModel)
      item.modelPricingKeys.set(key, pricingKey(normalizedModel))
    } else {
      const existing = item.models[existingIndex]
      addModelChannel(existing, channel)
      const previousPricingKey = item.modelPricingKeys.get(key) ?? null
      const nextPricingKey = pricingKey(normalizedModel)
      if (previousPricingKey !== nextPricingKey) {
        item.models[existingIndex] = {
          ...existing,
          pricing: null,
          has_pricing: existing.has_pricing || normalizedModel.has_pricing,
          pricing_conflict: true
        }
        item.modelPricingKeys.set(key, null)
      } else if (existing.pricing == null && normalizedModel.pricing != null) {
        item.models[existingIndex] = {
          ...existing,
          ...normalizedModel,
          channels: existing.channels,
          has_pricing: true
        }
      }
    }
    if (normalizedModel.has_pricing) {
      item.has_pricing = true
    }
  }
}

function finalizeGroupItem(item: ModelAccumulator): ModelMarketItem {
  return {
    group: item.group,
    platform: item.platform,
    channels: sortChannels(item.channels),
    models: sortModels(item.models.map((model) => ({ ...model, channels: sortChannels(model.channels) }))),
    channel_count: item.channels.length,
    model_count: item.models.length,
    has_pricing: item.has_pricing
  }
}

function filterGroupItem(
  item: ModelMarketItem,
  filters: ModelMarketFilters,
  search: string
): ModelMarketItem | null {
  if (filters.platform && item.platform !== filters.platform) return null

  const models = item.models.filter((model) => modelMatches(model, item, filters, search))
  if (models.length === 0) return null
  const channels = channelsForModels(item.channels, models, filters.channel)

  return {
    ...item,
    channels,
    models,
    channel_count: channels.length,
    model_count: models.length,
    has_pricing: models.some((model) => model.has_pricing)
  }
}

function modelMatches(
  model: ModelMarketModel,
  item: ModelMarketItem,
  filters: ModelMarketFilters,
  search: string
): boolean {
  if (filters.channel && !model.channels.some((channel) => channel.name === filters.channel)) return false
  if (filters.pricing === 'with' && !model.has_pricing) return false
  if (filters.pricing === 'without' && model.has_pricing) return false
  if (!search) return true

  return (
    item.group.name.toLowerCase().includes(search) ||
    item.platform.toLowerCase().includes(search) ||
    model.name.toLowerCase().includes(search) ||
    model.platform.toLowerCase().includes(search) ||
    model.channels.some((channel) =>
      channel.name.toLowerCase().includes(search) ||
      channel.description.toLowerCase().includes(search)
    )
  )
}

function channelsForModels(
  channels: ModelMarketChannelRef[],
  models: ModelMarketModel[],
  channelFilter: string
): ModelMarketChannelRef[] {
  const modelChannelNames = new Set(models.flatMap((model) => model.channels.map((channel) => channel.name)))
  return channels.filter((channel) =>
    (!channelFilter || channel.name === channelFilter) &&
    modelChannelNames.has(channel.name)
  )
}

function sortChannels(channels: ModelMarketChannelRef[]): ModelMarketChannelRef[] {
  return [...channels].sort((a, b) => compareStrings(a.name, b.name))
}

function createChannelRef(channel: UserAvailableChannel): ModelMarketChannelRef {
  return {
    name: channel.name,
    description: channel.description || ''
  }
}

function addModelChannel(model: ModelMarketModel, channel: UserAvailableChannel) {
  if (model.channels.some((existing) => existing.name === channel.name)) return
  model.channels.push(createChannelRef(channel))
}

function sortModels(models: ModelMarketModel[]): ModelMarketModel[] {
  return [...models].sort((a, b) => {
    const platform = compareStrings(a.platform, b.platform)
    if (platform !== 0) return platform
    return compareStrings(a.name, b.name)
  })
}

function compareModelMarketItems(a: ModelMarketItem, b: ModelMarketItem): number {
  const platform = compareStrings(a.platform, b.platform)
  if (platform !== 0) return platform
  if (a.group.is_exclusive !== b.group.is_exclusive) return a.group.is_exclusive ? -1 : 1
  return compareStrings(a.group.name, b.group.name)
}

function modelKey(platform: string, model: string): string {
  return `${platform.trim().toLowerCase()}::${model.trim().toLowerCase()}`
}

function pricingKey(model: UserSupportedModel): string | null {
  return model.pricing == null ? null : JSON.stringify(model.pricing)
}

function unique(values: string[]): string[] {
  return Array.from(new Set(values))
}

function sortStrings(values: string[]): string[] {
  return [...values].sort(compareStrings)
}

function compareStrings(a: string, b: string): number {
  return a.localeCompare(b)
}
