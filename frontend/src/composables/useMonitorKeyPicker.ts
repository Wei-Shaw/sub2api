import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { keysAPI } from '@/api/keys'
import { userGroupsAPI } from '@/api/groups'
import type { ApiKey } from '@/types'

/**
 * 封装 MonitorFormDialog 里"选择我的 API key"对话框的状态机：
 *   - showKeyPicker / loading / 缓存的 active keys / userGroupRates
 *   - openMyKeyPicker：首次打开时拉取 active keys + group rates，失败弹错
 *   - pickMyKey：选中后写入 form.api_key 并关闭对话框
 *
 * 单元测试由 dialog 组件级测试覆盖；这里只是把同一坨状态从 dialog 主文件挪走，
 * 行为完全等价（保留 cache：active keys 已加载过则不再请求）。
 */
export function useMonitorKeyPicker(setApiKey: (key: string) => void) {
  const { t } = useI18n()
  const appStore = useAppStore()

  const showKeyPicker = ref(false)
  const myKeysLoading = ref(false)
  const myActiveKeys = ref<ApiKey[]>([])
  const userGroupRates = ref<Record<number, number>>({})

  async function openMyKeyPicker() {
    showKeyPicker.value = true
    if (myActiveKeys.value.length > 0) return
    myKeysLoading.value = true
    try {
      const [res, rates] = await Promise.all([
        keysAPI.list(1, 100, { status: 'active' }),
        userGroupsAPI.getUserGroupRates(),
      ])
      const items = res.items || []
      const now = Date.now()
      myActiveKeys.value = items.filter((k) => {
        if (k.status !== 'active') return false
        if (!k.expires_at) return true
        return new Date(k.expires_at).getTime() > now
      })
      userGroupRates.value = rates
    } catch (err: unknown) {
      appStore.showError(
        extractApiErrorMessage(err, t('admin.channelMonitor.form.noActiveKey')),
      )
    } finally {
      myKeysLoading.value = false
    }
  }

  function pickMyKey(k: ApiKey) {
    setApiKey(k.key)
    showKeyPicker.value = false
  }

  function closeKeyPicker() {
    showKeyPicker.value = false
  }

  return {
    showKeyPicker,
    myKeysLoading,
    myActiveKeys,
    userGroupRates,
    openMyKeyPicker,
    pickMyKey,
    closeKeyPicker,
  }
}
