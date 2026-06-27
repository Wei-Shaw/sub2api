<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type {
  ModelMarketplaceFacets,
  ModelMarketplaceServiceTier,
  ModelMarketplaceUnit,
} from '@/api/modelMarketplace'
import { formatRateMultiplier } from '@/composables/useModelPricingDisplay'

const props = defineProps<{
  query: string
  provider: string
  mode: string
  billingMode: string
  endpoint: string
  groupId: number | null
  serviceTier: ModelMarketplaceServiceTier | string
  unit: ModelMarketplaceUnit | string
  viewMode: 'cards' | 'table'
  facets: ModelMarketplaceFacets
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:query': [value: string]
  'update:provider': [value: string]
  'update:mode': [value: string]
  'update:billingMode': [value: string]
  'update:endpoint': [value: string]
  'update:groupId': [value: number | null]
  'update:serviceTier': [value: string]
  'update:unit': [value: string]
  'update:viewMode': [value: 'cards' | 'table']
  'reset-filters': []
  refresh: []
  'copy-visible': []
}>()

const { t } = useI18n()
const filtersOpen = shallowRef(false)

const providerOptions = computed(() => [
  { value: '', label: t('modelMarketplace.filters.allProviders') },
  ...props.facets.providers.map((provider) => ({
    value: provider.value,
    label: `${provider.label} (${provider.count})`,
    icon: provider.icon,
  })),
])

const modeOptions = computed(() => [
  { value: '', label: t('modelMarketplace.filters.allModes') },
  ...props.facets.modes.map((mode) => ({
    value: mode.value,
    label: `${mode.label || mode.value} (${mode.count})`,
  })),
])

const billingModeOptions = computed(() => [
  { value: '', label: t('modelMarketplace.filters.allBillingModes') },
  ...props.facets.billing_modes.map((mode) => ({
    value: mode.value,
    label: `${billingModeLabel(mode.value)} (${mode.count})`,
  })),
])

const endpointOptions = computed(() => [
  { value: '', label: t('modelMarketplace.filters.allEndpoints') },
  ...props.facets.endpoints.map((endpoint) => ({
    value: endpoint.value,
    label: `${endpoint.label || endpoint.value} (${endpoint.count})`,
  })),
])

const groupOptions = computed(() => [
  { value: null, label: t('modelMarketplace.filters.basePricing'), rate: 1 },
  ...props.facets.groups.map((group) => ({
    value: group.id,
    label: `${group.name} · ${formatRateMultiplier(group.rate_multiplier)}`,
    platform: group.platform,
    rate: group.rate_multiplier,
    subscription_type: group.subscription_type,
  })),
])

const serviceTierOptions = computed(() => [
  { value: 'standard', label: t('modelMarketplace.serviceTier.standard') },
  { value: 'priority', label: t('modelMarketplace.serviceTier.priority') },
  { value: 'flex', label: t('modelMarketplace.serviceTier.flex') },
])

const unitOptions = computed(() => [
  { value: '1M', label: t('modelMarketplace.units.oneMillion') },
  { value: '1K', label: t('modelMarketplace.units.oneThousand') },
])

const hasAdvancedFilters = computed(() =>
  Boolean(props.provider || props.mode || props.billingMode || props.endpoint || props.groupId)
)

function billingModeLabel(mode: string): string {
  if (mode === 'image') return t('modelMarketplace.billingMode.image')
  return t('modelMarketplace.billingMode.token')
}

function updateQuery(event: Event) {
  emit('update:query', (event.target as HTMLInputElement).value)
}

function updateGroup(value: string | number | boolean | null) {
  emit('update:groupId', typeof value === 'number' ? value : null)
}

function stringValue(value: string | number | boolean | null): string {
  return typeof value === 'string' ? value : ''
}

function updateProvider(value: string | number | boolean | null) {
  emit('update:provider', stringValue(value))
}

function updateMode(value: string | number | boolean | null) {
  emit('update:mode', stringValue(value))
}

function updateBillingMode(value: string | number | boolean | null) {
  emit('update:billingMode', stringValue(value))
}

function updateEndpoint(value: string | number | boolean | null) {
  emit('update:endpoint', stringValue(value))
}

function updateServiceTier(value: string | number | boolean | null) {
  emit('update:serviceTier', stringValue(value))
}

function updateUnit(value: string | number | boolean | null) {
  emit('update:unit', stringValue(value))
}
</script>

