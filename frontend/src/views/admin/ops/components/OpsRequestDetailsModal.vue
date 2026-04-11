<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'
import { opsAPI, type OpsRequestDetailsParams, type OpsRequestDetail } from '@/api/admin/ops'
import { parseTimeRangeMinutes, formatDateTime } from '../utils/opsFormatters'

export interface OpsRequestDetailsPreset {
  title: string
  kind?: OpsRequestDetailsParams['kind']
  sort?: OpsRequestDetailsParams['sort']
  min_duration_ms?: number
  max_duration_ms?: number
  retried_only?: boolean
  routing_target_group?: string
}

interface Props {
  modelValue: boolean
  timeRange: string
  preset: OpsRequestDetailsPreset
  platform?: string
  groupId?: number | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'openErrorDetail', errorId: number): void
  (e: 'openRequestErrors', requestId: string): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const loading = ref(false)
const items = ref<OpsRequestDetail[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const close = () => emit('update:modelValue', false)

const rangeLabel = computed(() => {
  const minutes = parseTimeRangeMinutes(props.timeRange)
  if (minutes >= 60) return t('admin.ops.requestDetails.rangeHours', { n: Math.round(minutes / 60) })
  return t('admin.ops.requestDetails.rangeMinutes', { n: minutes })
})

function buildTimeParams(): Pick<OpsRequestDetailsParams, 'start_time' | 'end_time'> {
  const minutes = parseTimeRangeMinutes(props.timeRange)
  const endTime = new Date()
  const startTime = new Date(endTime.getTime() - minutes * 60 * 1000)
  return {
    start_time: startTime.toISOString(),
    end_time: endTime.toISOString()
  }
}

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
    }

    const platform = (props.platform || '').trim()
    if (platform) params.platform = platform
    if (typeof props.groupId === 'number' && props.groupId > 0) params.group_id = props.groupId

    if (typeof props.preset.min_duration_ms === 'number') params.min_duration_ms = props.preset.min_duration_ms
    if (typeof props.preset.max_duration_ms === 'number') params.max_duration_ms = props.preset.max_duration_ms
    if (props.preset.retried_only) params.retried_only = true
    if (props.preset.routing_target_group) params.routing_target_group = props.preset.routing_target_group

    const res = await opsAPI.listRequestDetails(params)
    items.value = res.items || []
    total.value = res.total || 0
  } catch (e: any) {
    console.error('[OpsRequestDetailsModal] Failed to fetch request details', e)
    appStore.showError(e?.message || t('admin.ops.requestDetails.failedToLoad'))
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      page.value = 1
      pageSize.value = 10
      fetchData()
    }
  }
)

watch(
  () => [
    props.timeRange,
    props.platform,
    props.groupId,
    props.preset.kind,
    props.preset.sort,
    props.preset.min_duration_ms,
    props.preset.max_duration_ms,
    props.preset.retried_only,
    props.preset.routing_target_group
  ],
  () => {
    if (!props.modelValue) return
    page.value = 1
    fetchData()
  }
)

function handlePageChange(next: number) {
  page.value = next
  fetchData()
}

function handlePageSizeChange(next: number) {
  pageSize.value = next
  page.value = 1
  fetchData()
}

async function handleCopyRequestId(requestId: string) {
  const ok = await copyToClipboard(requestId, t('admin.ops.requestDetails.requestIdCopied'))
  if (ok) return
  // `useClipboard` already shows toast on failure; this keeps UX consistent with older ops modal.
  appStore.showWarning(t('admin.ops.requestDetails.copyFailed'))
}

function openErrorDetail(errorId: number | null | undefined) {
  if (!errorId) return
  close()
  emit('openErrorDetail', errorId)
}

function openRequestErrors(requestId: string | null | undefined) {
  const normalized = String(requestId || '').trim()
  if (!normalized) return
  close()
  emit('openRequestErrors', normalized)
}

const kindBadgeClass = (kind: string) => {
  if (kind === 'error') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
}

function hasStickyData(row: OpsRequestDetail): boolean {
  return !!(
    row.sticky_session_source ||
    row.sticky_eval_result ||
    row.sticky_parent_session_key ||
    row.sticky_session_hash_present != null ||
    row.sticky_selected_account_changed != null ||
    row.sticky_parent_session_present != null
  )
}

