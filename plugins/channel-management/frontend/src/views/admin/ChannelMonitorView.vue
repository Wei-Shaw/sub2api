<template>
  <!-- title/description 已上移到 channel-management manifest descriptions,
       host AppHeader 唯一渲染标题区. View 直接以 TablePageLayout 为根, 抄
       host AccountsView 写法: #filters slot 内一个 flex 行同时容纳筛选 + 操作,
       不再使用 PluginPageLayout / FilterBar / PageActions 包装组件.
       Dialogs 作为 sibling 渲染 (Vue 3 支持多 root). -->
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
/**
 * V5 W7.1 — Channel Monitor admin list view (host AccountsView style port).
 *
 * 简化范围 (vs 09fd83ab host):
 *   - 删除 HelpTooltip + api_key_decrypt_failed 视觉提示 (decrypt_failed
 *     仍然在 i18n 中描述, plugin 后续可加)
 *   - 删除 MonitorTemplateManagerDialog 入口 (V5 W6 后端未注册 template
 *     endpoints, plugin 端无相应 API)
 *   - 删除 MonitorActionsCell / MonitorPrimaryModelCell 子组件, 内联 actions
 *     和 primary_model 单元格
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
import {
  channelMonitorAPI,
  type ChannelMonitor,
  type CheckResult,
  type ListParams,
  type Provider,
} from '../../api/admin/channelMonitor'
import { useChannelMonitorFormat } from '../../composables/useChannelMonitorFormat'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
} from '../../utils/channelMonitorConstants'
import MonitorFormDialog from '../../components/admin/monitor/MonitorFormDialog.vue'
import MonitorRunResultDialog from '../../components/admin/monitor/MonitorRunResultDialog.vue'
import { getSdk } from '../../api/sdk'

const { t } = useI18n()
const sdk = getSdk()
const {
  providerLabel,
  providerBadgeClass,
  statusLabel,
  statusBadgeClass,
  formatLatency,
  formatAvailability,
} = useChannelMonitorFormat()

const PAGE_SIZE_STORAGE_KEY = 'channelMonitor.pageSize'

function readPersistedPageSize(): number {
  try {
    const v = window.localStorage.getItem(PAGE_SIZE_STORAGE_KEY)
    if (v) {
      const n = parseInt(v, 10)
      if (Number.isFinite(n) && n > 0) return n
    }
  } catch {
    // SSR / sandboxed iframe — ignore
  }
  return 20
}

function writePersistedPageSize(n: number) {
  try {
    window.localStorage.setItem(PAGE_SIZE_STORAGE_KEY, String(n))
  } catch {
    // ignore
  }
}

const monitors = ref<ChannelMonitor[]>([])
const loading = ref(false)
const runningId = ref<number | null>(null)
const searchQuery = ref('')
const providerFilter = ref<Provider | ''>('')
const enabledFilter = ref<'' | 'true' | 'false'>('')
const pagination = reactive({
  page: 1,
  page_size: readPersistedPageSize(),
  total: 0,
})

const showDialog = ref(false)
const editing = ref<ChannelMonitor | null>(null)
const showDeleteDialog = ref(false)
const deleting = ref<ChannelMonitor | null>(null)
const showRunResult = ref(false)
const runResults = ref<CheckResult[]>([])

let abortController: AbortController | null = null

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

function extractMessage(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message
    if (typeof m === 'string' && m) return m
  }
  return fallback
}

async function reload() {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  loading.value = true
  try {
    const params: ListParams = {
      page: pagination.page,
      page_size: pagination.page_size,
    }
    if (providerFilter.value) params.provider = providerFilter.value
    if (enabledFilter.value === 'true') params.enabled = true
    if (enabledFilter.value === 'false') params.enabled = false
    if (searchQuery.value.trim()) params.search = searchQuery.value.trim()

    const res = await channelMonitorAPI.list(params, { signal: ctrl.signal })
    if (ctrl.signal.aborted || abortController !== ctrl) return
    monitors.value = res.items || []
    pagination.total = res.total
  } catch (err: unknown) {
    const e = err as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    sdk.notify.error(extractMessage(err, t('admin.channelMonitor.loadError')))
  } finally {
    if (abortController === ctrl) {
      loading.value = false
      abortController = null
    }
  }
}

// SearchInput 自带 300ms debounce, 这里只需 reset page + reload
function handleSearchImmediate() {
  pagination.page = 1
  reload()
}

function onPageChange(page: number) {
  pagination.page = page
  reload()
}

function onPageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  writePersistedPageSize(size)
  reload()
}

function openCreateDialog() {
  editing.value = null
  showDialog.value = true
}

function openEditDialog(row: ChannelMonitor) {
  editing.value = row
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
  editing.value = null
}

async function toggleEnabled(row: ChannelMonitor) {
  const next = !row.enabled
  try {
    await channelMonitorAPI.update(row.id, { enabled: next })
    row.enabled = next
  } catch (err: unknown) {
    sdk.notify.error(extractMessage(err, t('common.error')))
  }
}

async function handleRunNow(row: ChannelMonitor) {
  if (runningId.value != null) return
  runningId.value = row.id
  try {
    const res = await channelMonitorAPI.runNow(row.id)
    runResults.value = res.results || []
    showRunResult.value = true
    sdk.notify.success(t('admin.channelMonitor.runSuccess'))
    void reload()
  } catch (err: unknown) {
    sdk.notify.error(extractMessage(err, t('admin.channelMonitor.runFailed')))
  } finally {
    runningId.value = null
  }
}

function handleDelete(row: ChannelMonitor) {
  deleting.value = row
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deleting.value) return
  try {
    await channelMonitorAPI.del(deleting.value.id)
    sdk.notify.success(t('admin.channelMonitor.deleteSuccess'))
    showDeleteDialog.value = false
    deleting.value = null
    reload()
  } catch (err: unknown) {
    sdk.notify.error(extractMessage(err, t('common.error')))
  }
}

onMounted(reload)
onUnmounted(() => {
  abortController?.abort()
})
</script>
