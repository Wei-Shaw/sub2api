<template>
  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
    <button
      v-for="model in sortedModels"
      :key="model.name"
      type="button"
      class="model-card flex min-h-[220px] flex-col overflow-hidden rounded-lg border border-gray-200 bg-white text-left shadow-sm transition duration-150 hover:-translate-y-0.5 hover:border-gray-300 hover:shadow-md focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-dark-600"
      data-test="model-card"
      :aria-label="t('modelPlaza.detail.open', { model: model.name })"
      @click="$emit('select', model)"
    >
      <div class="flex items-start justify-between gap-3 px-4 pb-3 pt-4">
        <div class="flex min-w-0 items-center gap-3">
          <div
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-900"
          >
            <ModelIcon
              :model="model.name"
              size="24px"
              theme-aware
              class="text-gray-900 dark:text-gray-100"
            />
          </div>
          <div class="min-w-0">
            <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="model.name">
              {{ model.name }}
            </h3>
            <div class="mt-1 flex items-center gap-1.5 text-xs text-gray-400 dark:text-dark-400">
              <PlatformIcon :platform="model.platform as GroupPlatform" size="xs" />
              <span class="truncate">{{ model.platform || platform }}</span>
            </div>
          </div>
        </div>
        <span
          v-if="billingMode(model) !== BILLING_MODE_TOKEN"
          class="shrink-0 rounded-md bg-gray-100 px-2 py-1 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-300"
        >
          {{ billingModeLabel(model) }}
        </span>
      </div>

      <div class="flex-1 border-t border-gray-100 px-4 py-3 dark:border-dark-700/70">
        <template v-if="billingMode(model) === BILLING_MODE_TOKEN">
          <div v-if="tokenIntervals(model).length" class="space-y-2">
            <div
              v-for="(interval, index) in tokenIntervals(model)"
              :key="index"
              class="rounded-md bg-gray-50 px-3 py-2 dark:bg-dark-900/60"
            >
              <div class="mb-1 text-[10px] font-medium text-gray-400 dark:text-dark-500">
                {{ tierLabel(interval) }}
              </div>
              <div class="grid grid-cols-2 gap-3">
                <PriceValue
                  :label="t('modelPlaza.table.input')"
                  :value="paidPerMillion(interval.input_price)"
                />
                <PriceValue
                  :label="t('modelPlaza.table.output')"
                  :value="paidPerMillion(interval.output_price)"
                />
              </div>
            </div>
          </div>
          <div v-else class="grid grid-cols-2 gap-4">
            <PriceValue
              :label="t('modelPlaza.table.input')"
              :value="paidPerMillion(model.pricing?.input_price)"
            />
            <PriceValue
              :label="t('modelPlaza.table.output')"
              :value="paidPerMillion(model.pricing?.output_price)"
            />
          </div>

          <div
            v-if="hasCachePricing(model)"
            class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-gray-500 dark:text-dark-400"
          >
            <span class="font-medium text-gray-400 dark:text-dark-500">
              {{ t('modelPlaza.table.cache') }}
            </span>
            <span>
              {{ t('modelPlaza.table.cacheWrite') }}
              <strong class="font-mono font-medium text-gray-700 dark:text-gray-200">
                {{ paidPerMillion(model.pricing?.cache_write_price) }}
              </strong>
            </span>
            <span>
              {{ t('modelPlaza.table.cacheRead') }}
              <strong class="font-mono font-medium text-gray-700 dark:text-gray-200">
                {{ paidPerMillion(model.pricing?.cache_read_price) }}
              </strong>
            </span>
          </div>
        </template>

        <template v-else>
          <div class="text-[10px] font-medium uppercase text-gray-400 dark:text-dark-500">
            {{ t('modelPlaza.table.paidPrice') }}
          </div>
          <div v-if="requestIntervals(model).length" class="mt-2 flex flex-wrap gap-2">
            <span
              v-for="(interval, index) in requestIntervals(model)"
              :key="index"
              class="inline-flex items-center gap-1.5 rounded-md bg-gray-50 px-2.5 py-1.5 text-xs dark:bg-dark-900/60"
            >
              <span class="text-gray-400 dark:text-dark-500">{{ tierLabel(interval) }}</span>
              <strong class="font-mono text-gray-900 dark:text-white">
                {{ paidRequestPrice(interval.per_request_price) }}
              </strong>
              <span class="text-gray-400 dark:text-dark-500">{{ perUnitSuffix(model) }}</span>
            </span>
          </div>
          <div v-else class="mt-1">
            <span class="font-mono text-xl font-semibold text-gray-900 dark:text-white">
              {{ paidRequestPrice(model.pricing?.per_request_price) }}
            </span>
            <span class="ml-1 text-xs text-gray-400 dark:text-dark-500">
              {{ perUnitSuffix(model) }}
            </span>
          </div>
        </template>
      </div>

      <footer
        class="flex min-h-11 items-center justify-between gap-3 border-t border-gray-100 bg-gray-50/70 px-4 py-2 text-[11px] dark:border-dark-700/70 dark:bg-dark-900/40"
      >
        <div class="min-w-0 truncate text-gray-400 dark:text-dark-500">
          <template v-if="model.official_pricing">
            {{ t('modelPlaza.table.officialPrice') }}
            <span class="ml-1 font-mono text-gray-600 dark:text-dark-300">
              {{ official(model.official_pricing.input_price) }}
              /
              {{ official(model.official_pricing.output_price) }}
            </span>
          </template>
          <span v-else>{{ t('modelPlaza.detail.noPricing') }}</span>
        </div>
        <div class="shrink-0 font-mono">
          <template v-if="hasCustomRate">
            <span class="mr-1 text-gray-400 line-through dark:text-dark-500">
              {{ rateMultiplier }}x
            </span>
            <span class="font-semibold text-primary-600 dark:text-primary-400">
              {{ effectiveRate }}x
            </span>
          </template>
          <span v-else class="font-semibold text-gray-600 dark:text-gray-300">
            {{ effectiveRate }}x
          </span>
        </div>
      </footer>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { formatScaled } from '@/utils/pricing'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_TOKEN,
  type BillingMode
} from '@/constants/channel'
import type { PlazaModel } from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'
import type { GroupPlatform } from '@/types'

