<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import type { ModelMarketplaceItem, ModelMarketplacePricePart } from '@/api/modelMarketplace'
import {
  formatContextTokens,
  formatModelPrice,
  formatRateMultiplier,
  pricePartVisible,
} from '@/composables/useModelPricingDisplay'

const props = defineProps<{
  item: ModelMarketplaceItem
  unitLabel: string
}>()

const emit = defineEmits<{
  copy: [model: string]
}>()

const { t } = useI18n()

interface PriceRow {
  key: string
  label: string
  part: ModelMarketplacePricePart
  tone: string
}

const providerPlatform = computed(() => (props.item.provider_icon || props.item.provider) as GroupPlatform)

const priceRows = computed<PriceRow[]>(() => {
  const rows: PriceRow[] = [
    { key: 'input', label: t('modelMarketplace.price.input'), part: props.item.pricing.input, tone: 'price-tone-blue' },
    { key: 'output', label: t('modelMarketplace.price.output'), part: props.item.pricing.output, tone: 'price-tone-emerald' },
    { key: 'cache_read', label: t('modelMarketplace.price.cacheRead'), part: props.item.pricing.cache_read, tone: 'price-tone-sky' },
    { key: 'cache_write', label: t('modelMarketplace.price.cacheWrite'), part: props.item.pricing.cache_write, tone: 'price-tone-amber' },
    { key: 'cache_write_5m', label: t('modelMarketplace.price.cacheWrite5m'), part: props.item.pricing.cache_write_5m, tone: 'price-tone-orange' },
    { key: 'cache_write_1h', label: t('modelMarketplace.price.cacheWrite1h'), part: props.item.pricing.cache_write_1h, tone: 'price-tone-purple' },
    { key: 'image_output', label: t('modelMarketplace.price.imageOutput'), part: props.item.pricing.image_output, tone: 'price-tone-pink' },
  ]
  return rows.filter((row) => pricePartVisible(row.part))
})

const visibleTags = computed(() => props.item.tags.slice(0, 5))

const featureLabels = computed(() => {
  const labels: string[] = []
  if (props.item.features.prompt_caching) labels.push(t('modelMarketplace.features.promptCaching'))
  if (props.item.features.service_tier) labels.push(t('modelMarketplace.features.serviceTier'))
  if (props.item.features.vision) labels.push(t('modelMarketplace.features.vision'))
  if (props.item.features.reasoning) labels.push(t('modelMarketplace.features.reasoning'))
  if (props.item.features.web_search) labels.push(t('modelMarketplace.features.webSearch'))
  if (props.item.features.audio_output) labels.push(t('modelMarketplace.features.audioOutput'))
  return labels
})

const sourceLabel = computed(() =>
  props.item.pricing.source === 'litellm_catalog'
    ? t('modelMarketplace.source.catalog')
    : t('modelMarketplace.source.fallback')
)

const billingModeLabel = computed(() =>
  props.item.billing_mode === 'image'
    ? t('modelMarketplace.billingMode.image')
    : t('modelMarketplace.billingMode.token')
)

const contextLabel = computed(() => {
  const input = formatContextTokens(props.item.context.max_input_tokens)
  const output = formatContextTokens(props.item.context.max_output_tokens)
  if (input === '—' && output === '—') return '—'
  if (output === '—') return input
  return `${input} / ${output}`
})

const formulaLabel = computed(() => {
  const rate = formatRateMultiplier(props.item.pricing.rate_multiplier)
  if (props.item.pricing.group_name) {
    return t('modelMarketplace.formula.group', { group: props.item.pricing.group_name, rate })
  }
  return t('modelMarketplace.formula.base', { rate })
})

function displayPrice(part: ModelMarketplacePricePart): string {
  return formatModelPrice(part.display_usd)
}
</script>

