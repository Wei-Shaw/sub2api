<template>
  <BaseDialog :show="show" :title="title" width="full" :close-on-click-outside="true" @close="close">
    <div v-if="loading" class="flex items-center justify-center py-16">
      <div class="flex flex-col items-center gap-3">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
        <div class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.errorDetail.loading') REDACTEDREDACTED</div>
      </div>
    </div>

    <div v-else-if="!detail" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ emptyText REDACTEDREDACTED
    </div>

    <div v-else class="space-y-6 p-6">
      <!-- Top Summary -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-4">
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.requestId') REDACTEDREDACTED</div>
          <div class="mt-1 break-all font-mono text-sm font-medium text-gray-900 dark:text-white">
            {{ detail.request_id || detail.client_request_id || '—' REDACTEDREDACTED
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.time') REDACTEDREDACTED</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ formatDateTime(detail.created_at) REDACTEDREDACTED
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.phase') REDACTEDREDACTED</div>
          <div class="mt-1 text-sm font-bold uppercase text-gray-900 dark:text-white">
            {{ detail.phase || '—' REDACTEDREDACTED
          </div>
          <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ detail.type || '—' REDACTEDREDACTED
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.status') REDACTEDREDACTED</div>
          <div class="mt-1 flex flex-wrap items-center gap-2">
            <span :class="['inline-flex items-center rounded-lg px-2 py-1 text-xs font-black ring-1 ring-inset shadow-sm', statusClass]">
              {{ detail.status_code REDACTEDREDACTED
            </span>
            <span
              v-if="detail.severity"
              :class="['rounded-md px-2 py-0.5 text-[10px] font-black shadow-sm', severityClass]"
            >
              {{ detail.severity REDACTEDREDACTED
            </span>
          </div>
        </div>
      </div>

      <!-- Message -->
      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <h3 class="mb-4 text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.message') REDACTEDREDACTED</h3>
        <div class="text-sm font-medium text-gray-800 dark:text-gray-200 break-words">
          {{ detail.message || '—' REDACTEDREDACTED
        </div>
      </div>

      <!-- Basic Info -->
      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <h3 class="mb-4 text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.basicInfo') REDACTEDREDACTED</h3>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <div>
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.platform') REDACTEDREDACTED</div>
            <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ detail.platform || '—' REDACTEDREDACTED</div>
          </div>
          <div>
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.model') REDACTEDREDACTED</div>
            <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ detail.model || '—' REDACTEDREDACTED</div>
          </div>
          <div>
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.latency') REDACTEDREDACTED</div>
            <div class="mt-1 font-mono text-sm font-bold text-gray-900 dark:text-white">
              {{ detail.latency_ms != null ? `${detail.latency_msREDACTEDms` : '—' REDACTEDREDACTED
            </div>
          </div>
          <div>
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.ttft') REDACTEDREDACTED</div>
            <div class="mt-1 font-mono text-sm font-bold text-gray-900 dark:text-white">
              {{ detail.time_to_first_token_ms != null ? `${detail.time_to_first_token_msREDACTEDms` : '—' REDACTEDREDACTED
            </div>
          </div>
          <div>
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.businessLimited') REDACTEDREDACTED</div>
            <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
              {{ detail.is_business_limited ? 'true' : 'false' REDACTEDREDACTED
            </div>
          </div>
          <div>
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.requestPath') REDACTEDREDACTED</div>
            <div class="mt-1 font-mono text-xs text-gray-700 dark:text-gray-200 break-all">
              {{ detail.request_path || '—' REDACTEDREDACTED
            </div>
          </div>
        </div>
      </div>

      <!-- Timings (best-effort fields) -->
      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <h3 class="mb-4 text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.timings') REDACTEDREDACTED</h3>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div class="rounded-lg bg-white p-4 shadow-sm dark:bg-dark-800">
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.auth') REDACTEDREDACTED</div>
            <div class="mt-1 font-mono text-sm font-bold text-gray-900 dark:text-white">
              {{ detail.auth_latency_ms != null ? `${detail.auth_latency_msREDACTEDms` : '—' REDACTEDREDACTED
            </div>
          </div>
          <div class="rounded-lg bg-white p-4 shadow-sm dark:bg-dark-800">
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.routing') REDACTEDREDACTED</div>
            <div class="mt-1 font-mono text-sm font-bold text-gray-900 dark:text-white">
              {{ detail.routing_latency_ms != null ? `${detail.routing_latency_msREDACTEDms` : '—' REDACTEDREDACTED
            </div>
          </div>
          <div class="rounded-lg bg-white p-4 shadow-sm dark:bg-dark-800">
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.upstream') REDACTEDREDACTED</div>
            <div class="mt-1 font-mono text-sm font-bold text-gray-900 dark:text-white">
              {{ detail.upstream_latency_ms != null ? `${detail.upstream_latency_msREDACTEDms` : '—' REDACTEDREDACTED
            </div>
          </div>
          <div class="rounded-lg bg-white p-4 shadow-sm dark:bg-dark-800">
            <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.response') REDACTEDREDACTED</div>
            <div class="mt-1 font-mono text-sm font-bold text-gray-900 dark:text-white">
              {{ detail.response_latency_ms != null ? `${detail.response_latency_msREDACTEDms` : '—' REDACTEDREDACTED
            </div>
          </div>
        </div>
      </div>

      <!-- Retry -->
      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <div class="flex flex-col justify-between gap-4 md:flex-row md:items-start">
          <div class="space-y-1">
            <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.retry') REDACTEDREDACTED</h3>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.errorDetail.retryNote1') REDACTEDREDACTED
            </div>
          </div>
          <div class="flex flex-wrap gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="retrying" @click="openRetryConfirm('client')">
              {{ t('admin.ops.errorDetail.retryClient') REDACTEDREDACTED
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="retrying || !pinnedAccountId"
              @click="openRetryConfirm('upstream')"
              :title="pinnedAccountId ? '' : t('admin.ops.errorDetail.retryUpstreamHint')"
            >
              {{ t('admin.ops.errorDetail.retryUpstream') REDACTEDREDACTED
            </button>
          </div>
        </div>

        <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-3">
          <div class="md:col-span-1">
            <label class="mb-1 block text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.pinnedAccountId') REDACTEDREDACTED</label>
            <input v-model="pinnedAccountIdInput" type="text" class="input font-mono text-sm" :placeholder="t('admin.ops.errorDetail.pinnedAccountIdHint')" />
            <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.errorDetail.retryNote2') REDACTEDREDACTED
            </div>
          </div>
          <div class="md:col-span-2">
            <div class="rounded-lg bg-white p-4 shadow-sm dark:bg-dark-800">
              <div class="text-xs font-bold uppercase text-gray-400">{{ t('admin.ops.errorDetail.retryNotes') REDACTEDREDACTED</div>
              <ul class="mt-2 list-disc space-y-1 pl-5 text-xs text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.ops.errorDetail.retryNote3') REDACTEDREDACTED</li>
                <li>{{ t('admin.ops.errorDetail.retryNote4') REDACTEDREDACTED</li>
              </ul>
            </div>
          </div>
        </div>
      </div>

      <!-- Request body -->
      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.requestBody') REDACTEDREDACTED</h3>
          <div
            v-if="detail.request_body_truncated"
            class="rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
          >
            {{ t('admin.ops.errorDetail.trimmed') REDACTEDREDACTED
          </div>
        </div>
        <pre
          class="mt-4 max-h-[420px] overflow-auto rounded-xl border border-gray-200 bg-white p-4 text-xs text-gray-800 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100"
        ><code>{{ prettyJSON(detail.request_body) REDACTEDREDACTED</code></pre>
      </div>

      <!-- Error body -->
      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.errorBody') REDACTEDREDACTED</h3>
        <pre
          class="mt-4 max-h-[420px] overflow-auto rounded-xl border border-gray-200 bg-white p-4 text-xs text-gray-800 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100"
        ><code>{{ prettyJSON(detail.error_body) REDACTEDREDACTED</code></pre>
      </div>
    </div>
  </BaseDialog>

  <ConfirmDialog
    :show="showRetryConfirm"
    :title="t('admin.ops.errorDetail.confirmRetry')"
    :message="retryConfirmMessage"
    @confirm="runConfirmedRetry"
    @cancel="cancelRetry"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useAppStore REDACTED from '@/stores'
import { opsAPI, type OpsErrorDetail, type OpsRetryMode REDACTED from '@/api/admin/ops'
import { formatDateTime REDACTED from '@/utils/format'
import { getSeverityClass REDACTED from '../utils/opsFormatters'

interface Props {
  show: boolean
  errorId: number | null
REDACTED

interface Emits {
  (e: 'update:show', value: boolean): void
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t REDACTED = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const detail = ref<OpsErrorDetail | null>(null)

const retrying = ref(false)
const showRetryConfirm = ref(false)
const pendingRetryMode = ref<OpsRetryMode>('client')

const pinnedAccountIdInput = ref('')
const pinnedAccountId = computed<number | null>(() => {
  const raw = String(pinnedAccountIdInput.value || '').trim()
  if (!raw) return null
  const n = Number.parseInt(raw, 10)
  return Number.isFinite(n) && n > 0 ? n : null
REDACTED)

const title = computed(() => {
  if (!props.errorId) return 'Error Detail'
  return `Error #${props.errorIdREDACTED`
REDACTED)

const emptyText = computed(() => 'No error selected.')

function close() {
  emit('update:show', false)
REDACTED

function prettyJSON(raw?: string): string {
  if (!raw) return t('admin.ops.errorDetail.na')
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  REDACTED catch {
    return raw
  REDACTED
REDACTED

async function fetchDetail(id: number) {
  loading.value = true
  try {
    const d = await opsAPI.getErrorLogDetail(id)
    detail.value = d

    // Default pinned account from error log if present.
    if (d.account_id && d.account_id > 0) {
      pinnedAccountIdInput.value = String(d.account_id)
    REDACTED else {
      pinnedAccountIdInput.value = ''
    REDACTED
  REDACTED catch (err: any) {
    detail.value = null
    appStore.showError(err?.message || t('admin.ops.failedToLoadErrorDetail'))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

watch(
  () => [props.show, props.errorId] as const,
  ([show, id]) => {
    if (!show) {
      detail.value = null
      return
    REDACTED
    if (typeof id === 'number' && id > 0) {
      fetchDetail(id)
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

function openRetryConfirm(mode: OpsRetryMode) {
  pendingRetryMode.value = mode
  showRetryConfirm.value = true
REDACTED

const retryConfirmMessage = computed(() => {
  const mode = pendingRetryMode.value
  if (mode === 'upstream') {
    return t('admin.ops.errorDetail.confirmRetryMessage')
  REDACTED
  return t('admin.ops.errorDetail.confirmRetryHint')
REDACTED)

const severityClass = computed(() => {
  if (!detail.value?.severity) return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  return getSeverityClass(detail.value.severity)
REDACTED)

const statusClass = computed(() => {
  const code = detail.value?.status_code ?? 0
  if (code >= 500) return 'bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-400 dark:ring-red-500/30'
  if (code === 429) return 'bg-purple-50 text-purple-700 ring-purple-600/20 dark:bg-purple-900/30 dark:text-purple-400 dark:ring-purple-500/30'
  if (code >= 400) return 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-900/30 dark:text-amber-400 dark:ring-amber-500/30'
  return 'bg-gray-50 text-gray-700 ring-gray-600/20 dark:bg-gray-900/30 dark:text-gray-400 dark:ring-gray-500/30'
REDACTED)

async function runConfirmedRetry() {
  if (!props.errorId) return
  const mode = pendingRetryMode.value
  showRetryConfirm.value = false

  retrying.value = true
  try {
    const req =
      mode === 'upstream'
        ? { mode, pinned_account_id: pinnedAccountId.value ?? undefined REDACTED
        : { mode REDACTED

    const res = await opsAPI.retryErrorRequest(props.errorId, req)
    const summary = res.status === 'succeeded' ? t('admin.ops.errorDetail.retrySuccess') : t('admin.ops.errorDetail.retryFailed')
    appStore.showSuccess(summary)
  REDACTED catch (err: any) {
    appStore.showError(err?.message || t('admin.ops.retryFailed'))
  REDACTED finally {
    retrying.value = false
  REDACTED
REDACTED

function cancelRetry() {
  showRetryConfirm.value = false
REDACTED
</script>