const props = defineProps<{
  models: PlazaModel[]
  platform?: string
  rateMultiplier: number
  userRateMultiplier?: number | null
}>()

defineEmits<{ select: [model: PlazaModel] }>()

const { t } = useI18n()
const PER_MILLION = 1_000_000
const MIN_DECIMALS = 2

const PriceValue = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true }
  },
  setup(priceProps) {
    return () =>
      h('div', [
        h('div', { class: 'text-[10px] font-medium uppercase text-gray-400 dark:text-dark-500' }, priceProps.label),
        h('div', { class: 'mt-0.5 font-mono text-lg font-semibold text-gray-900 dark:text-white' }, priceProps.value),
        h('div', { class: 'mt-0.5 text-[10px] text-gray-400 dark:text-dark-500' }, t('modelPlaza.table.unitPerMillion'))
      ])
  }
})

const effectiveRate = computed(() => props.userRateMultiplier ?? props.rateMultiplier)
const hasCustomRate = computed(
  () => props.userRateMultiplier != null && props.userRateMultiplier !== props.rateMultiplier
)

const sortedModels = computed(() =>
  [...props.models].sort((a, b) => {
    const aPrice = a.official_pricing?.output_price ?? null
    const bPrice = b.official_pricing?.output_price ?? null
    if (aPrice != null && bPrice != null && aPrice !== bPrice) return bPrice - aPrice
    if (aPrice != null && bPrice == null) return -1
    if (aPrice == null && bPrice != null) return 1
    return a.name.localeCompare(b.name)
  })
)

function billingMode(model: PlazaModel): BillingMode {
  return (model.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
}

function billingModeLabel(model: PlazaModel): string {
  return billingMode(model) === BILLING_MODE_IMAGE
    ? t('modelPlaza.table.perImage')
    : t('modelPlaza.table.perRequest')
}

function paidPerMillion(value: number | null | undefined): string {
  if (value == null) return '-'
  return formatScaled(value * effectiveRate.value, PER_MILLION, MIN_DECIMALS)
}

function paidRequestPrice(value: number | null | undefined): string {
  if (value == null) return '-'
  return formatScaled(value * effectiveRate.value, 1, MIN_DECIMALS)
}

function official(value: number | null | undefined): string {
  if (value == null) return '-'
  return formatScaled(value, PER_MILLION, MIN_DECIMALS)
}

function perUnitSuffix(model: PlazaModel): string {
  return billingMode(model) === BILLING_MODE_IMAGE
    ? t('modelPlaza.table.perUnitImage')
    : t('modelPlaza.table.perUnitRequest')
}

function hasCachePricing(model: PlazaModel): boolean {
  return model.pricing?.cache_write_price != null || model.pricing?.cache_read_price != null
}

function tokenIntervals(model: PlazaModel): UserPricingInterval[] {
  return model.pricing?.intervals ?? []
}

function requestIntervals(model: PlazaModel): UserPricingInterval[] {
  return (model.pricing?.intervals ?? []).filter((interval) => interval.per_request_price != null)
}

function tierLabel(interval: UserPricingInterval): string {
  if (interval.tier_label) return interval.tier_label
  const { min_tokens: min, max_tokens: max } = interval
  if (max == null) return `>${formatTokenCount(min)}`
  if (min === 0) return `≤${formatTokenCount(max)}`
  return `${formatTokenCount(min)}–${formatTokenCount(max)}`
}

function formatTokenCount(value: number): string {
  if (value >= 1_000_000) return `${trimZero(value / 1_000_000)}M`
  if (value >= 1_000) return `${trimZero(value / 1_000)}K`
  return String(value)
}

function trimZero(value: number): string {
  return String(Math.round(value * 100) / 100)
}
</script>
