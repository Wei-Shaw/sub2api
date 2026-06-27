<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelMarketplaceFilters from '@/components/model-marketplace/ModelMarketplaceFilters.vue'
import ModelPricingCard from '@/components/model-marketplace/ModelPricingCard.vue'
import ModelPricingTable from '@/components/model-marketplace/ModelPricingTable.vue'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getConfiguredTableDefaultPageSize, normalizeTablePageSize } from '@/utils/tablePreferences'
import {
  modelMarketplaceAPI,
  type ModelMarketplaceCatalogState,
  type ModelMarketplaceFacets,
  type ModelMarketplaceItem,
  type ModelMarketplacePagination,
  type ModelMarketplaceServiceTier,
  type ModelMarketplaceUnit,
} from '@/api/modelMarketplace'
import {
  formatModelPrice,
  formatRateMultiplier,
  useModelPricingSummary,
} from '@/composables/useModelPricingDisplay'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const emptyFacets: ModelMarketplaceFacets = {
  providers: [],
  modes: [],
  billing_modes: [],
  endpoints: [],
  groups: [],
}

const filters = reactive({
  q: '',
  provider: '',
  mode: '',
  billing_mode: '',
  endpoint: '',
  group_id: null as number | null,
  service_tier: 'standard' as ModelMarketplaceServiceTier | string,
  unit: '1M' as ModelMarketplaceUnit | string,
})

const items = ref<ModelMarketplaceItem[]>([])
const facets = ref<ModelMarketplaceFacets>({ ...emptyFacets })
const catalog = ref<ModelMarketplaceCatalogState>({ model_count: 0 })
const pagination = reactive<ModelMarketplacePagination>({
  page: 1,
  page_size: normalizeTablePageSize(getConfiguredTableDefaultPageSize()),
  total: 0,
  pages: 1,
})
const loading = shallowRef(false)
const initialLoaded = shallowRef(false)
const viewMode = shallowRef<'cards' | 'table'>('cards')

let abortController: AbortController | null = null

const { tokenModels, imageModels, providerCount } = useModelPricingSummary(items)

const unitLabel = computed(() =>
  filters.unit === '1K' ? t('modelMarketplace.units.oneThousandShort') : t('modelMarketplace.units.oneMillionShort')
)

const selectedGroup = computed(() =>
  filters.group_id == null ? null : facets.value.groups.find((group) => group.id === filters.group_id) ?? null
)

const activeRateLabel = computed(() => formatRateMultiplier(selectedGroup.value?.rate_multiplier ?? 1))

const heroFormula = computed(() => {
  if (selectedGroup.value) {
    return t('modelMarketplace.hero.groupFormula', {
      group: selectedGroup.value.name,
      rate: activeRateLabel.value,
    })
  }
  return t('modelMarketplace.hero.baseFormula', { rate: activeRateLabel.value })
})

const pageSubtitle = computed(() =>
  t('modelMarketplace.resultSummary', {
    shown: items.value.length,
    total: pagination.total,
    page: pagination.page,
    pages: pagination.pages || 1,
  })
)

const lowestInputPrice = computed(() => {
  const prices = items.value
    .map((item) => item.pricing.input.display_usd)
    .filter((value) => Number.isFinite(value) && value > 0)
  if (prices.length === 0) return '—'
  return formatModelPrice(Math.min(...prices))
})

const visibleModelNames = computed(() => items.value.map((item) => item.model).join('\n'))

const isAbortError = (error: unknown) => {
  const candidate = error as { name?: string; code?: string }
  return candidate?.name === 'AbortError' || candidate?.name === 'CanceledError' || candidate?.code === 'ERR_CANCELED'
}

let queryReloadTimer: ReturnType<typeof setTimeout> | null = null

function clearQueryReloadTimer() {
  if (queryReloadTimer) {
    clearTimeout(queryReloadTimer)
    queryReloadTimer = null
  }
}

function scheduleQueryReload() {
  clearQueryReloadTimer()
  queryReloadTimer = setTimeout(() => {
    queryReloadTimer = null
    void loadModels()
  }, 300)
}

async function loadModels() {
  abortController?.abort()
  const currentController = new AbortController()
  abortController = currentController
  loading.value = true

  try {
    const response = await modelMarketplaceAPI.getModelPricing(
      {
        q: filters.q,
        provider: filters.provider,
        mode: filters.mode,
        billing_mode: filters.billing_mode,
        endpoint: filters.endpoint,
        group_id: filters.group_id,
        service_tier: filters.service_tier,
        unit: filters.unit,
        page: pagination.page,
        page_size: pagination.page_size,
      },
      { signal: currentController.signal },
    )

    items.value = response.items
    facets.value = response.facets
    catalog.value = response.catalog
    pagination.page = response.pagination.page
    pagination.page_size = response.pagination.page_size
    pagination.total = response.pagination.total
    pagination.pages = response.pagination.pages || 1
  } catch (err: unknown) {
    if (!isAbortError(err)) {
      appStore.showError(extractApiErrorMessage(err, t('modelMarketplace.loadFailed')))
    }
  } finally {
    if (abortController === currentController) {
      loading.value = false
      initialLoaded.value = true
    }
  }
}