<template>
  <article class="model-card" data-testid="model-pricing-card">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0 flex items-start gap-3">
        <div class="provider-avatar">
          <PlatformIcon :platform="providerPlatform" size="lg" />
        </div>
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="truncate text-base font-semibold text-gray-950 dark:text-white" :title="item.model">
              {{ item.model }}
            </h3>
            <span class="badge badge-primary">{{ billingModeLabel }}</span>
          </div>
          <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ item.provider_label }}</span>
            <span>·</span>
            <span>{{ item.family }}</span>
            <span>·</span>
            <span>{{ item.mode }}</span>
          </div>
        </div>
      </div>

      <button
        type="button"
        class="btn btn-ghost btn-sm shrink-0"
        :aria-label="t('modelMarketplace.actions.copyModel')"
        @click="emit('copy', item.model)"
      >
        <Icon name="copy" size="sm" />
      </button>
    </div>

    <div class="mt-4 grid gap-2 sm:grid-cols-2">
      <div v-for="row in priceRows" :key="row.key" class="price-row" :class="row.tone">
        <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ row.label }}</div>
        <div class="mt-1 text-lg font-bold text-gray-950 dark:text-white">
          {{ displayPrice(row.part) }}
        </div>
        <div class="text-[11px] text-gray-500 dark:text-gray-400">/ {{ unitLabel }}</div>
      </div>

      <div v-if="item.pricing.per_request" class="price-row price-tone-purple">
        <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('modelMarketplace.price.perRequest') }}</div>
        <div class="mt-1 text-lg font-bold text-gray-950 dark:text-white">
          {{ formatModelPrice(item.pricing.per_request.display_usd) }}
        </div>
        <div class="text-[11px] text-gray-500 dark:text-gray-400">/ {{ t('modelMarketplace.units.request') }}</div>
      </div>
    </div>

    <div class="mt-4 grid gap-3 border-t border-gray-100 pt-4 text-xs dark:border-dark-700 sm:grid-cols-3">
      <div>
        <div class="metric-label">{{ t('modelMarketplace.table.endpoints') }}</div>
        <div class="mt-1 flex flex-wrap gap-1">
          <span v-for="endpoint in item.supported_endpoints" :key="endpoint" class="chip">{{ endpoint }}</span>
        </div>
      </div>
      <div>
        <div class="metric-label">{{ t('modelMarketplace.table.context') }}</div>
        <div class="mt-1 font-medium text-gray-900 dark:text-gray-100">{{ contextLabel }}</div>
      </div>
      <div>
        <div class="metric-label">{{ t('modelMarketplace.table.source') }}</div>
        <div class="mt-1 font-medium text-gray-900 dark:text-gray-100">{{ sourceLabel }}</div>
      </div>
    </div>

    <div v-if="featureLabels.length || visibleTags.length" class="mt-4 flex flex-wrap gap-1.5">
      <span v-for="feature in featureLabels" :key="`feature-${feature}`" class="badge badge-success">
        {{ feature }}
      </span>
      <span v-for="tag in visibleTags" :key="`tag-${tag}`" class="badge badge-gray">
        {{ tag }}
      </span>
    </div>

    <div class="mt-4 rounded-xl bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-900/70 dark:text-gray-300">
      <div class="flex items-start gap-2">
        <Icon name="calculator" size="sm" class="mt-0.5 text-primary-500" />
        <div class="min-w-0">
          <div class="font-medium text-gray-800 dark:text-gray-100">{{ formulaLabel }}</div>
          <div v-if="item.pricing.long_context" class="mt-1 text-gray-500 dark:text-gray-400">
            {{ t('modelMarketplace.formula.longContext', {
              threshold: formatContextTokens(item.pricing.long_context.input_token_threshold),
              input: formatRateMultiplier(item.pricing.long_context.input_cost_multiplier),
              output: formatRateMultiplier(item.pricing.long_context.output_cost_multiplier)
            }) }}
          </div>
        </div>
      </div>
    </div>
  </article>
</template>

<style scoped>
.model-card {
  @apply card card-hover flex h-full flex-col p-4 sm:p-5;
}

.provider-avatar {
  @apply flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-50 to-sky-50 text-primary-600 dark:from-primary-900/30 dark:to-sky-900/20 dark:text-primary-300;
}

.price-row {
  @apply rounded-xl border px-3 py-3;
}

.price-tone-blue {
  @apply border-blue-100 bg-blue-50/60 dark:border-blue-900/40 dark:bg-blue-900/10;
}

.price-tone-emerald {
  @apply border-emerald-100 bg-emerald-50/60 dark:border-emerald-900/40 dark:bg-emerald-900/10;
}

.price-tone-sky {
  @apply border-sky-100 bg-sky-50/60 dark:border-sky-900/40 dark:bg-sky-900/10;
}

.price-tone-amber {
  @apply border-amber-100 bg-amber-50/60 dark:border-amber-900/40 dark:bg-amber-900/10;
}

.price-tone-orange {
  @apply border-orange-100 bg-orange-50/60 dark:border-orange-900/40 dark:bg-orange-900/10;
}

.price-tone-purple {
  @apply border-purple-100 bg-purple-50/60 dark:border-purple-900/40 dark:bg-purple-900/10;
}

.price-tone-pink {
  @apply border-pink-100 bg-pink-50/60 dark:border-pink-900/40 dark:bg-pink-900/10;
}

.metric-label {
  @apply text-[11px] font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400;
}

.chip {
  @apply rounded-md bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300;
}
</style>
