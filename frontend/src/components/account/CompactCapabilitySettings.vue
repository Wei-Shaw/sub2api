<template>
  <div class="space-y-4" data-testid="compact-capability-settings">
    <div class="flex items-start justify-between gap-4">
      <div class="min-w-0">
        <label class="input-label mb-0" :for="modeSelectId">
          {{ t('admin.accounts.openai.compactNativePolicy') }}
        </label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.compactModeDesc') }}
        </p>
        <p
          v-if="mode === 'force_on'"
          class="mt-1 text-xs text-amber-700 dark:text-amber-300"
          data-testid="compact-force-on-hint"
        >
          {{ t('admin.accounts.openai.compactModeForceOnHint') }}
        </p>
        <p
          v-else-if="mode === 'force_off'"
          class="mt-1 text-xs text-gray-500 dark:text-gray-400"
          data-testid="compact-force-off-hint"
        >
          {{ t('admin.accounts.openai.compactModeForceOffHint') }}
        </p>
      </div>
      <div class="flex flex-shrink-0 items-center gap-2">
        <label
          v-if="showModeApply"
          class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400"
        >
          <span>{{ t('admin.accounts.openai.compactApply') }}</span>
          <input
            :id="modeApplyId"
            :checked="modeApplied"
            type="checkbox"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            @change="$emit('update:modeApplied', ($event.target as HTMLInputElement).checked)"
          />
        </label>
        <div class="w-48">
          <Select
            :id="modeSelectId"
            :model-value="mode"
            :options="modeOptions"
            :disabled="modeDisabled"
            :data-testid="modeTestId"
            @update:model-value="$emit('update:mode', $event as OpenAICompactMode)"
          />
        </div>
      </div>
    </div>

    <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700">
      <div class="flex items-center justify-between gap-4">
        <div class="min-w-0">
          <label class="text-sm font-medium text-gray-800 dark:text-gray-200" :for="summaryInputId">
            {{ t('admin.accounts.openai.compactSummaryEnabled') }}
          </label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.compactSummaryEnabledDesc') }}
          </p>
        </div>
        <label
          v-if="showSummaryApply"
          class="flex flex-shrink-0 items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400"
        >
          <span>{{ t('admin.accounts.openai.compactApply') }}</span>
          <input
            :id="summaryApplyId"
            :checked="summaryApplied"
            type="checkbox"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            @change="$emit('update:summaryApplied', ($event.target as HTMLInputElement).checked)"
          />
        </label>
        <input
          :id="summaryInputId"
          :checked="summaryEnabled"
          type="checkbox"
          :disabled="summaryDisabled"
          class="flex-shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-50"
          @change="$emit('update:summaryEnabled', ($event.target as HTMLInputElement).checked)"
        />
      </div>
    </div>

    <div
      v-if="showProviderDeclaration && providerMode"
      class="rounded-lg border border-gray-200 px-3 py-2 text-xs text-gray-600 dark:border-dark-600 dark:text-gray-300"
      data-testid="compact-provider-declaration"
    >
      <span class="font-medium">{{ t('admin.accounts.openai.compactProviderDeclaration') }}:</span>
      <span class="ml-2">{{ t(providerCapabilityKey) }}</span>
      <span class="mt-1 block text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.compactProviderDeclarationHint') }}
      </span>
    </div>

    <div
      v-if="showLegacyEvidence && hasLegacyEvidence"
      class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200"
      data-testid="compact-legacy-evidence"
    >
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span class="font-medium">{{ t('admin.accounts.openai.compactLegacyEvidence') }}:</span>
        <span>
          {{ legacySupported ? t('admin.accounts.openai.compactSupported') : t('admin.accounts.openai.compactUnsupported') }}
        </span>
        <span v-if="legacyCheckedAt">
          {{ t('admin.accounts.openai.compactLastChecked') }}: {{ formatProbeTime(legacyCheckedAt) }}
        </span>
      </div>
      <p class="mt-1 text-amber-800/80 dark:text-amber-200/80">
        {{ t('admin.accounts.openai.compactLegacyEvidenceHint') }}
      </p>
    </div>

    <div v-if="showProbeHistory" class="space-y-2" data-testid="compact-probe-history">
      <div class="flex items-start justify-between gap-4">
        <div>
          <div class="text-sm font-medium text-gray-800 dark:text-gray-200">
            {{ t('admin.accounts.openai.compactTargetHistory') }}
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.compactTargetHistoryHint') }}
          </p>
        </div>
        <span v-if="probeRecords.length" class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.compactTargetRecords', { count: probeRecords.length }) }}
        </span>
      </div>
      <p
        v-if="probeRecords.length === 0"
        class="rounded-lg border border-dashed border-gray-300 px-3 py-3 text-xs text-gray-500 dark:border-dark-500 dark:text-gray-400"
      >
        {{ t('admin.accounts.openai.compactTargetHistoryEmpty') }}
      </p>
      <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <table class="min-w-full divide-y divide-gray-200 text-left text-xs dark:divide-dark-600">
          <thead class="bg-gray-50 text-gray-500 dark:bg-dark-700 dark:text-gray-400">
            <tr>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.openai.compactTargetKind') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.openai.compactTargetModel') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.openai.compactTargetProtocol') }}</th>
              <th class="min-w-48 px-3 py-2 font-medium">{{ t('admin.accounts.openai.compactTargetEndpoint') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.openai.compactTargetVerdict') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.openai.compactTargetCheckedAt') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.openai.compactTargetError') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 text-gray-700 dark:divide-dark-600 dark:text-gray-300">
            <tr v-for="record in probeRecords" :key="record.key">
              <td class="whitespace-nowrap px-3 py-2">{{ t(probeKindKey(record.target.kind)) }}</td>
              <td class="max-w-52 px-3 py-2">
                <div class="truncate" :title="record.target.model">{{ record.target.model }}</div>
                <div
                  v-if="record.target.requested_model && record.target.requested_model !== record.target.model"
                  class="truncate text-[11px] text-gray-500 dark:text-gray-400"
                  :title="record.target.requested_model"
                >
                  {{ t('admin.accounts.openai.compactTargetRequestedModel') }}: {{ record.target.requested_model }}
                </div>
              </td>
              <td class="whitespace-nowrap px-3 py-2">{{ record.target.protocol }}</td>
              <td class="max-w-72 px-3 py-2">
                <span class="block truncate" :title="displayEndpoint(record.target.endpoint)">
                  {{ displayEndpoint(record.target.endpoint) || '-' }}
                </span>
              </td>
              <td class="whitespace-nowrap px-3 py-2">
                <span :class="probeVerdictClass(record.verdict)">
                  {{ t(probeVerdictKey(record.verdict)) }}
                </span>
                <div v-if="record.verified_at" class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.openai.compactTargetVerifiedAt') }}: {{ formatProbeTime(record.verified_at) }}
                </div>
              </td>
              <td class="whitespace-nowrap px-3 py-2">{{ formatProbeTime(record.last_checked_at) }}</td>
              <td class="max-w-52 px-3 py-2">
                <span class="block truncate" :title="record.error_class || ''">{{ record.error_class || '-' }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="text-[11px] text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.compactTargetHistoryWarning') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import { getProviderMode, getProviderProfile } from '@/constants/platformCatalog'
import type {
  OpenAICompactMode,
  OpenAICompactProbeRecord,
  OpenAICompactProbeTargets
} from '@/types'
import { formatDateTime } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    mode: OpenAICompactMode
    summaryEnabled: boolean
    idPrefix: string
    extra?: Record<string, unknown>
    platform?: string
    accountMode?: string
    modeDisabled?: boolean
    summaryDisabled?: boolean
    modeTestId?: string
    modeApplyId?: string
    summaryApplyId?: string
    summaryInputId?: string
    showModeApply?: boolean
    showSummaryApply?: boolean
    modeApplied?: boolean
    summaryApplied?: boolean
    showProviderDeclaration?: boolean
    showLegacyEvidence?: boolean
    showProbeHistory?: boolean
  }>(),
  {
    extra: undefined,
    platform: '',
    accountMode: '',
    modeDisabled: false,
    summaryDisabled: false,
    modeTestId: '',
    modeApplyId: '',
    summaryApplyId: '',
    summaryInputId: '',
    showModeApply: false,
    showSummaryApply: false,
    modeApplied: false,
    summaryApplied: false,
    showProviderDeclaration: false,
    showLegacyEvidence: false,
    showProbeHistory: false
  }
)

