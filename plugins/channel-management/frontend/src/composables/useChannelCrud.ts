/**
 * Channel CRUD composable (T9 拆分).
 *
 * 把"拉取列表 / 拉取分组 / 创建 / 更新 / 删除 / 切换状态"等所有 admin 调用
 * 封装到一个 composable, 主 view 只负责调用 + 触发 reload, 不再持有 axios /
 * 异常处理 / abort 逻辑 (解耦原则).
 *
 * 公共错误处理: 所有方法都用 extractApiErrorMessage 提取并通过 SDK notify
 * 推送 toast, 调用方只需 await + 决定要不要 reload.
 */
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getSdk } from '../api/sdk'
import channelsAPI, {
  type Channel,
  type CreateChannelRequest,
  type UpdateChannelRequest,
} from '../api/channels'
import type { AdminGroup } from '../components/channels/types'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'

interface ListFilters {
  status?: string
  search?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

interface PaginationState {
  page: number
  page_size: number
  total: number
}

interface ListParams {
  pagination: PaginationState
  filters: ListFilters
}

export function useChannelCrud() {
  const { t } = useI18n()
  const sdk = getSdk()

  const channels = ref<Channel[]>([])
  const allGroups = ref<AdminGroup[]>([])
  const allChannelsForConflict = ref<Channel[]>([])
  const loading = ref(false)
  const groupsLoading = ref(false)

  let abortController: AbortController | null = null

  /** Wrap mutation: 成功 toast 'success', 失败 toast 'errorKey'. */
  async function runMutation(
    op: () => Promise<unknown>,
    successKey: string,
    successFallback: string,
    errorKey: string,
    errorFallback: string,
  ): Promise<boolean> {
    try {
      await op()
      sdk.notify.success(t(successKey, successFallback))
      return true
    } catch (error: unknown) {
      sdk.notify.error(
        extractApiErrorMessage(error, t(errorKey, errorFallback)),
      )
      console.error(`[useChannelCrud] ${errorKey}:`, error)
      return false
    }
  }

  async function loadChannels(params: ListParams): Promise<void> {
    if (abortController) abortController.abort()
    const ctrl = new AbortController()
    abortController = ctrl
    loading.value = true
    try {
      const response = await channelsAPI.list(
        params.pagination.page,
        params.pagination.page_size,
        {
          status: params.filters.status || undefined,
          search: params.filters.search || undefined,
          sort_by: params.filters.sort_by,
          sort_order: params.filters.sort_order,
        },
        { signal: ctrl.signal },
      )
      if (ctrl.signal.aborted || abortController !== ctrl) return
      channels.value = response.items || []
      params.pagination.total = response.total
    } catch (error: unknown) {
      const e = error as { name?: string; code?: string }
      if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
      sdk.notify.error(
        extractApiErrorMessage(
          error,
          t('admin.channels.loadError', 'Failed to load channels'),
        ),
      )
      console.error('Error loading channels:', error)
    } finally {
      if (abortController === ctrl) {
        loading.value = false
        abortController = null
      }
    }
  }

  async function loadGroups(): Promise<void> {
    groupsLoading.value = true
    try {
      const response = await sdk.http.apiClient.get<AdminGroup[]>(
        '/admin/groups/all',
      )
      allGroups.value = response.data
    } catch (error: unknown) {
      console.error('Error loading groups:', error)
    } finally {
      groupsLoading.value = false
    }
  }

  async function loadAllChannelsForConflict(): Promise<void> {
    try {
      const response = await channelsAPI.list(1, 1000)
      allChannelsForConflict.value = response.items || []
    } catch {
      // 拉不到时退化为当前页, 防止误判全部分组无冲突
      allChannelsForConflict.value = channels.value
    }
  }

  function createChannel(req: CreateChannelRequest): Promise<boolean> {
    return runMutation(
      () => channelsAPI.create(req),
      'admin.channels.createSuccess', 'Channel created',
      'admin.channels.createError', 'Failed to create channel',
    )
  }

  function updateChannel(id: number, req: UpdateChannelRequest): Promise<boolean> {
    return runMutation(
      () => channelsAPI.update(id, req),
      'admin.channels.updateSuccess', 'Channel updated',
      'admin.channels.updateError', 'Failed to update channel',
    )
  }

  function deleteChannel(id: number): Promise<boolean> {
    return runMutation(
      () => channelsAPI.remove(id),
      'admin.channels.deleteSuccess', 'Channel deleted',
      'admin.channels.deleteError', 'Failed to delete channel',
    )
  }

  /** 切换 channel.status (active <-> disabled). 失败时不修改, 返回 null. */
  async function toggleStatus(channel: Channel): Promise<string | null> {
    const newStatus = channel.status === 'active' ? 'disabled' : 'active'
    try {
      await channelsAPI.update(channel.id, { status: newStatus })
      return newStatus
    } catch (error: unknown) {
      sdk.notify.error(t('admin.channels.updateError', 'Failed to update channel'))
      console.error('Error toggling channel status:', error)
      return null
    }
  }

  function abortPending(): void {
    abortController?.abort()
    abortController = null
  }

  return {
    channels,
    allGroups,
    allChannelsForConflict,
    loading,
    groupsLoading,
    loadChannels,
    loadGroups,
    loadAllChannelsForConflict,
    createChannel,
    updateChannel,
    toggleStatus,
    deleteChannel,
    abortPending,
  }
}

export type ChannelCrud = ReturnType<typeof useChannelCrud>

/** Pagination state factory. */
export function createPaginationState(initialPageSize: number): PaginationState {
  return reactive({ page: 1, page_size: initialPageSize, total: 0 })
}
