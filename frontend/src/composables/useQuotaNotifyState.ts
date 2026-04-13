import { reactive, ref REDACTED from 'vue'
import { adminAPI REDACTED from '@/api/admin'
import { QUOTA_THRESHOLD_TYPE_FIXED, type QuotaThresholdType REDACTED from '@/constants/account'

export const QUOTA_NOTIFY_DIMS = ['daily', 'weekly', 'total'] as const
export type QuotaNotifyDim = (typeof QUOTA_NOTIFY_DIMS)[number]

interface DimState {
  enabled: boolean | null
  threshold: number | null
  thresholdType: QuotaThresholdType | null
REDACTED

export function useQuotaNotifyState() {
  const globalEnabled = ref(false)
  const state = reactive<Record<QuotaNotifyDim, DimState>>({
    daily: { enabled: null, threshold: null, thresholdType: null REDACTED,
    weekly: { enabled: null, threshold: null, thresholdType: null REDACTED,
    total: { enabled: null, threshold: null, thresholdType: null REDACTED,
  REDACTED)

  function loadGlobalState() {
    adminAPI.settings
      .getSettings()
      .then((settings) => {
        globalEnabled.value = settings.account_quota_notify_enabled === true
      REDACTED)
      .catch(() => {
        globalEnabled.value = false
      REDACTED)
  REDACTED

  function loadFromExtra(extra: Record<string, unknown> | null | undefined) {
    for (const d of QUOTA_NOTIFY_DIMS) {
      state[d].enabled = (extra?.[`quota_notify_${dREDACTED_enabled`] as boolean) ?? null
      state[d].threshold = (extra?.[`quota_notify_${dREDACTED_threshold`] as number) ?? null
      state[d].thresholdType = (extra?.[`quota_notify_${dREDACTED_threshold_type`] as QuotaThresholdType) ?? null
    REDACTED
  REDACTED

  function writeToExtra(extra: Record<string, unknown>, mode: 'create' | 'update') {
    for (const d of QUOTA_NOTIFY_DIMS) {
      const s = state[d]
      if (s.enabled) {
        extra[`quota_notify_${dREDACTED_enabled`] = true
        if (s.threshold != null) {
          extra[`quota_notify_${dREDACTED_threshold`] = s.threshold
        REDACTED else if (mode === 'update') {
          delete extra[`quota_notify_${dREDACTED_threshold`]
        REDACTED
        extra[`quota_notify_${dREDACTED_threshold_type`] = s.thresholdType || QUOTA_THRESHOLD_TYPE_FIXED
      REDACTED else if (mode === 'update') {
        delete extra[`quota_notify_${dREDACTED_enabled`]
        delete extra[`quota_notify_${dREDACTED_threshold`]
        delete extra[`quota_notify_${dREDACTED_threshold_type`]
      REDACTED
    REDACTED
  REDACTED

  function reset() {
    for (const d of QUOTA_NOTIFY_DIMS) {
      state[d].enabled = null
      state[d].threshold = null
      state[d].thresholdType = null
    REDACTED
  REDACTED

  return { globalEnabled, state, loadGlobalState, loadFromExtra, writeToExtra, reset REDACTED
REDACTED