function handleQueryUpdate(value: string) {
  filters.q = value
  pagination.page = 1
  scheduleQueryReload()
}

function handleProviderUpdate(value: string) {
  clearQueryReloadTimer()
  filters.provider = value
  pagination.page = 1
  void loadModels()
}

function handleModeUpdate(value: string) {
  clearQueryReloadTimer()
  filters.mode = value
  pagination.page = 1
  void loadModels()
}

function handleBillingModeUpdate(value: string) {
  clearQueryReloadTimer()
  filters.billing_mode = value
  pagination.page = 1
  void loadModels()
}

function handleEndpointUpdate(value: string) {
  clearQueryReloadTimer()
  filters.endpoint = value
  pagination.page = 1
  void loadModels()
}

function handleGroupUpdate(value: number | null) {
  clearQueryReloadTimer()
  filters.group_id = value
  pagination.page = 1
  void loadModels()
}

function handleServiceTierUpdate(value: string) {
  clearQueryReloadTimer()
  filters.service_tier = value || 'standard'
  pagination.page = 1
  void loadModels()
}

function handleUnitUpdate(value: string) {
  clearQueryReloadTimer()
  filters.unit = value || '1M'
  pagination.page = 1
  void loadModels()
}

function handleViewModeUpdate(value: 'cards' | 'table') {
  viewMode.value = value
}

function resetFilters() {
  clearQueryReloadTimer()
  filters.q = ''
  filters.provider = ''
  filters.mode = ''
  filters.billing_mode = ''
  filters.endpoint = ''
  filters.group_id = null
  filters.service_tier = 'standard'
  filters.unit = '1M'
  pagination.page = 1
  void loadModels()
}

function refreshModels() {
  clearQueryReloadTimer()
  void loadModels()
}

function handlePageChange(page: number) {
  clearQueryReloadTimer()
  pagination.page = page
  void loadModels()
}

function handlePageSizeChange(pageSize: number) {
  clearQueryReloadTimer()
  pagination.page_size = pageSize
  pagination.page = 1
  void loadModels()
}

async function copyModel(model: string) {
  await copyToClipboard(model, t('modelMarketplace.actions.modelCopied'))
}

async function copyVisibleModels() {
  if (!visibleModelNames.value) {
    appStore.showInfo(t('modelMarketplace.actions.noModelsToCopy'))
    return
  }
  await copyToClipboard(visibleModelNames.value, t('modelMarketplace.actions.visibleCopied'))
}

onMounted(() => {
  void loadModels()
})

onBeforeUnmount(() => {
  clearQueryReloadTimer()
  abortController?.abort()
})
</script>

