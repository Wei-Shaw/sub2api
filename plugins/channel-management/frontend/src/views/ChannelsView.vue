<template>
  <!-- title/description 已上移到 channel-management manifest descriptions, host
       AppHeader 唯一渲染标题区. -->
  <TablePageLayout>
    <template #filters>
      <div class="flex flex-wrap-reverse items-start justify-between gap-3">
        <div class="flex flex-1 flex-wrap items-center gap-3">
          <div class="w-full sm:w-64">
            <SearchInput
              v-model="searchQuery"
              :placeholder="t('admin.channels.searchChannels', 'Search channels...')"
              @search="handleSearchImmediate"
            />
          </div>
          <Select
            v-model="filters.status"
            :options="statusFilterOptions"
            :placeholder="t('admin.channels.allStatus', 'All Status')"
            class="w-40"
            @change="reload"
          />
        </div>
        <div class="flex flex-shrink-0 flex-wrap items-center gap-2">
          <button
            @click="reload"
            :disabled="loading"
            class="btn btn-secondary"
            :title="t('common.refresh', 'Refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button @click="openCreateDialog" class="btn btn-primary">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('admin.channels.createChannel', 'Create Channel') }}
          </button>
        </div>
      </div>
    </template>

    <template #table>
      <DataTable
        :columns="columns"
        :data="channels"
        :loading="loading"
        :server-side-sort="true"
        default-sort-key="created_at"
        default-sort-order="desc"
        @sort="handleSort"
      >
        <template #cell-name="{ value }">
          <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
        </template>
        <template #cell-description="{ value }">
          <span class="text-sm text-gray-600 dark:text-gray-400">{{ value || '-' }}</span>
        </template>
        <template #cell-status="{ row }">
          <Toggle :modelValue="row.status === 'active'" @update:modelValue="onToggleStatus(row)" />
        </template>
        <template #cell-group_count="{ row }">
          <span class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300">
            {{ (row.group_ids || []).length }} {{ t('admin.channels.groupsUnit', 'groups') }}
          </span>
        </template>
        <template #cell-pricing_count="{ row }">
          <span class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300">
            {{ (row.model_pricing || []).length }} {{ t('admin.channels.pricingUnit', 'pricing rules') }}
          </span>
        </template>
        <template #cell-created_at="{ value }">
          <span class="text-sm text-gray-600 dark:text-gray-400">{{ formatDate(value) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="flex items-center gap-1">
            <button @click="openEditDialog(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400">
              <Icon name="edit" size="sm" />
              <span class="text-xs">{{ t('common.edit', 'Edit') }}</span>
            </button>
            <button @click="onRequestDelete(row)" class="btn-icon-danger flex flex-col items-center gap-0.5 p-1.5 text-gray-500">
              <Icon name="trash" size="sm" />
              <span class="text-xs">{{ t('common.delete', 'Delete') }}</span>
            </button>
          </div>
        </template>
        <template #empty>
          <EmptyState
            :title="t('admin.channels.noChannelsYet', 'No Channels Yet')"
            :description="t('admin.channels.createFirstChannel', 'Create your first channel to manage model pricing')"
            :action-text="t('admin.channels.createChannel', 'Create Channel')"
            @action="openCreateDialog"
          />
        </template>
      </DataTable>
    </template>

    <template v-if="pagination.total > 0" #pagination>
      <Pagination
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
    </template>
  </TablePageLayout>

  <ChannelDialog
    :show="dialog.show"
    :channel="dialog.editing"
    :all-groups="allGroups"
    :groups-loading="groupsLoading"
    :all-channels-for-conflict="allChannelsForConflict"
    @update:show="dialog.show = $event"
    @saved="onChannelSaved"
  />

  <ConfirmDialog
    :show="deleteState.show"
    :title="t('admin.channels.deleteChannel', 'Delete Channel')"
    :message="deleteConfirmMessage"
    :confirm-text="t('common.delete', 'Delete')"
    :cancel-text="t('common.cancel', 'Cancel')"
    :danger="true"
    @confirm="onConfirmDelete"
    @cancel="deleteState.show = false"
  />
</template>

<script setup lang="ts">
/**
 * ChannelsView — 渠道管理列表 (T9 拆分后).
 *
 * 主 view 只负责: 列表渲染 / 筛选 / 分页 / 排序 / 编排子组件 (ChannelDialog +
 * ConfirmDialog). form 维护 / 校验 / API 序列化 / CRUD 已下沉到
 * components/channels/ + composables/.
 */
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ConfirmDialog,
  DataTable,
  EmptyState,
  Icon,
  Pagination,
  SearchInput,
  Select,
  TablePageLayout,
  Toggle,
  type Column,
} from '@sub2api/plugin-sdk'
import ChannelDialog from '../components/channels/ChannelDialog.vue'
import { useChannelCrud } from '../composables/useChannelCrud'
import { loadPageSize, savePageSize } from '../utils/pageSize'
import type { Channel } from '../api/channels'

const PAGE_SIZE_SCOPE = 'channels'

const { t } = useI18n()

const {
  channels,
  allGroups,
  allChannelsForConflict,
  loading,
  groupsLoading,
  loadChannels,
  loadGroups,
  loadAllChannelsForConflict,
  toggleStatus,
  deleteChannel,
  abortPending,
} = useChannelCrud()

// ── State ──
const searchQuery = ref('')
const filters = reactive({ status: '' })
const pagination = reactive({ page: 1, page_size: loadPageSize(PAGE_SIZE_SCOPE), total: 0 })
const sortState = reactive({ sort_by: 'created_at', sort_order: 'desc' as 'asc' | 'desc' })
const dialog = reactive<{ show: boolean; editing: Channel | null }>({ show: false, editing: null })
const deleteState = reactive<{ show: boolean; target: Channel | null }>({ show: false, target: null })

// ── Columns ──
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.channels.columns.name', 'Name'), sortable: true },
  { key: 'description', label: t('admin.channels.columns.description', 'Description'), sortable: false },
  { key: 'status', label: t('admin.channels.columns.status', 'Status'), sortable: true },
  { key: 'group_count', label: t('admin.channels.columns.groups', 'Groups'), sortable: false },
  { key: 'pricing_count', label: t('admin.channels.columns.pricing', 'Pricing'), sortable: false },
  { key: 'created_at', label: t('admin.channels.columns.createdAt', 'Created'), sortable: true },
  { key: 'actions', label: t('admin.channels.columns.actions', 'Actions'), sortable: false },
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.channels.allStatus', 'All Status') },
  { value: 'active', label: t('admin.channels.statusActive', 'Active') },
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') },
])

