<template>
  <AppLayout>
    <div class="space-y-6">
      <section>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('models.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('models.description') }}</p>
      </section>

      <section class="space-y-4">
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div class="flex flex-col gap-3 sm:flex-row">
            <div class="relative w-full sm:w-80">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('models.searchPlaceholder')"
                class="input pl-10"
              />
            </div>
            <label class="sr-only" for="model-platform-filter">{{ t('models.platformFilter') }}</label>
            <div
              class="relative w-full after:pointer-events-none after:absolute after:right-3 after:top-1/2 after:-mt-[2px] after:border-x-[5px] after:border-t-[6px] after:border-x-transparent after:border-t-gray-500 dark:after:border-t-gray-300 sm:w-48"
              data-testid="platform-filter-wrap"
            >
              <select
                id="model-platform-filter"
                v-model="selectedPlatform"
                data-testid="platform-filter"
                class="input w-full appearance-none pr-9"
              >
                <option value="">{{ t('models.allPlatforms') }}</option>
                <option v-for="platform in platformOptions" :key="platform" :value="platform">
                  {{ platformLabel(platform) }}
                </option>
              </select>
            </div>
          </div>

          <div class="flex items-center gap-3" data-testid="model-plaza-actions">
            <button
              @click="loadChannels"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
              data-testid="models-refresh"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <div class="inline-flex rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900" :aria-label="t('models.currency')">
              <button
                v-for="currency in currencies"
                :key="currency"
                type="button"
                :data-testid="`currency-${currency.toLowerCase()}`"
                class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                :class="selectedCurrency === currency ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
                @click="selectedCurrency = currency"
              >
                {{ currency }}
              </button>
            </div>
          </div>
        </div>

        <div v-if="loading" class="card py-12 text-center">
          <Icon name="refresh" size="lg" class="inline-block animate-spin text-gray-400" />
        </div>
        <div v-else-if="modelChannelCards.length === 0" class="card py-12 text-center">
          <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-400" />
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('availableChannels.empty') }}</p>
        </div>
        <div v-else class="grid gap-3">
              <article
                v-for="channel in modelChannelCards"
                :key="`${channel.platform}-${channel.name}`"
                class="model-channel-card card overflow-hidden"
              >
                <div class="flex flex-col gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700 md:flex-row md:items-start md:justify-between">
                  <div class="min-w-0">
                    <div class="flex flex-wrap items-center gap-2">
                      <span
                        :class="[
                          'model-channel-platform-badge inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium',
                          platformBadgeClass(channel.platform),
                        ]"
                      >
                        <PlatformIcon :platform="channel.platform as GroupPlatform" size="xs" />
                        {{ platformLabel(channel.platform) }}
                      </span>
                      <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ channel.name }}</h3>
                      <span class="model-channel-group-badges flex flex-wrap items-center gap-1.5">
                        <span
                          v-for="groupItem in channel.groups"
                          :key="groupItem.id"
                          class="inline-flex items-center gap-1 rounded-md border border-gray-200 bg-gray-50 px-1.5 py-0.5 text-[11px] text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300"
                        >
                          <span>{{ t('models.groupRate') }}</span>
                          <span class="font-mono text-gray-500 dark:text-gray-400">x{{ formatRateMultiplier(groupItem) }}</span>
                        </span>
                      </span>
                      <span class="model-channel-count rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                        {{ channel.models.length }} {{ t('models.modelsUnit') }}
                      </span>
                    </div>
                    <p v-if="channel.description" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ channel.description }}</p>
                  </div>
                </div>

                <div class="overflow-x-auto">
                  <table class="w-full min-w-[780px] table-fixed border-collapse text-sm">
                    <colgroup>
                      <col class="model-name-width w-[15%]" />
                      <col class="model-billing-mode-width w-[10%]" />
                      <col class="w-[20%]" />
                      <col class="w-[20%]" />
                      <col class="w-[20%]" />
                      <col class="w-[15%]" />
                    </colgroup>
                    <thead>
                      <tr class="border-b border-gray-100 bg-gray-50/70 text-xs font-semibold text-gray-700 dark:border-dark-700 dark:bg-dark-800/60 dark:text-gray-200">
                        <th class="px-4 py-2.5 text-left">{{ t('models.modelName') }}</th>
                        <th class="model-billing-mode-column py-2.5 pl-2 pr-5 text-left">{{ t('models.billingMode') }}</th>
                        <th class="px-3 py-2.5 text-left">{{ t('models.priceInput') }}</th>
                        <th class="px-3 py-2.5 text-left">{{ t('models.priceOutput') }}</th>
                        <th class="px-3 py-2.5 text-left">{{ t('models.priceCacheRead') }}</th>
                        <th class="px-3 py-2.5 text-left">{{ t('models.discount') }}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr
                        v-for="model in channel.models"
                        :key="`${channel.name}-${model.name}`"
                        class="border-b border-gray-100 last:border-b-0 dark:border-dark-700"
                      >
                        <td class="px-4 py-3">
                          <div class="font-medium text-gray-900 dark:text-white">{{ model.name }}</div>
                        </td>
                        <td class="model-billing-mode-cell py-3 pl-2 pr-5 text-xs text-gray-500 dark:text-gray-400">
                          <template v-if="formatTokenIntervalLabels(model).length > 0">
                            <div class="model-billing-context-layout flex items-center gap-4">
                              <span class="model-billing-context-label shrink-0 font-medium leading-5 text-gray-600 dark:text-gray-300">{{ t('models.billingModeContext') }}</span>
                              <div class="model-billing-context-list space-y-1.5 font-mono">
                                <div
                                  v-for="label in formatTokenIntervalLabels(model)"
                                  :key="label"
                                  class="model-billing-context-row h-5 leading-5"
                                >
                                  {{ label }}
                                </div>
                              </div>
                            </div>
                          </template>
                          <template v-else>
                            {{ model.pricing?.billing_mode ? billingModeLabel(model.pricing.billing_mode) : '-' }}
                          </template>
                        </td>
                        <td
                          v-if="isRequestPricedModel(model)"
                          colspan="3"
                          class="model-price-cell px-3 py-3 text-center"
                        >
                          <div class="flex flex-wrap justify-center gap-2">
                            <span
                              v-for="item in formatRequestPriceItems(model, channel.groups)"
                              :key="`${item.tier}-${item.price}-${item.unit}`"
                              class="model-request-price-item inline-flex items-baseline gap-1.5 rounded-md border border-gray-200 bg-gray-50 px-2.5 py-1 dark:border-dark-700 dark:bg-dark-800"
                            >
                              <span v-if="item.tier" class="model-request-price-tier text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.tier }} </span>
                              <span class="model-request-price-value font-mono text-sm font-semibold text-gray-700 dark:text-gray-300">{{ item.price }}{{ item.unit }}</span>
                            </span>
                          </div>
                        </td>
                        <td v-if="isRequestPricedModel(model)" class="model-request-discount-spacer px-3 py-3"></td>
                        <template v-else>
                          <td class="model-price-cell px-3 py-3 text-left">
                            <div v-if="formatTokenIntervalRows(model, 'input_price', channel.groups).length > 0" class="space-y-1.5">
                              <div
                                v-for="row in formatTokenIntervalRows(model, 'input_price', channel.groups)"
                                :key="row.label"
                                class="model-price-interval-row h-5 whitespace-nowrap font-mono leading-5"
                              >
                                <span class="model-price-interval-values grid grid-cols-[minmax(3.5rem,auto)_auto_minmax(3.5rem,auto)] items-baseline justify-start gap-2 text-left">
                                  <span class="model-site-price text-sm font-semibold text-gray-700 dark:text-gray-300">{{ row.site }}</span>
                                  <span class="model-price-separator text-sm text-gray-300 dark:text-gray-600">/</span>
                                  <span class="model-official-price text-sm text-gray-400 line-through decoration-gray-300 dark:text-gray-500 dark:decoration-gray-600">{{ row.official }}</span>
                                </span>
                              </div>
                            </div>
                            <div v-else class="flex items-baseline gap-2 whitespace-nowrap font-mono">
                              <span class="model-site-price text-sm font-semibold text-gray-700 dark:text-gray-300">{{ formatTokenComparison(model, 'input_price', channel.groups).site }}</span>
                              <span class="model-price-separator text-sm text-gray-300 dark:text-gray-600">/</span>
                              <span class="model-official-price text-sm text-gray-400 line-through decoration-gray-300 dark:text-gray-500 dark:decoration-gray-600">{{ formatTokenComparison(model, 'input_price', channel.groups).official }}</span>
                            </div>
                          </td>
                          <td class="model-price-cell px-3 py-3 text-left">
                            <div v-if="formatTokenIntervalRows(model, 'output_price', channel.groups).length > 0" class="space-y-1.5">
                              <div
                                v-for="row in formatTokenIntervalRows(model, 'output_price', channel.groups)"
                                :key="row.label"
                                class="model-price-interval-row h-5 whitespace-nowrap font-mono leading-5"
                              >
                                <span class="model-price-interval-values grid grid-cols-[minmax(3.5rem,auto)_auto_minmax(3.5rem,auto)] items-baseline justify-start gap-2 text-left">
                                  <span class="model-site-price text-sm font-semibold text-gray-700 dark:text-gray-300">{{ row.site }}</span>
                                  <span class="model-price-separator text-sm text-gray-300 dark:text-gray-600">/</span>
                                  <span class="model-official-price text-sm text-gray-400 line-through decoration-gray-300 dark:text-gray-500 dark:decoration-gray-600">{{ row.official }}</span>
                                </span>
                              </div>
                            </div>
                            <div v-else class="flex items-baseline gap-2 whitespace-nowrap font-mono">
                              <span class="model-site-price text-sm font-semibold text-gray-700 dark:text-gray-300">{{ formatTokenComparison(model, 'output_price', channel.groups).site }}</span>
                              <span class="model-price-separator text-sm text-gray-300 dark:text-gray-600">/</span>
                              <span class="model-official-price text-sm text-gray-400 line-through decoration-gray-300 dark:text-gray-500 dark:decoration-gray-600">{{ formatTokenComparison(model, 'output_price', channel.groups).official }}</span>
                            </div>
                          </td>
                          <td class="model-price-cell px-3 py-3 text-left">
                            <div v-if="formatTokenIntervalRows(model, 'cache_read_price', channel.groups).length > 0" class="space-y-1.5">
                              <div
                                v-for="row in formatTokenIntervalRows(model, 'cache_read_price', channel.groups)"
                                :key="row.label"
                                class="model-price-interval-row h-5 whitespace-nowrap font-mono leading-5"
                              >
                                <span class="model-price-interval-values grid grid-cols-[minmax(3.5rem,auto)_auto_minmax(3.5rem,auto)] items-baseline justify-start gap-2 text-left">
                                  <span class="model-site-price text-sm font-semibold text-gray-700 dark:text-gray-300">{{ row.site }}</span>
                                  <span class="model-price-separator text-sm text-gray-300 dark:text-gray-600">/</span>
                                  <span class="model-official-price text-sm text-gray-400 line-through decoration-gray-300 dark:text-gray-500 dark:decoration-gray-600">{{ row.official }}</span>
                                </span>
                              </div>
                            </div>
                            <div v-else class="flex items-baseline gap-2 whitespace-nowrap font-mono">
                              <span class="model-site-price text-sm font-semibold text-gray-700 dark:text-gray-300">{{ formatTokenComparison(model, 'cache_read_price', channel.groups).site }}</span>
                              <span class="model-price-separator text-sm text-gray-300 dark:text-gray-600">/</span>
                              <span class="model-official-price text-sm text-gray-400 line-through decoration-gray-300 dark:text-gray-500 dark:decoration-gray-600">{{ formatTokenComparison(model, 'cache_read_price', channel.groups).official }}</span>
                            </div>
                          </td>
                        </template>
                        <td v-if="!isRequestPricedModel(model)" class="px-3 py-3 text-left">
                          <span class="model-discount-badge inline-flex rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-1 font-mono text-xs font-semibold text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300">
                            {{ formatDiscount(model, channel.groups) }}
                          </span>
                        </td>
                      </tr>
                      <tr v-if="channel.models.length === 0">
                        <td colspan="6" class="px-4 py-6 text-center text-xs text-gray-400">
                          {{ t('availableChannels.noModels') }}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </article>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import userChannelsAPI, { type UserAvailableChannel, type UserAvailableGroup, type UserDefaultModelPricing, type UserPricingInterval, type UserSupportedModel } from '@/api/channels'
