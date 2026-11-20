<template>
  <section
    data-testid="grok-model-diagnostics"
    class="min-w-0 max-w-full overflow-hidden rounded-xl border border-gray-200 bg-white text-sm text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
  >
    <div
      data-testid="grok-diagnostics-summary"
      class="flex flex-wrap items-center justify-between gap-3 p-4"
    >
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="text-sm font-medium text-gray-900 dark:text-gray-100">
            {{ t('admin.accounts.grokModelDiagnostics.catalogTitle') }}
          </h3>
          <p data-testid="grok-diagnostics-state" :class="stateClass">{{ headline }}</p>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
          <span v-if="modelCountLabel">{{ modelCountLabel }}</span>
          <span v-if="catalogHost">{{ modelCountLabel ? ' · ' : '' }}{{ catalogHost }}</span>
          <span data-testid="grok-diagnostics-last-success">
            {{ modelCountLabel || catalogHost ? ' · ' : '' }}{{ t('admin.accounts.grokModelDiagnostics.lastSuccessRelative', { time: lastSuccessRelative }) }}
          </span>
        </p>
      </div>
      <button
        v-if="canSync"
        type="button"
        data-testid="grok-diagnostics-sync"
        class="btn btn-secondary btn-sm"
        :title="t('admin.accounts.grokModelDiagnostics.syncSavedConfig')"
        :aria-label="`${t('admin.accounts.grokModelDiagnostics.syncCatalog')} — ${t('admin.accounts.grokModelDiagnostics.syncSavedConfig')}`"
        :disabled="syncing || loading"
        @click="requestSync"
      >
        <Icon name="refresh" size="xs" aria-hidden="true" class="max-sm:hidden" />
        {{ syncing ? t('common.loading') : t('admin.accounts.grokModelDiagnostics.syncCatalog') }}
      </button>
    </div>

    <p v-if="loading" class="px-4 pb-3 text-xs text-gray-500 dark:text-dark-400">
      {{ t('common.loading') }}
    </p>
    <p
      v-if="refreshFailed"
      data-testid="grok-diagnostics-refresh-failed"
      class="px-4 pb-3 text-xs text-red-600 dark:text-red-400"
    >
      {{ t('admin.accounts.grokModelDiagnostics.refreshFailed') }}
    </p>

    <div
      v-if="showFailureNotice || showFallbackNotice || showStateNotice || showPreviousValid"
      class="mx-4 mb-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
    >
      <p v-if="showFailureNotice" class="flex items-start gap-2">
        <Icon name="exclamationTriangle" size="xs" aria-hidden="true" class="mt-0.5 shrink-0" />
        <span>{{ attemptErrorLabel }}</span>
      </p>
      <p v-if="showFallbackNotice" class="flex items-start gap-2">
        <Icon name="exclamationTriangle" size="xs" aria-hidden="true" class="mt-0.5 shrink-0" />
        <span>{{ t('admin.accounts.grokModelDiagnostics.fallbackNotice') }}</span>
      </p>
      <p v-if="showPreviousValid">{{ t('admin.accounts.grokModelDiagnostics.previousCatalogValid') }}</p>
      <p v-if="effectiveState === 'stale'">{{ t('admin.accounts.grokModelDiagnostics.staleNotice') }}</p>
      <p v-if="effectiveState === 'source_changed'">{{ t('admin.accounts.grokModelDiagnostics.changedNotice') }}</p>
      <p v-if="effectiveState === 'unknown'">{{ t('admin.accounts.grokModelDiagnostics.unknownNotice') }}</p>
    </div>

    <div
      v-if="missingMappings.length > 0"
      class="mx-4 mb-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
    >
      <p data-testid="grok-diagnostics-missing-summary" class="flex items-start gap-2">
        <Icon name="exclamationTriangle" size="xs" aria-hidden="true" class="mt-0.5 shrink-0" />
        <span>{{ t('admin.accounts.grokModelDiagnostics.missingSummary', { count: missingMappings.length }) }}</span>
      </p>
      <p class="mt-1">{{ t('admin.accounts.grokModelDiagnostics.summaryNotice') }}</p>
      <button
        type="button"
        data-testid="grok-diagnostics-difference-toggle"
        class="mt-2 inline-flex items-center gap-1 font-medium"
        :aria-expanded="differencesOpen"
        :aria-controls="differencesId"
        @click="differencesOpen = !differencesOpen"
      >
        <Icon
          name="chevronRight"
          size="xs"
          aria-hidden="true"
          :class="differencesOpen ? 'rotate-90' : ''"
        />
        {{ differencesOpen
          ? t('admin.accounts.grokModelDiagnostics.hideDifferences')
          : t('admin.accounts.grokModelDiagnostics.viewDifferences') }}
      </button>
      <div
        v-if="differencesOpen"
        :id="differencesId"
        data-testid="grok-diagnostics-difference-list"
        class="mt-2 space-y-2"
      >
        <p>{{ t('admin.accounts.grokModelDiagnostics.notListed') }} {{ t('admin.accounts.grokModelDiagnostics.compatibilityNotice') }}</p>
        <p v-if="attemptStatus === 'configured_fallback'">
          {{ t('admin.accounts.grokModelDiagnostics.lastSuccessRelative', { time: lastSuccessRelative }) }}
        </p>
        <ul class="space-y-2">
          <li v-for="(item, index) in missingMappings" :key="`${item.requested}-${index}`">
            <MappingRow :item="item" :status="''" />
          </li>
        </ul>
      </div>
    </div>

    <div class="border-t border-gray-200 dark:border-dark-600">
      <button
        type="button"
        data-testid="grok-diagnostics-details-toggle"
        class="flex w-full items-center gap-2 px-4 py-3 text-left text-sm font-medium text-gray-800 dark:text-gray-100"
        :aria-expanded="detailsOpen"
        :aria-controls="detailsId"
        @click="detailsOpen = !detailsOpen"
      >
        <Icon
          name="chevronRight"
          size="xs"
          aria-hidden="true"
          :class="detailsOpen ? 'rotate-90' : ''"
        />
        {{ t('admin.accounts.grokModelDiagnostics.details') }}
      </button>
      <div
        v-if="detailsOpen"
        :id="detailsId"
        data-testid="grok-diagnostics-details"
        class="border-t border-gray-200 p-4 dark:border-dark-600"
      >
        <dl class="space-y-3 text-xs">
          <div>
            <dt class="text-gray-500 dark:text-dark-400">
              {{ effectiveState !== 'fresh'
                ? t('admin.accounts.grokModelDiagnostics.historicalSource')
                : t('admin.accounts.grokModelDiagnostics.source') }}
            </dt>
            <dd data-testid="grok-diagnostics-endpoint" class="min-w-0 break-all font-mono text-xs">
              {{ diagnostics?.endpoint || t('admin.accounts.grokModelDiagnostics.unknown') }}
            </dd>
          </div>
          <div>
            <dt class="text-gray-500 dark:text-dark-400">{{ t('admin.accounts.grokModelDiagnostics.lastSuccess') }}</dt>
            <dd>{{ observedTime(diagnostics?.fetched_at) }}</dd>
          </div>
          <div>
            <dt class="text-gray-500 dark:text-dark-400">{{ t('admin.accounts.grokModelDiagnostics.lastAttempt') }}</dt>
            <dd data-testid="grok-diagnostics-last-attempt" class="min-w-0 break-all">
              <template v-if="diagnostics?.last_attempt">
                {{ attemptLabel }}
                <template v-if="attemptDetail"> {{ attemptDetail }}</template>
                {{ observedTime(diagnostics.last_attempt.finished_at) }}
              </template>
              <template v-else>{{ t('admin.accounts.grokModelDiagnostics.unknown') }}</template>
            </dd>
          </div>
        </dl>

        <div class="mt-4">
          <h4 class="text-sm font-medium text-gray-900 dark:text-gray-100">
            {{ effectiveState !== 'fresh'
              ? t('admin.accounts.grokModelDiagnostics.historicalModels')
              : t('admin.accounts.grokModelDiagnostics.upstreamModels') }}
          </h4>
          <ul class="mt-1 max-h-48 space-y-1 overflow-y-auto">
            <li
              v-for="model in diagnostics?.models ?? []"
              :key="model"
              data-testid="grok-diagnostics-model"
              class="min-w-0 break-all font-mono text-xs"
            >
              {{ model }}
            </li>
          </ul>
        </div>

        <div class="mt-4" data-testid="grok-diagnostics-mappings">
          <h4 class="text-sm font-medium text-gray-900 dark:text-gray-100">
            {{ t('admin.accounts.grokModelDiagnostics.savedMappings') }}
          </h4>
          <p v-if="hasFreshGaps" class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.accounts.grokModelDiagnostics.compatibilityNotice') }}
          </p>
          <div class="mt-2 hidden text-xs text-gray-500 dark:text-dark-400 sm:grid sm:grid-cols-2 sm:gap-3">
            <span>{{ t('admin.accounts.grokModelDiagnostics.requestColumn') }}</span>
            <span>{{ t('admin.accounts.grokModelDiagnostics.targetColumn') }}</span>
          </div>
          <ul class="mt-2 space-y-2">
            <li v-for="(item, index) in diagnostics?.mappings ?? []" :key="`${item.requested}-${index}`">
              <MappingRow :item="item" :status="mappingStatus(item)" />
            </li>
          </ul>
        </div>
      </div>
    </div>

    <p
      v-if="dirty"
      data-testid="grok-diagnostics-dirty"
      class="flex items-start gap-2 px-4 py-3 text-xs text-gray-500 dark:text-dark-400"
    >
      <Icon name="infoCircle" size="xs" aria-hidden="true" class="mt-0.5 shrink-0" />
      <span>{{ t('admin.accounts.grokModelDiagnostics.savedConfigHint') }}</span>
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onBeforeUnmount, onMounted, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GrokModelDiagnosticItem, GrokModelDiagnostics } from '@/types'
import { formatDateTime, formatRelativeTime } from '@/utils/format'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  diagnostics: GrokModelDiagnostics | null
  loading?: boolean
  dirty?: boolean
  refreshFailed?: boolean
  canSync?: boolean
  syncing?: boolean
}>(), {
  diagnostics: null,
  loading: false,
  dirty: false,
  refreshFailed: false,
  canSync: false,
  syncing: false
})

