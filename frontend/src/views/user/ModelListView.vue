<template>
  <AppLayout>
    <div class="model-list-page">
      <section class="summary-grid" aria-label="model catalog summary">
        <div class="summary-card">
          <div class="summary-icon summary-icon-blue">
            <Icon name="database" size="md" />
          </div>
          <div>
            <p>{{ t('modelList.stats.total') }}</p>
            <strong>{{ models.length.toLocaleString() }}</strong>
          </div>
        </div>
        <div class="summary-card">
          <div class="summary-icon summary-icon-emerald">
            <Icon name="badge" size="md" />
          </div>
          <div>
            <p>{{ t('modelList.stats.mainstream') }}</p>
            <strong>{{ categoryCounts.mainstream.toLocaleString() }}</strong>
          </div>
        </div>
        <div class="summary-card">
          <div class="summary-icon summary-icon-amber">
            <Icon name="globe" size="md" />
          </div>
          <div>
            <p>{{ t('modelList.stats.providers') }}</p>
            <strong>{{ providerOptions.length.toLocaleString() }}</strong>
          </div>
        </div>
        <div class="summary-card">
          <div class="summary-icon summary-icon-slate">
            <Icon name="dollar" size="md" />
          </div>
          <div>
            <p>{{ t('modelList.stats.withPricing') }}</p>
            <strong>{{ pricedModelCount.toLocaleString() }}</strong>
          </div>
        </div>
      </section>

      <section class="catalog-toolbar">
        <div class="search-field">
          <Icon name="search" size="md" />
          <input
            v-model="searchQuery"
            type="search"
            class="input"
            :placeholder="t('modelList.searchPlaceholder')"
          />
        </div>

        <select v-model="selectedProvider" class="input toolbar-select" :aria-label="t('modelList.providerFilter')">
          <option value="">{{ t('modelList.providerAll') }}</option>
          <option v-for="provider in providerOptions" :key="provider" :value="provider">
            {{ provider }}
          </option>
        </select>

        <select v-model="selectedSort" class="input toolbar-select" :aria-label="t('modelList.sort.label')">
          <option value="recommended">{{ t('modelList.sort.recommended') }}</option>
          <option value="inputAsc">{{ t('modelList.sort.inputAsc') }}</option>
          <option value="outputAsc">{{ t('modelList.sort.outputAsc') }}</option>
          <option value="contextDesc">{{ t('modelList.sort.contextDesc') }}</option>
        </select>

        <button type="button" class="btn btn-secondary toolbar-refresh" :disabled="loading" @click="loadModels">
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          <span>{{ t('common.refresh') }}</span>
        </button>
      </section>

      <section class="category-tabs" :aria-label="t('modelList.categoryLabel')">
        <button
          v-for="category in categoryOptions"
          :key="category.key"
          type="button"
          :class="{ active: selectedCategory === category.key }"
          @click="selectedCategory = category.key"
        >
          <span>{{ category.label }}</span>
          <small>{{ category.count.toLocaleString() }}</small>
        </button>
      </section>

      <section class="catalog-status-row">
        <span>
          {{ t('modelList.showingCount', { shown: displayedModels.length, total: filteredModels.length }) }}
        </span>
        <span v-if="lastUpdated">{{ t('modelList.sourceUpdated', { time: lastUpdatedLabel }) }}</span>
      </section>

      <div v-if="loading && models.length === 0" class="state-panel">
        <div class="state-spinner"></div>
        <p>{{ t('modelList.loading') }}</p>
      </div>

      <div v-else-if="errorMessage" class="state-panel error">
        <Icon name="exclamationTriangle" size="lg" />
        <p>{{ errorMessage }}</p>
        <button type="button" class="btn btn-secondary btn-sm" @click="loadModels">
          <Icon name="refresh" size="sm" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <div v-else-if="filteredModels.length === 0" class="state-panel">
        <Icon name="inbox" size="xl" />
        <p>{{ t('modelList.empty') }}</p>
      </div>

      <template v-else>
        <section class="model-grid">
          <article v-for="model in displayedModels" :key="model.model_id" class="model-card">
            <header class="model-card-header">
              <div class="model-title-group">
                <div class="model-icon-shell" aria-hidden="true">
                  <ModelIcon :model="model.model_id || model.model_name" size="34px" />
                </div>
                <div class="min-w-0">
                  <div class="model-provider-row">
                    <span class="provider-pill">{{ inferProvider(model) }}</span>
                    <span v-if="model.context_length" class="context-pill">
                      {{ formatTokenCount(model.context_length) }}
                    </span>
                  </div>
                  <h2>{{ model.model_name || model.model_id }}</h2>
                </div>
              </div>
              <div class="model-card-actions">
                <span v-if="isNewModel(model)" class="new-badge">{{ t('modelList.newBadge') }}</span>
                <button
                  type="button"
                  class="copy-id-button"
                  :title="t('modelList.copyModelId')"
                  @click="copyModelId(model.model_id)"
                >
                  <Icon name="copy" size="sm" />
                  <span>{{ t('modelList.copyId') }}</span>
                </button>
              </div>
            </header>

            <div class="model-id-row">
              <span>{{ t('modelList.modelId') }}</span>
              <code>{{ model.model_id }}</code>
            </div>

            <p class="model-description">
              {{ model.desc || t('modelList.noDescription') }}
            </p>

            <div class="tag-row">
              <span v-for="type in splitCSV(model.types)" :key="type" class="type-badge" :class="typeBadgeClass(type)">
                {{ formatType(type) }}
              </span>
              <span v-if="splitCSV(model.types).length === 0" class="type-badge muted">
                {{ t('modelList.types.unknown') }}
              </span>
            </div>

            <div class="pricing-block">
              <div v-if="pricingEntries(model).length > 0" class="pricing-summary">
                <span class="pricing-prefix">{{ t('modelList.pricing.billingPrefix') }}</span>
                <span v-for="entry in pricingEntries(model)" :key="entry.key" class="pricing-item">
                  {{ entry.label }} {{ formatPrice(entry.value) }} / 1M tokens
                </span>
              </div>
              <p v-else class="no-pricing">{{ t('modelList.pricing.empty') }}</p>
            </div>

            <dl class="model-meta-grid">
              <div>
                <dt>{{ t('modelList.contextLength') }}</dt>
                <dd>{{ formatTokenCount(model.context_length) }}</dd>
              </div>
              <div>
                <dt>{{ t('modelList.maxOutput') }}</dt>
                <dd>{{ formatTokenCount(model.max_output) }}</dd>
              </div>
            </dl>

            <div v-if="splitCSV(model.input_modalities).length > 0" class="compact-tags">
              <span>{{ t('modelList.modalitiesLabel') }}</span>
              <b v-for="item in splitCSV(model.input_modalities).slice(0, 4)" :key="item">
                {{ formatModality(item) }}
              </b>
            </div>

            <div v-if="splitCSV(model.features).length > 0" class="compact-tags">
              <span>{{ t('modelList.featuresLabel') }}</span>
              <b v-for="item in splitCSV(model.features).slice(0, 4)" :key="item">
                {{ formatFeature(item) }}
              </b>
            </div>
          </article>
        </section>

        <div v-if="displayedModels.length < filteredModels.length" class="load-more-row">
          <button type="button" class="btn btn-secondary" @click="showMore">
            {{ t('modelList.loadMore') }}
          </button>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import modelCatalogAPI, { type ModelCatalogItem } from '@/api/modelCatalog'