import userGroupsAPI from '@/api/groups'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { BILLING_MODE_IMAGE, BILLING_MODE_PER_REQUEST, BILLING_MODE_TOKEN, type BillingMode } from '@/constants/channel'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import { getOfficialModelPricing, type OfficialModelPricing } from '@/utils/officialModelPricing'
import type { GroupPlatform } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const channels = ref<UserAvailableChannel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const officialPricingByModel = ref<Record<string, UserDefaultModelPricing>>({})
const loading = ref(false)
const searchQuery = ref('')
const selectedPlatform = ref('')
const currencies = ['CNY', 'USD'] as const
type Currency = typeof currencies[number]
const selectedCurrency = ref<Currency>('CNY')
const CNY_RATE = 7

const filteredChannels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return channels.value
    .map((channel) => {
      const channelHit = !q
        || channel.name.toLowerCase().includes(q)
        || (channel.description || '').toLowerCase().includes(q)

      const matchingPlatforms = channel.platforms.filter(
        (platform) => {
          if (selectedPlatform.value && platform.platform !== selectedPlatform.value) return false
          if (channelHit) return true
          return platform.platform.toLowerCase().includes(q) ||
            platform.groups.some((group) => group.name.toLowerCase().includes(q)) ||
            platform.supported_models.some((model) => model.name.toLowerCase().includes(q))
        },
      )

      if (matchingPlatforms.length === 0) return null
      return { ...channel, platforms: matchingPlatforms }
    })
    .filter((channel): channel is UserAvailableChannel => channel !== null)
})

