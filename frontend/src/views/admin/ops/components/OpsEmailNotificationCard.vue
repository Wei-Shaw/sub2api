<script setup lang="ts">
import { ref, onMounted, computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { opsAPI REDACTED from '@/api/admin/ops'
import type { EmailNotificationConfig, AlertSeverity REDACTED from '../types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'

const { t REDACTED = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const config = ref<EmailNotificationConfig | null>(null)

const showEditor = ref(false)
const saving = ref(false)
const draft = ref<EmailNotificationConfig | null>(null)
const alertRecipientInput = ref('')
const reportRecipientInput = ref('')
const alertRecipientError = ref('')
const reportRecipientError = ref('')

const severityOptions: Array<{ value: AlertSeverity | ''; label: string REDACTED> = [
  { value: '', label: t('admin.ops.email.minSeverityAll') REDACTED,
  { value: 'critical', label: t('common.critical') REDACTED,
  { value: 'warning', label: t('common.warning') REDACTED,
  { value: 'info', label: t('common.info') REDACTED
]

async function loadConfig() {
  loading.value = true
  try {
    const data = await opsAPI.getEmailNotificationConfig()
    config.value = data
  REDACTED catch (err: any) {
    console.error('[OpsEmailNotificationCard] Failed to load config', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.email.loadFailed'))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

async function saveConfig() {
  if (!draft.value) return
  if (!editorValidation.value.valid) {
    appStore.showError(editorValidation.value.errors[0] || t('admin.ops.email.validation.invalid'))
    return
  REDACTED
  saving.value = true
  try {
    config.value = await opsAPI.updateEmailNotificationConfig(draft.value)
    showEditor.value = false
    appStore.showSuccess(t('admin.ops.email.saveSuccess'))
  REDACTED catch (err: any) {
    console.error('[OpsEmailNotificationCard] Failed to save config', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.email.saveFailed'))
  REDACTED finally {
    saving.value = false
  REDACTED
REDACTED

function openEditor() {
  if (!config.value) return
  draft.value = JSON.parse(JSON.stringify(config.value))
  alertRecipientInput.value = ''
  reportRecipientInput.value = ''
  alertRecipientError.value = ''
  reportRecipientError.value = ''
  showEditor.value = true
REDACTED

function isValidEmailAddress(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)
REDACTED

function isNonNegativeNumber(value: unknown): boolean {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
REDACTED

function validateCronField(enabled: boolean, cron: string): string | null {
  if (!enabled) return null
  if (!cron || !cron.trim()) return t('admin.ops.email.validation.cronRequired')
  if (cron.trim().split(/\s+/).length < 5) return t('admin.ops.email.validation.cronFormat')
  return null
REDACTED

const editorValidation = computed(() => {
  const errors: string[] = []
  if (!draft.value) return { valid: true, errors REDACTED

  if (draft.value.alert.enabled && draft.value.alert.recipients.length === 0) {
    errors.push(t('admin.ops.email.validation.alertRecipientsRequired'))
  REDACTED
  if (draft.value.report.enabled && draft.value.report.recipients.length === 0) {
    errors.push(t('admin.ops.email.validation.reportRecipientsRequired'))
  REDACTED

  const invalidAlertRecipients = draft.value.alert.recipients.filter((e) => !isValidEmailAddress(e))
  if (invalidAlertRecipients.length > 0) errors.push(t('admin.ops.email.validation.invalidRecipients'))

  const invalidReportRecipients = draft.value.report.recipients.filter((e) => !isValidEmailAddress(e))
  if (invalidReportRecipients.length > 0) errors.push(t('admin.ops.email.validation.invalidRecipients'))

  if (!isNonNegativeNumber(draft.value.alert.rate_limit_per_hour)) {
    errors.push(t('admin.ops.email.validation.rateLimitRange'))
  REDACTED
  if (
    !isNonNegativeNumber(draft.value.alert.batching_window_seconds) ||
    draft.value.alert.batching_window_seconds > 86400
  ) {
    errors.push(t('admin.ops.email.validation.batchWindowRange'))
  REDACTED

  const dailyErr = validateCronField(
    draft.value.report.daily_summary_enabled,
    draft.value.report.daily_summary_schedule
  )
  if (dailyErr) errors.push(dailyErr)
  const weeklyErr = validateCronField(
    draft.value.report.weekly_summary_enabled,
    draft.value.report.weekly_summary_schedule
  )
  if (weeklyErr) errors.push(weeklyErr)
  const digestErr = validateCronField(
    draft.value.report.error_digest_enabled,
    draft.value.report.error_digest_schedule
  )
  if (digestErr) errors.push(digestErr)
  const accErr = validateCronField(
    draft.value.report.account_health_enabled,
    draft.value.report.account_health_schedule
  )
  if (accErr) errors.push(accErr)

  if (!isNonNegativeNumber(draft.value.report.error_digest_min_count)) {
    errors.push(t('admin.ops.email.validation.digestMinCountRange'))
  REDACTED

  const thr = draft.value.report.account_health_error_rate_threshold
  if (!(typeof thr === 'number' && Number.isFinite(thr) && thr >= 0 && thr <= 100)) {
    errors.push(t('admin.ops.email.validation.accountHealthThresholdRange'))
  REDACTED

  return { valid: errors.length === 0, errors REDACTED
REDACTED)

function addRecipient(target: 'alert' | 'report') {
  if (!draft.value) return
  const raw = (target === 'alert' ? alertRecipientInput.value : reportRecipientInput.value).trim()
  if (!raw) return

  if (!isValidEmailAddress(raw)) {
    const msg = t('common.invalidEmail')
    if (target === 'alert') alertRecipientError.value = msg
    else reportRecipientError.value = msg
    return
  REDACTED

  const normalized = raw.toLowerCase()
  const list = target === 'alert' ? draft.value.alert.recipients : draft.value.report.recipients
  if (!list.includes(normalized)) {
    list.push(normalized)
  REDACTED
  if (target === 'alert') alertRecipientInput.value = ''
  else reportRecipientInput.value = ''
  if (target === 'alert') alertRecipientError.value = ''
  else reportRecipientError.value = ''
REDACTED

function removeRecipient(target: 'alert' | 'report', email: string) {
  if (!draft.value) return
  const list = target === 'alert' ? draft.value.alert.recipients : draft.value.report.recipients
  const idx = list.indexOf(email)
  if (idx >= 0) list.splice(idx, 1)
REDACTED

onMounted(() => {
  loadConfig()
REDACTED)
</script>

<template>
  <div class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <div class="mb-4 flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">{{ t('admin.ops.email.title') REDACTEDREDACTED</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.email.description') REDACTEDREDACTED</p>
      </div>
      <div class="flex items-center gap-2">
        <button
          class="flex items-center gap-1.5 rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-bold text-gray-700 transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
          :disabled="loading"
          @click="loadConfig"
        >
          <svg class="h-3.5 w-3.5" :class="{ 'animate-spin': loading REDACTED" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          {{ t('common.refresh') REDACTEDREDACTED
        </button>
        <button class="btn btn-sm btn-secondary" :disabled="!config" @click="openEditor">{{ t('common.edit') REDACTEDREDACTED</button>
      </div>
    </div>

    <div v-if="!config" class="text-sm text-gray-500 dark:text-gray-400">
      <span v-if="loading">{{ t('admin.ops.email.loading') REDACTEDREDACTED</span>
      <span v-else>{{ t('admin.ops.email.noData') REDACTEDREDACTED</span>
    </div>

    <div v-else class="space-y-6">
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.email.alertTitle') REDACTEDREDACTED</h4>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('common.enabled') REDACTEDREDACTED:
            <span class="ml-1 font-medium text-gray-900 dark:text-white">
              {{ config.alert.enabled ? t('common.enabled') : t('common.disabled') REDACTEDREDACTED
            </span>
          </div>
          <div class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('admin.ops.email.recipients') REDACTEDREDACTED:
            <span class="ml-1 font-medium text-gray-900 dark:text-white">{{ config.alert.recipients.length REDACTEDREDACTED</span>
          </div>
          <div class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('admin.ops.email.minSeverity') REDACTEDREDACTED:
            <span class="ml-1 font-medium text-gray-900 dark:text-white">{{
              config.alert.min_severity || t('admin.ops.email.minSeverityAll')
            REDACTEDREDACTED</span>
          </div>
          <div class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('admin.ops.email.rateLimitPerHour') REDACTEDREDACTED:
            <span class="ml-1 font-medium text-gray-900 dark:text-white">{{ config.alert.rate_limit_per_hour REDACTEDREDACTED</span>
          </div>
        </div>
      </div>

      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.email.reportTitle') REDACTEDREDACTED</h4>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('common.enabled') REDACTEDREDACTED:
            <span class="ml-1 font-medium text-gray-900 dark:text-white">
              {{ config.report.enabled ? t('common.enabled') : t('common.disabled') REDACTEDREDACTED
            </span>
          </div>
          <div class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('admin.ops.email.recipients') REDACTEDREDACTED:
            <span class="ml-1 font-medium text-gray-900 dark:text-white">{{ config.report.recipients.length REDACTEDREDACTED</span>
          </div>
        </div>
      </div>
    </div>
  </div>

  <BaseDialog :show="showEditor" :title="t('admin.ops.email.title')" width="extra-wide" @close="showEditor = false">
    <div v-if="draft" class="space-y-6">
      <div
        v-if="!editorValidation.valid"
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200"
      >
        <div class="font-bold">{{ t('admin.ops.email.validation.title') REDACTEDREDACTED</div>
        <ul class="mt-1 list-disc space-y-1 pl-4">
          <li v-for="msg in editorValidation.errors" :key="msg">{{ msg REDACTEDREDACTED</li>
        </ul>
      </div>
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.email.alertTitle') REDACTEDREDACTED</h4>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('common.enabled') REDACTEDREDACTED</div>
            <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input v-model="draft.alert.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
              <span>{{ draft.alert.enabled ? t('common.enabled') : t('common.disabled') REDACTEDREDACTED</span>
            </label>
          </div>

          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.minSeverity') REDACTEDREDACTED</div>
            <Select v-model="draft.alert.min_severity" :options="severityOptions" />
          </div>

          <div class="md:col-span-2">
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.recipients') REDACTEDREDACTED</div>
            <div class="flex gap-2">
              <input
                v-model="alertRecipientInput"
                type="email"
                class="input"
                :placeholder="t('admin.ops.email.recipients')"
                @keydown.enter.prevent="addRecipient('alert')"
              />
              <button class="btn btn-secondary whitespace-nowrap" type="button" @click="addRecipient('alert')">
                {{ t('common.add') REDACTEDREDACTED
              </button>
            </div>
            <p v-if="alertRecipientError" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ alertRecipientError REDACTEDREDACTED</p>
            <div class="mt-2 flex flex-wrap gap-2">
              <span
                v-for="email in draft.alert.recipients"
                :key="email"
                class="inline-flex items-center gap-2 rounded-full bg-blue-100 px-3 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
              >
                {{ email REDACTEDREDACTED
                <button
                  type="button"
                  class="text-blue-700/80 hover:text-blue-900 dark:text-blue-300"
                  @click="removeRecipient('alert', email)"
                >
                  ×
                </button>
              </span>
            </div>
            <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.email.recipientsHint') REDACTEDREDACTED</div>
          </div>

          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.rateLimitPerHour') REDACTEDREDACTED</div>
            <input v-model.number="draft.alert.rate_limit_per_hour" type="number" min="0" max="100000" class="input" />
          </div>

          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.batchWindowSeconds') REDACTEDREDACTED</div>
            <input v-model.number="draft.alert.batching_window_seconds" type="number" min="0" max="86400" class="input" />
          </div>

          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.includeResolved') REDACTEDREDACTED</div>
            <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input v-model="draft.alert.include_resolved_alerts" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
              <span>{{ draft.alert.include_resolved_alerts ? t('common.enabled') : t('common.disabled') REDACTEDREDACTED</span>
            </label>
          </div>
        </div>
      </div>

      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.email.reportTitle') REDACTEDREDACTED</h4>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('common.enabled') REDACTEDREDACTED</div>
            <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input v-model="draft.report.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
              <span>{{ draft.report.enabled ? t('common.enabled') : t('common.disabled') REDACTEDREDACTED</span>
            </label>
          </div>

          <div class="md:col-span-2">
            <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.recipients') REDACTEDREDACTED</div>
            <div class="flex gap-2">
              <input
                v-model="reportRecipientInput"
                type="email"
                class="input"
                :placeholder="t('admin.ops.email.recipients')"
                @keydown.enter.prevent="addRecipient('report')"
              />
              <button class="btn btn-secondary whitespace-nowrap" type="button" @click="addRecipient('report')">
                {{ t('common.add') REDACTEDREDACTED
              </button>
            </div>
            <p v-if="reportRecipientError" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ reportRecipientError REDACTEDREDACTED</p>
            <div class="mt-2 flex flex-wrap gap-2">
              <span
                v-for="email in draft.report.recipients"
                :key="email"
                class="inline-flex items-center gap-2 rounded-full bg-blue-100 px-3 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
              >
                {{ email REDACTEDREDACTED
                <button
                  type="button"
                  class="text-blue-700/80 hover:text-blue-900 dark:text-blue-300"
                  @click="removeRecipient('report', email)"
                >
                  ×
                </button>
              </span>
            </div>
          </div>

          <div class="md:col-span-2">
            <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.dailySummary') REDACTEDREDACTED</div>
                <div class="flex items-center gap-2">
                  <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <input v-model="draft.report.daily_summary_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
                  </label>
                  <input v-model="draft.report.daily_summary_schedule" type="text" class="input" :placeholder="t('admin.ops.email.cronPlaceholder')" />
                </div>
              </div>
              <div>
                <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.weeklySummary') REDACTEDREDACTED</div>
                <div class="flex items-center gap-2">
                  <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <input v-model="draft.report.weekly_summary_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
                  </label>
                  <input v-model="draft.report.weekly_summary_schedule" type="text" class="input" :placeholder="t('admin.ops.email.cronPlaceholder')" />
                </div>
              </div>
              <div>
                <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.errorDigest') REDACTEDREDACTED</div>
                <div class="flex items-center gap-2">
                  <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <input v-model="draft.report.error_digest_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
                  </label>
                  <input v-model="draft.report.error_digest_schedule" type="text" class="input" :placeholder="t('admin.ops.email.cronPlaceholder')" />
                </div>
              </div>
              <div>
                <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.errorDigestMinCount') REDACTEDREDACTED</div>
                <input v-model.number="draft.report.error_digest_min_count" type="number" min="0" max="1000000" class="input" />
              </div>
              <div>
                <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.accountHealth') REDACTEDREDACTED</div>
                <div class="flex items-center gap-2">
                  <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <input v-model="draft.report.account_health_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
                  </label>
                  <input v-model="draft.report.account_health_schedule" type="text" class="input" :placeholder="t('admin.ops.email.cronPlaceholder')" />
                </div>
              </div>
              <div>
                <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.email.accountHealthThreshold') REDACTEDREDACTED</div>
                <input v-model.number="draft.report.account_health_error_rate_threshold" type="number" min="0" max="100" step="0.1" class="input" />
              </div>
            </div>
            <div class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.email.reportHint') REDACTEDREDACTED</div>
          </div>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn btn-secondary" @click="showEditor = false">{{ t('common.cancel') REDACTEDREDACTED</button>
        <button class="btn btn-primary" :disabled="saving || !editorValidation.valid" @click="saveConfig">
          {{ saving ? t('common.saving') : t('common.save') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>
</template>
