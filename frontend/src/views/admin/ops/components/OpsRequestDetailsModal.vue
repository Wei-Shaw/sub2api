<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { onKeyStroke REDACTED from '@vueuse/core'
import Pagination from '@/components/common/Pagination.vue'
import { useClipboard REDACTED from '@/composables/useClipboard'
import { useAppStore REDACTED from '@/stores'
import { opsAPI, type OpsRequestDetailsParams, type OpsRequestDetail REDACTED from '@/api/admin/ops'
import { parseTimeRangeMinutes, formatDateTime REDACTED from '../utils/opsFormatters'

export interface OpsRequestDetailsPreset {
  title: string
  kind?: OpsRequestDetailsParams['kind']
  sort?: OpsRequestDetailsParams['sort']
  min_duration_ms?: number
  max_duration_ms?: number
REDACTED

interface Props {
  modelValue: boolean
  timeRange: string
  preset: OpsRequestDetailsPreset
  platform?: string
  groupId?: number | null
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'openErrorDetail', errorId: number): void
REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()
const { copyToClipboard REDACTED = useClipboard()

const loading = ref(false)
const items = ref<OpsRequestDetail[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)

const close = () => emit('update:modelValue', false)

onKeyStroke('Escape', () => {
  if (props.modelValue) close()
REDACTED)

const handleOverlayClick = (e: MouseEvent) => {
  if (e.target === e.currentTarget) close()
REDACTED

const rangeLabel = computed(() => {
  const minutes = parseTimeRangeMinutes(props.timeRange)
  if (minutes >= 60) return t('admin.ops.requestDetails.rangeHours', { n: Math.round(minutes / 60) REDACTED)
  return t('admin.ops.requestDetails.rangeMinutes', { n: minutes REDACTED)
REDACTED)

function buildTimeParams(): Pick<OpsRequestDetailsParams, 'start_time' | 'end_time'> {
  const minutes = parseTimeRangeMinutes(props.timeRange)
  const endTime = new Date()
  const startTime = new Date(endTime.getTime() - minutes * 60 * 1000)
  return {
    start_time: startTime.toISOString(),
    end_time: endTime.toISOString()
  REDACTED
REDACTED

const fetchData = async () => {
  if (!props.modelValue) return
  loading.value = true
  try {
    const params: OpsRequestDetailsParams = {
      ...buildTimeParams(),
      page: page.value,
      page_size: pageSize.value,
      kind: props.preset.kind ?? 'all',
      sort: props.preset.sort ?? 'created_at_desc'
    REDACTED

    const platform = (props.platform || '').trim()
    if (platform) params.platform = platform
    if (typeof props.groupId === 'number' && props.groupId > 0) params.group_id = props.groupId

    if (typeof props.preset.min_duration_ms === 'number') params.min_duration_ms = props.preset.min_duration_ms
    if (typeof props.preset.max_duration_ms === 'number') params.max_duration_ms = props.preset.max_duration_ms

    const res = await opsAPI.listRequestDetails(params)
    items.value = res.items || []
    total.value = res.total || 0
  REDACTED catch (e: any) {
    console.error('[OpsRequestDetailsModal] Failed to fetch request details', e)
    appStore.showError(e?.message || t('admin.ops.requestDetails.failedToLoad'))
    items.value = []
    total.value = 0
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      page.value = 1
      fetchData()
    REDACTED
  REDACTED
)

watch(
  () => [
    props.timeRange,
    props.platform,
    props.groupId,
    props.preset.kind,
    props.preset.sort,
    props.preset.min_duration_ms,
    props.preset.max_duration_ms
  ],
  () => {
    if (!props.modelValue) return
    page.value = 1
    fetchData()
  REDACTED
)

function handlePageChange(next: number) {
  page.value = next
  fetchData()
REDACTED

function handlePageSizeChange(next: number) {
  pageSize.value = next
  page.value = 1
  fetchData()
REDACTED

async function handleCopyRequestId(requestId: string) {
  const ok = await copyToClipboard(requestId, t('admin.ops.requestDetails.requestIdCopied'))
  if (ok) return
  // `useClipboard` already shows toast on failure; this keeps UX consistent with older ops modal.
  appStore.showWarning(t('admin.ops.requestDetails.copyFailed'))
REDACTED

function openErrorDetail(errorId: number | null | undefined) {
  if (!errorId) return
  close()
  emit('openErrorDetail', errorId)
REDACTED

const kindBadgeClass = (kind: string) => {
  if (kind === 'error') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
REDACTED
</script>

<template>
  <Teleport to="body">
    <div v-if="modelValue" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click="handleOverlayClick">
      <div class="relative max-h-[90vh] w-full max-w-5xl overflow-y-auto rounded-2xl bg-white shadow-2xl dark:bg-dark-800" @click.stop>
        <!-- Header -->
        <div class="sticky top-0 z-10 flex items-center justify-between border-b border-gray-200 bg-white px-6 py-4 dark:border-dark-700 dark:bg-dark-800">
          <div>
            <div class="text-sm font-bold text-gray-900 dark:text-white">
              {{ props.preset.title || t('admin.ops.requestDetails.title') REDACTEDREDACTED
            </div>
            <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.requestDetails.rangeLabel', { range: rangeLabel REDACTED) REDACTEDREDACTED
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              class="rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-xs font-bold text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300 dark:hover:bg-dark-800"
              @click="fetchData"
            >
              {{ t('common.refresh') REDACTEDREDACTED
            </button>
            <button
              type="button"
              class="rounded-lg bg-gray-100 p-2 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
              @click="close"
              :title="t('common.close')"
            >
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        <!-- Loading -->
        <div v-if="loading" class="flex items-center justify-center py-16">
          <div class="flex flex-col items-center gap-3">
            <svg class="h-8 w-8 animate-spin text-blue-500" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            <span class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('common.loading') REDACTEDREDACTED</span>
          </div>
        </div>

        <!-- Table -->
        <div v-else class="p-6">
          <div v-if="items.length === 0" class="rounded-xl border border-dashed border-gray-200 p-10 text-center dark:border-dark-700">
            <div class="text-sm font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.requestDetails.empty') REDACTEDREDACTED</div>
            <div class="mt-1 text-xs text-gray-400">{{ t('admin.ops.requestDetails.emptyHint') REDACTEDREDACTED</div>
          </div>

          <div v-else class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
                <thead class="bg-gray-50 dark:bg-dark-900">
                  <tr>
                    <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.requestDetails.table.time') REDACTEDREDACTED
                    </th>
                    <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.requestDetails.table.kind') REDACTEDREDACTED
                    </th>
                    <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.requestDetails.table.platform') REDACTEDREDACTED
                    </th>
                    <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.requestDetails.table.model') REDACTEDREDACTED
                    </th>
                    <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.requestDetails.table.duration') REDACTEDREDACTED
                    </th>
                    <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.requestDetails.table.status') REDACTEDREDACTED
                    </th>
                    <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.requestDetails.table.requestId') REDACTEDREDACTED
                    </th>
                    <th class="px-4 py-3 text-right text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.requestDetails.table.actions') REDACTEDREDACTED
                    </th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
                  <tr v-for="(row, idx) in items" :key="idx" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
                    <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                      {{ formatDateTime(row.created_at) REDACTEDREDACTED
                    </td>
                    <td class="whitespace-nowrap px-4 py-3">
                      <span class="rounded-full px-2 py-1 text-[10px] font-bold" :class="kindBadgeClass(row.kind)">
                        {{ row.kind === 'error' ? t('admin.ops.requestDetails.kind.error') : t('admin.ops.requestDetails.kind.success') REDACTEDREDACTED
                      </span>
                    </td>
                    <td class="whitespace-nowrap px-4 py-3 text-xs font-medium text-gray-700 dark:text-gray-200">
                      {{ (row.platform || 'unknown').toUpperCase() REDACTEDREDACTED
                    </td>
                    <td class="max-w-[240px] truncate px-4 py-3 text-xs text-gray-600 dark:text-gray-300" :title="row.model || ''">
                      {{ row.model || '-' REDACTEDREDACTED
                    </td>
                    <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                      {{ typeof row.duration_ms === 'number' ? `${row.duration_msREDACTED ms` : '-' REDACTEDREDACTED
                    </td>
                    <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                      {{ row.status_code ?? '-' REDACTEDREDACTED
                    </td>
                    <td class="px-4 py-3">
                      <div v-if="row.request_id" class="flex items-center gap-2">
                        <span class="max-w-[220px] truncate font-mono text-[11px] text-gray-700 dark:text-gray-200" :title="row.request_id">
                          {{ row.request_id REDACTEDREDACTED
                        </span>
                        <button
                          class="rounded-md bg-gray-100 px-2 py-1 text-[10px] font-bold text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
                          @click="handleCopyRequestId(row.request_id)"
                        >
                          {{ t('admin.ops.requestDetails.copy') REDACTEDREDACTED
                        </button>
                      </div>
                      <span v-else class="text-xs text-gray-400">-</span>
                    </td>
                    <td class="whitespace-nowrap px-4 py-3 text-right">
                      <button
                        v-if="row.kind === 'error' && row.error_id"
                        class="rounded-lg bg-red-50 px-3 py-1.5 text-xs font-bold text-red-600 hover:bg-red-100 dark:bg-red-900/20 dark:text-red-300 dark:hover:bg-red-900/30"
                        @click="openErrorDetail(row.error_id)"
                      >
                        {{ t('admin.ops.requestDetails.viewError') REDACTEDREDACTED
                      </button>
                      <span v-else class="text-xs text-gray-400">-</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            <Pagination
              :total="total"
              :page="page"
              :page-size="pageSize"
              @update:page="handlePageChange"
              @update:pageSize="handlePageSizeChange"
            />
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
