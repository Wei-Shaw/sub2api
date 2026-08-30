<template>
  <div class="overflow-x-auto">
    <table class="w-full min-w-[900px] table-fixed border-collapse text-left" :style="accentStyle">
      <colgroup v-if="billingMode === BILLING_MODE_TOKEN" data-testid="token-columns">
        <col style="width: 32%" />
        <col style="width: 13%" />
        <col style="width: 13%" />
        <col style="width: 13%" />
        <col style="width: 13%" />
        <col style="width: 16%" />
      </colgroup>
      <colgroup v-else-if="billingMode === BILLING_MODE_PER_REQUEST" data-testid="per-request-columns">
        <col style="width: 34%" />
        <col style="width: 22%" />
        <col style="width: 22%" />
        <col style="width: 22%" />
      </colgroup>
      <colgroup v-else-if="billingMode === BILLING_MODE_IMAGE">
        <col style="width: 34%" />
        <col
          v-for="label in imageTierLabels"
          :key="label"
          :style="{ width: `${66 / Math.max(imageTierLabels.length, 1)}%` }"
        />
      </colgroup>
      <colgroup v-else>
        <col style="width: 34%" />
        <col style="width: 66%" />
      </colgroup>
      <thead>
        <tr class="border-b border-[#e9edf7] bg-[#f4f6fd] text-[11px] font-semibold uppercase tracking-wide text-[#677594] dark:border-dark-700/60 dark:bg-dark-900/40 dark:text-dark-400">
          <th class="px-5 py-3">{{ t('modelPlaza.table.model') }}</th>

          <template v-if="billingMode === BILLING_MODE_TOKEN">
            <th class="px-3 py-3 text-center">{{ t('modelPlaza.table.input') }}</th>
            <th class="px-3 py-3 text-center">{{ t('modelPlaza.table.output') }}</th>
            <th class="px-3 py-3 text-center">{{ t('modelPlaza.table.cacheWrite') }}</th>
            <th class="px-3 py-3 text-center">{{ t('modelPlaza.table.cacheRead') }}</th>
            <th class="px-5 py-3 text-center">{{ t('modelPlaza.table.displayMultiplier') }}</th>
          </template>

          <template v-else-if="billingMode === BILLING_MODE_PER_REQUEST">
            <th class="px-3 py-3 text-center">
              <span class="block">≤ 256K</span>
              <span class="font-normal normal-case opacity-70">{{ perRequestUnit }}</span>
            </th>
            <th class="px-3 py-3 text-center">
              <span class="block">256K–512K</span>
              <span class="font-normal normal-case opacity-70">{{ perRequestUnit }}</span>
            </th>
            <th class="px-5 py-3 text-center">
              <span class="block">&gt; 512K</span>
              <span class="font-normal normal-case opacity-70">{{ perRequestUnit }}</span>
            </th>
          </template>

          <template v-else-if="billingMode === BILLING_MODE_IMAGE">
            <th v-for="label in imageTierLabels" :key="label" class="px-4 py-3 text-center">
              {{ label }}
            </th>
          </template>
          <th v-else class="px-5 py-3 text-right">{{ t('modelPlaza.table.unavailable') }}</th>
        </tr>
      </thead>

      <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
        <tr
          v-for="model in sortedModels"
          :key="model.id ?? `${model.provider}:${model.model_name}:${model.billing_mode}`"
          class="transition hover:bg-[#f8faff] dark:hover:bg-dark-700/35"
        >
          <td class="px-5 py-3.5">
            <div class="flex items-start gap-2.5">
              <PlatformIcon :platform="platform" size="sm" class="mt-0.5 shrink-0 text-[var(--price-accent)]" />
              <div class="min-w-0">
                <span class="block truncate font-mono text-sm font-semibold text-gray-800 dark:text-gray-100" :title="model.model_name">
                  {{ model.model_name }}
                </span>
                <p
                  v-if="model.model_note"
                  data-testid="model-note"
                  class="mt-1 whitespace-pre-line text-xs font-semibold leading-4 text-orange-600 dark:text-orange-300"
                >
                  {{ model.model_note }}
                </p>
              </div>
            </div>
          </td>

          <template v-if="billingMode === BILLING_MODE_TOKEN">
            <td class="price-cell">
              <div class="price-pair">
                <span class="official-price" data-testid="official-price">
                  <span>{{ t('modelPlaza.table.officialPrice') }}</span>
                  <b>{{ formatPrice(model.official_prices?.input_per_million) }}</b>
                </span>
                <span class="site-price" data-testid="site-price">
                  <span>{{ t('modelPlaza.table.sitePrice') }}</span>
                  <strong>{{ formatPrice(model.display_prices?.input_per_million) }}</strong>
                </span>
              </div>
            </td>
            <td class="price-cell">
              <div class="price-pair">
                <span class="official-price" data-testid="official-price">
                  <span>{{ t('modelPlaza.table.officialPrice') }}</span>
                  <b>{{ formatPrice(model.official_prices?.output_per_million) }}</b>
                </span>
                <span class="site-price" data-testid="site-price">
                  <span>{{ t('modelPlaza.table.sitePrice') }}</span>
                  <strong>{{ formatPrice(model.display_prices?.output_per_million) }}</strong>
                </span>
              </div>
            </td>
            <td class="price-cell">
              <div class="price-pair">
                <span class="official-price" data-testid="official-price">
                  <span>{{ t('modelPlaza.table.officialPrice') }}</span>
                  <b>{{ formatPrice(model.official_prices?.cache_write_per_million) }}</b>
                </span>
                <span class="site-price" data-testid="site-price">
                  <span>{{ t('modelPlaza.table.sitePrice') }}</span>
                  <strong>{{ formatPrice(model.display_prices?.cache_write_per_million) }}</strong>
                </span>
              </div>
            </td>
            <td class="price-cell">
              <div class="price-pair">
                <span class="official-price" data-testid="official-price">
                  <span>{{ t('modelPlaza.table.officialPrice') }}</span>
                  <b>{{ formatPrice(model.official_prices?.cache_read_per_million) }}</b>
                </span>
                <span class="site-price" data-testid="site-price">
                  <span>{{ t('modelPlaza.table.sitePrice') }}</span>
                  <strong>{{ formatPrice(model.display_prices?.cache_read_per_million) }}</strong>
                </span>
              </div>
            </td>
            <td class="px-5 py-3.5 text-center">
              <span
                v-if="model.effective_multiplier != null"
                data-testid="multiplier-badge"
                class="inline-flex min-w-[64px] justify-center rounded-md border border-[#c7eee5] bg-[#effbf8] px-2.5 py-1 font-mono text-xs font-bold text-[#07866f] dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300"
              >
                ×{{ formatMultiplier(model.effective_multiplier) }}
              </span>
              <span v-else class="text-gray-400 dark:text-dark-500">-</span>
            </td>
          </template>

          <template v-else-if="billingMode === BILLING_MODE_PER_REQUEST">
            <td class="price-cell site-only-price">{{ formatPrice(model.per_request?.lte_256k) }}</td>
            <td class="price-cell site-only-price">{{ formatPrice(model.per_request?.from_256k_to_512k) }}</td>
            <td class="price-cell site-only-price px-5">{{ formatPrice(model.per_request?.gt_512k) }}</td>
          </template>

          <template v-else-if="billingMode === BILLING_MODE_IMAGE">
            <td v-for="label in imageTierLabels" :key="label" class="price-cell px-4">
              <div class="price-pair">
                <span class="official-price" data-testid="image-base-price">
                  <span>{{ t('modelPlaza.table.basePrice') }}</span>
                  <b>{{ formatPrice(imageBasePrice(model, label)) }}</b>
                </span>
                <span class="site-price" data-testid="image-site-price">
                  <span>{{ t('modelPlaza.table.sitePrice') }}</span>
                  <strong>{{ formatPrice(imagePrice(model, label)) }}</strong>
                </span>
              </div>
            </td>
          </template>
          <td v-else class="price-cell px-5">-</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { DisplayPriceCurrency, DisplayPriceModel } from '@/api/modelPrices'
