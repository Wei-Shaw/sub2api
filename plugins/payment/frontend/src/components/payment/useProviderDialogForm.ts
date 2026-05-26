/**
 * Composable: form state + logic for PaymentProviderDialog.
 *
 * Extracted from PaymentProviderDialog.vue to keep the dialog shell
 * under 300 lines.
 */
import { reactive, computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../../stores/host'
import type { ProviderInstance } from '../../types/payment'
import type { TypeOption } from './providerConfig'
import {
  PROVIDER_CONFIG_FIELDS, PROVIDER_SUPPORTED_TYPES,
  PROVIDER_CALLBACK_PATHS, WEBHOOK_PATHS,
  PAYMENT_MODE_QRCODE, PAYMENT_MODE_POPUP, STRIPE_SDK_API_VERSION,
  getAvailableTypes, extractBaseUrl,
} from './providerConfig'

export interface ProviderDialogPayload {
  provider_key: string; name: string; supported_types: string[]
  enabled: boolean; payment_mode: string
  refund_enabled: boolean; allow_user_refund: boolean
  config: Record<string, string>; limits: string
}

export function useProviderDialogForm(props: {
  editing: ProviderInstance | null
  allPaymentTypes: TypeOption[]
  redirectLabel: string
}) {
  const { t } = useI18n()
  const appStore = useAppStore()

  const form = reactive({
    name: '', provider_key: 'easypay', supported_types: [] as string[],
    enabled: true, payment_mode: PAYMENT_MODE_QRCODE,
    refund_enabled: false, allow_user_refund: false,
  })
  const config = reactive<Record<string, string>>({})
  const limits = reactive<Record<string, Record<string, number>>>({})
  const notifyBaseUrl = ref('')
  const returnBaseUrl = ref('')
  const limitsExpanded = ref(false)
  const visibleFields = reactive<Record<string, boolean>>({})
  const defaultBaseUrl = typeof window !== 'undefined' ? window.location.origin : ''

  const webhookHints: Record<string, string> = {
    stripe: 'admin.settings.payment.stripeWebhookHint',
    airwallex: 'admin.settings.payment.airwallexWebhookHint',
  }
  const providerWebhookUrl = computed(() => {
    const path = WEBHOOK_PATHS[form.provider_key]
    return webhookHints[form.provider_key] && path ? defaultBaseUrl + path : ''
  })
  const providerWebhookHint = computed(() =>
    webhookHints[form.provider_key] || 'admin.settings.payment.stripeWebhookHint',
  )
  const callbackPaths = computed(() => PROVIDER_CALLBACK_PATHS[form.provider_key] || null)
  const paymentModeOptions = computed(() => [
    { value: PAYMENT_MODE_QRCODE, label: t('payment.adminSettings.modeQRCode') },
    { value: PAYMENT_MODE_POPUP, label: t('payment.adminSettings.modePopup') },
  ])

  const availableTypes = computed(() => {
    const base = getAvailableTypes(form.provider_key, props.allPaymentTypes, props.redirectLabel)
    return base.map(opt =>
      opt.label === opt.value ? { ...opt, label: t(`payment.methods.${opt.value}`, opt.value) } : opt,
    )
  })
  const resolvedFields = computed(() =>
    (PROVIDER_CONFIG_FIELDS[form.provider_key] || []).map(f => ({
      ...f, label: f.label || t(`payment.adminSettings.field_${f.key}`),
    })),
  )
  const limitableTypes = computed(() => {
    if (form.provider_key === 'stripe') return [{ value: 'stripe', label: 'Stripe' }]
    return form.supported_types.filter(t => t !== 'easypay').map(v =>
      props.allPaymentTypes.find(pt => pt.value === v) || { value: v, label: v },
    )
  })

  function isTypeSelected(type: string): boolean { return form.supported_types.includes(type) }
  function toggleType(type: string) {
    form.supported_types = form.supported_types.includes(type)
      ? form.supported_types.filter(t => t !== type)
      : [...form.supported_types, type]
  }
  function onKeyChange() {
    form.supported_types = [...(PROVIDER_SUPPORTED_TYPES[form.provider_key] || [])]
    clearConfig(); applyDefaults()
  }
  function clearConfig() {
    Object.keys(config).forEach(k => delete config[k])
    Object.keys(limits).forEach(k => delete limits[k])
    Object.keys(visibleFields).forEach(k => delete visibleFields[k])
    notifyBaseUrl.value = ''; returnBaseUrl.value = ''; limitsExpanded.value = false
  }
  function applyDefaults() {
    for (const f of PROVIDER_CONFIG_FIELDS[form.provider_key] || [])
      if (f.defaultValue && !config[f.key]) config[f.key] = f.defaultValue
  }

  // --- Limits helpers ---
  function getLimitVal(paymentType: string, field: string): string {
    const val = limits[paymentType]?.[field]
    return val && val > 0 ? String(val) : ''
  }
  function limitPlaceholder(paymentType: string): string {
    const l = limits[paymentType]
    const has = l && ((l.singleMin > 0) || (l.singleMax > 0) || (l.dailyLimit > 0))
    return has ? t('payment.adminSettings.limitsNoLimit') : t('payment.adminSettings.limitsUseGlobal')
  }
  function setLimitVal(paymentType: string, field: string, val: string) {
    if (!limits[paymentType]) limits[paymentType] = {}
    const num = Number(val)
    if (val === '' || isNaN(num)) { delete limits[paymentType][field]; return }
    if (num > 0) limits[paymentType][field] = num
  }
  function serializeLimits(): string {
    const result: Record<string, Record<string, number>> = {}
    for (const [pt, fields] of Object.entries(limits)) {
      const clean: Record<string, number> = {}
      for (const [k, v] of Object.entries(fields)) if (v > 0) clean[k] = v
      if (Object.keys(clean).length > 0) result[pt] = clean
    }
    return Object.keys(result).length > 0 ? JSON.stringify(result) : ''
  }

  function buildPayload(): ProviderDialogPayload | null {
    if (!form.name.trim()) { appStore.showError(t('payment.adminSettings.validationNameRequired')); return null }
    for (const f of PROVIDER_CONFIG_FIELDS[form.provider_key] || []) {
      if (f.optional || (props.editing && f.sensitive)) continue
      if (!(config[f.key] || '').trim()) {
        appStore.showError(t('payment.adminSettings.validationFieldRequired', {
          field: f.label || t(`payment.adminSettings.field_${f.key}`),
        }))
        return null
      }
    }
    const clearableKeys = new Set(
      (PROVIDER_CONFIG_FIELDS[form.provider_key] || []).filter(f => f.clearable).map(f => f.key),
    )
    const filteredConfig: Record<string, string> = {}
    for (const [k, v] of Object.entries(config)) {
      if (!v || !v.trim()) { if (clearableKeys.has(k)) filteredConfig[k] = ''; continue }
      filteredConfig[k] = v
    }
    const paths = PROVIDER_CALLBACK_PATHS[form.provider_key]
    if (paths) {
      const nb = notifyBaseUrl.value.trim() || defaultBaseUrl
      const rb = returnBaseUrl.value.trim() || defaultBaseUrl
      notifyBaseUrl.value = nb; returnBaseUrl.value = rb
      if (paths.notifyUrl) filteredConfig['notifyUrl'] = nb + paths.notifyUrl
      if (paths.returnUrl) filteredConfig['returnUrl'] = rb + paths.returnUrl
    }
    return {
      provider_key: form.provider_key, name: form.name,
      supported_types: form.supported_types, enabled: form.enabled,
      payment_mode: form.provider_key === 'easypay' ? form.payment_mode : '',
      refund_enabled: form.refund_enabled,
      allow_user_refund: form.refund_enabled ? form.allow_user_refund : false,
      config: filteredConfig, limits: serializeLimits(),
    }
  }

  function reset(defaultKey: string) {
    form.name = ''; form.provider_key = defaultKey
    form.supported_types = [...(PROVIDER_SUPPORTED_TYPES[defaultKey] || [])]
    form.enabled = true
    form.payment_mode = defaultKey === 'easypay' ? PAYMENT_MODE_QRCODE : ''
    form.refund_enabled = false; form.allow_user_refund = false
    clearConfig(); applyDefaults()
  }

  function loadProvider(p: ProviderInstance) {
    form.name = p.name; form.provider_key = p.provider_key
    form.supported_types = p.supported_types; form.enabled = p.enabled
    form.payment_mode = p.payment_mode || (p.provider_key === 'easypay' ? PAYMENT_MODE_QRCODE : '')
    form.refund_enabled = p.refund_enabled; form.allow_user_refund = p.allow_user_refund
    clearConfig()
    if (p.config) {
      for (const [k, v] of Object.entries(p.config)) {
        if (k === 'notifyUrl' || k === 'returnUrl') continue
        config[k] = v
      }
      const paths = PROVIDER_CALLBACK_PATHS[p.provider_key]
      if (paths?.notifyUrl && p.config['notifyUrl'])
        notifyBaseUrl.value = extractBaseUrl(p.config['notifyUrl'], paths.notifyUrl)
      if (paths?.returnUrl && p.config['returnUrl'])
        returnBaseUrl.value = extractBaseUrl(p.config['returnUrl'], paths.returnUrl)
    }
    applyDefaults()
    if (p.limits) {
      try {
        const parsed = JSON.parse(p.limits) as Record<string, Record<string, number>>
        for (const [pt, fields] of Object.entries(parsed)) limits[pt] = { ...fields }
        limitsExpanded.value = Object.keys(limits).length > 0
      } catch { /* ignore */ }
    }
  }

  return {
    form, config, limits, notifyBaseUrl, returnBaseUrl, limitsExpanded,
    visibleFields, defaultBaseUrl, providerWebhookUrl, providerWebhookHint,
    callbackPaths, paymentModeOptions, availableTypes, resolvedFields,
    limitableTypes, isTypeSelected, toggleType, onKeyChange,
    getLimitVal, limitPlaceholder, setLimitVal,
    buildPayload, reset, loadProvider, STRIPE_SDK_API_VERSION,
  }
}