defineEmits<{
  (e: 'update:mode', value: OpenAICompactMode): void
  (e: 'update:summaryEnabled', value: boolean): void
  (e: 'update:modeApplied', value: boolean): void
  (e: 'update:summaryApplied', value: boolean): void
}>()

const { t } = useI18n()

const modeSelectId = computed(() => `${props.idPrefix}-compact-mode-select`)
const modeTestId = computed(() => props.modeTestId || `${props.idPrefix}-compact-mode-select`)
const modeApplyId = computed(() => props.modeApplyId || `${props.idPrefix}-compact-mode-apply`)
const summaryApplyId = computed(() => props.summaryApplyId || `${props.idPrefix}-compact-summary-apply`)
const summaryInputId = computed(() => props.summaryInputId || `${props.idPrefix}-compact-summary-enabled`)

const modeOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') },
  { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') },
  { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') }
])

const providerMode = computed(() =>
  getProviderProfile(props.platform) ? getProviderMode(props.platform, props.accountMode) : undefined
)

const providerCapability = computed<'supported' | 'unsupported' | 'unknown'>(() => {
  const value = providerMode.value?.remote_compaction_v2
  return value === 'supported' || value === 'unsupported' ? value : 'unknown'
})

const providerCapabilityKey = computed(() => {
  switch (providerCapability.value) {
    case 'supported':
      return 'admin.accounts.openai.compactProviderSupported'
    case 'unsupported':
      return 'admin.accounts.openai.compactProviderUnsupported'
    default:
      return 'admin.accounts.openai.compactProviderUnknown'
  }
})