import type { BillingMode } from '@/constants/channel'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN
} from '@/constants/channel'
import { platformAccentColor } from '@/utils/platformColors'

const props = defineProps<{
  models: DisplayPriceModel[]
  platform: string
  billingMode: BillingMode
  currency: DisplayPriceCurrency
}>()

const { t } = useI18n()
const accentStyle = computed(() => ({
  '--price-accent': platformAccentColor(props.platform)
}))
// The API already applies administrator-defined sort_order. Preserve that order.
const sortedModels = computed(() => props.models)
const currencySymbol = computed(() => (props.currency === 'CNY' ? '¥' : '$'))
const perRequestUnit = computed(() => `${currencySymbol.value} / ${t('modelPlaza.table.requestUnit')}`)
const imageTierLabels = computed(() => {
  const labels = new Set<string>()
  for (const model of props.models) {
    for (const tier of model.image_prices ?? []) labels.add(tier.label)
  }
  return [...labels]
})

function formatNumber(value: number): string {
  if (!Number.isFinite(value)) return '-'
  const absolute = Math.abs(value)
  const digits = absolute > 0 && absolute < 0.01 ? 6 : absolute < 1 ? 4 : 2
  return value.toFixed(digits).replace(/(\.\d*?[1-9])0+$|\.0+$/, '$1')
}

function formatPrice(value: number | null | undefined): string {
  return value == null ? '-' : `${currencySymbol.value}${formatNumber(value)}`
}

function formatMultiplier(value: number): string {
  return value.toFixed(3).replace(/(\.\d*?[1-9])0+$|\.0+$/, '$1')
}

function imagePrice(model: DisplayPriceModel, label: string): number | null {
  return model.image_prices?.find((tier) => tier.label === label)?.price ?? null
}

function imageBasePrice(model: DisplayPriceModel, label: string): number | null {
  return model.image_base_prices?.find((tier) => tier.label === label)?.price ?? null
}
</script>

<style scoped>
.price-cell {
  @apply px-3 py-3.5 text-right;
}

.price-pair {
  @apply flex w-full min-w-0 flex-col items-center gap-1.5;
}

.official-price,
.site-price {
  @apply flex items-baseline justify-center gap-1.5 whitespace-nowrap;
}

.official-price {
  @apply text-[10px] font-medium text-[#8b96ad] dark:text-dark-500;
}

.official-price b {
  @apply font-mono text-xs font-semibold text-[#64748b] dark:text-dark-400;
}

.site-price {
  @apply text-[10px] font-semibold text-[#315bd6] dark:text-blue-300;
}

.site-price strong {
  @apply font-mono text-sm font-bold text-[#315bd6] dark:text-blue-300;
}

.site-only-price {
  @apply text-center font-mono text-sm font-semibold text-[#315bd6] dark:text-blue-300;
}
</style>