import { useAppStore } from '@/stores/app'

type CatalogRow = ModelCatalogItem & { sourceIndex: number }
type CategoryKey = 'mainstream' | 'all' | 'llm' | 'image' | 'video' | 'audio' | 'embedding'
type SortKey = 'recommended' | 'inputAsc' | 'outputAsc' | 'contextDesc'

const { t } = useI18n()
const appStore = useAppStore()

const CATEGORY_KEYS: CategoryKey[] = ['mainstream', 'all', 'llm', 'image', 'video', 'audio', 'embedding']
const PAGE_SIZE = 36

const models = ref<CatalogRow[]>([])
const loading = ref(false)
const errorMessage = ref('')
const lastUpdated = ref<Date | null>(null)
const searchQuery = ref('')
const selectedCategory = ref<CategoryKey>('mainstream')
const selectedProvider = ref('')
const selectedSort = ref<SortKey>('recommended')
const visibleCount = ref(PAGE_SIZE)

const mainstreamNeedles = [
  'gpt',
  'o1',
  'o3',
  'o4',
  'claude',
  'gemini',
  'grok',
  'deepseek',
  'qwen',
  'kimi',
  'moonshot',
  'doubao',
  'seed',
  'glm',
  'llama',
  'mistral',
  'ernie',
  'yi-',
  'minimax',
  'cohere',
  'xiao',
  'whisper',
  'tts'
]

