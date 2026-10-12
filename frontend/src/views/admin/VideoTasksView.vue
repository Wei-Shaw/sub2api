<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="space-y-3">
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.videoTasks.coverage') }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.videoTasks.billingNote') }}</p>
          <form class="flex flex-wrap items-end gap-3" @submit.prevent="search">
            <div class="w-full sm:w-72">
              <label for="video-task-search" class="input-label">{{ t('admin.videoTasks.search') }}</label>
              <input id="video-task-search" v-model="filters.search" type="text" maxlength="100" class="input"
                :placeholder="t('admin.videoTasks.searchPlaceholder')" />
            </div>
            <div class="w-48">
              <label for="video-task-status" class="input-label">{{ t('admin.videoTasks.status') }}</label>
              <Select id="video-task-status" v-model="filters.status" :options="statusOptions" @change="search" />
            </div>
            <div class="w-36">
              <label for="video-task-days" class="input-label">{{ t('admin.videoTasks.recent') }}</label>
              <Select id="video-task-days" v-model="filters.days" :options="dayOptions" @change="search" />
            </div>
            <button type="submit" class="btn btn-primary" :disabled="loading">{{ t('common.search') }}</button>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="refresh">
              <Icon name="refresh" size="sm" class="mr-1.5" />{{ t('common.refresh') }}
            </button>
          </form>
          <p v-if="loadFailed" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ t('admin.videoTasks.loadFailed') }}</p>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="rows" :loading="loading" row-key="_rowKey">
          <template #cell-task="{ row }">
            <div class="max-w-xs break-all font-mono text-sm">{{ row.task_id || '—' }}</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ row.model || '—' }}</div>
            <div v-if="row.kind === 'submission'" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.videoTasks.kinds.submission') }}</div>
          </template>
          <template #cell-user="{ row }">
            <div class="max-w-xs break-all">{{ row.user_email || '—' }}</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ row.api_key_name || '—' }}</div>
          </template>
          <template #cell-account="{ row }">
            <div class="max-w-xs break-all">{{ row.account_name || '—' }}</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ row.group_name || '—' }}</div>
          </template>
          <template #cell-status="{ row }">
            <span class="badge whitespace-nowrap" :class="statusBadge(displayStatus(row))">{{ statusLabel(row) }}</span>
            <p class="mt-2 max-w-xs whitespace-normal text-xs text-gray-500 dark:text-gray-400">{{ explanation(row) }}</p>
            <p v-if="reasonKey(row.reason_code)" class="mt-1 max-w-xs whitespace-normal text-xs text-gray-500 dark:text-gray-400">{{ reason(row) }}</p>
          </template>
          <template #cell-billing="{ row }">
            <span class="badge whitespace-nowrap" :class="statusBadge(row.billing_status)">{{ billingLabel(row) }}</span>
            <div class="mt-2 whitespace-nowrap text-xs text-gray-500 dark:text-gray-400">{{ t('admin.videoTasks.fields.expected') }}: {{ formatCost(row.expected_cost, locale) }}</div>
            <div class="mt-1 whitespace-nowrap text-xs text-gray-500 dark:text-gray-400">{{ t('admin.videoTasks.fields.charged') }}: {{ formatCost(row.charged_cost, locale) }}</div>
          </template>
          <template #cell-created="{ row }">
            <span class="whitespace-nowrap text-gray-500 dark:text-gray-400">{{ formatTaskTime(row.created_at, locale) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <button type="button" class="inline-flex items-center gap-1 font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="selected = row">
              <Icon name="eye" size="sm" />{{ t('admin.videoTasks.detail') }}
            </button>
          </template>
          <template #empty>
            <p class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t(loadFailed ? 'admin.videoTasks.loadFailed' : 'admin.videoTasks.empty') }}</p>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
            <label for="video-task-page-size">{{ t('pagination.perPage') }}</label>
            <Select id="video-task-page-size" class="w-24" :model-value="pageSize" :options="pageSizeOptions" @update:model-value="changePageSize" />
          </div>
          <Pagination v-if="total > 0" :total="total" :page="page" :page-size="pageSize"
            :show-page-size-selector="false" @update:page="changePage" />
        </div>
      </template>
    </TablePageLayout>

    <BaseDialog :show="selected !== null" :title="t('admin.videoTasks.detail')" width="wide" @close="selected = null">
      <div v-if="selected" class="space-y-4">
        <div class="flex flex-wrap gap-2">
          <span class="badge" :class="statusBadge(displayStatus(selected))">{{ statusLabel(selected) }}</span>
          <span class="badge" :class="statusBadge(selected.billing_status)">{{ billingLabel(selected) }}</span>
        </div>
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ explanation(selected) }}</p>
        <p v-if="reasonKey(selected.reason_code)" class="text-sm text-gray-600 dark:text-gray-300">{{ reason(selected) }}</p>
        <dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div v-for="field in detailFields" :key="field.key">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t(`admin.videoTasks.fields.${field.key}`) }}</dt>
            <dd class="mt-1 break-all text-sm text-gray-900 dark:text-gray-100">{{ field.value }}</dd>
          </div>
        </dl>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="selected = null">{{ t('common.close') }}</button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { videoTasksAPI, videoTaskStatuses, type VideoTaskAdminItem, type VideoTaskAdminQuery, type VideoTaskStatus } from '@/api/admin/videoTasks'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { displayStatus, statusBadge, reasonKey, formatCost, formatTaskTime, elapsedParts } from './videoTasksPresentation'

