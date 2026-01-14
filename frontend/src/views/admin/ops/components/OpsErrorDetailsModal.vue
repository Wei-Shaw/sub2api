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
const errorOwner = ref<string>('')
const resolvedStatus = ref<string>('unresolved')
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

const ownerSelectOptions = computed(() => {
  return [
    { value: '', label: t('common.all') REDACTED,
    { value: 'provider', label: 'provider' REDACTED,
    { value: 'client', label: 'client' REDACTED,
    { value: 'platform', label: 'platform' REDACTED
  ]
REDACTED)

const resolvedSelectOptions = computed(() => {
  return [
    { value: 'unresolved', label: t('admin.ops.errorDetails.unresolved') || 'unresolved' REDACTED,
    { value: 'all', label: t('common.all') REDACTED,
    { value: 'resolved', label: t('admin.ops.errorDetails.resolved') || 'resolved' REDACTED
  ]
REDACTED)

const phaseSelectOptions = computed(() => {
  const options = [
    { value: '', label: t('common.all') REDACTED,
    { value: 'request', label: 'request' REDACTED,
    { value: 'auth', label: 'auth' REDACTED,
    { value: 'routing', label: 'routing' REDACTED,
    { value: 'upstream', label: 'upstream' REDACTED,
    { value: 'network', label: 'network' REDACTED,
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

    const ownerVal = String(errorOwner.value || '').trim()
    if (ownerVal) params.error_owner = ownerVal

    const resolvedVal = String(resolvedStatus.value || '').trim()
    if (resolvedVal === 'resolved') params.resolved = 'true'
    else if (resolvedVal === 'unresolved') params.resolved = 'false'
    // 'all' -> omit

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
  errorOwner.value = ''
  resolvedStatus.value = 'unresolved'
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
  () => [statusCode.value, phase.value, errorOwner.value, resolvedStatus.value] as const,
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
    <div class="flex h-full min-h-0 flex-col">
      <!-- Filters -->
      <div class="mb-4 flex-shrink-0 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div class="flex flex-col gap-2">
          <!-- 第一行: 搜索框 -->
          <div class="flex items-center gap-2">
            <div class="relative flex-1 group">
              <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
                <svg
                  class="h-3.5 w-3.5 text-gray-400 transition-colors group-focus-within:text-blue-500"
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
                class="w-full rounded-lg border-gray-200 bg-gray-50/50 py-1.5 pl-9 pr-3 text-xs font-medium text-gray-700 transition-all focus:border-blue-500 focus:bg-white focus:ring-2 focus:ring-blue-500/10 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300 dark:focus:bg-dark-800"
                :placeholder="t('admin.ops.errorDetails.searchPlaceholder')"
              />
            </div>
          </div>

          <!-- 第二行: 筛选选项 -->
          <div class="grid grid-cols-6 gap-2">
            <div class="col-span-1">
              <Select :model-value="statusCode" :options="statusCodeSelectOptions" size="sm" @update:model-value="statusCode = $event as any" />
            </div>

            <div class="col-span-1">
              <Select :model-value="phase" :options="phaseSelectOptions" size="sm" @update:model-value="phase = String($event ?? '')" />
            </div>

            <div class="col-span-1">
              <Select :model-value="errorOwner" :options="ownerSelectOptions" size="sm" @update:model-value="errorOwner = String($event ?? '')" />
            </div>

            <div class="col-span-1">
              <Select :model-value="resolvedStatus" :options="resolvedSelectOptions" size="sm" @update:model-value="resolvedStatus = String($event ?? 'unresolved')" />
            </div>

            <div class="col-span-1">
              <input
                v-model="accountIdInput"
                type="text"
                inputmode="numeric"
                class="w-full rounded-lg border-gray-200 bg-gray-50/50 py-1.5 px-3 text-xs font-medium text-gray-700 transition-all focus:border-blue-500 focus:bg-white focus:ring-2 focus:ring-blue-500/10 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300 dark:focus:bg-dark-800"
                :placeholder="t('admin.ops.errorDetails.accountIdPlaceholder')"
              />
            </div>

            <div class="col-span-1 flex items-center justify-end">
              <button type="button" class="rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-semibold text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600" @click="resetFilters">
                {{ t('common.reset') REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Body -->
      <div class="flex min-h-0 flex-1 flex-col">
        <div class="mb-2 flex-shrink-0 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.errorDetails.total') REDACTEDREDACTED {{ total REDACTEDREDACTED
        </div>

        <OpsErrorLogTable
          class="min-h-0 flex-1"
          :rows="rows"
          :total="total"
          :loading="loading"
          :page="page"
          :page-size="pageSize"
          @openErrorDetail="emit('openErrorDetail', $event)"
          @update:page="page = $event"
          @update:pageSize="pageSize = $event"
        />
      </div>
    </div>
  </BaseDialog>
</template>
