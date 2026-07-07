<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { opsAPI } from '@/api/admin/ops'
import type { OpsAlertRuntimeSettings } from '../types'
import BaseDialog from '@/components/common/BaseDialog.vue'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)

const alertSettings = ref<OpsAlertRuntimeSettings | null>(null)

const showAlertEditor = ref(false)
const draftAlert = ref<OpsAlertRuntimeSettings | null>(null)

type ValidationResult = { valid: boolean; errors: string[] }

function normalizeSeverities(input: Array<string | null | undefined> | null | undefined): string[] {
  if (!input || input.length === 0) return []
  const allowed = new Set(['P0', 'P1', 'P2', 'P3'])
  const out: string[] = []
  const seen = new Set<string>()
  for (const raw of input) {
    const s = String(raw || '')
      .trim()
      .toUpperCase()
    if (!s) continue
    if (!allowed.has(s)) continue
    if (seen.has(s)) continue
    seen.add(s)
    out.push(s)
  }
  return out
}

function validateRuntimeSettings(settings: OpsAlertRuntimeSettings): ValidationResult {
  const errors: string[] = []

  const evalSeconds = settings.evaluation_interval_seconds
  if (!Number.isFinite(evalSeconds) || evalSeconds < 1 || evalSeconds > 86400) {
    errors.push(t('admin.ops.runtime.validation.evalIntervalRange'))
  }

  // Thresholds validation
  const thresholds = settings.thresholds
  if (thresholds) {
    if (thresholds.sla_percent_min != null) {
      if (!Number.isFinite(thresholds.sla_percent_min) || thresholds.sla_percent_min < 0 || thresholds.sla_percent_min > 100) {
        errors.push(t('admin.ops.runtime.validation.slaMinPercentRange'))
      }
    }
    if (thresholds.ttft_p99_ms_max != null) {
      if (!Number.isFinite(thresholds.ttft_p99_ms_max) || thresholds.ttft_p99_ms_max < 0) {
        errors.push(t('admin.ops.runtime.validation.ttftP99MaxRange'))
      }
    }
    if (thresholds.request_error_rate_percent_max != null) {
      if (!Number.isFinite(thresholds.request_error_rate_percent_max) || thresholds.request_error_rate_percent_max < 0 || thresholds.request_error_rate_percent_max > 100) {
        errors.push(t('admin.ops.runtime.validation.requestErrorRateMaxRange'))
      }
    }
    if (thresholds.upstream_error_rate_percent_max != null) {
      if (!Number.isFinite(thresholds.upstream_error_rate_percent_max) || thresholds.upstream_error_rate_percent_max < 0 || thresholds.upstream_error_rate_percent_max > 100) {
        errors.push(t('admin.ops.runtime.validation.upstreamErrorRateMaxRange'))
      }
    }
  }

  const lock = settings.distributed_lock
  if (lock?.enabled) {
    if (!lock.key || lock.key.trim().length < 3) {
      errors.push(t('admin.ops.runtime.validation.lockKeyRequired'))
    } else if (!lock.key.startsWith('ops:')) {
      errors.push(t('admin.ops.runtime.validation.lockKeyPrefix', { prefix: 'ops:' }))
    }
    if (!Number.isFinite(lock.ttl_seconds) || lock.ttl_seconds < 1 || lock.ttl_seconds > 86400) {
      errors.push(t('admin.ops.runtime.validation.lockTtlRange'))
    }
  }

  // Silencing validation (alert-only)
  const silencing = settings.silencing
  if (silencing?.enabled) {
    const until = (silencing.global_until_rfc3339 || '').trim()
    if (until) {
      const parsed = Date.parse(until)
      if (!Number.isFinite(parsed)) errors.push(t('admin.ops.runtime.silencing.validation.timeFormat'))
    }

    const entries = Array.isArray(silencing.entries) ? silencing.entries : []
    for (let idx = 0; idx < entries.length; idx++) {
      const entry = entries[idx]
      const untilEntry = (entry?.until_rfc3339 || '').trim()
      if (!untilEntry) {
        errors.push(t('admin.ops.runtime.silencing.entries.validation.untilRequired'))
        break
      }
      const parsedEntry = Date.parse(untilEntry)
      if (!Number.isFinite(parsedEntry)) {
        errors.push(t('admin.ops.runtime.silencing.entries.validation.untilFormat'))
        break
      }
      const ruleId = (entry as any)?.rule_id
      if (typeof ruleId === 'number' && (!Number.isFinite(ruleId) || ruleId <= 0)) {
        errors.push(t('admin.ops.runtime.silencing.entries.validation.ruleIdPositive'))
        break
      }
      if ((entry as any)?.severities) {
        const raw = (entry as any).severities
        const normalized = normalizeSeverities(Array.isArray(raw) ? raw : [raw])
        if (Array.isArray(raw) && raw.length > 0 && normalized.length === 0) {
          errors.push(t('admin.ops.runtime.silencing.entries.validation.severitiesFormat'))
          break
        }
      }
    }
  }

  return { valid: errors.length === 0, errors }
}