const { t, locale } = useI18n()
const loading = ref(false)
const loadFailed = ref(false)
const rows = ref<(VideoTaskAdminItem & { _rowKey: string })[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const selected = ref<VideoTaskAdminItem | null>(null)
const filters = reactive({ search: '', status: '' as VideoTaskStatus | '', days: 7 as 1 | 7 | 30 })
let appliedFilters: VideoTaskAdminQuery = { days: 7 }
let requestId = 0
let controller: AbortController | undefined

const columns = computed<Column[]>(() => ['task', 'user', 'account', 'status', 'billing', 'created', 'actions'].map(key => ({
  key, label: t(`admin.videoTasks.columns.${key}`)
})))
const statusOptions = computed(() => [
  { value: '', label: t('admin.videoTasks.allStatuses') },
  ...videoTaskStatuses.map(value => ({ value, label: t(`admin.videoTasks.statuses.${value}`) }))
])
const dayOptions = computed(() => [1, 7, 30].map(value => ({ value, label: t('admin.videoTasks.days', { count: value }) })))
// Pagination's shared selector uses global preferences that may exceed this API's limit.
const pageSizeOptions = [20, 50, 100].map(value => ({ value, label: String(value) }))

function statusLabel(item: VideoTaskAdminItem) { return t(`admin.videoTasks.statuses.${displayStatus(item)}`) }
function explanation(item: VideoTaskAdminItem) { return t(`admin.videoTasks.explanations.${displayStatus(item)}`) }
function reason(item: VideoTaskAdminItem) { return t(reasonKey(item.reason_code) || 'admin.videoTasks.reasons.unknown') }
function billingLabel(item: VideoTaskAdminItem) {
  const status = ['not_charged', 'pending', 'charged', 'review'].includes(item.billing_status) ? item.billing_status : 'unknown'
  return t(`admin.videoTasks.billing.${status}`)
}

const detailFields = computed(() => {
  const item = selected.value
  if (!item) return []
  const elapsed = elapsedParts(item)
  // Explicit field allowlist: never dump raw payloads, prompts, URLs or error text.
  return [
    { key: 'kind', value: t(`admin.videoTasks.kinds.${item.kind === 'submission' ? 'submission' : 'task'}`) },
    { key: 'taskId', value: item.task_id || '—' },
    { key: 'model', value: item.model || '—' },
    { key: 'user', value: item.user_email || '—' },
    { key: 'userId', value: item.user_id || '—' },
    { key: 'apiKey', value: item.api_key_name || '—' },
    { key: 'apiKeyId', value: item.api_key_id || '—' },
    { key: 'account', value: item.account_name || '—' },
    { key: 'accountId', value: item.account_id || '—' },
    { key: 'group', value: item.group_name || '—' },
    { key: 'groupId', value: item.group_id || '—' },
    { key: 'expected', value: formatCost(item.expected_cost, locale.value) },
    { key: 'charged', value: formatCost(item.charged_cost, locale.value) },
    { key: 'created', value: formatTaskTime(item.created_at, locale.value) },
    { key: 'updated', value: formatTaskTime(item.updated_at, locale.value) },
    { key: 'completed', value: formatTaskTime(item.completed_at, locale.value) },
    { key: 'elapsed', value: elapsed ? t('admin.videoTasks.elapsed', elapsed) : '—' },
    { key: 'nextCheck', value: formatTaskTime(item.next_attempt_at, locale.value) },
    { key: 'attempts', value: item.attempts ?? '—' },
    { key: 'duration', value: item.video_duration_seconds ?? '—' },
    { key: 'resolution', value: item.video_resolution || '—' },
    { key: 'usageLogId', value: item.usage_log_id || '—' }
  ]
})

async function loadTasks() {
  const id = ++requestId
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  loadFailed.value = false
  selected.value = null
  rows.value = []
  total.value = 0
  try {
    const result = await videoTasksAPI.list({ ...appliedFilters, page: page.value, page_size: pageSize.value }, controller.signal)
    if (id !== requestId) return
    // Unresolved submissions have no task ID. Use response-local keys to avoid collisions.
    rows.value = result.items.map((item, index) => ({ ...item, _rowKey: `${id}:${index}` }))
    total.value = result.total
    page.value = result.page
    pageSize.value = result.page_size
  } catch {
    if (id === requestId) loadFailed.value = true
  } finally {
    if (id === requestId) loading.value = false
  }
}

function search() {
  appliedFilters = { search: filters.search || undefined, status: filters.status || undefined, days: filters.days }
  page.value = 1
  void loadTasks()
}
function refresh() { void loadTasks() }
function changePage(value: number) { page.value = value; void loadTasks() }
function changePageSize(value: string | number | boolean | null) {
  const size = Number(value)
  if (![20, 50, 100].includes(size)) return
  pageSize.value = size
  page.value = 1
  void loadTasks()
}

onMounted(loadTasks)
onUnmounted(() => { requestId++; controller?.abort() })
</script>