const emit = defineEmits<{
  sync: []
}>()

const { t } = useI18n()
const nowMs = ref(Date.now())
const detailsOpen = ref(false)
const differencesOpen = ref(false)
const instanceId = useId()
const detailsId = 'grok-catalog-' + instanceId
const differencesId = 'grok-differences-' + instanceId
const RFC3339_RE = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/
const ATTEMPT_LABEL_KEYS: Record<string, string> = {
  success: 'admin.accounts.grokModelDiagnostics.attemptSuccess',
  failed: 'admin.accounts.grokModelDiagnostics.syncFailed',
  configured_fallback: 'admin.accounts.grokModelDiagnostics.attemptFallback'
}
const ERROR_LABEL_KEYS: Record<string, string> = {
  configuration: 'admin.accounts.grokModelDiagnostics.errorConfiguration',
  http_error: 'admin.accounts.grokModelDiagnostics.errorHTTP',
  transport: 'admin.accounts.grokModelDiagnostics.errorTransport',
  invalid_response: 'admin.accounts.grokModelDiagnostics.errorInvalidResponse',
  empty_catalog: 'admin.accounts.grokModelDiagnostics.errorEmptyCatalog',
  too_large: 'admin.accounts.grokModelDiagnostics.errorTooLarge',
  persistence: 'admin.accounts.grokModelDiagnostics.errorPersistence'
}

