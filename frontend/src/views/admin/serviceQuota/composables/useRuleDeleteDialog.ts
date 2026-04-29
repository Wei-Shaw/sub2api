/**
 * useRuleDeleteDialog — 规则删除二次确认 + API 调用 + toast 提示。
 *
 * 把 askDelete / confirmDelete / deletingRule 三件套封装出来，让 ConfigView
 * 只关心 "弹框打开 + 确认后 reload"。
 *
 * onDeleted：删除成功后回调（通常用来触发 reload 列表）。
 */
import { ref, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  deleteServiceQuotaRule,
  type ServiceQuotaRule,
} from '@/api/admin/serviceQuota'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'

export interface UseRuleDeleteDialogResult {
  deletingRule: Ref<ServiceQuotaRule | null>
  askDelete: (rule: ServiceQuotaRule) => void
  cancelDelete: () => void
  confirmDelete: () => Promise<void>
}

export function useRuleDeleteDialog(onDeleted: () => void | Promise<void>): UseRuleDeleteDialogResult {
  const { t } = useI18n()
  const appStore = useAppStore()
  const deletingRule = ref<ServiceQuotaRule | null>(null)

  function askDelete(rule: ServiceQuotaRule): void {
    deletingRule.value = rule
  }

  function cancelDelete(): void {
    deletingRule.value = null
  }

  async function confirmDelete(): Promise<void> {
    const rule = deletingRule.value
    if (!rule) return
    try {
      await deleteServiceQuotaRule(rule.id)
      appStore.showSuccess(t('admin.serviceQuota.deleteSuccess'))
      deletingRule.value = null
      await onDeleted()
    } catch (error: unknown) {
      // 优先按后端 reason 查 common.errors.*（INVALID_ID / SERVICE_QUOTA_UNAVAILABLE 等），
      // miss 则走 deleteError 兜底
      appStore.showError(
        extractI18nErrorMessage(error, t, 'common.errors', t('admin.serviceQuota.deleteError')),
      )
    }
  }

  return { deletingRule, askDelete, cancelDelete, confirmDelete }
}