const platformOptions = computed(() => {
  const platforms = new Set<string>()
  for (const channel of channels.value) {
    for (const section of channel.platforms) platforms.add(section.platform)
  }
  return [...platforms].sort(comparePlatformOption)
})

const preferredPlatformOrder = ['openai', 'anthropic', 'gemini']

function comparePlatformOption(a: string, b: string): number {
  const aIndex = preferredPlatformOrder.indexOf(a.toLowerCase())
  const bIndex = preferredPlatformOrder.indexOf(b.toLowerCase())
  if (aIndex !== -1 || bIndex !== -1) {
    if (aIndex === -1) return 1
    if (bIndex === -1) return -1
    return aIndex - bIndex
  }
  return platformLabel(a).localeCompare(platformLabel(b))
}

interface ModelChannelCard {
  name: string
  description: string
  platform: string
  groups: UserAvailableGroup[]
  models: UserSupportedModel[]
}

const modelChannelCards = computed<ModelChannelCard[]>(() => {
  const cards: ModelChannelCard[] = []
  for (const channel of filteredChannels.value) {
    for (const section of channel.platforms) {
      cards.push({
        name: channel.name,
        description: channel.description,
        platform: section.platform,
        groups: section.groups,
        models: section.supported_models,
      })
    }
  }
  return cards.sort((a, b) => comparePlatformOption(a.platform, b.platform) || a.name.localeCompare(b.name))
})