const legacySupported = computed(() => props.extra?.openai_compact_supported === true)
const hasLegacyEvidence = computed(() => typeof props.extra?.openai_compact_supported === 'boolean')
const legacyCheckedAt = computed(() =>
  typeof props.extra?.openai_compact_checked_at === 'string' ? props.extra.openai_compact_checked_at : ''
)

type ProbeRecordRow = OpenAICompactProbeRecord & { key: string }

const probeRecords = computed<ProbeRecordRow[]>(() => {
  const raw = props.extra?.openai_compact_probe_targets as OpenAICompactProbeTargets | undefined
  const values = Array.isArray(raw)
    ? raw
    : raw && typeof raw === 'object'
      ? Object.entries(raw).map(([key, record]) => ({ key, record }))
      : []

  return values
    .map((entry) => {
      if ('record' in entry) {
        return { key: entry.key, ...entry.record }
      }
      return { key: entry.target?.endpoint_hash || entry.last_checked_at || '', ...entry }
    })
    .filter((record): record is ProbeRecordRow => {
      return (
        typeof record.key === 'string' &&
        !!record.target &&
        typeof record.target.kind === 'string' &&
        typeof record.target.model === 'string' &&
        typeof record.target.protocol === 'string' &&
        typeof record.target.endpoint === 'string' &&
        typeof record.verdict === 'string' &&
        typeof record.last_checked_at === 'string'
      )
    })
    .sort((left, right) => {
      const checked = right.last_checked_at.localeCompare(left.last_checked_at)
      return checked || left.key.localeCompare(right.key)
    })
    .slice(0, 8)
})

const probeKindKey = (kind: string) =>
  kind === 'legacy_compact'
    ? 'admin.accounts.openai.compactKindLegacy'
    : 'admin.accounts.openai.compactKindNativeV2'

const probeVerdictKey = (verdict: string) => {
  switch (verdict) {
    case 'supported':
      return 'admin.accounts.openai.compactVerdictSupported'
    case 'unsupported':
      return 'admin.accounts.openai.compactVerdictUnsupported'
    default:
      return 'admin.accounts.openai.compactVerdictUnknown'
  }
}

const probeVerdictClass = (verdict: string) => {
  switch (verdict) {
    case 'supported':
      return 'text-emerald-700 dark:text-emerald-300'
    case 'unsupported':
      return 'text-rose-700 dark:text-rose-300'
    default:
      return 'text-amber-700 dark:text-amber-300'
  }
}

const formatProbeTime = (value: string) => {
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : formatDateTime(parsed)
}

const displayEndpoint = (value: string) => {
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return ''
    return `${parsed.origin}${parsed.pathname}`.replace(/\/$/, '')
  } catch {
    return ''
  }
}
</script>
