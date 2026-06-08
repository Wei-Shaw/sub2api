/**
 * Anthropic OAuth-specific quota control state.
 * Handles window cost, session limit, RPM, TLS, session masking, cache TTL, custom base URL.
 */
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getClient } from '../api/client'
import type { SdkAccount } from '@sub2api/plugin-sdk'

export function useAnthropicOAuthForm() {
  const { t } = useI18n()

  const windowCostEnabled = ref(false)
  const windowCostLimit = ref<number | null>(null)
  const windowCostStickyReserve = ref<number | null>(null)
  const sessionLimitEnabled = ref(false)
  const maxSessions = ref<number | null>(null)
  const sessionIdleTimeout = ref<number | null>(null)
  const rpmLimitEnabled = ref(false)
  const baseRpm = ref<number | null>(null)
  const rpmStrategy = ref<'tiered' | 'sticky_exempt'>('tiered')
  const rpmStickyBuffer = ref<number | null>(null)
  const userMsgQueueMode = ref('')
  const tlsFingerprintEnabled = ref(false)
  const tlsFingerprintProfileId = ref<number | null>(null)
  const tlsFingerprintProfiles = ref<{ id: number; name: string }[]>([])
  const sessionIdMaskingEnabled = ref(false)
  const cacheTTLOverrideEnabled = ref(false)
  const cacheTTLOverrideTarget = ref<string>('5m')
  const customBaseUrlEnabled = ref(false)
  const customBaseUrl = ref('')

  const umqModeOptions = computed(() => [
    { value: '', label: t('admin.accounts.quotaControl.rpmLimit.umqModeOff') },
    { value: 'throttle', label: t('admin.accounts.quotaControl.rpmLimit.umqModeThrottle') },
    { value: 'serialize', label: t('admin.accounts.quotaControl.rpmLimit.umqModeSerialize') },
  ])

  async function loadTlsProfiles(): Promise<void> {
    try {
      const { data } = await getClient().get<{ id: number; name: string }[]>('/admin/tls-fingerprint-profiles')
      tlsFingerprintProfiles.value = data.map(p => ({ id: p.id, name: p.name }))
    } catch { tlsFingerprintProfiles.value = [] }
  }

  function buildOAuthExtra(baseExtra?: Record<string, unknown>): Record<string, unknown> {
    const extra: Record<string, unknown> = { ...(baseExtra || {}) }
    if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
      extra.window_cost_limit = windowCostLimit.value
      extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
    }
    if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
      extra.max_sessions = maxSessions.value
      extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
    }
    if (rpmLimitEnabled.value) {
      const DEFAULT_BASE_RPM = 15
      extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0) ? baseRpm.value : DEFAULT_BASE_RPM
      extra.rpm_strategy = rpmStrategy.value
      if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) extra.rpm_sticky_buffer = rpmStickyBuffer.value
    }
    if (userMsgQueueMode.value) extra.user_msg_queue_mode = userMsgQueueMode.value
    if (tlsFingerprintEnabled.value) {
      extra.enable_tls_fingerprint = true
      if (tlsFingerprintProfileId.value) extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
    }
    if (sessionIdMaskingEnabled.value) extra.session_id_masking_enabled = true
    if (cacheTTLOverrideEnabled.value) {
      extra.cache_ttl_override_enabled = true
      extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
    }
    if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
      extra.custom_base_url_enabled = true
      extra.custom_base_url = customBaseUrl.value.trim()
    }
    return extra
  }

  function resetOAuthQuota(): void {
    windowCostEnabled.value = false
    windowCostLimit.value = null
    windowCostStickyReserve.value = null
    sessionLimitEnabled.value = false
    maxSessions.value = null
    sessionIdleTimeout.value = null
    rpmLimitEnabled.value = false
    baseRpm.value = null
    rpmStrategy.value = 'tiered'
    rpmStickyBuffer.value = null
    userMsgQueueMode.value = ''
    tlsFingerprintEnabled.value = false
    tlsFingerprintProfileId.value = null
    sessionIdMaskingEnabled.value = false
    cacheTTLOverrideEnabled.value = false
    cacheTTLOverrideTarget.value = '5m'
    customBaseUrlEnabled.value = false
    customBaseUrl.value = ''
  }

  function initOAuthFromAccount(account: SdkAccount): void {
    const extra = account.extra
    windowCostEnabled.value = (extra?.window_cost_limit as number) > 0
    windowCostLimit.value = (extra?.window_cost_limit as number) || null
    windowCostStickyReserve.value = (extra?.window_cost_sticky_reserve as number) ?? null
    sessionLimitEnabled.value = (extra?.max_sessions as number) > 0
    maxSessions.value = (extra?.max_sessions as number) || null
    sessionIdleTimeout.value = (extra?.session_idle_timeout_minutes as number) ?? null
    rpmLimitEnabled.value = (extra?.base_rpm as number) > 0
    baseRpm.value = (extra?.base_rpm as number) || null
    rpmStrategy.value = (extra?.rpm_strategy as 'tiered' | 'sticky_exempt') || 'tiered'
    rpmStickyBuffer.value = (extra?.rpm_sticky_buffer as number) ?? null
    userMsgQueueMode.value = (extra?.user_msg_queue_mode as string) || ''
    tlsFingerprintEnabled.value = extra?.enable_tls_fingerprint === true
    tlsFingerprintProfileId.value = (extra?.tls_fingerprint_profile_id as number) ?? null
    sessionIdMaskingEnabled.value = extra?.session_id_masking_enabled === true
    cacheTTLOverrideEnabled.value = extra?.cache_ttl_override_enabled === true
    cacheTTLOverrideTarget.value = (extra?.cache_ttl_override_target as string) || '5m'
    customBaseUrlEnabled.value = extra?.custom_base_url_enabled === true
    customBaseUrl.value = (extra?.custom_base_url as string) || ''
  }

  function applyOAuthEditExtra(newExtra: Record<string, unknown>): void {
    if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
      newExtra.window_cost_limit = windowCostLimit.value
      newExtra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
    } else { delete newExtra.window_cost_limit; delete newExtra.window_cost_sticky_reserve }
    if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
      newExtra.max_sessions = maxSessions.value
      newExtra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
    } else { delete newExtra.max_sessions; delete newExtra.session_idle_timeout_minutes }
    if (rpmLimitEnabled.value) {
      newExtra.base_rpm = baseRpm.value ?? 15
      newExtra.rpm_strategy = rpmStrategy.value
      if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) newExtra.rpm_sticky_buffer = rpmStickyBuffer.value
      else delete newExtra.rpm_sticky_buffer
    } else { delete newExtra.base_rpm; delete newExtra.rpm_strategy; delete newExtra.rpm_sticky_buffer }
    if (userMsgQueueMode.value) newExtra.user_msg_queue_mode = userMsgQueueMode.value
    else delete newExtra.user_msg_queue_mode
    if (tlsFingerprintEnabled.value) {
      newExtra.enable_tls_fingerprint = true
      if (tlsFingerprintProfileId.value) newExtra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
      else delete newExtra.tls_fingerprint_profile_id
    } else { delete newExtra.enable_tls_fingerprint; delete newExtra.tls_fingerprint_profile_id }
    if (sessionIdMaskingEnabled.value) newExtra.session_id_masking_enabled = true
    else delete newExtra.session_id_masking_enabled
    if (cacheTTLOverrideEnabled.value) {
      newExtra.cache_ttl_override_enabled = true
      newExtra.cache_ttl_override_target = cacheTTLOverrideTarget.value
    } else { delete newExtra.cache_ttl_override_enabled; delete newExtra.cache_ttl_override_target }
    if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
      newExtra.custom_base_url_enabled = true
      newExtra.custom_base_url = customBaseUrl.value.trim()
    } else { delete newExtra.custom_base_url_enabled; delete newExtra.custom_base_url }
  }

  return {
    windowCostEnabled, windowCostLimit, windowCostStickyReserve,
    sessionLimitEnabled, maxSessions, sessionIdleTimeout,
    rpmLimitEnabled, baseRpm, rpmStrategy, rpmStickyBuffer,
    userMsgQueueMode, umqModeOptions,
    tlsFingerprintEnabled, tlsFingerprintProfileId, tlsFingerprintProfiles,
    sessionIdMaskingEnabled,
    cacheTTLOverrideEnabled, cacheTTLOverrideTarget,
    customBaseUrlEnabled, customBaseUrl,
    loadTlsProfiles, buildOAuthExtra, resetOAuthQuota,
    initOAuthFromAccount, applyOAuthEditExtra,
  }
}
