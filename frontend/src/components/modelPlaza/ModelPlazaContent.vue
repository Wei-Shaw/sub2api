<template>
  <div class="space-y-5">
    <div v-if="!embedded">
      <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white sm:text-3xl">{{ t('modelPlaza.title') }}</h1>
      <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">{{ t('modelPlaza.description') }}</p>
    </div>

    <div
      v-if="descriptionHtml"
      class="plaza-description rounded-2xl border border-gray-100 bg-white px-5 py-4 text-sm shadow-card dark:border-dark-700/50 dark:bg-dark-800/50"
      v-html="descriptionHtml"
    ></div>

    <p v-if="!isAuthenticated" class="flex items-center gap-1.5 text-xs text-gray-400 dark:text-dark-500">
      <Icon name="infoCircle" size="xs" class="h-3.5 w-3.5" />
      {{ t('modelPlaza.anonymousHint') }}
    </p>

    <div v-if="loading" class="flex min-h-[240px] items-center justify-center">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600 dark:border-primary-400/25 dark:border-t-primary-400"></div>
    </div>
    <div v-else-if="error" role="alert" class="rounded-2xl border border-red-200 bg-red-50 px-5 py-8 text-center text-sm text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
      <p>{{ t('modelPlaza.loadFailed') }}</p>
      <button type="button" class="mt-4 rounded-lg bg-red-600 px-4 py-2 font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2" @click="$emit('retry')">
        {{ t('modelPlaza.retry') }}
      </button>
    </div>
    <template v-else>
      <div class="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-gray-100 bg-white p-2 shadow-card dark:border-dark-700/50 dark:bg-dark-800/50">
        <div class="flex rounded-xl bg-gray-100 p-1 dark:bg-dark-900/70" role="group" :aria-label="t('modelPlaza.view.label')">
          <button
            v-for="item in viewOptions"
            :key="item.value"
            type="button"
            :aria-pressed="view === item.value"
            class="rounded-lg px-3 py-2 text-sm font-semibold transition focus:outline-none focus:ring-2 focus:ring-primary-500"
            :class="view === item.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-900 dark:text-dark-400 dark:hover:text-white'"
            @click="view = item.value"
          >
            {{ t(item.label) }}
          </button>
        </div>
        <p class="px-2 text-xs text-gray-500 dark:text-dark-400" aria-live="polite">
          {{ t('modelPlaza.resultCount', { count: resultCount }) }}
        </p>
      </div>

      <PlazaFilterBar
        :platforms="platforms"
        :groups="groupOptions"
        :rates="rates"
        :platform="selectedPlatform"
        :group-id="selectedGroupId"
        :rate="selectedRate"
        :search="searchQuery"
        :sort="sort"
        @update:platform="selectedPlatform = $event"
        @update:group-id="selectedGroupId = $event"
        @update:rate="selectedRate = $event"
        @update:search="searchQuery = $event"
        @update:sort="sort = $event"
        @clear="clearFilters"
      />

      <div v-if="mutationError" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
        {{ mutationError }}
      </div>

      <section v-if="view === 'discover'" aria-labelledby="model-plaza-discover-heading">
        <h2 id="model-plaza-discover-heading" class="sr-only">{{ t('modelPlaza.view.discover') }}</h2>
        <div v-if="aggregatedModels.length" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <article
            v-for="model in aggregatedModels"
            :key="model.key"
            class="rounded-2xl border border-gray-100 bg-white p-5 shadow-card transition dark:border-dark-700/50 dark:bg-dark-800/50"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">{{ model.platform }}</p>
                <h3 class="mt-1 break-words font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ model.name }}</h3>
              </div>
            </div>

            <div class="mt-4 flex items-center justify-between gap-2 text-xs text-gray-500 dark:text-dark-400">
              <span>{{ t('modelPlaza.offerCount', { count: model.offers.length }) }}</span>
              <span class="font-mono">×{{ formatRate(model.offers[0]?.rate ?? 1) }}</span>
            </div>
            <div class="mt-3 flex flex-wrap gap-1.5">
              <span v-for="offer in model.offers.slice(0, 3)" :key="`${model.key}-${offer.group.id}`" class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-300">
                {{ offer.group.name }}
              </span>
              <span v-if="model.offers.length > 3" class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-500 dark:bg-dark-700 dark:text-dark-400">
                +{{ model.offers.length - 3 }}
              </span>
            </div>

            <details class="mt-4 rounded-xl border border-gray-100 bg-gray-50/70 dark:border-dark-700/60 dark:bg-dark-900/30">
              <summary class="flex min-h-10 cursor-pointer list-none items-center justify-between gap-3 px-3 py-2 text-xs font-semibold text-gray-700 outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-200">
                <span>{{ t('modelPlaza.offerDetails') }}</span>
                <span class="text-gray-400 dark:text-dark-500">{{ t('modelPlaza.offerCount', { count: model.offers.length }) }}</span>
              </summary>
              <div class="space-y-2 border-t border-gray-100 px-3 py-3 dark:border-dark-700/60">
                <div v-for="offer in model.offers" :key="`${model.key}-detail-${offer.group.id}`" class="rounded-lg bg-white px-3 py-2 dark:bg-dark-800/70">
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <span class="text-xs font-semibold text-gray-800 dark:text-dark-100">{{ offer.group.name }}</span>
                    <span class="font-mono text-xs text-gray-500 dark:text-dark-400">{{ t('modelPlaza.offerRate', { rate: formatRate(offer.rate) }) }}</span>
                  </div>
                  <p v-if="offer.group.description" class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ offer.group.description }}</p>
                </div>
              </div>
            </details>

            <div class="mt-5 flex flex-wrap items-center gap-2">
              <button type="button" class="inline-flex min-h-10 items-center rounded-lg border border-gray-200 px-3 py-2 text-xs font-semibold text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:border-dark-600 dark:text-dark-200 dark:hover:bg-dark-700" @click="copyModelId(model)">
                {{ copyKey === model.key ? t('modelPlaza.copy.copied') : t('modelPlaza.copy.id') }}
              </button>
            </div>
          </article>
        </div>
        <div v-else class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
          {{ searchActive ? t('modelPlaza.noSearchResult') : t('modelPlaza.empty') }}
        </div>
      </section>

      <section v-else aria-labelledby="model-plaza-pricing-heading" class="space-y-5">
        <h2 id="model-plaza-pricing-heading" class="sr-only">{{ t('modelPlaza.view.pricing') }}</h2>
        <div v-if="filteredGroups.length" class="space-y-5">
          <PlazaGroupSection v-for="group in filteredGroups" :key="group.id" :group="group" />
        </div>
        <div v-else class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
          {{ t('modelPlaza.noSearchResult') }}
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import PlazaFilterBar from './PlazaFilterBar.vue'
import PlazaGroupSection from './PlazaGroupSection.vue'
import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'
import { aggregatePlazaModels, sortAggregatedPlazaModels, type PlazaSort } from '@/api/modelPlaza'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  response: ModelPlazaResponse | null
  loading: boolean
  error?: boolean
  embedded?: boolean
}>()