function billingModeLabel(mode: BillingMode): string {
  if (mode === BILLING_MODE_TOKEN) return t('availableChannels.pricing.billingModeToken')
  if (mode === BILLING_MODE_PER_REQUEST) return t('availableChannels.pricing.billingModePerRequest')
  if (mode === BILLING_MODE_IMAGE) return t('availableChannels.pricing.billingModeImage')
  return mode
}

function isRequestPricedModel(model: UserSupportedModel): boolean {
  const mode = model.pricing?.billing_mode
  return mode === BILLING_MODE_IMAGE || mode === BILLING_MODE_PER_REQUEST
}

function formatCurrencyAmount(amount: number): string {
  return `${selectedCurrency.value === 'CNY' ? '¥' : '$'}${amount.toFixed(2)}`
}

function siteAmount(value: number, scale: number, rateMultiplier: number): number {
  const cnyAmount = value * scale * rateMultiplier
  return selectedCurrency.value === 'CNY' ? cnyAmount : cnyAmount / CNY_RATE
}

function officialAmount(value: number, scale: number): number {
  const usdAmount = value * scale
  return selectedCurrency.value === 'CNY' ? usdAmount * CNY_RATE : usdAmount
}

function formatSitePrice(value: number | null, scale: number, rateMultiplier: number): string {
  if (value == null) return '-'
  return formatCurrencyAmount(siteAmount(value, scale, rateMultiplier))
}