const alertValidation = computed(() => {
  if (!draftAlert.value) return { valid: true, errors: [] as string[] }
  return validateRuntimeSettings(draftAlert.value)
})

async function loadSettings() {
  loading.value = true
  try {
    alertSettings.value = await opsAPI.getAlertRuntimeSettings()
  } catch (err: any) {
    console.error('[OpsRuntimeSettingsCard] Failed to load runtime settings', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.runtime.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openAlertEditor() {
  if (!alertSettings.value) return
  draftAlert.value = JSON.parse(JSON.stringify(alertSettings.value))

  // Backwards-compat: ensure nested settings exist even if API payload is older.
  if (draftAlert.value) {
    if (!draftAlert.value.distributed_lock) {
      draftAlert.value.distributed_lock = { enabled: true, key: 'ops:alert:evaluator:leader', ttl_seconds: 30 }
    }
    if (!draftAlert.value.silencing) {
      draftAlert.value.silencing = { enabled: false, global_until_rfc3339: '', global_reason: '', entries: [] }
    }
    if (!Array.isArray(draftAlert.value.silencing.entries)) {
      draftAlert.value.silencing.entries = []
    }
    if (!draftAlert.value.thresholds) {
      draftAlert.value.thresholds = {
        sla_percent_min: 99.5,
        ttft_p99_ms_max: 500,
        request_error_rate_percent_max: 5,
        upstream_error_rate_percent_max: 5
      }
    }
  }

  showAlertEditor.value = true
}

function addSilenceEntry() {
  if (!draftAlert.value) return
  if (!draftAlert.value.silencing) {
    draftAlert.value.silencing = { enabled: true, global_until_rfc3339: '', global_reason: '', entries: [] }
  }
  if (!Array.isArray(draftAlert.value.silencing.entries)) {
    draftAlert.value.silencing.entries = []
  }
  draftAlert.value.silencing.entries.push({
    rule_id: undefined,
    severities: [],
    until_rfc3339: '',
    reason: ''
  })
}

function removeSilenceEntry(index: number) {
  if (!draftAlert.value?.silencing?.entries) return
  draftAlert.value.silencing.entries.splice(index, 1)
}

function updateSilenceEntryRuleId(index: number, raw: string) {
  const entries = draftAlert.value?.silencing?.entries
  if (!entries || !entries[index]) return
  const trimmed = raw.trim()
  if (!trimmed) {
    delete (entries[index] as any).rule_id
    return
  }
  const n = Number.parseInt(trimmed, 10)
  ;(entries[index] as any).rule_id = Number.isFinite(n) ? n : undefined
}

function updateSilenceEntrySeverities(index: number, raw: string) {
  const entries = draftAlert.value?.silencing?.entries
  if (!entries || !entries[index]) return
  const parts = raw
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  ;(entries[index] as any).severities = normalizeSeverities(parts)
}

async function saveAlertSettings() {
  if (!draftAlert.value) return
  if (!alertValidation.value.valid) {
    appStore.showError(alertValidation.value.errors[0] || t('admin.ops.runtime.validation.invalid'))
    return
  }

  saving.value = true
  try {
    alertSettings.value = await opsAPI.updateAlertRuntimeSettings(draftAlert.value)
    showAlertEditor.value = false
    appStore.showSuccess(t('admin.ops.runtime.saveSuccess'))
  } catch (err: any) {
    console.error('[OpsRuntimeSettingsCard] Failed to save alert runtime settings', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.runtime.saveFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadSettings()
})
</script>

<template>
  <div class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <div class="mb-4 flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">{{ t('admin.ops.runtime.title') }}</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.runtime.description') }}</p>
      </div>
      <button
        class="flex items-center gap-1.5 rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-bold text-gray-700 transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
        :disabled="loading"
        @click="loadSettings"
      >
        <svg class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
        {{ t('common.refresh') }}
      </button>
    </div>

    <div v-if="!alertSettings" class="text-sm text-gray-500 dark:text-gray-400">
      <span v-if="loading">{{ t('admin.ops.runtime.loading') }}</span>
      <span v-else>{{ t('admin.ops.runtime.noData') }}</span>
    </div>

    <div v-else class="space-y-6">
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <div class="mb-3 flex items-center justify-between">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.runtime.alertTitle') }}</h4>
          <button class="btn btn-sm btn-secondary" @click="openAlertEditor">{{ t('common.edit') }}</button>
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('admin.ops.runtime.evalIntervalSeconds') }}:
            <span class="ml-1 font-medium text-gray-900 dark:text-white">{{ alertSettings.evaluation_interval_seconds }}s</span>
          </div>
          <div
            v-if="alertSettings.silencing?.enabled && alertSettings.silencing.global_until_rfc3339"
            class="text-xs text-gray-600 dark:text-gray-300 md:col-span-2"
          >
            {{ t('admin.ops.runtime.silencing.globalUntil') }}:
            <span class="ml-1 font-mono text-gray-900 dark:text-white">{{ alertSettings.silencing.global_until_rfc3339 }}</span>
          </div>

          <details class="col-span-1 md:col-span-2">
            <summary class="cursor-pointer text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400">
              {{ t('admin.ops.runtime.showAdvancedDeveloperSettings') }}
            </summary>
            <div class="mt-2 grid grid-cols-1 gap-3 rounded-lg bg-gray-100 p-3 dark:bg-dark-800 md:grid-cols-2">
              <div class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.runtime.lockEnabled') }}:
                <span class="ml-1 font-mono text-gray-700 dark:text-gray-300">{{ alertSettings.distributed_lock.enabled }}</span>
              </div>
              <div class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.runtime.lockKey') }}:
                <span class="ml-1 font-mono text-gray-700 dark:text-gray-300">{{ alertSettings.distributed_lock.key }}</span>
              </div>
              <div class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.runtime.lockTTLSeconds') }}:
                <span class="ml-1 font-mono text-gray-700 dark:text-gray-300">{{ alertSettings.distributed_lock.ttl_seconds }}s</span>
              </div>
            </div>
          </details>
        </div>
      </div>
    </div>
  </div>

  <BaseDialog :show="showAlertEditor" :title="t('admin.ops.runtime.alertTitle')" width="extra-wide" @close="showAlertEditor = false">
    <div v-if="draftAlert" class="space-y-4">
      <div
        v-if="!alertValidation.valid"
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200"
      >
        <div class="font-bold">{{ t('admin.ops.runtime.validation.title') }}</div>
        <ul class="mt-1 list-disc space-y-1 pl-4">
          <li v-for="msg in alertValidation.errors" :key="msg">{{ msg }}</li>
        </ul>
      </div>

      <div>
        <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.evalIntervalSeconds') }}</div>
        <input
          v-model.number="draftAlert.evaluation_interval_seconds"
          type="number"
          min="1"
          max="86400"
          class="input"
          :aria-invalid="!alertValidation.valid"
        />
        <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.runtime.evalIntervalHint') }}</p>
      </div>

      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <div class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.runtime.metricThresholds') }}</div>
        <p class="mb-4 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.runtime.metricThresholdsHint') }}</p>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.slaMinPercent') }}</div>
            <input
              v-model.number="draftAlert.thresholds.sla_percent_min"
              type="number"
              min="0"
              max="100"
              step="0.1"
              class="input"
              placeholder="99.5"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.runtime.slaMinPercentHint') }}</p>
          </div>



          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.ttftP99MaxMs') }}</div>
            <input
              v-model.number="draftAlert.thresholds.ttft_p99_ms_max"
              type="number"
              min="0"
              step="100"
              class="input"
              placeholder="500"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.runtime.ttftP99MaxMsHint') }}</p>
          </div>

          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.requestErrorRateMaxPercent') }}</div>
            <input
              v-model.number="draftAlert.thresholds.request_error_rate_percent_max"
              type="number"
              min="0"
              max="100"
              step="0.1"
              class="input"
              placeholder="5"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.runtime.requestErrorRateMaxPercentHint') }}</p>
          </div>

          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.upstreamErrorRateMaxPercent') }}</div>
            <input
              v-model.number="draftAlert.thresholds.upstream_error_rate_percent_max"
              type="number"
              min="0"
              max="100"
              step="0.1"
              class="input"
              placeholder="5"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.runtime.upstreamErrorRateMaxPercentHint') }}</p>
          </div>
        </div>
      </div>

      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <div class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.runtime.silencing.title') }}</div>

        <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="draftAlert.silencing.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
          <span>{{ t('admin.ops.runtime.silencing.enabled') }}</span>
        </label>

        <div v-if="draftAlert.silencing.enabled" class="mt-4 space-y-4">
          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.silencing.globalUntil') }}</div>
            <input
              v-model="draftAlert.silencing.global_until_rfc3339"
              type="text"
              class="input font-mono text-sm"
                      placeholder="2026-01-05T00:00:00Z"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.runtime.silencing.untilHint') }}</p>
          </div>

          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.silencing.reason') }}</div>
            <input
              v-model="draftAlert.silencing.global_reason"
              type="text"
              class="input"
              :placeholder="t('admin.ops.runtime.silencing.reasonPlaceholder')"
            />
          </div>

          <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <div class="flex items-start justify-between gap-4">
              <div>
                <div class="text-xs font-bold text-gray-900 dark:text-white">{{ t('admin.ops.runtime.silencing.entries.title') }}</div>
                <p class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.ops.runtime.silencing.entries.hint') }}</p>
              </div>
              <button class="btn btn-sm btn-secondary" type="button" @click="addSilenceEntry">
                {{ t('admin.ops.runtime.silencing.entries.add') }}
              </button>
            </div>

            <div v-if="!draftAlert.silencing.entries?.length" class="mt-3 rounded-lg bg-gray-50 p-3 text-xs text-gray-500 dark:bg-dark-900 dark:text-gray-400">
              {{ t('admin.ops.runtime.silencing.entries.empty') }}
            </div>

            <div v-else class="mt-4 space-y-4">
              <div
                v-for="(entry, idx) in draftAlert.silencing.entries"
                :key="idx"
                class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900"
              >
                <div class="mb-3 flex items-center justify-between">
                  <div class="text-xs font-bold text-gray-900 dark:text-white">
                    {{ t('admin.ops.runtime.silencing.entries.entryTitle', { n: idx + 1 }) }}
                  </div>
                  <button class="btn btn-sm btn-danger" type="button" @click="removeSilenceEntry(idx)">{{ t('common.delete') }}</button>
                </div>

                <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                  <div>
                    <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.silencing.entries.ruleId') }}</div>
                    <input
                      :value="typeof (entry as any).rule_id === 'number' ? String((entry as any).rule_id) : ''"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="t('admin.ops.runtime.silencing.entries.ruleIdPlaceholder')"
                      @input="updateSilenceEntryRuleId(idx, ($event.target as HTMLInputElement).value)"
                    />
                  </div>

                  <div>
                    <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.silencing.entries.severities') }}</div>
                    <input
                      :value="Array.isArray((entry as any).severities) ? (entry as any).severities.join(', ') : ''"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="t('admin.ops.runtime.silencing.entries.severitiesPlaceholder')"
                      @input="updateSilenceEntrySeverities(idx, ($event.target as HTMLInputElement).value)"
                    />
                  </div>

                  <div>
                    <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.silencing.entries.until') }}</div>
                    <input
                      v-model="(entry as any).until_rfc3339"
                      type="text"
                      class="input font-mono text-sm"
              placeholder="2026-01-05T00:00:00Z"
                    />
                  </div>

                  <div>
                    <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.runtime.silencing.entries.reason') }}</div>
                    <input
                      v-model="(entry as any).reason"
                      type="text"
                      class="input"
                      :placeholder="t('admin.ops.runtime.silencing.reasonPlaceholder')"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <details class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
        <summary class="cursor-pointer text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.ops.runtime.advancedSettingsSummary') }}</summary>
        <div class="mt-3 grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="inline-flex items-center gap-2 text-xs text-gray-700 dark:text-gray-300">
              <input v-model="draftAlert.distributed_lock.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
              <span>{{ t('admin.ops.runtime.lockEnabled') }}</span>
            </label>
          </div>
          <div class="md:col-span-2">
            <div class="mb-1 text-xs font-medium text-gray-500">{{ t('admin.ops.runtime.lockKey') }}</div>
            <input v-model="draftAlert.distributed_lock.key" type="text" class="input text-xs font-mono" />
            <p v-if="draftAlert.distributed_lock.enabled" class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.runtime.validation.lockKeyHint', { prefix: 'ops:' }) }}
            </p>
          </div>
          <div>
            <div class="mb-1 text-xs font-medium text-gray-500">{{ t('admin.ops.runtime.lockTTLSeconds') }}</div>
            <input v-model.number="draftAlert.distributed_lock.ttl_seconds" type="number" min="1" max="86400" class="input text-xs font-mono" />
          </div>
        </div>
      </details>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn btn-secondary" @click="showAlertEditor = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving || !alertValidation.valid" @click="saveAlertSettings">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

