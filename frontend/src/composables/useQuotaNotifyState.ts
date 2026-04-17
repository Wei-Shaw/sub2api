import { reactive, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import { QUOTA_THRESHOLD_TYPE_FIXED, type QuotaThresholdType } from '@/constants/account'

export const QUOTA_NOTIFY_DIMS = ['daily', 'weekly', 'total'] as const
export type QuotaNotifyDim = (typeof QUOTA_NOTIFY_DIMS)[number]

interface DimState {
  enabled: boolean | null
  threshold: number | null
  thresholdType: QuotaThresholdType | null
}

export function useQuotaNotifyState() {
  const globalEnabled = ref(false)
  const state = reactive<Record<QuotaNotifyDim, DimState>>({
    daily: { enabled: null, threshold: null, thresholdType: null },
    weekly: { enabled: null, threshold: null, thresholdType: null },
    total: { enabled: null, threshold: null, thresholdType: null },
  })

  function loadGlobalState() {
    adminAPI.settings
      .getSettings()
      .then((settings) => {
        globalEnabled.value = settings.account_quota_notify_enabled === true
      })
      .catch(() => {
        globalEnabled.value = false
      })
  }

  function loadFromExtra(extra: Record<string, unknown> | null | undefined) {
    for (const d of QUOTA_NOTIFY_DIMS) {
      state[d].enabled = (extra?.[`quota_notify_${d}_enabled`] as boolean) ?? null
      state[d].threshold = (extra?.[`quota_notify_${d}_threshold`] as number) ?? null
      state[d].thresholdType = (extra?.[`quota_notify_${d}_threshold_type`] as QuotaThresholdType) ?? null
    }
  }

  function writeToExtra(extra: Record<string, unknown>, mode: 'create' | 'update') {
    for (const d of QUOTA_NOTIFY_DIMS) {
      const s = state[d]
      if (s.enabled) {
        extra[`quota_notify_${d}_enabled`] = true
        if (s.threshold != null) {
          extra[`quota_notify_${d}_threshold`] = s.threshold
        } else if (mode === 'update') {
          delete extra[`quota_notify_${d}_threshold`]
        }
        extra[`quota_notify_${d}_threshold_type`] = s.thresholdType || QUOTA_THRESHOLD_TYPE_FIXED
      } else if (mode === 'update') {
        delete extra[`quota_notify_${d}_enabled`]
        delete extra[`quota_notify_${d}_threshold`]
        delete extra[`quota_notify_${d}_threshold_type`]
      }
    }
  }

  function reset() {
    for (const d of QUOTA_NOTIFY_DIMS) {
      state[d].enabled = null
      state[d].threshold = null
      state[d].thresholdType = null
    }
  }

  return { globalEnabled, state, loadGlobalState, loadFromExtra, writeToExtra, reset }
}