function formatOfficialPrice(value: number | null, scale: number): string {
  if (value == null) return '-'
  return formatCurrencyAmount(officialAmount(value, scale))
}

type TokenPriceField = 'input_price' | 'output_price' | 'cache_write_price' | 'cache_read_price'

interface PriceComparisonDisplay {
  site: string
  official: string
}

interface TokenIntervalPriceDisplay extends PriceComparisonDisplay {
  label: string
}

interface RequestPriceDisplay {
  tier: string
  price: string
  unit: string
}

function formatTokenComparison(model: UserSupportedModel, field: TokenPriceField, groups: UserAvailableGroup[]): PriceComparisonDisplay {
  return formatComparison(model, field, 1_000_000, lowestEffectiveRateMultiplier(groups))
}

function formatTokenIntervalLabels(model: UserSupportedModel): string[] {
  const pricing = model.pricing
  if (!pricing || pricing.billing_mode !== BILLING_MODE_TOKEN) return []
  const labels = pricing.intervals
    .filter((interval) => (
      typeof interval.input_price === 'number' ||
      typeof interval.output_price === 'number' ||
      typeof interval.cache_read_price === 'number'
    ))
    .map((interval) => formatTokenIntervalLabel(interval))
  return [...new Set(labels)]
}

function formatTokenIntervalRows(model: UserSupportedModel, field: TokenPriceField, groups: UserAvailableGroup[]): TokenIntervalPriceDisplay[] {
  const pricing = model.pricing
  if (!pricing || pricing.billing_mode !== BILLING_MODE_TOKEN) return []
  const official = officialPricingByModel.value[normalizeModelKey(model.name)] ?? getOfficialModelPricing(model.platform, model.name)
  const officialValue = official?.[field]
  const officialDisplay = typeof officialValue === 'number'
    ? formatOfficialPrice(officialValue, 1_000_000)
    : '-'
  const rateMultiplier = lowestEffectiveRateMultiplier(groups)
  return pricing.intervals
    .filter((interval) => typeof interval[field] === 'number')
    .map((interval) => ({
      label: formatTokenIntervalLabel(interval),
      site: formatSitePrice(interval[field], 1_000_000, rateMultiplier),
      official: officialDisplay,
    }))
}

function formatTokenIntervalLabel(interval: UserPricingInterval): string {
  const explicitLabel = interval.tier_label?.trim()
  if (explicitLabel) return explicitLabel
  const min = Math.max(Number(interval.min_tokens) || 0, 0)
  const max = typeof interval.max_tokens === 'number' ? Math.max(interval.max_tokens, 0) : null
  if (max == null) return `${formatCompactTokens(min)}+`
  if (min === 0) return `0-${formatCompactTokens(max)}`
  return `${formatCompactTokens(min)}-${formatCompactTokens(max)}`
}

function formatCompactTokens(tokens: number): string {
  if (tokens >= 1_000_000 && tokens % 1_000_000 === 0) return `${tokens / 1_000_000}M`
  if (tokens >= 1_000 && tokens % 1_000 === 0) return `${tokens / 1_000}K`
  return tokens.toString()
}

