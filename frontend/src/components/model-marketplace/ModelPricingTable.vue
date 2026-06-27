<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import FeaturePills from '@/components/model-marketplace/FeaturePills.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { Column } from '@/components/common/types'
import type { GroupPlatform } from '@/types'
import type { ModelMarketplaceItem, ModelMarketplacePricePart } from '@/api/modelMarketplace'
import {
  formatContextTokens,
  formatModelPrice,
  pricePartVisible,
} from '@/composables/useModelPricingDisplay'

const props = defineProps<{
  items: ModelMarketplaceItem[]
  loading: boolean
  unitLabel: string
}>()

const emit = defineEmits<{
  copy: [model: string]
}>()

const { t } = useI18n()

const columns = computed<Column[]>(() => [
  { key: 'model', label: t('modelMarketplace.table.model'), sortable: true, class: 'min-w-[260px]' },
  { key: 'provider', label: t('modelMarketplace.table.provider'), sortable: true, class: 'min-w-[180px]' },
  { key: 'pricing', label: t('modelMarketplace.table.pricing'), class: 'min-w-[320px]' },
  { key: 'endpoints', label: t('modelMarketplace.table.endpoints'), class: 'min-w-[150px]' },
  { key: 'features', label: t('modelMarketplace.table.features'), class: 'min-w-[240px]' },
  { key: 'context', label: t('modelMarketplace.table.context'), class: 'min-w-[140px]' },
  { key: 'source', label: t('modelMarketplace.table.source'), class: 'min-w-[140px]' },
  { key: 'actions', label: t('modelMarketplace.table.actions'), class: 'min-w-[110px]' },
])

function providerPlatform(item: ModelMarketplaceItem): GroupPlatform {
  return (item.provider_icon || item.provider) as GroupPlatform
}

function sourceLabel(item: ModelMarketplaceItem): string {
  return item.pricing.source === 'litellm_catalog'
    ? t('modelMarketplace.source.catalog')
    : t('modelMarketplace.source.fallback')
}

function billingModeLabel(item: ModelMarketplaceItem): string {
  return item.billing_mode === 'image'
    ? t('modelMarketplace.billingMode.image')
    : t('modelMarketplace.billingMode.token')
}

function contextLabel(item: ModelMarketplaceItem): string {
  const input = formatContextTokens(item.context.max_input_tokens)
  const output = formatContextTokens(item.context.max_output_tokens)
  if (input === '—' && output === '—') return '—'
  if (output === '—') return input
  return `${input} / ${output}`
}

function priceRows(item: ModelMarketplaceItem): Array<{ key: string; label: string; part: ModelMarketplacePricePart }> {
  return [
    { key: 'input', label: t('modelMarketplace.price.input'), part: item.pricing.input },
    { key: 'output', label: t('modelMarketplace.price.output'), part: item.pricing.output },
    { key: 'cache_read', label: t('modelMarketplace.price.cacheRead'), part: item.pricing.cache_read },
    { key: 'cache_write', label: t('modelMarketplace.price.cacheWrite'), part: item.pricing.cache_write },
    { key: 'cache_write_5m', label: t('modelMarketplace.price.cacheWrite5m'), part: item.pricing.cache_write_5m },
    { key: 'cache_write_1h', label: t('modelMarketplace.price.cacheWrite1h'), part: item.pricing.cache_write_1h },
    { key: 'image_output', label: t('modelMarketplace.price.imageOutput'), part: item.pricing.image_output },
  ].filter((row) => pricePartVisible(row.part))
}

function featureLabels(item: ModelMarketplaceItem): string[] {
  const labels: string[] = []
  if (item.features.prompt_caching) labels.push(t('modelMarketplace.features.promptCaching'))
  if (item.features.service_tier) labels.push(t('modelMarketplace.features.serviceTier'))
  if (item.features.vision) labels.push(t('modelMarketplace.features.vision'))
  if (item.features.reasoning) labels.push(t('modelMarketplace.features.reasoning'))
  if (item.features.web_search) labels.push(t('modelMarketplace.features.webSearch'))
  if (item.features.audio_output) labels.push(t('modelMarketplace.features.audioOutput'))
  return labels
}
</script>

<template>
  <DataTable
    :columns="columns"
    :data="props.items"
    :loading="loading"
    row-key="model"
    default-sort-key="model"
    :estimate-row-height="120"
    :overscan="8"
    data-testid="model-pricing-table"
  >
    <template #cell-model="{ row }">
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <span class="font-semibold text-gray-950 dark:text-white">{{ row.model }}</span>
          <span class="badge badge-primary">{{ billingModeLabel(row) }}</span>
        </div>
        <div class="mt-1 flex flex-wrap gap-1">
          <span v-for="tag in row.tags.slice(0, 4)" :key="tag" class="badge badge-gray">
            {{ tag }}
          </span>
        </div>
      </div>
    </template>

    <template #cell-provider="{ row }">
      <div class="flex items-center gap-2">
        <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200">
          <PlatformIcon :platform="providerPlatform(row)" size="md" />
        </div>
        <div>
          <div class="font-medium text-gray-900 dark:text-white">{{ row.provider_label }}</div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ row.family }} · {{ row.mode }}</div>
        </div>
      </div>
    </template>

    <template #cell-pricing="{ row }">
      <div class="grid gap-1.5">
        <div
          v-for="price in priceRows(row)"
          :key="price.key"
          class="flex items-center justify-between gap-3 rounded-lg bg-gray-50 px-2 py-1 dark:bg-dark-800"
        >
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ price.label }}</span>
          <span class="font-mono text-xs font-semibold text-gray-900 dark:text-white">
            {{ formatModelPrice(price.part.display_usd) }} / {{ unitLabel }}
          </span>
        </div>
        <div
          v-if="row.pricing.per_request"
          class="flex items-center justify-between gap-3 rounded-lg bg-purple-50 px-2 py-1 dark:bg-purple-900/10"
        >
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelMarketplace.price.perRequest') }}</span>
          <span class="font-mono text-xs font-semibold text-gray-900 dark:text-white">
            {{ formatModelPrice(row.pricing.per_request.display_usd) }} / {{ t('modelMarketplace.units.request') }}
          </span>
        </div>
      </div>
    </template>

    <template #cell-endpoints="{ row }">
      <div class="flex flex-wrap gap-1">
        <span v-for="endpoint in row.supported_endpoints" :key="endpoint" class="chip">
          {{ endpoint }}
        </span>
      </div>
    </template>

    <template #cell-features="{ row }">
      <FeaturePills :labels="featureLabels(row)" />
    </template>

    <template #cell-context="{ row }">
      <span class="font-medium text-gray-900 dark:text-white">{{ contextLabel(row) }}</span>
    </template>

    <template #cell-source="{ row }">
      <span class="badge" :class="row.pricing.source === 'litellm_catalog' ? 'badge-primary' : 'badge-warning'">
        {{ sourceLabel(row) }}
      </span>
    </template>

    <template #cell-actions="{ row }">
      <button type="button" class="btn btn-secondary btn-sm" @click="emit('copy', row.model)">
        <Icon name="copy" size="sm" />
        {{ t('common.copy') }}
      </button>
    </template>
  </DataTable>
</template>

<style scoped>
.chip {
  @apply rounded-md bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300;
}
</style>