const pricedModelCount = computed(() => models.value.filter(hasAnyPricing).length)

const providerOptions = computed(() => {
  const providers = new Set(models.value.map(inferProvider))
  return Array.from(providers).sort((a, b) => a.localeCompare(b))
})

const categoryCounts = computed<Record<CategoryKey, number>>(() => {
  const counts = {} as Record<CategoryKey, number>
  for (const key of CATEGORY_KEYS) {
    counts[key] = models.value.filter((model) => matchesCategory(model, key)).length
  }
  return counts
})

const categoryOptions = computed(() =>
  CATEGORY_KEYS.map((key) => ({
    key,
    label: t(`modelList.categories.${key}`),
    count: categoryCounts.value[key]
  }))
)

const filteredModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const list = models.value.filter((model) => {
    if (!matchesCategory(model, selectedCategory.value)) return false
    if (selectedProvider.value && inferProvider(model) !== selectedProvider.value) return false
    if (!q) return true
    return searchableText(model).includes(q)
  })

  return [...list].sort(compareModels)
})

const displayedModels = computed(() => filteredModels.value.slice(0, visibleCount.value))

const lastUpdatedLabel = computed(() => {
  if (!lastUpdated.value) return ''
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit'
  }).format(lastUpdated.value)
})

watch([searchQuery, selectedCategory, selectedProvider, selectedSort], () => {
  visibleCount.value = PAGE_SIZE
})

async function loadModels() {
  loading.value = true
  errorMessage.value = ''
  try {
    const list = await modelCatalogAPI.list()
    models.value = list.map((model, index) => ({
      ...model,
      sourceIndex: index
    }))
    lastUpdated.value = new Date()
  } catch (err) {
    const message = err instanceof Error ? err.message : t('modelList.loadFailed')
    errorMessage.value = message
    appStore.showError(message)
  } finally {
    loading.value = false
  }
}

function showMore() {
  visibleCount.value += PAGE_SIZE
}