function formatRequestPriceItems(model: UserSupportedModel, groups: UserAvailableGroup[]): RequestPriceDisplay[] {
  const pricing = model.pricing
  if (!pricing) return [{ tier: '', price: t('models.noPricing'), unit: '' }]
  const unit = pricing.billing_mode === BILLING_MODE_IMAGE ? t('models.perImageUnit') : ''
  if (pricing.billing_mode === BILLING_MODE_IMAGE) {
    const groupTiers = formatGroupImagePriceItems(groups, unit)
    if (groupTiers.length > 0) return groupTiers
  }
  const rateMultiplier = pricing.billing_mode === BILLING_MODE_IMAGE
    ? lowestEffectiveImageRateMultiplier(groups)
    : lowestEffectiveRateMultiplier(groups)
  const intervals = pricing.intervals
    .filter((interval) => typeof interval.per_request_price === 'number')
    .map((interval) => {
      const label = interval.tier_label?.trim() || t('models.defaultTier')
      return {
        tier: label,
        price: formatSitePrice(interval.per_request_price, 1, rateMultiplier),
        unit,
      }
    })
  if (intervals.length > 0) return intervals
  if (typeof pricing.per_request_price === 'number') {
    return [{ tier: '', price: formatSitePrice(pricing.per_request_price, 1, rateMultiplier), unit }]
  }
  return [{ tier: '', price: t('models.noPricing'), unit: '' }]
}

function formatGroupImagePriceItems(groups: UserAvailableGroup[], unit: string): RequestPriceDisplay[] {
  const group = lowestEffectiveGroupImagePrices(groups)
  if (!group) return []
  const rateMultiplier = effectiveImageRateMultiplier(group)
  const tiers: Array<[string, number | null | undefined]> = [
    ['1K', group.image_price_1k],
    ['2K', group.image_price_2k],
    ['4K', group.image_price_4k],
  ]
  return tiers
    .filter(([, price]) => typeof price === 'number')
    .map(([label, price]) => ({
      tier: label,
      price: formatSitePrice(price ?? null, 1, rateMultiplier),
      unit,
    }))
}

function formatComparison(model: UserSupportedModel, field: keyof OfficialModelPricing, scale: number, rateMultiplier: number): PriceComparisonDisplay {
  const siteValue = model.pricing?.[field]
  if (typeof siteValue !== 'number') {
    return {
      site: '-',
      official: '-',
    }
  }
  const official = officialPricingByModel.value[normalizeModelKey(model.name)] ?? getOfficialModelPricing(model.platform, model.name)
  const officialValue = official?.[field]
  return {
    site: formatSitePrice(siteValue, scale, rateMultiplier),
    official: typeof officialValue === 'number' ? formatOfficialPrice(officialValue, scale) : '-',
  }
}

function formatDiscount(model: UserSupportedModel, groups: UserAvailableGroup[]): string {
  if (!model.pricing) return '-'
  if (isRequestPricedModel(model)) return '-'
  const official = officialPricingByModel.value[normalizeModelKey(model.name)] ?? getOfficialModelPricing(model.platform, model.name)
  const rateMultiplier = lowestEffectiveRateMultiplier(groups)
  const fields: Array<{ field: keyof OfficialModelPricing, scale: number }> = [
    { field: 'input_price', scale: 1_000_000 },
    { field: 'output_price', scale: 1_000_000 },
    { field: 'cache_read_price', scale: 1_000_000 },
    { field: 'per_request_price', scale: 1 },
    { field: 'image_output_price', scale: 1 },
  ]
  for (const { field, scale } of fields) {
    const siteValue = model.pricing[field]
    const officialValue = official?.[field]
    if (typeof siteValue !== 'number' || typeof officialValue !== 'number' || officialValue <= 0) continue
    return `${(siteAmount(siteValue, scale, rateMultiplier) / officialAmount(officialValue, scale) * 10).toFixed(1)}折`
  }
  return '-'
}

