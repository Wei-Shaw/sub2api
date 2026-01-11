<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import OpsErrorLogTable from './OpsErrorLogTable.vue'
import { opsAPI, type OpsErrorLog REDACTED from '@/api/admin/ops'

interface Props {
  show: boolean
  timeRange: string
  platform?: string
  groupId?: number | null
  errorType: 'request' | 'upstream'
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'openErrorDetail', errorId: number): void
REDACTED>()

const { t REDACTED = useI18n()

const loading = ref(false)
const rows = ref<OpsErrorLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const q = ref('')
const statusCode = ref<number | null>(null)
const phase = ref<string>('')
const accountIdInput = ref<string>('')

const accountId = computed<number | null>(() => {
  const raw = String(accountIdInput.value || '').trim()
  if (!raw) return null
  const n = Number.parseInt(raw, 10)
  return Number.isFinite(n) && n > 0 ? n : null
REDACTED)

const modalTitle = computed(() => {
  return props.errorType === 'upstream' ? t('admin.ops.errorDetails.upstreamErrors') : t('admin.ops.errorDetails.requestErrors')
REDACTED)

const statusCodeSelectOptions = computed(() => {
  const codes = [400, 401, 403, 404, 409, 422, 429, 500, 502, 503, 504, 529]
  return [
    { value: null, label: t('common.all') REDACTED,
    ...codes.map((c) => ({ value: c, label: String(c) REDACTED))
  ]
REDACTED)

const phaseSelectOptions = computed(() => {
  const options = [
    { value: '', label: t('common.all') REDACTED,
    { value: 'upstream', label: 'upstream' REDACTED,
    { value: 'network', label: 'network' REDACTED,
    { value: 'routing', label: 'routing' REDACTED,
    { value: 'auth', label: 'auth' REDACTED,
    { value: 'billing', label: 'billing' REDACTED,
    { value: 'concurrency', label: 'concurrency' REDACTED,
    { value: 'internal', label: 'internal' REDACTED
  ]
  return options
REDACTED)

function close() {
  emit('update:show', false)
REDACTED

async function fetchErrorLogs() {
  if (!props.show) return

  loading.value = true
  try {
    const params: Record<string, any> = {
      page: page.value,
      page_size: pageSize.value,
      time_range: props.timeRange
    REDACTED

    const platform = String(props.platform || '').trim()
    if (platform) params.platform = platform
    if (typeof props.groupId === 'number' && props.groupId > 0) params.group_id = props.groupId

    if (q.value.trim()) params.q = q.value.trim()
    if (typeof statusCode.value === 'number') params.status_codes = String(statusCode.value)
    if (typeof accountId.value === 'number') params.account_id = accountId.value

    const phaseVal = String(phase.value || '').trim()
    if (phaseVal) params.phase = phaseVal

    const res = await opsAPI.listErrorLogs(params)
    rows.value = res.items || []
    total.value = res.total || 0
  REDACTED catch (err) {
    console.error('[OpsErrorDetailsModal] Failed to fetch error logs', err)
    rows.value = []
    total.value = 0
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

function resetFilters() {
  q.value = ''
  statusCode.value = null
  phase.value = props.errorType === 'upstream' ? 'upstream' : ''
  accountIdInput.value = ''
  page.value = 1
  fetchErrorLogs()
REDACTED

watch(
  () => props.show,
  (open) => {
    if (!open) return
    page.value = 1
    pageSize.value = 20
    resetFilters()
  REDACTED
)

watch(
  () => [props.timeRange, props.platform, props.groupId] as const,
  () => {
    if (!props.show) return
    page.value = 1
    fetchErrorLogs()
  REDACTED
)

watch(
  () => [page.value, pageSize.value] as const,
  () => {
    if (!props.show) return
    fetchErrorLogs()
  REDACTED
)

let searchTimeout: number | null = null
watch(
  () => q.value,
  () => {
    if (!props.show) return
    if (searchTimeout) window.clearTimeout(searchTimeout)
    searchTimeout = window.setTimeout(() => {
      page.value = 1
      fetchErrorLogs()
    REDACTED, 350)
  REDACTED
)

watch(
  () => [statusCode.value, phase.value] as const,
  () => {
    if (!props.show) return
    page.value = 1
    fetchErrorLogs()
  REDACTED
)

watch(
  () => accountId.value,
  () => {
    if (!props.show) return
    page.value = 1
    fetchErrorLogs()
  REDACTED
)
</script>

<template>
  <BaseDialog :show="show" :title="modalTitle" width="full" @close="close">
    <!-- Filters -->
    <div class="border-b border-gray-200 pb-4 mb-4 dark:border-dark-700">
      <div class="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <div class="lg:col-span-5">
          <div class="relative group">
            <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5">
              <svg
                class="h-4 w-4 text-gray-400 transition-colors group-focus-within:text-blue-500"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
            </div>
            <input
              v-model="q"
              type="text"
              class="w-full rounded-2xl border-gray-200 bg-gray-50/50 py-2 pl-10 pr-4 text-sm font-medium text-gray-700 transition-all focus:border-blue-500 focus:bg-white focus:ring-4 focus:ring-blue-500/10 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300 dark:focus:bg-dark-800"
              :placeholder="t('admin.ops.errorDetails.searchPlaceholder')"
            />
          </div>
        </div>

        <div class="lg:col-span-2">
          <Select :model-value="statusCode" :options="statusCodeSelectOptions" class="w-full" @update:model-value="statusCode = $event as any" />
        </div>

        <div class="lg:col-span-2">
          <Select :model-value="phase" :options="phaseSelectOptions" class="w-full" @update:model-value="phase = String($event ?? '')" />
        </div>

        <div class="lg:col-span-2">
          <input
            v-model="accountIdInput"
            type="text"
            inputmode="numeric"
            class="input w-full text-sm"
            :placeholder="t('admin.ops.errorDetails.accountIdPlaceholder')"
          />
        </div>

        <div class="lg:col-span-1 flex items-center justify-end">
          <button type="button" class="btn btn-secondary btn-sm" @click="resetFilters">
            {{ t('common.reset') REDACTEDREDACTED
          </button>
        </div>
      </div>
    </div>

    <!-- Body -->
    <div class="text-xs text-gray-500 dark:text-gray-400 mb-2">
      {{ t('admin.ops.errorDetails.total') REDACTEDREDACTED {{ total REDACTEDREDACTED
    </div>
    <OpsErrorLogTable
      :rows="rows"
      :total="total"
      :loading="loading"
      :page="page"
      :page-size="pageSize"
      @openErrorDetail="emit('openErrorDetail', $event)"
      @update:page="page = $event"
      @update:pageSize="pageSize = $event"
    />
  </BaseDialog>
</template>
