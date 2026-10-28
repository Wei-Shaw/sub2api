<template>
  <section
    data-testid="grok-model-diagnostics"
    class="mt-4 rounded-lg border border-gray-200 p-4 dark:border-dark-600"
  >
    <div
      data-testid="grok-diagnostics-summary"
      class="flex items-center gap-3"
    >
      <div
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
        :class="iconToneClass"
      >
        <Icon name="database" size="sm" aria-hidden="true" />
      </div>
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.accounts.grokModelDiagnostics.catalogTitle') }}
          </h3>
          <span data-testid="grok-diagnostics-state" :class="badgeClass">{{ headline }}</span>
        </div>
        <p
          v-if="modelCountLabel || hasUsableSyncTime"
          class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
        >
          <span v-if="modelCountLabel">{{ modelCountLabel }}</span>
          <span
            v-if="hasUsableSyncTime"
            data-testid="grok-diagnostics-last-success"
          >{{ modelCountLabel ? ' · ' : '' }}{{ syncedRelativeLabel }}</span>
        </p>
      </div>
      <button
        v-if="canSync"
        type="button"
        data-testid="grok-diagnostics-sync"
        class="btn btn-secondary btn-sm ml-auto shrink-0"
        :title="t('admin.accounts.grokModelDiagnostics.syncSavedConfig')"
        :aria-label="`${t('admin.accounts.grokModelDiagnostics.syncCatalog')} — ${t('admin.accounts.grokModelDiagnostics.syncSavedConfig')}`"
        :disabled="syncing || loading"
        @click="requestSync"
      >
        <Icon name="refresh" size="xs" aria-hidden="true" class="max-sm:hidden" />
        {{ syncing ? t('common.loading') : t('admin.accounts.grokModelDiagnostics.syncCatalog') }}
      </button>
    </div>

    <div
      v-if="statusNotice || hasFreshGaps"
      class="mt-3 rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400"
    >
      <div class="flex items-start gap-1.5">
        <Icon name="exclamationTriangle" size="xs" aria-hidden="true" class="mt-0.5 shrink-0" />
        <div class="min-w-0 space-y-1">
          <p v-if="statusNotice">{{ statusNotice }}</p>
          <p
            v-if="hasFreshGaps"
            data-testid="grok-diagnostics-missing-summary"
            class="flex flex-wrap items-center gap-2"
          >
            <span>{{ t('admin.accounts.grokModelDiagnostics.missingDetail', { count: missingMappings.length }) }}</span>
            <button
              type="button"
              class="font-medium underline-offset-2 hover:underline"
              @click="detailsOpen = true"
            >
              {{ t('admin.accounts.grokModelDiagnostics.viewDetails') }}
            </button>
          </p>
        </div>
      </div>
    </div>

    <p
      v-if="loading"
      class="mt-3 flex items-start gap-1.5 text-xs text-gray-500 dark:text-gray-400"
    >
      {{ t('common.loading') }}
    </p>
    <p
      v-if="refreshFailed"
      data-testid="grok-diagnostics-refresh-failed"
      class="mt-3 flex items-start gap-1.5 text-xs text-red-600 dark:text-red-400"
    >
      {{ t('admin.accounts.grokModelDiagnostics.refreshFailed') }}
    </p>
    <p
      v-if="dirty"
      data-testid="grok-diagnostics-dirty"
      class="mt-3 flex items-start gap-1.5 text-xs text-gray-500 dark:text-gray-400"
    >
      <Icon name="infoCircle" size="xs" aria-hidden="true" class="mt-0.5 shrink-0" />
      <span>{{ t('admin.accounts.grokModelDiagnostics.savedConfigHint') }}</span>
    </p>

    <button
      type="button"
      data-testid="grok-diagnostics-details-toggle"
      class="mt-3 inline-flex items-center gap-1 text-xs text-gray-500 hover:text-gray-700 dark:text-gray-400"
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
      {{ detailsOpen
        ? t('admin.accounts.grokModelDiagnostics.hideDetails')
        : t('admin.accounts.grokModelDiagnostics.viewDetails') }}
    </button>

    <div
      v-if="detailsOpen"
      :id="detailsId"
      data-testid="grok-diagnostics-details"
      class="mt-3 space-y-4 rounded-lg bg-gray-50 p-3 dark:bg-dark-700/50"
    >
      <div
        v-if="(diagnostics?.mappings?.length ?? 0) > 0"
        data-testid="grok-diagnostics-mappings"
      >
        <div class="flex flex-wrap items-baseline gap-2">
          <h4 class="text-xs font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.accounts.grokModelDiagnostics.mappingCheck') }}
          </h4>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.grokModelDiagnostics.mappingCheckHint') }}
          </span>
        </div>
        <ul class="mt-2 space-y-2">
          <li
            v-for="(item, index) in diagnostics?.mappings ?? []"
            :key="`${item.requested}-${index}`"
            class="flex items-center gap-2 rounded-lg px-3 py-2"
            :class="mappingRowClass(item)"
          >
            <span class="min-w-0 flex-1 break-all font-mono text-xs">
              <template v-if="item.upstream">{{ item.requested }} → {{ item.upstream }}</template>
              <template v-else>{{ item.requested }}</template>
            </span>
            <span :class="mappingBadgeClass(item)">{{ mappingStatus(item) }}</span>
          </li>
        </ul>
      </div>

      <div v-if="(diagnostics?.models?.length ?? 0) > 0">
        <h4 class="text-xs font-medium text-gray-700 dark:text-gray-300">
          {{ modelsSectionTitle }}
        </h4>
        <div class="mt-2 flex flex-wrap gap-1.5">
          <span
            v-for="model in visibleModels"
            :key="model"
            data-testid="grok-diagnostics-model"
            class="rounded-md border border-gray-200 bg-white px-2 py-0.5 font-mono text-xs dark:border-dark-600 dark:bg-dark-800"
          >
            {{ model }}
          </span>
          <button
            v-if="hiddenModelCount > 0 || modelsExpanded"
            type="button"
            data-testid="grok-diagnostics-models-more"
            class="text-xs font-medium text-gray-500 hover:text-gray-700 dark:text-gray-400"
            @click="modelsExpanded = !modelsExpanded"
          >
            {{ modelsExpanded
              ? t('admin.accounts.grokModelDiagnostics.collapseModels')
              : t('admin.accounts.grokModelDiagnostics.showMoreModels', { count: hiddenModelCount }) }}
          </button>
        </div>
      </div>

      <p
        v-if="diagnostics?.endpoint"
        data-testid="grok-diagnostics-endpoint"
        class="break-all font-mono text-xs text-gray-400"
      >
        {{ t('admin.accounts.grokModelDiagnostics.sourcePrefix') }} {{ diagnostics.endpoint }}
      </p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GrokModelDiagnosticItem, GrokModelDiagnostics } from '@/types'