function formatBool(value: boolean | null | undefined): string {
  if (value == null) return '-'
  return value ? t('common.yes') : t('common.no')
}

function shouldShowStickySelectedAccountChanged(row: OpsRequestDetail): boolean {
  if (row.sticky_selected_account_changed == null) return false
  const evalResult = String(row.sticky_eval_result || '').trim()
  return evalResult !== 'no_session_signal' && evalResult !== 'miss_no_binding'
}
</script>

<template>
  <BaseDialog :show="modelValue" :title="props.preset.title || t('admin.ops.requestDetails.title')" width="full" @close="close">
    <template #default>
      <div class="flex h-full min-h-0 flex-col">
        <div class="mb-4 flex flex-shrink-0 items-center justify-between">
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.requestDetails.rangeLabel', { range: rangeLabel }) }}
          </div>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            @click="fetchData"
          >
            {{ t('common.refresh') }}
          </button>
        </div>

        <!-- Loading -->
        <div v-if="loading" class="flex flex-1 items-center justify-center py-16">
          <div class="flex flex-col items-center gap-3">
            <svg class="h-8 w-8 animate-spin text-blue-500" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            <span class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</span>
          </div>
        </div>

        <!-- Table -->
        <div v-else class="flex min-h-0 flex-1 flex-col">
          <div v-if="items.length === 0" class="rounded-xl border border-dashed border-gray-200 p-10 text-center dark:border-dark-700">
            <div class="text-sm font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.requestDetails.empty') }}</div>
            <div class="mt-1 text-xs text-gray-400">{{ t('admin.ops.requestDetails.emptyHint') }}</div>
          </div>

          <div v-else class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
            <div class="min-h-0 flex-1 overflow-auto">
              <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
                <thead class="sticky top-0 z-10 bg-gray-50 dark:bg-dark-900">
                <tr>
                  <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.requestDetails.table.time') }}
                  </th>
                  <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.requestDetails.table.kind') }}
                  </th>
                  <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.requestDetails.table.platform') }}
                  </th>
                  <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.requestDetails.table.model') }}
                  </th>
                  <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.requestDetails.table.routing') }}
                  </th>
                  <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.requestDetails.table.duration') }}
                  </th>
                  <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.requestDetails.table.status') }}
                  </th>
                  <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.requestDetails.table.requestId') }}
                  </th>
                  <th class="px-4 py-3 text-right text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.requestDetails.table.actions') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
                <tr v-for="(row, idx) in items" :key="idx" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
                  <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                    {{ formatDateTime(row.created_at) }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-3">
                    <span class="rounded-full px-2 py-1 text-[10px] font-bold" :class="kindBadgeClass(row.kind)">
                      {{ row.kind === 'error' ? t('admin.ops.requestDetails.kind.error') : t('admin.ops.requestDetails.kind.success') }}
                    </span>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-xs font-medium text-gray-700 dark:text-gray-200">
                    {{ (row.platform || 'unknown').toUpperCase() }}
                  </td>
                  <td class="max-w-[240px] px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                    <div class="space-y-1">
                      <div class="break-all" :title="row.model || ''">{{ row.model || '-' }}</div>
                      <div v-if="row.routing_effective_model && row.routing_effective_model !== row.model" class="break-all text-gray-500 dark:text-gray-400" :title="row.routing_effective_model">
                        <span class="font-medium">{{ t('admin.usage.effectiveModel') }}:</span>
                        <span class="ml-1">{{ row.routing_effective_model }}</span>
                      </div>
                    </div>
                  </td>
                  <td class="max-w-[260px] px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                    <div v-if="row.routing_target_group || row.routing_schedule_layer || row.routing_selected_account_name || row.routing_failover_count != null || hasStickyData(row)" class="space-y-2">
                      <div v-if="row.routing_target_group">
                        <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.routingTargetGroup') }}:</span>
                        <span class="ml-1">{{ row.routing_target_group }}</span>
                      </div>
                      <div v-if="row.routing_schedule_layer">
                        <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.routingScheduleLayer') }}:</span>
                        <span class="ml-1">{{ row.routing_schedule_layer }}</span>
                      </div>
                      <div v-if="row.routing_selected_account_name">
                        <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.routedAccount') }}:</span>
                        <span class="ml-1">{{ row.routing_selected_account_name }}</span>
                        <span v-if="row.routing_selected_account_id" class="ml-1 font-mono text-gray-400">#{{ row.routing_selected_account_id }}</span>
                      </div>
                      <div v-if="row.routing_failover_count != null">
                        <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.routingFailover') }}:</span>
                        <span class="ml-1">{{ row.routing_failover_count }}</span>
                        <span v-if="row.routing_failover_final_reason" class="ml-1 text-gray-400">({{ row.routing_failover_final_reason }})</span>
                      </div>

                      <div v-if="hasStickyData(row)" class="rounded-lg bg-gray-50 px-2 py-2 dark:bg-dark-900/40">
                        <div class="mb-1 text-[11px] font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                          {{ t('admin.ops.requestDetails.sticky.title') }}
                        </div>
                        <div class="space-y-1">
                          <div v-if="row.sticky_session_source">
                            <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.requestDetails.sticky.sessionSource') }}:</span>
                            <span class="ml-1 break-all">{{ row.sticky_session_source }}</span>
                          </div>
                          <div v-if="row.sticky_eval_result">
                            <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.requestDetails.sticky.evalResult') }}:</span>
                            <span class="ml-1 break-all">{{ row.sticky_eval_result }}</span>
                          </div>
                          <div v-if="row.sticky_parent_session_key">
                            <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.requestDetails.sticky.parentSessionKey') }}:</span>
                            <span class="ml-1 break-all font-mono text-[11px]">{{ row.sticky_parent_session_key }}</span>
                          </div>
                          <div v-if="row.sticky_session_hash_present != null">
                            <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.requestDetails.sticky.sessionHashPresent') }}:</span>
                            <span class="ml-1">{{ formatBool(row.sticky_session_hash_present) }}</span>
                          </div>
                          <div v-if="shouldShowStickySelectedAccountChanged(row)">
                            <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.requestDetails.sticky.selectedAccountChanged') }}:</span>
                            <span class="ml-1">{{ formatBool(row.sticky_selected_account_changed) }}</span>
                          </div>
                          <div v-if="row.sticky_parent_session_present != null">
                            <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.requestDetails.sticky.parentSessionPresent') }}:</span>
                            <span class="ml-1">{{ formatBool(row.sticky_parent_session_present) }}</span>
                          </div>
                        </div>
                      </div>
                    </div>
                    <span v-else>-</span>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                    {{ typeof row.duration_ms === 'number' ? `${row.duration_ms} ms` : '-' }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                    {{ row.status_code ?? '-' }}
                  </td>
                  <td class="px-4 py-3">
                    <div v-if="row.request_id" class="flex items-center gap-2">
                      <span class="max-w-[220px] truncate font-mono text-[11px] text-gray-700 dark:text-gray-200" :title="row.request_id">
                        {{ row.request_id }}
                      </span>
                      <button
                        class="rounded-md bg-gray-100 px-2 py-1 text-[10px] font-bold text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
                        @click="handleCopyRequestId(row.request_id)"
                      >
                        {{ t('admin.ops.requestDetails.copy') }}
                      </button>
                    </div>
                    <span v-else class="text-xs text-gray-400">-</span>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-right">
                    <div class="flex items-center justify-end gap-2">
                      <button
                        v-if="row.request_id && row.routing_failover_count != null && row.routing_failover_count > 0"
                        class="rounded-lg bg-blue-50 px-3 py-1.5 text-xs font-bold text-blue-600 hover:bg-blue-100 dark:bg-blue-900/20 dark:text-blue-300 dark:hover:bg-blue-900/30"
                        @click="openRequestErrors(row.request_id)"
                      >
                        {{ t('admin.ops.requestDetails.viewRetryErrors') }}
                      </button>
                      <button
                        v-if="row.kind === 'error' && row.error_id"
                        class="rounded-lg bg-red-50 px-3 py-1.5 text-xs font-bold text-red-600 hover:bg-red-100 dark:bg-red-900/20 dark:text-red-300 dark:hover:bg-red-900/30"
                        @click="openErrorDetail(row.error_id)"
                      >
                        {{ t('admin.ops.requestDetails.viewError') }}
                      </button>
                      <span v-if="!(row.request_id && row.routing_failover_count != null && row.routing_failover_count > 0) && !(row.kind === 'error' && row.error_id)" class="text-xs text-gray-400">-</span>
                    </div>
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
    </template>
  </BaseDialog>
</template>