const deleteConfirmMessage = computed(() => {
  const name = deleteState.target?.name || ''
  return t(
    'admin.channels.deleteConfirm',
    { name },
    `Are you sure you want to delete channel "${name}"? This action cannot be undone.`,
  )
})

function formatDate(value: string): string {
  return value ? new Date(value).toLocaleDateString() : '-'
}

async function reload(): Promise<void> {
  await loadChannels({
    pagination,
    filters: {
      status: filters.status,
      search: searchQuery.value,
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order,
    },
  })
}

function handleSearchImmediate(): void {
  pagination.page = 1
  reload()
}

function handlePageChange(page: number): void {
  pagination.page = page
  reload()
}

function handlePageSizeChange(pageSize: number): void {
  pagination.page_size = pageSize
  pagination.page = 1
  savePageSize(PAGE_SIZE_SCOPE, pageSize)
  reload()
}

function handleSort(key: string, order: 'asc' | 'desc'): void {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  reload()
}

async function openDialog(channel: Channel | null): Promise<void> {
  dialog.editing = channel
  await Promise.all([loadGroups(), loadAllChannelsForConflict()])
  dialog.show = true
}
const openCreateDialog = () => openDialog(null)
const openEditDialog = (channel: Channel) => openDialog(channel)
const onChannelSaved = () => reload()

async function onToggleStatus(channel: Channel): Promise<void> {
  const newStatus = await toggleStatus(channel)
  if (!newStatus) return
  if (filters.status && filters.status !== newStatus) await reload()
  else channel.status = newStatus
}

function onRequestDelete(channel: Channel): void {
  deleteState.target = channel
  deleteState.show = true
}

async function onConfirmDelete(): Promise<void> {
  if (!deleteState.target) return
  const ok = await deleteChannel(deleteState.target.id)
  deleteState.show = false
  deleteState.target = null
  if (ok) await reload()
}

onMounted(() => {
  reload()
  loadGroups()
})
onUnmounted(() => abortPending())
</script>
