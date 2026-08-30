<template>
  <div class="space-y-5">
    <section
      class="relative overflow-hidden rounded-3xl border border-orange-200/70 bg-gradient-to-br from-[#fffaf2] via-white to-[#fff5e8] px-5 py-6 shadow-sm dark:border-orange-500/20 dark:from-dark-900 dark:via-dark-900 dark:to-orange-950/20 sm:px-7 sm:py-7"
    >
      <div class="pointer-events-none absolute -right-16 -top-20 h-52 w-52 rounded-full bg-orange-200/35 blur-3xl dark:bg-orange-500/10"></div>
      <div class="relative flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
        <div class="max-w-2xl">
          <div class="mb-3 inline-flex items-center gap-2 rounded-full border border-orange-200 bg-white/80 px-3 py-1 text-xs font-semibold text-orange-700 shadow-sm dark:border-orange-500/25 dark:bg-dark-800/80 dark:text-orange-300">
            <span class="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-500"></span>
            {{ t('modelPlaza.liveCatalog') }}
          </div>
          <h1 class="text-2xl font-bold tracking-tight text-gray-950 dark:text-white sm:text-3xl">
            {{ t('modelPlaza.title') }}
          </h1>
          <p class="mt-2 max-w-xl text-sm leading-6 text-gray-600 dark:text-dark-300">
            {{ t('modelPlaza.dynamicDescription') }}
          </p>
          <p class="mt-2 inline-flex items-center gap-1.5 text-xs font-medium text-amber-700 dark:text-amber-300">
            <Icon name="infoCircle" size="xs" />
            {{ t('modelPlaza.displayOnlyNotice') }}
          </p>
        </div>

        <div class="grid grid-cols-2 gap-2.5 sm:min-w-[300px]">
          <div class="rounded-2xl border border-white/80 bg-white/80 px-4 py-3 shadow-sm backdrop-blur dark:border-dark-700 dark:bg-dark-800/80">
            <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('modelPlaza.stats.models') }}</p>
            <p class="mt-1 text-xl font-bold text-gray-950 dark:text-white">{{ totalModelCount }}</p>
          </div>
          <div class="rounded-2xl border border-white/80 bg-white/80 px-4 py-3 shadow-sm backdrop-blur dark:border-dark-700 dark:bg-dark-800/80">
            <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('modelPlaza.stats.providers') }}</p>
            <p class="mt-1 text-xl font-bold text-gray-950 dark:text-white">{{ providerOptions.length }}</p>
          </div>
        </div>
      </div>
    </section>

    <div v-if="loading" class="flex min-h-[280px] items-center justify-center">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600 dark:border-primary-400/25 dark:border-t-primary-400"></div>
    </div>
    <div
      v-else-if="error"
      class="rounded-2xl border border-red-200 bg-red-50 px-5 py-10 text-center text-sm text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
    >
      {{ t('modelPlaza.loadFailed') }}
    </div>

    <div v-else class="grid items-start gap-5 lg:grid-cols-[270px_minmax(0,1fr)]">
      <PlazaFilterBar
        :billing-options="billingOptions"
        :billing-mode="selectedBillingMode"
        :providers="providerOptions"
        :platform="selectedProvider"
        :search="searchQuery"
        :result-count="currentResultCount"
        :total-count="totalModelCount"
        :last-updated-label="lastUpdatedLabel"
        :refreshing="refreshing"
        @update:billing-mode="updateBillingMode"
        @update:platform="updateProvider"
        @update:search="searchQuery = $event"
        @reset="resetFilters"
        @refresh="$emit('refresh')"
      />

      <div ref="resultsEl" class="min-w-0 scroll-mt-4">
        <div v-if="filteredBillingRegions.length > 0" class="space-y-12">
          <section
            v-for="region in filteredBillingRegions"
            :key="region.billingMode"
            :data-billing-mode="region.billingMode"
            class="scroll-mt-4"
          >
            <header
              class="mb-5 overflow-hidden rounded-2xl border border-indigo-200/80 bg-gradient-to-r from-indigo-50/90 via-white to-blue-50/70 shadow-sm dark:border-indigo-500/25 dark:from-indigo-500/10 dark:via-dark-800 dark:to-blue-500/10"
            >
              <div class="border-l-4 border-indigo-500 px-5 py-4 sm:px-6">
                <p class="text-xs font-semibold tracking-wide text-indigo-500 dark:text-indigo-300">
                  {{ region.eyebrow }}
                </p>
                <div class="mt-1 flex flex-wrap items-end justify-between gap-2">
                  <div>
                    <h2 class="text-xl font-black text-gray-950 dark:text-white">{{ region.title }}</h2>
                    <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
                      {{ region.description }}
                    </p>
                  </div>
                  <span class="rounded-xl bg-white/80 px-3 py-1.5 text-xs font-semibold text-indigo-700 shadow-sm dark:bg-dark-800/80 dark:text-indigo-300">
                    {{ t('modelPlaza.filters.modelCount', { count: region.modelCount }) }}
                  </span>
                </div>
              </div>
            </header>

            <div class="space-y-5">
              <PlazaGroupSection
                v-for="section in region.sections"
                :key="sectionKey(section)"
                :provider="section.provider"
                :provider-name="section.providerName"
                :provider-note="section.providerNote"
                :logo-key="section.logoKey"
                :logo-url="section.logoUrl"
                :currency="section.currency"
                :billing-mode="section.billingMode"
                :models="section.models"
                :section-id="sectionAnchor(section)"
              />
            </div>
          </section>
        </div>
        <div
          v-else
          class="rounded-2xl border border-dashed border-gray-300 bg-white/60 px-5 py-16 text-center text-sm text-gray-500 dark:border-dark-600 dark:bg-dark-900/30 dark:text-dark-400"
        >
          {{ filtersActive ? t('modelPlaza.noSearchResult') : t('modelPlaza.empty') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlazaFilterBar from './PlazaFilterBar.vue'
import PlazaGroupSection from './PlazaGroupSection.vue'
import type {
  DisplayPriceCurrency,
  DisplayPriceModel,
  ModelPricesResponse
} from '@/api/modelPrices'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  type BillingMode
} from '@/constants/channel'
import { platformLabel } from '@/utils/platformColors'

type BillingFilter = 'all' | BillingMode

interface BillingOption {
  value: BillingFilter
  label: string
  count: number
}

interface ProviderOption {
  value: string
  label: string
  count: number
  logoKey: string
  logoUrl: string
}

interface PricingSection {
  provider: string
  providerName: string
  providerNote: string
  logoKey: string
  logoUrl: string
  currency: DisplayPriceCurrency
  billingMode: BillingMode
  models: DisplayPriceModel[]
}

interface BillingRegion {
  billingMode: BillingMode
  eyebrow: string
  title: string
  description: string
  modelCount: number
  sections: PricingSection[]
}

const props = withDefaults(
  defineProps<{
    response: ModelPricesResponse | null
    loading: boolean
    error?: boolean
    refreshing?: boolean
    lastUpdated?: Date | null
    embedded?: boolean
  }>(),
  {
    error: false,
    refreshing: false,
    lastUpdated: null,
    embedded: false
  }
)

defineEmits<{ refresh: [] }>()

const { t, locale } = useI18n()
const selectedBillingMode = ref<BillingFilter>('all')
const selectedProvider = ref('all')
const searchQuery = ref('')
const resultsEl = ref<HTMLElement | null>(null)

const visibleProviders = computed(() =>
  (props.response?.providers ?? [])
    .map((provider) => ({
      ...provider,
      models: provider.models.filter((model) => model.configured && model.enabled)
    }))
    .filter((provider) => provider.models.length > 0)
)

const allModels = computed(() => visibleProviders.value.flatMap((provider) => provider.models))
const totalModelCount = computed(() => allModels.value.length)

function countBillingMode(mode: BillingMode): number {
  return allModels.value.filter((model) => model.billing_mode === mode).length
}

const billingOptions = computed<BillingOption[]>(() => {
  const options: BillingOption[] = [
    { value: 'all', label: t('modelPlaza.filters.all'), count: totalModelCount.value },
    { value: BILLING_MODE_TOKEN, label: t('modelPlaza.filters.token'), count: countBillingMode(BILLING_MODE_TOKEN) },
    { value: BILLING_MODE_PER_REQUEST, label: t('modelPlaza.filters.perRequest'), count: countBillingMode(BILLING_MODE_PER_REQUEST) },
    { value: BILLING_MODE_IMAGE, label: t('modelPlaza.filters.image'), count: countBillingMode(BILLING_MODE_IMAGE) },
    { value: BILLING_MODE_VIDEO, label: t('modelPlaza.filters.video'), count: countBillingMode(BILLING_MODE_VIDEO) }
  ]
  return options.filter((option) => option.value === 'all' || option.count > 0)
})

const providerOptions = computed<ProviderOption[]>(() =>
  visibleProviders.value
    .map((provider) => ({
      value: provider.provider,
      label: provider.display_name || platformLabel(provider.provider),
      count: provider.models.length,
      logoKey: provider.logo_key || provider.provider,
      logoUrl: provider.logo_url || ''
    }))
)

function modelMatches(model: DisplayPriceModel): boolean {
  if (selectedProvider.value !== 'all' && model.provider !== selectedProvider.value) return false
  if (selectedBillingMode.value !== 'all' && model.billing_mode !== selectedBillingMode.value) return false
  const query = searchQuery.value.trim().toLowerCase()
  return !query || model.model_name.toLowerCase().includes(query)
}

const billingOrder: Record<string, number> = {
  [BILLING_MODE_TOKEN]: 0,
  [BILLING_MODE_PER_REQUEST]: 1,
  [BILLING_MODE_IMAGE]: 2,
  [BILLING_MODE_VIDEO]: 3
}

const filteredSections = computed<PricingSection[]>(() => {
  const sections: PricingSection[] = []
  for (const provider of visibleProviders.value) {
    const groups = new Map<string, { billingMode: BillingMode; currency: DisplayPriceCurrency; models: DisplayPriceModel[] }>()
    for (const model of provider.models.filter(modelMatches)) {
      const key = `${model.billing_mode}:${model.currency}`
      const group = groups.get(key) ?? {
        billingMode: model.billing_mode,
        currency: model.currency,
        models: []
      }
      group.models.push(model)
      groups.set(key, group)
    }
    for (const { billingMode, currency, models } of groups.values()) {
      sections.push({
        provider: provider.provider,
        providerName: provider.display_name || platformLabel(provider.provider),
        providerNote: provider.provider_note || '',
        logoKey: provider.logo_key || provider.provider,
        logoUrl: provider.logo_url || '',
        currency,
        billingMode,
        models
      })
    }
  }
  return sections.sort(
    (a, b) => (billingOrder[a.billingMode] ?? 99) - (billingOrder[b.billingMode] ?? 99)
  )
})

function billingRegionCopy(mode: BillingMode): Pick<BillingRegion, 'eyebrow' | 'title' | 'description'> {
  if (mode === BILLING_MODE_TOKEN) {
    return {
      eyebrow: t('modelPlaza.sections.token.eyebrow'),
      title: t('modelPlaza.sections.token.title'),
      description: t('modelPlaza.sections.token.description')
    }
  }
  if (mode === BILLING_MODE_PER_REQUEST) {
    return {
      eyebrow: t('modelPlaza.sections.perRequest.eyebrow'),
      title: t('modelPlaza.sections.perRequest.title'),
      description: t('modelPlaza.sections.perRequest.description')
    }
  }
  if (mode === BILLING_MODE_IMAGE) {
    return {
      eyebrow: t('modelPlaza.sections.image.eyebrow'),
      title: t('modelPlaza.sections.image.title'),
      description: t('modelPlaza.sections.image.description')
    }
  }
  return {
    eyebrow: t('modelPlaza.sections.video.eyebrow'),
    title: t('modelPlaza.sections.video.title'),
    description: t('modelPlaza.sections.video.description')
  }
}

const filteredBillingRegions = computed<BillingRegion[]>(() => {
  const sectionsByMode = new Map<BillingMode, PricingSection[]>()
  for (const section of filteredSections.value) {
    const sections = sectionsByMode.get(section.billingMode) ?? []
    sections.push(section)
    sectionsByMode.set(section.billingMode, sections)
  }

  return [...sectionsByMode.entries()]
    .sort(([left], [right]) => (billingOrder[left] ?? 99) - (billingOrder[right] ?? 99))
    .map(([billingMode, sections]) => ({
      billingMode,
      ...billingRegionCopy(billingMode),
      modelCount: sections.reduce((sum, section) => sum + section.models.length, 0),
      sections
    }))
})

const currentResultCount = computed(() => filteredSections.value.reduce((sum, section) => sum + section.models.length, 0))
const filtersActive = computed(
  () => selectedBillingMode.value !== 'all' || selectedProvider.value !== 'all' || searchQuery.value.trim() !== ''
)

const lastUpdatedLabel = computed(() => {
  const value = props.response?.updated_at ? new Date(props.response.updated_at) : props.lastUpdated
  if (!value || Number.isNaN(value.getTime())) return t('modelPlaza.filters.waitingForUpdate')
  return new Intl.DateTimeFormat(locale.value, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  }).format(value)
})

function sectionKey(section: PricingSection): string {
  return `${section.billingMode}:${section.provider}:${section.currency}`
}

function sectionAnchor(section: PricingSection): string {
  return `model-price-${section.billingMode}-${section.currency.toLowerCase()}-${section.provider.replace(/[^a-z0-9_-]/gi, '-')}`
}

function scrollToResults(): void {
  void nextTick(() => resultsEl.value?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
}

function updateBillingMode(value: string): void {
  selectedBillingMode.value = value as BillingFilter
  scrollToResults()
}

function updateProvider(value: string): void {
  selectedProvider.value = value
  scrollToResults()
}

function resetFilters(): void {
  selectedBillingMode.value = 'all'
  selectedProvider.value = 'all'
  searchQuery.value = ''
}

watch(billingOptions, (options) => {
  if (!options.some((option) => option.value === selectedBillingMode.value)) selectedBillingMode.value = 'all'
})

watch(providerOptions, (providers) => {
  if (selectedProvider.value !== 'all' && !providers.some((provider) => provider.value === selectedProvider.value)) {
    selectedProvider.value = 'all'
  }
})
</script>
