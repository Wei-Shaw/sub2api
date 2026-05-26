<template>
  <TablePageLayout>
    <template #filters>
      <div class="flex flex-wrap-reverse items-start justify-between gap-3">
        <!-- Filters -->
        <div class="flex flex-1 flex-wrap items-center gap-3">
          <div class="w-full sm:w-64">
            <SearchInput
              v-model="searchQuery"
              :placeholder="t('admin.channelMonitor.searchPlaceholder')"
              @search="handleSearchImmediate"
            />
          </div>
          <Select
            v-model="providerFilter"
            :options="providerFilterOptions"
            :placeholder="t('admin.channelMonitor.allProviders')"
            class="w-44"
            @change="reload"
          />
          <Select
            v-model="enabledFilter"
            :options="enabledFilterOptions"
            :placeholder="t('admin.channelMonitor.enabledFilter')"
            class="w-40"
            @change="reload"
          />
        </div>

        <!-- Actions -->
        <div class="flex flex-shrink-0 flex-wrap items-center gap-2">
          <button
            @click="reload"
            :disabled="loading"
            class="btn btn-secondary"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button @click="showTemplateManager = true" class="btn btn-secondary">
            {{ t('admin.channelMonitor.template.manageButton') }}
          </button>
          <button @click="openCreateDialog" class="btn btn-primary">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('admin.channelMonitor.createButton') }}
          </button>
        </div>
      </div>
    </template>

    <template #table>
      <DataTable :columns="columns" :data="monitors" :loading="loading">
        <template #cell-name="{ value }">
          <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
        </template>

        <template #cell-provider="{ row }">
          <span
            class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium"
            :class="providerBadgeClass(row.provider)"
          >
            {{ providerLabel(row.provider) }}
          </span>
        </template>

        <template #cell-primary_model="{ row }">
          <div class="flex flex-col">
            <span class="text-sm text-gray-900 dark:text-gray-100">{{ row.primary_model }}</span>
            <span
              v-if="row.primary_status"
              class="mt-0.5 inline-flex w-fit items-center rounded px-1.5 py-0.5 text-[10px] font-medium"
              :class="statusBadgeClass(row.primary_status)"
            >
              {{ statusLabel(row.primary_status) }}
            </span>
          </div>
        </template>

        <template #cell-availability_7d="{ row }">
          <span class="text-sm text-gray-900 dark:text-gray-100">{{ formatAvailability(row) }}</span>
        </template>

        <template #cell-latency="{ row }">
          <span class="text-sm text-gray-900 dark:text-gray-100">{{ formatLatency(row.primary_latency_ms) }}</span>
        </template>

        <template #cell-enabled="{ row }">
          <Toggle :modelValue="row.enabled" @update:modelValue="toggleEnabled(row)" />
        </template>

        <template #cell-actions="{ row }">
          <div class="flex items-center gap-2">
            <button
              @click="handleRunNow(row)"
              :disabled="runningId === row.id"
              class="btn btn-xs btn-secondary"
              :title="t('admin.channelMonitor.runNow')"
            >
              <Icon name="refresh" size="sm" :class="runningId === row.id ? 'animate-spin' : ''" />
            </button>
            <button
              @click="openEditDialog(row)"
              class="btn btn-xs btn-secondary"
              :title="t('common.edit')"
            >
              <Icon name="edit" size="sm" />
            </button>
            <button
              @click="handleDelete(row)"
              class="btn btn-xs btn-danger"
              :title="t('common.delete')"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </template>

        <template #empty>
          <EmptyState
            :title="t('admin.channelMonitor.noMonitorsYet')"
            :description="t('admin.channelMonitor.createFirstMonitor')"
            :action-text="t('admin.channelMonitor.createButton')"
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
        @update:page="onPageChange"
        @update:pageSize="onPageSizeChange"
      />
    </template>
  </TablePageLayout>

  <MonitorFormDialog
    :show="showDialog"
    :monitor="editing"
    @close="closeDialog"
    @saved="reload"
  />

  <MonitorTemplateManagerDialog
    :show="showTemplateManager"
    @close="showTemplateManager = false"
    @updated="reload"
  />

  <MonitorRunResultDialog
    :show="showRunResult"
    :results="runResults"
    @close="showRunResult = false"
  />

  <ConfirmDialog
    :show="showDeleteDialog"
    :title="t('common.delete')"
    :message="deleteConfirmMessage"
    :confirm-text="t('common.delete')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="confirmDelete"
    @cancel="showDeleteDialog = false"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
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
import { useChannelMonitorFormat } from '../../composables/useChannelMonitorFormat'
import { useChannelMonitorCrud } from '../../composables/useChannelMonitorCrud'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
} from '../../utils/channelMonitorConstants'
import MonitorFormDialog from '../../components/admin/monitor/MonitorFormDialog.vue'
import MonitorRunResultDialog from '../../components/admin/monitor/MonitorRunResultDialog.vue'
import MonitorTemplateManagerDialog from '../../components/admin/monitor/MonitorTemplateManagerDialog.vue'

const { t } = useI18n()
const {
  providerLabel,
  providerBadgeClass,
  statusLabel,
  statusBadgeClass,
  formatLatency,
  formatAvailability,
} = useChannelMonitorFormat()

const {
  monitors, loading, runningId, searchQuery, providerFilter, enabledFilter,
  pagination, showDialog, editing, showDeleteDialog, deleting,
  showRunResult, runResults, showTemplateManager,
  reload, handleSearchImmediate, onPageChange, onPageSizeChange,
  openCreateDialog, openEditDialog, closeDialog,
  toggleEnabled, handleRunNow, handleDelete, confirmDelete,
} = useChannelMonitorCrud()

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.channelMonitor.columns.name'), sortable: false },
  { key: 'provider', label: t('admin.channelMonitor.columns.provider'), sortable: false },
  { key: 'primary_model', label: t('admin.channelMonitor.columns.primaryModel'), sortable: false },
  { key: 'availability_7d', label: t('admin.channelMonitor.columns.availability7d'), sortable: false },
  { key: 'latency', label: t('admin.channelMonitor.columns.latency'), sortable: false },
  { key: 'enabled', label: t('admin.channelMonitor.columns.enabled'), sortable: false },
  { key: 'actions', label: t('admin.channelMonitor.columns.actions'), sortable: false },
])

const providerFilterOptions = computed(() => [
  { value: '', label: t('admin.channelMonitor.allProviders') },
  { value: PROVIDER_OPENAI, label: t('monitorCommon.providers.openai') },
  { value: PROVIDER_ANTHROPIC, label: t('monitorCommon.providers.anthropic') },
  { value: PROVIDER_GEMINI, label: t('monitorCommon.providers.gemini') },
])

const enabledFilterOptions = computed(() => [
  { value: '', label: t('admin.channelMonitor.allStatus') },
  { value: 'true', label: t('admin.channelMonitor.onlyEnabled') },
  { value: 'false', label: t('admin.channelMonitor.onlyDisabled') },
])

const deleteConfirmMessage = computed(() => {
  const name = deleting.value?.name || ''
  return t('admin.channelMonitor.deleteConfirm', { name })
})
</script>