<template>
  <div class="card p-4 sm:p-5">
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div class="relative min-w-0 flex-1">
          <Icon
            name="search"
            size="md"
            class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
          />
          <input
            :value="query"
            type="search"
            class="input pl-10"
            :placeholder="t('modelMarketplace.searchPlaceholder')"
            @input="updateQuery"
          />
        </div>

        <div class="flex flex-wrap items-center gap-2 lg:flex-nowrap">
          <div class="inline-flex rounded-xl border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-800">
            <button
              type="button"
              class="view-toggle"
              :class="viewMode === 'cards' && 'view-toggle-active'"
              @click="emit('update:viewMode', 'cards')"
            >
              <Icon name="grid" size="sm" />
              {{ t('modelMarketplace.view.cards') }}
            </button>
            <button
              type="button"
              class="view-toggle"
              :class="viewMode === 'table' && 'view-toggle-active'"
              @click="emit('update:viewMode', 'table')"
            >
              <Icon name="database" size="sm" />
              {{ t('modelMarketplace.view.table') }}
            </button>
          </div>

          <button
            type="button"
            class="btn btn-secondary md:hidden"
            @click="filtersOpen = !filtersOpen"
          >
            <Icon name="filter" size="md" />
            {{ t('modelMarketplace.filters.title') }}
            <span v-if="hasAdvancedFilters" class="h-2 w-2 rounded-full bg-primary-500" />
          </button>

          <button
            type="button"
            class="btn btn-secondary"
            :disabled="loading"
            @click="emit('refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            <span class="hidden sm:inline">{{ t('common.refresh') }}</span>
          </button>

          <button type="button" class="btn btn-secondary" @click="emit('copy-visible')">
            <Icon name="copy" size="md" />
            <span class="hidden sm:inline">{{ t('modelMarketplace.actions.copyVisible') }}</span>
          </button>
        </div>
      </div>

      <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-6" :class="filtersOpen ? 'grid' : 'hidden md:grid'">
        <div class="min-w-0">
          <label class="input-label">{{ t('modelMarketplace.filters.provider') }}</label>
          <Select
            :model-value="provider"
            :options="providerOptions"
            searchable
            @update:model-value="updateProvider"
          />
        </div>

        <div class="min-w-0">
          <label class="input-label">{{ t('modelMarketplace.filters.group') }}</label>
          <Select
            :model-value="groupId"
            :options="groupOptions"
            searchable
            @update:model-value="updateGroup"
          />
        </div>

        <div class="min-w-0">
          <label class="input-label">{{ t('modelMarketplace.filters.billingMode') }}</label>
          <Select
            :model-value="billingMode"
            :options="billingModeOptions"
            @update:model-value="updateBillingMode"
          />
        </div>

        <div class="min-w-0">
          <label class="input-label">{{ t('modelMarketplace.filters.endpoint') }}</label>
          <Select
            :model-value="endpoint"
            :options="endpointOptions"
            @update:model-value="updateEndpoint"
          />
        </div>

        <div class="min-w-0">
          <label class="input-label">{{ t('modelMarketplace.filters.mode') }}</label>
          <Select
            :model-value="mode"
            :options="modeOptions"
            @update:model-value="updateMode"
          />
        </div>

        <div class="grid min-w-0 grid-cols-2 gap-3 xl:grid-cols-1">
          <div>
            <label class="input-label">{{ t('modelMarketplace.filters.serviceTier') }}</label>
            <Select
              :model-value="serviceTier"
              :options="serviceTierOptions"
              @update:model-value="updateServiceTier"
            />
          </div>
          <div>
            <label class="input-label">{{ t('modelMarketplace.filters.unit') }}</label>
            <Select
              :model-value="unit"
              :options="unitOptions"
              @update:model-value="updateUnit"
            />
          </div>
        </div>
      </div>

      <div class="flex flex-col gap-3 border-t border-gray-100 pt-3 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('modelMarketplace.filters.hint') }}
        </p>
        <button type="button" class="btn btn-ghost btn-sm self-start sm:self-auto" @click="emit('reset-filters')">
          <Icon name="x" size="sm" />
          {{ t('common.reset') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.view-toggle {
  @apply inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-gray-500 transition-colors dark:text-gray-400;
  @apply hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white;
}

.view-toggle-active {
  @apply bg-primary-50 text-primary-700 shadow-sm dark:bg-primary-900/30 dark:text-primary-300;
}
</style>