async function copyModelId(modelId: string) {
  try {
    await navigator.clipboard.writeText(modelId)
    appStore.showSuccess(t('modelList.copySuccess'))
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}

function splitCSV(value?: string): string[] {
  return (value || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

function searchableText(model: CatalogRow): string {
  return [
    model.model_id,
    model.model_name,
    model.desc,
    model.types,
    model.features,
    model.input_modalities,
    model.endpoints,
    inferProvider(model)
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
}

function inferProvider(model: Pick<ModelCatalogItem, 'model_id' | 'model_name'>): string {
  const text = `${model.model_id || ''} ${model.model_name || ''}`.toLowerCase()
  if (text.includes('claude')) return 'Anthropic'
  if (text.includes('gemini')) return 'Google'
  if (text.includes('grok')) return 'xAI'
  if (text.includes('deepseek')) return 'DeepSeek'
  if (text.includes('qwen')) return 'Qwen'
  if (text.includes('kimi') || text.includes('moonshot')) return 'Moonshot'
  if (text.includes('doubao') || text.includes('seed')) return 'ByteDance'
  if (text.includes('llama') || text.includes('meta')) return 'Meta'
  if (text.includes('mistral')) return 'Mistral'
  if (text.includes('glm') || text.includes('chatglm')) return 'Zhipu'
  if (text.includes('ernie') || text.includes('qianfan')) return 'Baidu'
  if (text.includes('minimax')) return 'MiniMax'
  if (text.includes('yi-') || text.includes(' yi ')) return '01.AI'
  if (text.includes('cohere')) return 'Cohere'
  if (text.includes('xiaomi') || text.includes('mimo')) return 'Xiaomi'
  if (text.includes('gpt') || text.includes('openai') || text.includes('whisper') || text.includes('tts-') || /^o\d/.test(text)) return 'OpenAI'
  return t('modelList.unknownProvider')
}

function matchesCategory(model: CatalogRow, category: CategoryKey): boolean {
  if (category === 'all') return true
  const types = splitCSV(model.types).map((item) => item.toLowerCase())
  const id = model.model_id.toLowerCase()
  if (category === 'mainstream') return isMainstreamModel(model)
  if (category === 'llm') return types.includes('llm')
  if (category === 'image') return types.includes('image_generation') || id.includes('image')
  if (category === 'video') return types.includes('video') || id.includes('video')
  if (category === 'audio') return types.includes('stt') || types.includes('tts') || id.includes('whisper') || id.includes('tts')
  return types.includes('embedding') || types.includes('rerank') || id.includes('embedding') || id.includes('rerank')
}

function isMainstreamModel(model: CatalogRow): boolean {
  const text = `${model.model_id} ${model.model_name}`.toLowerCase()
  return mainstreamNeedles.some((needle) => text.includes(needle)) || model.sourceIndex < 120
}

function isNewModel(model: CatalogRow): boolean {
  return model.sourceIndex < 20
}

function compareModels(a: CatalogRow, b: CatalogRow): number {
  if (selectedSort.value === 'inputAsc') {
    return compareNullableNumber(priceValue(a, 'input'), priceValue(b, 'input')) || a.sourceIndex - b.sourceIndex
  }
  if (selectedSort.value === 'outputAsc') {
    return compareNullableNumber(priceValue(a, 'output'), priceValue(b, 'output')) || a.sourceIndex - b.sourceIndex
  }
  if (selectedSort.value === 'contextDesc') {
    return (b.context_length || 0) - (a.context_length || 0) || a.sourceIndex - b.sourceIndex
  }
  return a.sourceIndex - b.sourceIndex
}

function compareNullableNumber(a: number | null, b: number | null): number {
  if (a === b) return 0
  if (a === null) return 1
  if (b === null) return -1
  return a - b
}

function priceValue(model: ModelCatalogItem, key: keyof NonNullable<ModelCatalogItem['pricing']>): number | null {
  const value = model.pricing?.[key]
  return typeof value === 'number' ? value : null
}

function hasAnyPricing(model: ModelCatalogItem): boolean {
  const pricing = model.pricing
  if (!pricing) return false
  return Object.values(pricing).some((value) => typeof value === 'number')
}

function pricingEntries(model: ModelCatalogItem) {
  const entries = [
    { key: 'input', label: t('modelList.pricing.input'), value: model.pricing?.input },
    { key: 'output', label: t('modelList.pricing.output'), value: model.pricing?.output },
    { key: 'cache_read', label: t('modelList.pricing.cacheRead'), value: model.pricing?.cache_read },
    { key: 'cache_write', label: t('modelList.pricing.cacheWrite'), value: model.pricing?.cache_write }
  ]
  return entries.filter((entry): entry is { key: string; label: string; value: number } => typeof entry.value === 'number')
}

function formatPrice(value: number): string {
  const formatted = new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 6
  }).format(value)
  return `$${formatted}`
}

function formatTokenCount(value?: number): string {
  if (!value) return '-'
  if (value >= 1_000_000) return `${formatCompact(value / 1_000_000)}M`
  if (value >= 1_000) return `${formatCompact(value / 1_000)}K`
  return value.toLocaleString()
}

function formatCompact(value: number): string {
  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: value >= 10 ? 0 : 1
  }).format(value)
}