<template>
  <AppLayout>
    <div class="space-y-6" data-testid="model-marketplace-view">
      <section class="hero-card">
        <div class="relative z-10 grid gap-6 lg:grid-cols-[1fr_auto] lg:items-end">
          <div class="max-w-4xl">
            <div class="mb-3 inline-flex items-center gap-2 rounded-full bg-white/80 px-3 py-1 text-xs font-semibold text-primary-700 shadow-sm dark:bg-dark-800/80 dark:text-primary-300">
              <Icon name="sparkles" size="sm" />
              {{ t('modelMarketplace.hero.badge') }}
            </div>
            <h1 class="text-2xl font-bold tracking-tight text-gray-950 dark:text-white sm:text-3xl">
              {{ t('modelMarketplace.title') }}
            </h1>
            <p class="mt-3 max-w-3xl text-sm leading-6 text-gray-600 dark:text-gray-300 sm:text-base">
              {{ t('modelMarketplace.description') }}
            </p>
            <div class="mt-4 rounded-2xl border border-white/70 bg-white/70 p-4 text-sm shadow-sm backdrop-blur dark:border-dark-700/70 dark:bg-dark-900/50">
              <div class="flex items-start gap-3">
                <div class="rounded-xl bg-primary-100 p-2 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">
                  <Icon name="calculator" size="md" />
                </div>
                <div>
                  <div class="font-semibold text-gray-900 dark:text-white">{{ t('modelMarketplace.hero.formulaTitle') }}</div>
                  <p class="mt-1 text-gray-600 dark:text-gray-300">{{ heroFormula }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('modelMarketplace.hero.billingNote') }}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <div class="grid min-w-[260px] grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-2">
            <div class="hero-stat">
              <div class="hero-stat-label">{{ t('modelMarketplace.stats.results') }}</div>
              <div class="hero-stat-value">{{ pagination.total.toLocaleString() }}</div>
              <div class="hero-stat-foot">{{ t('modelMarketplace.stats.catalog', { count: catalog.model_count || pagination.total }) }}</div>
            </div>
            <div class="hero-stat">
              <div class="hero-stat-label">{{ t('modelMarketplace.stats.providers') }}</div>
              <div class="hero-stat-value">{{ providerCount.toLocaleString() }}</div>
              <div class="hero-stat-foot">{{ t('modelMarketplace.stats.currentPage') }}</div>
            </div>
            <div class="hero-stat">
              <div class="hero-stat-label">{{ t('modelMarketplace.stats.tokenModels') }}</div>
              <div class="hero-stat-value">{{ tokenModels.toLocaleString() }}</div>
              <div class="hero-stat-foot">{{ t('modelMarketplace.billingMode.token') }}</div>
            </div>
            <div class="hero-stat">
              <div class="hero-stat-label">{{ t('modelMarketplace.stats.lowestInput') }}</div>
              <div class="hero-stat-value text-lg">{{ lowestInputPrice }}</div>
              <div class="hero-stat-foot">/ {{ unitLabel }}</div>
            </div>
          </div>
        </div>
      </section>

      <ModelMarketplaceFilters
        :query="filters.q"
        :provider="filters.provider"
        :mode="filters.mode"
        :billing-mode="filters.billing_mode"
        :endpoint="filters.endpoint"
        :group-id="filters.group_id"
        :service-tier="filters.service_tier"
        :unit="filters.unit"
        :view-mode="viewMode"
        :facets="facets"
        :loading="loading"
        @update:query="handleQueryUpdate"
        @update:provider="handleProviderUpdate"
        @update:mode="handleModeUpdate"
        @update:billing-mode="handleBillingModeUpdate"
        @update:endpoint="handleEndpointUpdate"
        @update:group-id="handleGroupUpdate"
        @update:service-tier="handleServiceTierUpdate"
        @update:unit="handleUnitUpdate"
        @update:view-mode="handleViewModeUpdate"
        @reset-filters="resetFilters"
        @refresh="refreshModels"
        @copy-visible="copyVisibleModels"
      />

      <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-950 dark:text-white">{{ t('modelMarketplace.catalogTitle') }}</h2>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ pageSubtitle }}</p>
        </div>
        <div class="flex flex-wrap gap-2 text-xs text-gray-500 dark:text-gray-400">
          <span class="badge badge-gray">{{ t('modelMarketplace.stats.imageModels', { count: imageModels }) }}</span>
          <span class="badge badge-gray">{{ t('modelMarketplace.filters.rate', { rate: activeRateLabel }) }}</span>
        </div>
      </div>

      <div v-if="viewMode === 'cards'" class="min-h-[320px]">
        <div v-if="loading && !initialLoaded" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <div v-for="idx in 6" :key="idx" class="card h-72 animate-pulse p-5">
            <div class="h-6 w-2/3 rounded bg-gray-200 dark:bg-dark-700" />
            <div class="mt-4 grid grid-cols-2 gap-2">
              <div class="h-20 rounded-xl bg-gray-100 dark:bg-dark-800" />
              <div class="h-20 rounded-xl bg-gray-100 dark:bg-dark-800" />
              <div class="h-20 rounded-xl bg-gray-100 dark:bg-dark-800" />
              <div class="h-20 rounded-xl bg-gray-100 dark:bg-dark-800" />
            </div>
          </div>
        </div>

        <EmptyState
          v-else-if="items.length === 0"
          :title="t('modelMarketplace.empty.title')"
          :description="t('modelMarketplace.empty.description')"
        />

        <div v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <ModelPricingCard
            v-for="item in items"
            :key="item.model"
            :item="item"
            :unit-label="unitLabel"
            @copy="copyModel"
          />
        </div>
      </div>

      <div v-else class="card min-h-[420px] overflow-hidden">
        <ModelPricingTable
          :items="items"
          :loading="loading"
          :unit-label="unitLabel"
          @copy="copyModel"
        />
      </div>

      <Pagination
        v-if="pagination.total > 0"
        :total="pagination.total"
        :page="pagination.page"
        :page-size="pagination.page_size"
        :show-jump="true"
        @update:page="handlePageChange"
        @update:page-size="handlePageSizeChange"
      />
    </div>
  </AppLayout>
</template>

<style scoped>
.hero-card {
  @apply relative overflow-hidden rounded-3xl border border-primary-100 bg-gradient-to-br from-primary-50 via-white to-sky-50 p-5 shadow-card dark:border-primary-900/40 dark:from-primary-950/30 dark:via-dark-900 dark:to-sky-950/20 sm:p-6;
}

.hero-card::after {
  content: '';
  @apply pointer-events-none absolute -right-24 -top-24 h-64 w-64 rounded-full bg-primary-300/20 blur-3xl dark:bg-primary-500/10;
}

.hero-stat {
  @apply rounded-2xl border border-white/70 bg-white/75 p-4 shadow-sm backdrop-blur dark:border-dark-700/70 dark:bg-dark-900/55;
}

.hero-stat-label {
  @apply text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400;
}

.hero-stat-value {
  @apply mt-1 text-2xl font-bold text-gray-950 dark:text-white;
}

.hero-stat-foot {
  @apply mt-1 text-xs text-gray-500 dark:text-gray-400;
}
</style>