defineEmits<{ retry: [] }>()

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const isAuthenticated = computed(() => authStore.isAuthenticated)

type PlazaView = 'discover' | 'pricing'
const viewOptions: Array<{ value: PlazaView; label: string }> = [
  { value: 'discover', label: 'modelPlaza.view.discover' },
  { value: 'pricing', label: 'modelPlaza.view.pricing' }
]

function queryString(key: string): string {
  const value = route.query[key]
  return typeof value === 'string' ? value : ''
}
function queryNumber(key: string, fallback: number | 'all'): number | 'all' {
  const value = Number(queryString(key))
  return Number.isFinite(value) && value > 0 ? value : fallback
}
function querySort(): PlazaSort {
  const value = queryString('sort')
  return value === 'name' || value === 'platform' || value === 'offers' ? value : 'default'
}

const view = ref<PlazaView>(queryString('view') === 'pricing' ? 'pricing' : 'discover')
const selectedPlatform = ref(queryString('platform') || 'all')
const selectedGroupId = ref<number | 'all'>(queryNumber('group', 'all'))
const selectedRate = ref<number | 'all'>(queryNumber('rate', 'all'))
const searchQuery = ref(queryString('search'))
const sort = ref<PlazaSort>(querySort())
const copyKey = ref('')
const mutationError = ref('')

const descriptionHtml = computed(() => {
  const md = props.response?.description?.trim()
  return md ? DOMPurify.sanitize(marked.parse(md) as string) : ''
})

function effectiveRate(group: ModelPlazaGroup): number {
  return group.user_rate_multiplier ?? group.rate_multiplier
}
function modelPlatform(group: ModelPlazaGroup, model: PlazaModel): string {
  return model.platform || group.platform
}
function formatRate(rate: number): string {
  return Number.isInteger(rate) ? String(rate) : rate.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
}

const platforms = computed(() => [...new Set((props.response?.groups ?? []).flatMap((group) => group.models.map((model) => modelPlatform(group, model))).filter(Boolean))].sort())
const groupOptions = computed(() => (props.response?.groups ?? []).map((group) => ({
  id: group.id,
  name: group.name,
  platform: group.platform,
  modelPlatforms: [...new Set(group.models.map((model) => modelPlatform(group, model)))],
  rate: effectiveRate(group)
})))
const rates = computed(() => [...new Set((props.response?.groups ?? []).map(effectiveRate))].sort((a, b) => a - b))
const searchActive = computed(() => searchQuery.value.trim() !== '')