function formatType(type: string): string {
  return translateToken(type, 'types')
}

function formatModality(modality: string): string {
  return translateToken(modality, 'modalities')
}

function formatFeature(feature: string): string {
  return translateToken(feature, 'features')
}

function translateToken(token: string, group: 'types' | 'modalities' | 'features'): string {
  const normalized = token.trim().toLowerCase().replace(/-/g, '_')
  const key = `modelList.${group}.${normalized}`
  const translated = t(key)
  return translated === key ? formatRawToken(token) : translated
}

function formatRawToken(token: string): string {
  return token
    .replace(/_/g, ' ')
    .replace(/-/g, ' ')
    .replace(/\b\w/g, (char: string) => char.toUpperCase())
}

function typeBadgeClass(type: string): string {
  const normalized = type.toLowerCase()
  if (normalized.includes('image')) return 'image'
  if (normalized.includes('video')) return 'video'
  if (normalized.includes('stt') || normalized.includes('tts')) return 'audio'
  if (normalized.includes('embedding') || normalized.includes('rerank')) return 'embedding'
  return 'llm'
}

onMounted(loadModels)
</script>

<style scoped>
.model-list-page {
  @apply mx-auto flex max-w-7xl flex-col gap-5;
}

.summary-grid {
  @apply grid gap-3 sm:grid-cols-2 xl:grid-cols-4;
}

.summary-card {
  @apply flex items-center gap-3 rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800;
}

.summary-card p {
  @apply text-xs font-medium text-gray-500 dark:text-dark-400;
}

.summary-card strong {
  @apply text-xl font-semibold text-gray-900 dark:text-white;
}

.summary-icon {
  @apply flex h-10 w-10 shrink-0 items-center justify-center rounded-lg;
}

.summary-icon-blue {
  @apply bg-sky-100 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300;
}

.summary-icon-emerald {
  @apply bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300;
}

.summary-icon-amber {
  @apply bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300;
}

.summary-icon-slate {
  @apply bg-slate-100 text-slate-700 dark:bg-slate-500/15 dark:text-slate-300;
}

.catalog-toolbar {
  @apply grid gap-3 rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 lg:grid-cols-[minmax(260px,1fr)_180px_180px_auto];
}

.search-field {
  @apply relative min-w-0;
}

.search-field svg {
  @apply pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500;
}

.search-field .input {
  @apply pl-10;
}

.toolbar-select {
  @apply min-w-0;
}

.toolbar-refresh {
  @apply justify-center whitespace-nowrap;
}

.category-tabs {
  @apply flex gap-2 overflow-x-auto rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900;
}

.category-tabs button {
  @apply flex shrink-0 items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-white hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white;
}

.category-tabs button.active {
  @apply bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300;
}

.category-tabs small {
  @apply rounded bg-gray-100 px-1.5 py-0.5 text-[11px] font-semibold text-gray-500 dark:bg-dark-700 dark:text-dark-300;
}

.catalog-status-row {
  @apply flex flex-wrap items-center justify-between gap-2 text-sm text-gray-500 dark:text-dark-400;
}

.state-panel {
  @apply flex min-h-72 flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-gray-300 bg-white p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300;
}

.state-panel.error {
  @apply border-red-200 text-red-600 dark:border-red-500/30 dark:text-red-300;
}

.state-spinner {
  @apply h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent;
}

.model-grid {
  @apply grid gap-4 lg:grid-cols-2 2xl:grid-cols-3;
}

.model-card {
  @apply flex min-h-[420px] flex-col rounded-lg border border-gray-200 bg-white p-5 shadow-sm transition-shadow hover:shadow-md dark:border-dark-700 dark:bg-dark-800;
}

.model-card-header {
  @apply flex items-start justify-between gap-3;
}

