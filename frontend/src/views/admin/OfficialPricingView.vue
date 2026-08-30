<template>
  <div class="w-full min-w-0 space-y-6 pb-8" data-testid="official-pricing-panel">
      <header
        class="overflow-hidden rounded-3xl bg-gradient-to-br from-[#101d3d] via-[#13294f] to-[#075e63] p-5 text-white shadow-[0_24px_64px_rgba(15,33,65,0.24)] sm:p-6"
      >
        <div class="flex flex-wrap items-start justify-between gap-5">
          <div class="max-w-3xl">
            <div class="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.18em] text-cyan-200">
              <Icon name="database" size="sm" />
              {{ t('admin.officialPricing.eyebrow') }}
            </div>
            <h1 class="mt-3 text-2xl font-black sm:text-3xl">{{ t('admin.officialPricing.title') }}</h1>
            <p class="mt-2 text-sm leading-6 text-slate-300">{{ t('admin.officialPricing.description') }}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl border border-white/15 bg-white/10 px-4 py-2.5 text-sm font-bold text-white transition hover:bg-white/15 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="checking"
              data-testid="official-sync-preview"
              @click="runSyncPreview"
            >
              <Icon name="refresh" size="sm" :class="checking ? 'animate-spin' : ''" />
              {{ checking ? t('admin.officialPricing.checking') : t('admin.officialPricing.check') }}
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl bg-cyan-300 px-4 py-2.5 text-sm font-black text-[#102044] transition hover:bg-cyan-200 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="loading"
              data-testid="official-pricing-refresh"
              @click="loadData"
            >
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              {{ t('admin.officialPricing.refresh') }}
            </button>
          </div>
        </div>

        <div class="mt-6 grid gap-3 md:grid-cols-2">
          <div class="flex items-start gap-2 rounded-2xl border border-cyan-200/15 bg-cyan-200/10 px-4 py-3 text-sm leading-6 text-cyan-50">
            <Icon name="infoCircle" size="sm" class="mt-1 shrink-0 text-cyan-200" />
            {{ t('admin.officialPricing.tokenOnly') }}
          </div>
          <div class="flex items-start gap-2 rounded-2xl border border-white/10 bg-white/[0.06] px-4 py-3 text-sm leading-6 text-slate-200">
            <Icon name="shield" size="sm" class="mt-1 shrink-0 text-emerald-300" />
            {{ t('admin.officialPricing.isolationNotice') }}
          </div>
        </div>
      </header>

      <section v-if="preview" class="official-panel overflow-hidden" data-testid="official-sync-panel">
        <div class="border-b border-slate-100 bg-[#f6f9fc] px-5 py-5 dark:border-dark-700 dark:bg-dark-900/40 sm:px-6">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div>
              <h2 class="text-lg font-black text-[#17264a] dark:text-white">{{ t('admin.officialPricing.sync.title') }}</h2>
              <p class="mt-1 max-w-3xl text-sm leading-6 text-slate-500 dark:text-dark-400">{{ t('admin.officialPricing.sync.hint') }}</p>
              <p v-if="preview.fetched_at" class="mt-2 text-xs font-medium text-slate-400">
                {{ t('admin.officialPricing.sync.fetchedAt', { time: formatDateTime(preview.fetched_at) }) }}
              </p>
            </div>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl bg-[#087f79] px-4 py-2.5 text-sm font-bold text-white shadow-sm transition hover:bg-[#066b67] disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="applying || selectedCandidates.length === 0"
              data-testid="apply-official-sync"
              @click="applySelected"
            >
              <Icon name="check" size="sm" />
              {{ applying ? t('admin.officialPricing.sync.applying') : t('admin.officialPricing.sync.applySelected') }}
            </button>
          </div>

          <div class="mt-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
            <div class="sync-stat">{{ t('admin.officialPricing.sync.candidates', { count: preview.items.length }) }}</div>
            <div class="sync-stat">{{ t('admin.officialPricing.sync.changed', { count: changedCandidateCount }) }}</div>
            <div class="sync-stat">{{ t('admin.officialPricing.sync.applicable', { count: applicableCandidateCount }) }}</div>
            <div class="sync-stat sync-stat-selected">{{ t('admin.officialPricing.sync.selected', { count: selectedCandidates.length }) }}</div>
          </div>

          <div
            v-if="hasAggregateCandidates"
            class="mt-4 flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-200"
            data-testid="aggregate-warning"
          >
            <Icon name="exclamationTriangle" size="sm" class="mt-1 shrink-0" />
            {{ t('admin.officialPricing.sync.unverifiedWarning') }}
          </div>
          <p v-if="preview.warning" class="mt-3 text-sm font-medium text-amber-700 dark:text-amber-300">{{ preview.warning }}</p>
        </div>

        <div v-if="preview.items.length" class="overflow-x-auto">
          <table class="w-full min-w-[1180px] text-left text-sm">
            <thead class="bg-[#eef3f8] text-xs font-bold uppercase tracking-wide text-[#60708d] dark:bg-dark-900/70 dark:text-dark-400">
              <tr>
                <th class="w-12 px-5 py-3">
                  <input
                    type="checkbox"
                    class="h-4 w-4 rounded border-slate-300 text-[#087f79] focus:ring-[#087f79]"
                    :checked="allApplicableSelected"
                    :aria-label="t('admin.officialPricing.sync.selectAll')"
                    data-testid="select-all-official-sync"
                    @change="toggleAllApplicable"
                  />
                </th>
                <th class="px-3 py-3">{{ t('admin.officialPricing.fields.model') }}</th>
                <th class="px-3 py-3">{{ t('admin.officialPricing.sync.current') }}</th>
                <th class="px-3 py-3">{{ t('admin.officialPricing.sync.proposed') }}</th>
                <th class="px-3 py-3">{{ t('admin.officialPricing.sync.evidence') }}</th>
                <th class="px-5 py-3">{{ t('admin.officialPricing.sync.status') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-100 dark:divide-dark-700/70">
              <tr
                v-for="candidate in preview.items"
                :key="candidate.model_id"
                class="align-top transition hover:bg-[#f8fbfd] dark:hover:bg-dark-700/20"
                :data-testid="`official-sync-candidate-${candidate.model_id}`"
              >
                <td class="px-5 py-4">
                  <input
                    type="checkbox"
                    class="h-4 w-4 rounded border-slate-300 text-[#087f79] focus:ring-[#087f79] disabled:cursor-not-allowed disabled:opacity-40"
                    :checked="selectedIds.includes(candidate.model_id)"
                    :disabled="!isSelectable(candidate)"
                    :data-testid="`official-sync-select-${candidate.model_id}`"
                    @change="toggleCandidate(candidate.model_id)"
                  />
                </td>
                <td class="px-3 py-4">
                  <p class="font-mono font-bold text-[#17264a] dark:text-white">{{ candidate.model_name }}</p>
                  <p class="mt-1 text-xs text-slate-500">{{ providerLabel(candidate.provider) }} · {{ candidate.currency }}</p>
                </td>
                <td class="px-3 py-4">
                  <PriceValues :values="candidate.current" :currency="candidate.currency" />
                </td>
                <td class="px-3 py-4">
                  <PriceValues
                    :values="candidate.proposed"
                    :currency="candidate.currency"
                    :comparison="candidate.current"
                    emphasized
                  />
                </td>
                <td class="px-3 py-4">
                  <div class="flex max-w-[260px] flex-col items-start gap-1.5">
                    <span class="source-badge" :class="candidate.source === 'herohao_aggregate' ? 'source-badge-unverified' : 'source-badge-trusted'">
                      {{ sourceLabel(candidate.source) }}
                    </span>
                    <span class="text-xs font-semibold text-slate-500">{{ confidenceLabel(candidate.confidence) }}</span>
                    <span v-if="candidate.source_updated_at" class="text-xs text-slate-400">
                      {{ t('admin.officialPricing.sync.sourceUpdatedAt', { time: formatDateTime(candidate.source_updated_at) }) }}
                    </span>
                    <a
                      v-if="safeSourceUrl(candidate.official_reference_url)"
                      :href="safeSourceUrl(candidate.official_reference_url) || undefined"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="inline-flex items-center gap-1 text-xs font-bold text-[#136f83] hover:underline dark:text-cyan-300"
                    >
                      {{ t('admin.officialPricing.sync.reference') }}
                      <Icon name="externalLink" size="xs" />
                    </a>
                  </div>
                </td>
                <td class="px-5 py-4">
                  <span class="status-badge" :class="candidateStatusClass(candidate)">{{ candidateStatusLabel(candidate) }}</span>
                  <p v-if="!candidate.applicable" class="mt-2 max-w-[260px] text-xs leading-5 text-rose-600 dark:text-rose-300">
                    {{ candidateReasonLabel(candidate.reason) }}
                  </p>
                  <p v-else-if="candidate.reason" class="mt-2 max-w-[260px] text-xs leading-5 text-slate-500">{{ candidate.reason }}</p>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="px-5 py-12 text-center text-sm text-slate-400">{{ t('admin.officialPricing.sync.empty') }}</p>
      </section>

      <section class="official-panel overflow-hidden">
        <div class="border-b border-slate-100 bg-[#f7f9fc] px-5 py-5 dark:border-dark-700 dark:bg-dark-900/40 sm:px-6">
          <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <h2 class="text-lg font-black text-[#17264a] dark:text-white">{{ t('admin.officialPricing.manual.title') }}</h2>
              <p class="mt-1 max-w-4xl text-sm leading-6 text-slate-500 dark:text-dark-400">{{ t('admin.officialPricing.manual.hint') }}</p>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <span class="summary-chip">{{ t('admin.officialPricing.modelCount', { count: filteredTokenModels.length }) }}</span>
              <span class="summary-chip">{{ t('admin.officialPricing.providerCount', { count: providerGroups.length }) }}</span>
              <label class="relative min-w-[260px]">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
                <input v-model.trim="search" class="input pl-9" :placeholder="t('admin.officialPricing.search')" data-testid="official-pricing-search" />
              </label>
            </div>
          </div>
        </div>

        <div v-if="loading" class="flex min-h-[320px] items-center justify-center">
          <span class="h-8 w-8 animate-spin rounded-full border-2 border-[#087f79]/20 border-t-[#087f79]"></span>
        </div>

        <div v-else-if="providerGroups.length" class="divide-y-8 divide-[#edf2f7] bg-[#edf2f7] dark:divide-dark-900 dark:bg-dark-900">
          <article v-for="group in providerGroups" :key="group.key" class="bg-white dark:bg-dark-800">
            <div class="flex items-center justify-between border-b border-slate-100 bg-white px-5 py-4 dark:border-dark-700 dark:bg-dark-800 sm:px-6">
              <div class="flex items-center gap-3">
                <span class="h-8 w-1 rounded-full bg-[#0f8d83]"></span>
                <div>
                  <h3 class="font-black text-[#17264a] dark:text-white">{{ group.name }}</h3>
                  <p class="mt-0.5 font-mono text-xs text-slate-400">{{ group.key }}</p>
                </div>
              </div>
              <span class="summary-chip">{{ t('admin.officialPricing.providerModels', { count: group.models.length }) }}</span>
            </div>

            <div class="overflow-x-auto">
              <table class="w-full min-w-[1220px] table-fixed text-left text-sm">
                <colgroup>
                  <col class="w-[220px]" />
                  <col class="w-[155px]" />
                  <col class="w-[155px]" />
                  <col class="w-[155px]" />
                  <col class="w-[155px]" />
                  <col class="w-[250px]" />
                  <col class="w-[130px]" />
                </colgroup>
                <thead class="bg-[#f1f5f9] text-xs font-bold uppercase tracking-wide text-[#60708d] dark:bg-dark-900/60 dark:text-dark-400">
                  <tr>
                    <th class="px-5 py-3">{{ t('admin.officialPricing.fields.model') }}</th>
                    <th class="px-3 py-3">{{ t('admin.officialPricing.fields.input') }}<small class="field-unit">{{ t('admin.officialPricing.fields.unit') }}</small></th>
                    <th class="px-3 py-3">{{ t('admin.officialPricing.fields.output') }}<small class="field-unit">{{ t('admin.officialPricing.fields.unit') }}</small></th>
                    <th class="px-3 py-3">{{ t('admin.officialPricing.fields.cacheWrite') }}<small class="field-unit">{{ t('admin.officialPricing.fields.unit') }}</small></th>
                    <th class="px-3 py-3">{{ t('admin.officialPricing.fields.cacheRead') }}<small class="field-unit">{{ t('admin.officialPricing.fields.unit') }}</small></th>
                    <th class="px-3 py-3">{{ t('admin.officialPricing.fields.source') }}</th>
                    <th class="px-5 py-3 text-right">{{ t('admin.officialPricing.fields.actions') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-100 dark:divide-dark-700/70">
                  <tr v-for="model in group.models" :key="model.id" class="transition hover:bg-[#f8fbfd] dark:hover:bg-dark-700/20" :data-testid="`official-model-${model.id}`">
                    <td class="px-5 py-4 align-top">
                      <p class="break-words font-mono font-bold text-[#17264a] dark:text-white">{{ model.model_name }}</p>
                      <p class="mt-1 text-xs text-slate-400">{{ model.currency }} · {{ model.platform }}</p>
                    </td>
                    <td v-for="field in draftFields" :key="field.key" class="px-3 py-4 align-top">
                      <label class="relative block">
                        <span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-xs font-bold text-slate-400">{{ currencySymbol(model.currency) }}</span>
                        <input
                          v-model="drafts[model.id][field.modelKey]"
                          type="number"
                          min="0"
                          step="any"
                          inputmode="decimal"
                          class="input w-full pl-7 font-mono text-sm"
                          :data-testid="`official-${field.key}-${model.id}`"
                          @keydown.enter.prevent="saveModelPrices(model)"
                        />
                      </label>
                    </td>
                    <td class="px-3 py-4 align-top">
                      <div class="space-y-1.5 text-xs">
                        <span class="source-badge source-badge-trusted">{{ persistedSourceLabel(model.official_price_source) }}</span>
                        <p class="text-slate-400">
                          {{ model.official_price_synced_at
                            ? t('admin.officialPricing.metadata.syncedAt', { time: formatDateTime(model.official_price_synced_at) })
                            : t('admin.officialPricing.metadata.neverSynced') }}
                        </p>
                        <a
                          v-if="safeSourceUrl(model.official_price_source_url)"
                          :href="safeSourceUrl(model.official_price_source_url) || undefined"
                          target="_blank"
                          rel="noopener noreferrer"
                          class="inline-flex items-center gap-1 font-bold text-[#136f83] hover:underline dark:text-cyan-300"
                        >
                          {{ t('admin.officialPricing.metadata.sourceLink') }}
                          <Icon name="externalLink" size="xs" />
                        </a>
                      </div>
                    </td>
                    <td class="px-5 py-4 text-right align-top">
                      <button
                        type="button"
                        class="rounded-lg bg-[#087f79] px-3 py-2 text-xs font-bold text-white transition hover:bg-[#066b67] disabled:cursor-not-allowed disabled:bg-slate-200 disabled:text-slate-400 dark:disabled:bg-dark-700"
                        :disabled="!isModelDirty(model) || isSaving(model.id)"
                        :data-testid="`save-official-model-${model.id}`"
                        @click="saveModelPrices(model)"
                      >
                        {{ isSaving(model.id) ? t('admin.officialPricing.manual.saving') : t('admin.officialPricing.manual.save') }}
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </article>
        </div>
        <p v-else-if="!loading" class="px-5 py-16 text-center text-sm text-slate-400">{{ t('admin.officialPricing.noModels') }}</p>
      </section>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import displayPricingAPI, {
  type DisplayOfficialPrices,
  type DisplayPricingModel,
  type DisplayPricingModelInput,
  type DisplayPricingProvider,
  type OfficialPriceSyncCandidate,
  type OfficialPriceSyncPreviewResponse,
} from '@/api/admin/displayPricing'
import type { DisplayPriceCurrency } from '@/api/modelPrices'
import { notifyDisplayPricingUpdated } from '@/utils/displayPricingSync'
import { formatDateTime } from '@/utils/format'

type ModelOfficialField =
  | 'official_input_per_million'
  | 'official_output_per_million'
  | 'official_cache_write_per_million'
  | 'official_cache_read_per_million'

type PriceValueField = keyof DisplayOfficialPrices
type OfficialPriceDraft = Record<ModelOfficialField, string>

interface ProviderGroup {
  key: string
  name: string
  models: DisplayPricingModel[]
}

const PriceValues = defineComponent({
  name: 'PriceValues',
  props: {
    values: { type: Object as PropType<DisplayOfficialPrices | null>, default: null },
    comparison: { type: Object as PropType<DisplayOfficialPrices | null>, default: null },
    currency: { type: String as PropType<DisplayPriceCurrency>, required: true },
    emphasized: Boolean,
  },
  setup(props) {
    const fields: Array<{ key: PriceValueField; label: string }> = [
      { key: 'input_per_million', label: 'IN' },
      { key: 'output_per_million', label: 'OUT' },
      { key: 'cache_write_per_million', label: 'CW' },
      { key: 'cache_read_per_million', label: 'CR' },
    ]
    return () => h('div', { class: 'grid min-w-[205px] grid-cols-2 gap-x-4 gap-y-1.5 font-mono text-xs' }, fields.map(field => {
      const value = props.values?.[field.key] ?? null
      const baseline = props.comparison?.[field.key] ?? null
      const changed = props.comparison != null && value !== baseline
      return h('div', { class: 'flex items-center justify-between gap-2' }, [
        h('span', { class: 'text-slate-400' }, field.label),
        h('strong', {
          class: changed
            ? 'text-[#087f79] dark:text-emerald-300'
            : props.emphasized ? 'text-[#17264a] dark:text-white' : 'text-slate-600 dark:text-dark-200',
        }, formatPrice(value, props.currency)),
      ])
    }))
  },
})

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const checking = ref(false)
const applying = ref(false)
const models = ref<DisplayPricingModel[]>([])
const providers = ref<DisplayPricingProvider[]>([])
const search = ref('')
const preview = ref<OfficialPriceSyncPreviewResponse | null>(null)
const selectedIds = ref<number[]>([])
const savingIds = ref<number[]>([])
const drafts = reactive<Record<number, OfficialPriceDraft>>({})

const draftFields: Array<{ key: string; modelKey: ModelOfficialField }> = [
  { key: 'input', modelKey: 'official_input_per_million' },
  { key: 'output', modelKey: 'official_output_per_million' },
  { key: 'cache-write', modelKey: 'official_cache_write_per_million' },
  { key: 'cache-read', modelKey: 'official_cache_read_per_million' },
]

const tokenModels = computed(() => models.value.filter(model => model.billing_mode === 'token'))
const filteredTokenModels = computed(() => {
  const needle = search.value.trim().toLowerCase()
  if (!needle) return tokenModels.value
  return tokenModels.value.filter(model =>
    model.model_name.toLowerCase().includes(needle)
    || model.provider.toLowerCase().includes(needle)
    || providerLabel(model.provider).toLowerCase().includes(needle)
  )
})

const providerGroups = computed<ProviderGroup[]>(() => {
  const providerOrder = new Map(providers.value.map((provider, index) => [provider.provider, index]))
  const grouped = new Map<string, DisplayPricingModel[]>()
  for (const model of filteredTokenModels.value) {
    const items = grouped.get(model.provider) ?? []
    items.push(model)
    grouped.set(model.provider, items)
  }
  return [...grouped.entries()]
    .sort(([left], [right]) =>
      (providerOrder.get(left) ?? Number.MAX_SAFE_INTEGER) - (providerOrder.get(right) ?? Number.MAX_SAFE_INTEGER)
      || providerLabel(left).localeCompare(providerLabel(right))
    )
    .map(([key, groupModels]) => ({
      key,
      name: providerLabel(key),
      models: [...groupModels].sort((left, right) => left.sort_order - right.sort_order || left.model_name.localeCompare(right.model_name)),
    }))
})

const selectedCandidates = computed(() => {
  const selected = new Set(selectedIds.value)
  return (preview.value?.items ?? []).filter(candidate => selected.has(candidate.model_id) && isSelectable(candidate))
})
const changedCandidateCount = computed(() => preview.value?.items.filter(candidateChanged).length ?? 0)
const applicableCandidateCount = computed(() => preview.value?.items.filter(isSelectable).length ?? 0)
const hasAggregateCandidates = computed(() => preview.value?.items.some(candidate => candidate.source === 'herohao_aggregate') ?? false)
const allApplicableSelected = computed(() => {
  const applicable = (preview.value?.items ?? []).filter(isSelectable)
  return applicable.length > 0 && applicable.every(candidate => selectedIds.value.includes(candidate.model_id))
})

async function loadData(): Promise<void> {
  loading.value = true
  try {
    const [loadedModels, loadedProviders] = await Promise.all([
      displayPricingAPI.listModels(),
      displayPricingAPI.listProviders(),
    ])
    models.value = loadedModels
    providers.value = loadedProviders
    for (const model of loadedModels.filter(item => item.billing_mode === 'token')) drafts[model.id] = createDraft(model)
  } catch {
    appStore.showError(t('admin.officialPricing.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function saveModelPrices(model: DisplayPricingModel): Promise<void> {
  if (!isModelDirty(model) || isSaving(model.id)) return
  const draft = drafts[model.id]
  let prices: Record<ModelOfficialField, number | null>
  try {
    prices = {
      official_input_per_million: parsePrice(draft.official_input_per_million),
      official_output_per_million: parsePrice(draft.official_output_per_million),
      official_cache_write_per_million: parsePrice(draft.official_cache_write_per_million),
      official_cache_read_per_million: parsePrice(draft.official_cache_read_per_million),
    }
  } catch {
    appStore.showError(t('admin.officialPricing.manual.invalid'))
    return
  }

  savingIds.value = [...savingIds.value, model.id]
  try {
    const saved = await displayPricingAPI.updateModel(model.id, buildModelPayload(model, prices))
    const index = models.value.findIndex(item => item.id === model.id)
    const merged: DisplayPricingModel = {
      ...model,
      ...saved,
      official_price_source: saved.official_price_source ?? model.official_price_source,
      official_price_source_url: saved.official_price_source_url ?? model.official_price_source_url,
      official_price_synced_at: saved.official_price_synced_at ?? model.official_price_synced_at,
    }
    if (index >= 0) models.value.splice(index, 1, merged)
    drafts[model.id] = createDraft(merged)
    notifyDisplayPricingUpdated()
    appStore.showSuccess(t('admin.officialPricing.manual.saved', { model: model.model_name }))
  } catch {
    appStore.showError(t('admin.officialPricing.manual.saveFailed'))
  } finally {
    savingIds.value = savingIds.value.filter(id => id !== model.id)
  }
}

async function runSyncPreview(): Promise<void> {
  checking.value = true
  try {
    const response = await displayPricingAPI.previewOfficialPriceSync()
    preview.value = {
      ...response,
      items: response.items.filter(candidate => !candidate.billing_mode || candidate.billing_mode === 'token'),
    }
    selectedIds.value = preview.value.items.filter(isSelectable).map(candidate => candidate.model_id)
  } catch {
    appStore.showError(t('admin.officialPricing.sync.checkFailed'))
  } finally {
    checking.value = false
  }
}

async function applySelected(): Promise<void> {
  if (!selectedCandidates.value.length) {
    appStore.showError(t('admin.officialPricing.sync.noSelection'))
    return
  }
  applying.value = true
  try {
    const result = await displayPricingAPI.applyOfficialPriceSync({
      models: selectedCandidates.value.map(candidate => ({
        model_id: candidate.model_id,
        expected_updated_at: candidate.expected_updated_at,
        proposal_hash: candidate.proposal_hash,
      })),
    })
    notifyDisplayPricingUpdated()
    appStore.showSuccess(t('admin.officialPricing.sync.applied', { count: result.applied_count }))
    await loadData()
    await runSyncPreview()
  } catch {
    appStore.showError(t('admin.officialPricing.sync.applyFailed'))
  } finally {
    applying.value = false
  }
}

function createDraft(model: DisplayPricingModel): OfficialPriceDraft {
  return {
    official_input_per_million: draftValue(model.official_input_per_million),
    official_output_per_million: draftValue(model.official_output_per_million),
    official_cache_write_per_million: draftValue(model.official_cache_write_per_million),
    official_cache_read_per_million: draftValue(model.official_cache_read_per_million),
  }
}

function buildModelPayload(
  model: DisplayPricingModel,
  prices: Record<ModelOfficialField, number | null>,
): DisplayPricingModelInput {
  return {
    platform: model.platform,
    model_name: model.model_name,
    provider: model.provider,
    billing_mode: model.billing_mode,
    currency: model.currency,
    enabled: model.enabled,
    sort_order: model.sort_order,
    model_note: model.model_note,
    ...prices,
    official_price_source: 'manual',
    official_price_source_url: '',
    official_price_synced_at: null,
    model_multiplier: model.model_multiplier,
    per_request_lte_256k: model.per_request_lte_256k,
    per_request_256k_512k_override: model.per_request_256k_512k_override,
    per_request_gt_512k_override: model.per_request_gt_512k_override,
    image_prices: (model.image_prices ?? []).map(price => ({ ...price })),
  }
}

function isModelDirty(model: DisplayPricingModel): boolean {
  const draft = drafts[model.id]
  if (!draft) return false
  return draftFields.some(field => normalizedDraftValue(draft[field.modelKey]) !== model[field.modelKey])
}

function normalizedDraftValue(value: string | number | null | undefined): number | null {
  if (value == null || String(value).trim() === '') return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

function parsePrice(value: string | number | null | undefined): number | null {
  if (value == null || String(value).trim() === '') return null
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed < 0) throw new Error('invalid price')
  return parsed
}

function draftValue(value: number | null): string {
  return value == null ? '' : String(value)
}

function isSaving(id: number): boolean {
  return savingIds.value.includes(id)
}

function isSelectable(candidate: OfficialPriceSyncCandidate): boolean {
  return candidate.applicable && candidateChanged(candidate)
}

function candidateChanged(candidate: OfficialPriceSyncCandidate): boolean {
  return candidate.changed ?? candidate.diff?.has_changes ?? false
}

function toggleCandidate(modelId: number): void {
  selectedIds.value = selectedIds.value.includes(modelId)
    ? selectedIds.value.filter(id => id !== modelId)
    : [...selectedIds.value, modelId]
}

function toggleAllApplicable(): void {
  selectedIds.value = allApplicableSelected.value
    ? []
    : (preview.value?.items ?? []).filter(isSelectable).map(candidate => candidate.model_id)
}

function providerLabel(provider: string): string {
  return providers.value.find(item => item.provider === provider)?.display_name || provider
}

function persistedSourceLabel(source?: string): string {
  if (!source) return t('admin.officialPricing.metadata.sourceUnknown')
  return sourceLabel(source)
}

function sourceLabel(source: string): string {
  const known = ['herohao_aggregate', 'provider_official', 'manual']
  return known.includes(source) ? t(`admin.officialPricing.source.${source}`) : source || t('admin.officialPricing.source.unknown')
}

function confidenceLabel(confidence: string): string {
  const normalized = confidence?.toLowerCase() || 'unknown'
  const known = ['high', 'medium', 'low', 'unverified', 'unknown']
  return known.includes(normalized) ? t(`admin.officialPricing.confidence.${normalized}`) : confidence
}

function candidateStatusLabel(candidate: OfficialPriceSyncCandidate): string {
  if (!candidate.applicable) return t('admin.officialPricing.sync.notApplicable')
  if (!candidateChanged(candidate)) return t('admin.officialPricing.sync.noChange')
  return t('admin.officialPricing.sync.changedStatus')
}

function candidateStatusClass(candidate: OfficialPriceSyncCandidate): string {
  if (!candidate.applicable) return 'status-badge-error'
  if (!candidateChanged(candidate)) return 'status-badge-neutral'
  return 'status-badge-success'
}

function candidateReasonLabel(reason: string): string {
  const known = [
    'unsupported_billing_mode',
    'currency_mismatch',
    'candidate_not_found',
    'provider_mismatch',
    'candidate_disabled',
    'candidate_price_missing',
  ]
  return known.includes(reason)
    ? t(`admin.officialPricing.reason.${reason}`)
    : reason || t('admin.officialPricing.sync.reasonFallback')
}

function safeSourceUrl(value?: string | null): string | null {
  if (!value) return null
  try {
    const url = new URL(value)
    return url.protocol === 'https:' ? url.toString() : null
  } catch {
    return null
  }
}

function currencySymbol(currency: DisplayPriceCurrency): string {
  return currency === 'CNY' ? '¥' : '$'
}

function formatPrice(value: number | null, currency: DisplayPriceCurrency): string {
  if (value == null || !Number.isFinite(value)) return '—'
  return `${currencySymbol(currency)}${Number(value).toFixed(value !== 0 && Math.abs(value) < 0.01 ? 8 : 6).replace(/\.?0+$/, '')}`
}

onMounted(() => void loadData())
</script>

<style scoped>
.official-panel {
  @apply rounded-2xl border border-[#dbe5ef] bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800;
}

.sync-stat {
  @apply rounded-xl border border-[#dbe5ef] bg-white px-3 py-2 text-center text-xs font-bold text-[#52627f] dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300;
}

.sync-stat-selected {
  @apply border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300;
}

.summary-chip {
  @apply inline-flex rounded-lg border border-[#dbe5ef] bg-white px-2.5 py-1.5 text-xs font-bold text-[#52627f] dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300;
}

.field-unit {
  @apply mt-0.5 block text-[10px] font-medium normal-case tracking-normal text-slate-400;
}

.source-badge {
  @apply inline-flex rounded-md border px-2 py-1 text-[11px] font-bold;
}

.source-badge-unverified {
  @apply border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-300;
}

.source-badge-trusted {
  @apply border-cyan-200 bg-cyan-50 text-[#136f83] dark:border-cyan-500/25 dark:bg-cyan-500/10 dark:text-cyan-300;
}

.status-badge {
  @apply inline-flex rounded-full px-2.5 py-1 text-xs font-bold;
}

.status-badge-success {
  @apply bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300;
}

.status-badge-neutral {
  @apply bg-slate-100 text-slate-600 dark:bg-dark-700 dark:text-dark-300;
}

.status-badge-error {
  @apply bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300;
}
</style>