const filteredGroups = computed(() => {
  let groups = props.response?.groups ?? []
  if (selectedGroupId.value !== 'all') groups = groups.filter((group) => group.id === selectedGroupId.value)
  if (selectedRate.value !== 'all') groups = groups.filter((group) => effectiveRate(group) === selectedRate.value)
  const query = searchQuery.value.trim().toLowerCase()

  return groups
    .map((group) => {
      const models = group.models.filter((model) => {
        const platform = modelPlatform(group, model)
        const matchesSearch = !query || model.name.toLowerCase().includes(query) || platform.toLowerCase().includes(query)
        const matchesPlatform = selectedPlatform.value === 'all' || platform === selectedPlatform.value
        return matchesPlatform && matchesSearch
      })
      return {
        ...group,
        models: sort.value === 'default' ? models : [...models].sort((a, b) => a.name.localeCompare(b.name))
      }
    })
    .filter((group) => group.models.length > 0)
    .sort((a, b) => {
      if (sort.value === 'name') return a.name.localeCompare(b.name)
      if (sort.value === 'platform') return a.platform.localeCompare(b.platform) || a.name.localeCompare(b.name)
      if (sort.value === 'offers') return b.models.length - a.models.length || a.name.localeCompare(b.name)
      return 0
    })
})

const aggregatedModels = computed(() => sortAggregatedPlazaModels(aggregatePlazaModels(filteredGroups.value), sort.value))

const resultCount = computed(() => (view.value === 'discover' ? aggregatedModels.value.length : filteredGroups.value.reduce((sum, group) => sum + group.models.length, 0)))

function clearFilters() {
  selectedPlatform.value = 'all'
  selectedGroupId.value = 'all'
  selectedRate.value = 'all'
  searchQuery.value = ''
  sort.value = 'default'
}

watch([view, selectedPlatform, selectedGroupId, selectedRate, searchQuery, sort], () => {
  const query = { ...route.query } as Record<string, string>
  const values: Record<string, string | undefined> = {
    view: view.value === 'discover' ? undefined : view.value,
    platform: selectedPlatform.value === 'all' ? undefined : selectedPlatform.value,
    group: selectedGroupId.value === 'all' ? undefined : String(selectedGroupId.value),
    rate: selectedRate.value === 'all' ? undefined : String(selectedRate.value),
    search: searchQuery.value.trim() || undefined,
    sort: sort.value === 'default' ? undefined : sort.value
  }
  for (const [key, value] of Object.entries(values)) {
    if (value) query[key] = value
    else delete query[key]
  }
  void router.replace({ query })
})

function syncStateFromRouteQuery() {
  const nextView: PlazaView = queryString('view') === 'pricing' ? 'pricing' : 'discover'
  const nextPlatform = queryString('platform') || 'all'
  const nextGroupId = queryNumber('group', 'all')
  const nextRate = queryNumber('rate', 'all')
  const nextSearch = queryString('search')
  const nextSort = querySort()
  if (view.value !== nextView) view.value = nextView
  if (selectedPlatform.value !== nextPlatform) selectedPlatform.value = nextPlatform
  if (selectedGroupId.value !== nextGroupId) selectedGroupId.value = nextGroupId
  if (selectedRate.value !== nextRate) selectedRate.value = nextRate
  if (searchQuery.value !== nextSearch) searchQuery.value = nextSearch
  if (sort.value !== nextSort) sort.value = nextSort
}

watch(() => route.query, syncStateFromRouteQuery, { deep: true })

watch(rates, (available) => {
  if (selectedRate.value !== 'all' && !available.includes(selectedRate.value)) selectedRate.value = 'all'
})

async function copyModelId(model: { key: string }) {
  try {
    await navigator.clipboard.writeText(model.key)
    copyKey.value = model.key
    window.setTimeout(() => {
      if (copyKey.value === model.key) copyKey.value = ''
    }, 1600)
  } catch {
    mutationError.value = t('modelPlaza.copy.failed')
  }
}
</script>

<style scoped>
.plaza-description { line-height: 1.7; overflow-wrap: anywhere; }
.plaza-description :deep(h1), .plaza-description :deep(h2), .plaza-description :deep(h3) { @apply mb-2 mt-3 font-semibold text-gray-900 first:mt-0 dark:text-white; }
.plaza-description :deep(p) { @apply mb-2 text-gray-700 last:mb-0 dark:text-dark-200; }
.plaza-description :deep(a) { @apply text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300; }
.plaza-description :deep(ul) { @apply mb-2 list-disc pl-5; }
.plaza-description :deep(ol) { @apply mb-2 list-decimal pl-5; }
.plaza-description :deep(li) { @apply mb-0.5 text-gray-700 dark:text-dark-200; }
.plaza-description :deep(code) { @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs dark:bg-dark-800; }
.plaza-description :deep(blockquote) { @apply my-2 border-l-4 border-gray-300 pl-3 text-gray-600 dark:border-dark-600 dark:text-dark-300; }
</style>