.model-title-group {
  @apply flex min-w-0 items-start gap-3;
}

.model-icon-shell {
  @apply flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-900 shadow-sm dark:border-dark-600 dark:bg-white dark:text-gray-900;
}

.model-provider-row {
  @apply mb-2 flex flex-wrap items-center gap-2;
}

.provider-pill,
.context-pill {
  @apply inline-flex items-center rounded-md px-2 py-0.5 text-xs font-semibold;
}

.provider-pill {
  @apply bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200;
}

.context-pill {
  @apply bg-sky-50 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300;
}

.model-card h2 {
  @apply truncate text-lg font-semibold text-gray-900 dark:text-white;
}

.copy-id-button {
  @apply inline-flex h-9 shrink-0 items-center justify-center gap-1.5 rounded-md border border-gray-200 px-3 text-sm font-medium text-gray-600 transition-colors hover:border-primary-300 hover:bg-primary-50 hover:text-primary-700 dark:border-dark-600 dark:text-dark-300 dark:hover:border-primary-500/40 dark:hover:bg-primary-500/10 dark:hover:text-primary-300;
}

.model-card-actions {
  @apply flex shrink-0 items-center gap-2;
}

.new-badge {
  @apply inline-flex h-8 items-center rounded-md border border-cyan-300 bg-cyan-50 px-2.5 text-sm font-semibold text-cyan-600 dark:border-cyan-400/50 dark:bg-cyan-400/10 dark:text-cyan-300;
}

.model-id-row {
  @apply mt-3 flex min-w-0 items-center gap-2 rounded-md bg-gray-50 px-3 py-2 text-xs dark:bg-dark-900;
}

.model-id-row span {
  @apply shrink-0 font-medium text-gray-500 dark:text-dark-400;
}

.model-id-row code {
  @apply truncate text-gray-700 dark:text-dark-200;
}

.model-description {
  @apply mt-4 min-h-[4.5rem] text-sm leading-6 text-gray-600 dark:text-dark-300;
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
}

.tag-row {
  @apply mt-4 flex flex-wrap gap-2;
}

.type-badge {
  @apply rounded-md px-2 py-0.5 text-xs font-semibold;
}

.type-badge.llm {
  @apply bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300;
}

.type-badge.image {
  @apply bg-sky-50 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300;
}

.type-badge.video {
  @apply bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300;
}

.type-badge.audio {
  @apply bg-indigo-50 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300;
}

.type-badge.embedding,
.type-badge.muted {
  @apply bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300;
}

.pricing-block {
  @apply mt-4 rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900;
}

.pricing-summary {
  @apply flex flex-wrap items-center gap-x-3 gap-y-1 text-sm leading-6 text-gray-600 dark:text-dark-300;
}

.pricing-prefix,
.pricing-item {
  @apply inline-flex items-center;
}

.pricing-prefix {
  @apply font-medium text-gray-700 dark:text-dark-200;
}

.pricing-item + .pricing-item::before {
  content: '';
  @apply mr-3 h-4 w-px bg-gray-300 dark:bg-dark-600;
}

.no-pricing {
  @apply text-sm text-gray-500 dark:text-dark-400;
}

.model-meta-grid {
  @apply mt-4 grid grid-cols-2 gap-3;
}

.model-meta-grid div {
  @apply rounded-md border border-gray-100 px-3 py-2 dark:border-dark-700;
}

.model-meta-grid dt {
  @apply text-xs text-gray-500 dark:text-dark-400;
}

.model-meta-grid dd {
  @apply mt-1 font-semibold text-gray-900 dark:text-white;
}

.compact-tags {
  @apply mt-3 flex flex-wrap items-center gap-1.5 text-xs;
}

.compact-tags span {
  @apply mr-1 font-medium text-gray-500 dark:text-dark-400;
}

.compact-tags b {
  @apply rounded bg-gray-100 px-2 py-0.5 font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-300;
}

.load-more-row {
  @apply flex justify-center pb-4 pt-1;
}
</style>
