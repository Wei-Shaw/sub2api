/**
 * Composable: CRUD + filtering logic for ChannelMonitorView.
 *
 * Extracted from ChannelMonitorView.vue to keep the view under 300 lines.
 */
import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  channelMonitorAPI,
  type ChannelMonitor,
  type CheckResult,
  type ListParams,
  type Provider,
} from '../api/admin/channelMonitor'
import { getSdk } from '../api/sdk'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'
import { loadPageSize, savePageSize } from '../utils/pageSize'

const PAGE_SIZE_SCOPE = 'channelMonitor'

export function useChannelMonitorCrud() {
  const { t } = useI18n()
  const sdk = getSdk()

  const monitors = ref<ChannelMonitor[]>([])
  const loading = ref(false)
  const runningId = ref<number | null>(null)
  const searchQuery = ref('')
  const providerFilter = ref<Provider | ''>('')
  const enabledFilter = ref<'' | 'true' | 'false'>('')
  const pagination = reactive({
    page: 1,
    page_size: loadPageSize(PAGE_SIZE_SCOPE),
    total: 0,
  })

  const showDialog = ref(false)
  const editing = ref<ChannelMonitor | null>(null)
  const showDeleteDialog = ref(false)
  const deleting = ref<ChannelMonitor | null>(null)
  const showRunResult = ref(false)
  const runResults = ref<CheckResult[]>([])
  const showTemplateManager = ref(false)

  let abortController: AbortController | null = null

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
      sdk.notify.error(extractApiErrorMessage(err, t('admin.channelMonitor.loadError')))
    } finally {
      if (abortController === ctrl) {
        loading.value = false
        abortController = null
      }
    }
  }

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
    savePageSize(PAGE_SIZE_SCOPE, size)
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
      sdk.notify.error(extractApiErrorMessage(err, t('common.error')))
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
      sdk.notify.error(extractApiErrorMessage(err, t('admin.channelMonitor.runFailed')))
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
      sdk.notify.error(extractApiErrorMessage(err, t('common.error')))
    }
  }

  onMounted(reload)
  onUnmounted(() => { abortController?.abort() })

  return {
    monitors,
    loading,
    runningId,
    searchQuery,
    providerFilter,
    enabledFilter,
    pagination,
    showDialog,
    editing,
    showDeleteDialog,
    deleting,
    showRunResult,
    runResults,
    showTemplateManager,
    reload,
    handleSearchImmediate,
    onPageChange,
    onPageSizeChange,
    openCreateDialog,
    openEditDialog,
    closeDialog,
    toggleEnabled,
    handleRunNow,
    handleDelete,
    confirmDelete,
  }
}