import { formatRelativeTime } from '@/utils/format'
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
const modelsExpanded = ref(false)
const instanceId = useId()
const detailsId = 'grok-catalog-' + instanceId
const MODEL_PREVIEW_LIMIT = 6
const RFC3339_RE = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/
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

const hasFreshGaps = computed(() => missingMappings.value.length > 0)
const hasUsableSyncTime = computed(() => fetchedUsable.value)

const syncedRelativeLabel = computed(() => {
  const current = nowMs.value
  if (fetchedMs.value === null || fetchedMs.value > current) return ''
  return t('admin.accounts.grokModelDiagnostics.syncedRelative', {
    time: formatRelativeTime(props.diagnostics?.fetched_at)
  })
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

const badgeTone = computed<'success' | 'warning' | 'gray'>(() => {
  const attention = effectiveState.value !== 'fresh'
    || attemptStatus.value === 'failed'
    || attemptStatus.value === 'configured_fallback'
  if (effectiveState.value === 'fresh' && !hasFreshGaps.value && !attention) {
    return 'success'
  }
  if (effectiveState.value === 'fresh' && hasFreshGaps.value) {
    return 'warning'
  }
  if (effectiveState.value === 'unknown' && attemptStatus.value !== 'failed' && attemptStatus.value !== 'configured_fallback') {
    return 'gray'
  }
  return 'warning'
})

const headline = computed(() => {
  if (effectiveState.value === 'fresh' && !hasFreshGaps.value
    && attemptStatus.value !== 'failed'
    && attemptStatus.value !== 'configured_fallback') {
    return t('admin.accounts.grokModelDiagnostics.fresh')
  }
  if (effectiveState.value === 'fresh' && hasFreshGaps.value) {
    return t('admin.accounts.grokModelDiagnostics.badgeMissing', { count: missingMappings.value.length })
  }
  if (effectiveState.value === 'stale') return t('admin.accounts.grokModelDiagnostics.stale')
  if (effectiveState.value === 'source_changed') return t('admin.accounts.grokModelDiagnostics.sourceChanged')
  if (effectiveState.value === 'unknown') return t('admin.accounts.grokModelDiagnostics.unknown')
  if (attemptStatus.value === 'failed') return t('admin.accounts.grokModelDiagnostics.syncFailed')
  if (attemptStatus.value === 'configured_fallback') return t('admin.accounts.grokModelDiagnostics.attemptFallback')
  return t('admin.accounts.grokModelDiagnostics.fresh')
})

const badgeClass = computed(() => {
  if (badgeTone.value === 'success') return 'badge badge-success'
  if (badgeTone.value === 'warning') return 'badge badge-warning'
  return 'badge badge-gray'
})

const iconToneClass = computed(() => {
  if (badgeTone.value === 'success') {
    return 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
  }
  if (badgeTone.value === 'warning') {
    return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
  }
  return 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
})

const attemptErrorLabel = computed(() => {
  const attempt = props.diagnostics?.last_attempt
  if (!attempt || attempt.status !== 'failed') return ''
  return t(ownedLabelKey(ERROR_LABEL_KEYS, attempt.error_code) ?? 'admin.accounts.grokModelDiagnostics.attemptUnavailable')
})

const showPreviousValid = computed(() => effectiveState.value === 'fresh'
  && (attemptStatus.value === 'failed' || attemptStatus.value === 'configured_fallback'))

const statusNotice = computed(() => {
  if (attemptStatus.value === 'failed') {
    let notice = attemptErrorLabel.value
    if (showPreviousValid.value) {
      notice += t('admin.accounts.grokModelDiagnostics.previousCatalogValid')
    }
    notice += t('admin.accounts.grokModelDiagnostics.retryHint')
    return notice
  }
  if (attemptStatus.value === 'configured_fallback') {
    return t('admin.accounts.grokModelDiagnostics.fallbackNotice')
  }
  if (effectiveState.value === 'stale') return t('admin.accounts.grokModelDiagnostics.staleNotice')
  if (effectiveState.value === 'source_changed') return t('admin.accounts.grokModelDiagnostics.changedNotice')
  if (effectiveState.value === 'unknown') return t('admin.accounts.grokModelDiagnostics.unknownNotice')
  return ''
})

const allModels = computed(() => props.diagnostics?.models ?? [])
const hiddenModelCount = computed(() => Math.max(0, allModels.value.length - MODEL_PREVIEW_LIMIT))
const visibleModels = computed(() => {
  if (modelsExpanded.value || allModels.value.length <= MODEL_PREVIEW_LIMIT) return allModels.value
  return allModels.value.slice(0, MODEL_PREVIEW_LIMIT)
})

const modelsSectionTitle = computed(() => {
  const title = effectiveState.value !== 'fresh'
    ? t('admin.accounts.grokModelDiagnostics.historicalModels')
    : t('admin.accounts.grokModelDiagnostics.upstreamModels')
  return t('admin.accounts.grokModelDiagnostics.modelsWithCount', {
    title,
    count: allModels.value.length
  })
})

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

function mappingBadgeClass(item: GrokModelDiagnosticItem): string {
  if (item.status === 'unverifiable' || item.status === 'out_of_scope') return 'badge badge-gray shrink-0'
  if (effectiveState.value !== 'fresh') return 'badge badge-gray shrink-0'
  if (item.status === 'listed') return 'badge badge-success shrink-0'
  if (item.status === 'not_listed') return 'badge badge-warning shrink-0'
  return 'badge badge-gray shrink-0'
}

function mappingRowClass(item: GrokModelDiagnosticItem): string {
  if (effectiveState.value === 'fresh' && item.status === 'not_listed') {
    return 'bg-amber-50 dark:bg-amber-900/10'
  }
  return 'bg-white dark:bg-dark-800'
}

function requestSync() {
  if (props.syncing || props.loading) return
  emit('sync')
}
</script>