function ownedLabelKey(table: Record<string, string>, value: string | undefined): string | undefined {
  if (!value || !Object.prototype.hasOwnProperty.call(table, value)) return undefined
  return table[value]
}

let timer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  timer = setInterval(() => {
    nowMs.value = Date.now()
  }, 1000)
})

onBeforeUnmount(() => {
  if (timer !== undefined) clearInterval(timer)
})

function daysInMonth(year: number, month: number): number {
  if (month === 2) {
    const leap = (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0
    return leap ? 29 : 28
  }
  return [31, 0, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1] ?? 0
}

function parseCatalogTime(value: string | undefined): number | null {
  if (!value) return null
  const match = RFC3339_RE.exec(value.trim())
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const hour = Number(match[4])
  const minute = Number(match[5])
  const second = Number(match[6])
  const offsetHour = match[7] === undefined ? 0 : Number(match[7])
  const offsetMinute = match[8] === undefined ? 0 : Number(match[8])
  if (
    month < 1 || month > 12 ||
    day < 1 || day > daysInMonth(year, month) ||
    hour > 23 || minute > 59 || second > 59 ||
    offsetHour > 23 || offsetMinute > 59
  ) {
    return null
  }
  const parsed = Date.parse(match[0])
  return Number.isFinite(parsed) ? parsed : null
}

const fetchedMs = computed(() => parseCatalogTime(props.diagnostics?.fetched_at))
const expiresMs = computed(() => parseCatalogTime(props.diagnostics?.expires_at))
const fetchedUsable = computed(() => fetchedMs.value !== null && fetchedMs.value <= nowMs.value)

const effectiveState = computed(() => {
  const diagnostics = props.diagnostics
  if (!diagnostics) return 'unknown'
  if (diagnostics.state === 'fresh') {
    if (!fetchedUsable.value || expiresMs.value === null) return 'unknown'
    if (nowMs.value >= expiresMs.value) return 'stale'
    return 'fresh'
  }
  if (diagnostics.state === 'stale' || diagnostics.state === 'source_changed') {
    if (!fetchedUsable.value) return 'unknown'
    return diagnostics.state
  }
  return 'unknown'
})

const attemptStatus = computed(() => props.diagnostics?.last_attempt?.status ?? '')

const missingMappings = computed(() => effectiveState.value === 'fresh'
  ? (props.diagnostics?.mappings ?? []).filter(item => item.status === 'not_listed')
  : [])

const catalogHost = computed(() => {
  try { return new URL(props.diagnostics?.endpoint ?? '').host }
  catch { return '' }
})

const lastSuccessRelative = computed(() => {
  const time = fetchedMs.value
  const current = nowMs.value
  if (time === null || time > current) return t('admin.accounts.grokModelDiagnostics.unknown')
  return formatRelativeTime(props.diagnostics?.fetched_at)
})

const modelCountLabel = computed(() => {
  const count = props.diagnostics?.models?.length ?? 0
  if (effectiveState.value === 'fresh') {
    return t('admin.accounts.grokModelDiagnostics.modelCount', { count })
  }
  if (effectiveState.value === 'stale' || effectiveState.value === 'source_changed') {
    return t('admin.accounts.grokModelDiagnostics.historicalModelCount', { count })
  }
  return ''
})

const headline = computed(() => {
  if (effectiveState.value === 'stale') return t('admin.accounts.grokModelDiagnostics.stale')
  if (effectiveState.value === 'source_changed') return t('admin.accounts.grokModelDiagnostics.sourceChanged')
  if (effectiveState.value === 'unknown') return t('admin.accounts.grokModelDiagnostics.unknown')
  if (attemptStatus.value === 'failed') return t('admin.accounts.grokModelDiagnostics.syncFailed')
  if (attemptStatus.value === 'configured_fallback') return t('admin.accounts.grokModelDiagnostics.attemptFallback')
  return t('admin.accounts.grokModelDiagnostics.fresh')
})

const stateClass = computed(() => {
  const attention = effectiveState.value !== 'fresh'
    || attemptStatus.value === 'failed'
    || attemptStatus.value === 'configured_fallback'
  if (!attention && effectiveState.value === 'fresh') {
    return 'text-xs font-medium text-emerald-700 dark:text-emerald-300'
  }
  if (effectiveState.value === 'unknown' && attemptStatus.value !== 'failed' && attemptStatus.value !== 'configured_fallback') {
    return 'text-xs font-medium text-gray-500 dark:text-dark-400'
  }
  return 'text-xs font-medium text-amber-800 dark:text-amber-200'
})

const attemptLabel = computed(() => {
  const status = props.diagnostics?.last_attempt?.status
  if (!status) return ''
  return t(ownedLabelKey(ATTEMPT_LABEL_KEYS, status) ?? 'admin.accounts.grokModelDiagnostics.attemptUnknown')
})

const attemptErrorLabel = computed(() => {
  const attempt = props.diagnostics?.last_attempt
  if (!attempt || attempt.status !== 'failed') return ''
  return t(ownedLabelKey(ERROR_LABEL_KEYS, attempt.error_code) ?? 'admin.accounts.grokModelDiagnostics.attemptUnavailable')
})

const attemptDetail = computed(() => {
  if (attemptStatus.value === 'configured_fallback') {
    return t('admin.accounts.grokModelDiagnostics.fallbackNotice')
  }
  return attemptErrorLabel.value
})

const showFailureNotice = computed(() => attemptStatus.value === 'failed')
const showFallbackNotice = computed(() => attemptStatus.value === 'configured_fallback')
const showPreviousValid = computed(() => effectiveState.value === 'fresh'
  && (attemptStatus.value === 'failed' || attemptStatus.value === 'configured_fallback'))
const showStateNotice = computed(() => effectiveState.value === 'stale'
  || effectiveState.value === 'source_changed'
  || effectiveState.value === 'unknown')
const hasFreshGaps = computed(() => missingMappings.value.length > 0)

function observedTime(value: string | undefined): string {
  const parsed = parseCatalogTime(value)
  if (parsed === null || parsed > nowMs.value) return t('admin.accounts.grokModelDiagnostics.unknown')
  return formatDateTime(value)
}

function mappingStatus(item: GrokModelDiagnosticItem): string {
  if (item.status === 'out_of_scope') return t('admin.accounts.grokModelDiagnostics.outOfScope')
  if (item.status === 'unverifiable') return t('admin.accounts.grokModelDiagnostics.unverifiable')
  if (item.status === 'not_listed') {
    return effectiveState.value === 'fresh'
      ? t('admin.accounts.grokModelDiagnostics.notListed')
      : t('admin.accounts.grokModelDiagnostics.historicalComparison')
  }
  if (item.status === 'listed') {
    return effectiveState.value === 'fresh'
      ? t('admin.accounts.grokModelDiagnostics.currentListed')
      : t('admin.accounts.grokModelDiagnostics.historicalComparison')
  }
  return ''
}

function requestSync() {
  if (props.syncing || props.loading) return
  emit('sync')
}

const MappingRow = defineComponent({
  name: 'GrokDiagnosticsMappingRow',
  props: {
    item: { type: Object as () => GrokModelDiagnosticItem, required: true },
    status: { type: String, default: '' }
  },
  setup(rowProps) {
    return () => h('div', { class: 'grid grid-cols-1 gap-1 sm:grid-cols-2 sm:items-start sm:gap-3' }, [
      h('div', { class: 'min-w-0 break-all font-mono text-xs' }, rowProps.item.requested),
      h('div', { class: 'flex min-w-0 items-start gap-1' }, [
        rowProps.item.upstream
          ? h(Icon, { name: 'arrowRight', size: 'xs', 'aria-hidden': 'true', class: 'mt-0.5 shrink-0' })
          : null,
        h('div', { class: 'min-w-0' }, [
          rowProps.item.upstream
            ? h('div', { class: 'min-w-0 break-all font-mono text-xs' }, rowProps.item.upstream)
            : null,
          rowProps.status
            ? h('div', { class: 'text-xs' }, rowProps.status)
            : null
        ])
      ])
    ])
  }
})
</script>