function lowestEffectiveRateMultiplier(groups: UserAvailableGroup[]): number {
  const rates = groups
    .map((group) => Number(userGroupRates.value[group.id] ?? group.rate_multiplier))
    .filter((rate) => Number.isFinite(rate))
    .map((rate) => Math.max(rate, 0))
  return rates.length > 0 ? Math.min(...rates) : 1
}

function lowestEffectiveImageRateMultiplier(groups: UserAvailableGroup[]): number {
  const rates = groups
    .map((group) => {
      if (group.image_rate_independent) return Number(group.image_rate_multiplier ?? 1)
      return Number(userGroupRates.value[group.id] ?? group.rate_multiplier)
    })
    .filter((rate) => Number.isFinite(rate))
    .map((rate) => Math.max(rate, 0))
  return rates.length > 0 ? Math.min(...rates) : 1
}

function lowestEffectiveGroupImagePrices(groups: UserAvailableGroup[]): UserAvailableGroup | null {
  const pricedGroups = groups.filter((group) => (
    typeof group.image_price_1k === 'number' ||
    typeof group.image_price_2k === 'number' ||
    typeof group.image_price_4k === 'number'
  ))
  if (pricedGroups.length === 0) return null
  return pricedGroups.reduce((best, group) => (
    lowestGroupImageFinalPrice(group) < lowestGroupImageFinalPrice(best) ? group : best
  ))
}

function lowestGroupImageFinalPrice(group: UserAvailableGroup): number {
  const prices = [group.image_price_1k, group.image_price_2k, group.image_price_4k]
    .filter((price): price is number => typeof price === 'number')
  if (prices.length === 0) return Number.POSITIVE_INFINITY
  return Math.min(...prices) * effectiveImageRateMultiplier(group)
}

function effectiveImageRateMultiplier(group: UserAvailableGroup): number {
  const rate = group.image_rate_independent
    ? Number(group.image_rate_multiplier ?? 1)
    : Number(userGroupRates.value[group.id] ?? group.rate_multiplier)
  return Number.isFinite(rate) ? Math.max(rate, 0) : 1
}

function normalizeModelKey(modelName: string): string {
  return modelName.trim().toLowerCase()
}

function collectModelNames(list: UserAvailableChannel[]): string[] {
  const seen = new Set<string>()
  const models: string[] = []
  for (const channel of list) {
    for (const section of channel.platforms) {
      for (const model of section.supported_models) {
        const name = model.name.trim()
        const key = normalizeModelKey(name)
        if (!name || seen.has(key)) continue
        seen.add(key)
        models.push(name)
      }
    }
  }
  return models
}

async function loadOfficialPricing(list: UserAvailableChannel[]) {
  const models = collectModelNames(list)
  if (models.length === 0) {
    officialPricingByModel.value = {}
    return
  }
  try {
    const result = await userChannelsAPI.getModelPricingBatch(models)
    const next: Record<string, UserDefaultModelPricing> = {}
    for (const [modelName, pricing] of Object.entries(result.prices || {})) {
      if (pricing.found) next[normalizeModelKey(modelName)] = pricing
    }
    officialPricingByModel.value = next
  } catch (err) {
    console.error('Failed to load official model pricing:', err)
    officialPricingByModel.value = {}
  }
}

function formatRateMultiplier(group: UserAvailableGroup): string {
  const rate = userGroupRates.value[group.id] ?? group.rate_multiplier
  return Number(rate).toString()
}

async function loadChannels() {
  loading.value = true
  try {
    const [list, rates] = await Promise.all([
      userChannelsAPI.getAvailable({ public: !authStore.isAuthenticated }),
      authStore.isAuthenticated
        ? userGroupsAPI.getUserGroupRates().catch((err: unknown) => {
          console.error('Failed to load user group rates:', err)
          return {} as Record<number, number>
        })
        : Promise.resolve({} as Record<number, number>),
    ])
    channels.value = list
    userGroupRates.value = rates
    await loadOfficialPricing(list)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadChannels)
</script>
