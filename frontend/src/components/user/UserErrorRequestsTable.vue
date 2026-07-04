<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="card flex min-h-0 flex-1 flex-col overflow-hidden">
      <IpGeoBatchToolbar :ips="rows.map((r) => r.client_ip)" @failed="emit('ipGeoBatchFailed')" />

      <DataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        clickable-rows
        server-side-sort
        default-sort-key="created_at"
        default-sort-order="desc"
        @sort="onSort"
        @rowClick="(row) => openDetail(row.id)"
      >
        <template #cell-model="{ row REDACTED">
          <span v-if="row.model" class="text-sm font-medium text-gray-900 dark:text-white">{{ row.model REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-key_name="{ row REDACTED">
          <div class="text-sm">
            <span class="text-gray-900 dark:text-white">{{ row.key_name || '-' REDACTEDREDACTED</span>
            <span
              v-if="row.key_deleted"
              class="ml-1 inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-100 text-rose-600 ring-1 ring-inset ring-rose-200 dark:bg-rose-500/20 dark:text-rose-400 dark:ring-rose-500/30"
            >{{ t('usage.errors.keyDeleted') REDACTEDREDACTED</span>
          </div>
        </template>

        <template #cell-endpoint="{ row REDACTED">
          <div class="max-w-[320px] text-xs">
            <div class="break-all text-gray-700 dark:text-gray-300">
              <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('usage.inbound') REDACTEDREDACTED:</span>
              <span class="ml-1">{{ row.inbound_endpoint?.trim() || '-' REDACTEDREDACTED</span>
            </div>
          </div>
        </template>

        <template #cell-status="{ row REDACTED">
          <span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium" :class="statusClass(row.status_code)">
            {{ row.status_code || '-' REDACTEDREDACTED
          </span>
        </template>

        <template #cell-category="{ row REDACTED">
          <span class="text-sm text-gray-900 dark:text-white">{{ t('usage.errors.categories.' + row.category) REDACTEDREDACTED</span>
        </template>

        <template #cell-message="{ row REDACTED">
          <span
            v-if="row.message"
            class="block max-w-[280px] truncate text-sm text-gray-600 dark:text-gray-400"
            :title="row.message"
          >{{ row.message REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-group="{ row REDACTED">
          <span
            v-if="row.group_name"
            class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200"
          >{{ row.group_name REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-type="{ row REDACTED">
          <span
            v-if="requestTypeBadge(row)"
            class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
            :class="requestTypeBadge(row)!.className"
          >{{ requestTypeBadge(row)!.label REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-platform="{ row REDACTED">
          <span class="text-sm text-gray-900 dark:text-white">{{ row.platform || '-' REDACTEDREDACTED</span>
        </template>

        <template #cell-client_ip="{ row REDACTED">
          <div @click.stop>
            <div v-if="row.client_ip">
              <span class="text-sm font-mono text-gray-600 dark:text-gray-400">{{ row.client_ip REDACTEDREDACTED</span>
              <IpGeoCell :ip="row.client_ip" />
            </div>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
          </div>
        </template>

        <template #cell-created_at="{ row REDACTED">
          <span class="text-sm text-gray-600 dark:text-gray-400">{{ formatDateTime(row.created_at) REDACTEDREDACTED</span>
        </template>

        <template #cell-user_agent="{ row REDACTED">
          <span
            v-if="row.user_agent"
            class="block max-w-[320px] truncate text-sm text-gray-600 dark:text-gray-400"
            :title="row.user_agent"
          >{{ row.user_agent REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #empty><EmptyState :message="t('usage.errors.empty')" /></template>
      </DataTable>
    </div>

    <div class="flex-shrink-0">
      <Pagination
        v-if="total > 0"
        :page="page"
        :page-size="pageSize"
        :total="total"
        @update:page="$emit('update:page', $event)"
        @update:pageSize="$emit('update:pageSize', $event)"
      />
    </div>

    <UserErrorDetailModal v-model:show="showDetail" :error-id="selectedId" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import UserErrorDetailModal from '@/components/user/UserErrorDetailModal.vue'
import IpGeoCell from '@/components/common/IpGeoCell.vue'
import IpGeoBatchToolbar from '@/components/common/IpGeoBatchToolbar.vue'
import { formatDateTime REDACTED from '@/utils/format'
import {
  mapErrorSortKey,
  numericRequestTypeKind,
  requestTypeBadgeClass,
  requestTypeLabelKey,
  statusCodeBadgeClass,
REDACTED from '@/utils/errorBadges'
import type { UserErrorRequest REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'

const props = defineProps<{
  rows: UserErrorRequest[]
  total: number
  loading: boolean
  page: number
  pageSize: number
  /** 列设置:仅显示这些 key 的列;不传则全显(key 须与 allColumns 一致) */
  visibleColumnKeys?: string[]
REDACTED>()

const emit = defineEmits<{
  (e: 'update:page', v: number): void
  (e: 'update:pageSize', v: number): void
  (e: 'ipGeoBatchFailed'): void
  (e: 'sort', sortBy: string, sortOrder: 'asc' | 'desc'): void
REDACTED>()

function onSort(key: string, order: 'asc' | 'desc') {
  emit('sort', mapErrorSortKey(key), order)
REDACTED

const { t REDACTED = useI18n()

// 列序对齐用户端用量明细:Key → 模型 → 端点 → IP → 分组 → 类型 → 平台 → 分类
// → 结果(状态→消息)→ 时间 → UA(用量明细 UA 同在时间之后的尾部)
const allColumns = computed<Column[]>(() => [
  { key: 'key_name', label: t('usage.errors.keyName') REDACTED,
  { key: 'model', label: t('usage.errors.model'), sortable: true REDACTED,
  { key: 'endpoint', label: t('usage.errors.endpoint') REDACTED,
  { key: 'client_ip', label: 'IP' REDACTED,
  { key: 'group', label: t('admin.usage.group') REDACTED,
  { key: 'type', label: t('usage.type') REDACTED,
  { key: 'platform', label: t('usage.errors.platform') REDACTED,
  { key: 'category', label: t('usage.errors.category') REDACTED,
  { key: 'status', label: t('usage.errors.status'), sortable: true REDACTED,
  { key: 'message', label: t('usage.errors.message') REDACTED,
  { key: 'created_at', label: t('usage.errors.time'), sortable: true REDACTED,
  { key: 'user_agent', label: t('usage.userAgent') REDACTED,
])

const columns = computed<Column[]>(() =>
  props.visibleColumnKeys
    ? allColumns.value.filter((c) => props.visibleColumnKeys!.includes(c.key))
    : allColumns.value
)

function requestTypeBadge(row: UserErrorRequest): { label: string; className: string REDACTED | null {
  const kind = numericRequestTypeKind(row.request_type, row.stream)
  if (!kind) return null
  return { label: t(requestTypeLabelKey(kind)), className: requestTypeBadgeClass(kind) REDACTED
REDACTED

const showDetail = ref(false)
const selectedId = ref<number | null>(null)

function openDetail(id: number) {
  selectedId.value = id
  showDetail.value = true
REDACTED

const statusClass = statusCodeBadgeClass
</script>
