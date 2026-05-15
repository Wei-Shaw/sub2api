/**
 * Composable: payment settings form state, loading, saving, option lists.
 *
 * Extracted from PaymentSettingsView.vue to keep the view under 300 lines.
 */
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/host'
import { adminPaymentAPI } from '../api/admin/payment'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'
import { money, formatMoney } from '../utils/decimal'

export function usePaymentSettings() {
  const { t } = useI18n()
  const appStore = useAppStore()

  const form = reactive({
    payment_enabled: false,
    payment_min_amount: '',
    payment_max_amount: '',
    payment_daily_limit: '',
    payment_order_timeout_minutes: 30,
    payment_max_pending_orders: 3,
    payment_balance_disabled: false,
    payment_balance_recharge_multiplier: '1',
    payment_recharge_fee_rate: '0',
    payment_load_balance_strategy: 'round-robin',
    payment_product_name_prefix: '',
    payment_product_name_suffix: '',
    payment_help_image_url: '',
    payment_help_text: '',
    payment_cancel_rate_limit_enabled: false,
    payment_cancel_rate_limit_max: 0,
    payment_cancel_rate_limit_window: 0,
    payment_cancel_rate_limit_unit: 'minute',
    payment_cancel_rate_limit_window_mode: 'rolling',
    payment_visible_method_alipay_enabled: true,
    payment_visible_method_alipay_source: 'official',
    payment_visible_method_wxpay_enabled: true,
    payment_visible_method_wxpay_source: 'official',
    payment_enabled_types: [] as string[],
  })

  const loading = ref(true)
  const saving = ref(false)

  // -- Option lists --
  const allPaymentTypes = computed(() => [
    { value: 'easypay', label: t('payment.adminSettings.providerEasypay') },
    { value: 'alipay', label: t('payment.adminSettings.providerAlipay') },
    { value: 'wxpay', label: t('payment.adminSettings.providerWxpay') },
    { value: 'stripe', label: t('payment.adminSettings.providerStripe') },
    { value: 'airwallex', label: t('payment.adminSettings.providerAirwallex') },
  ])

  const hasAnyPaymentTypeEnabled = computed(() => form.payment_enabled_types.length > 0)

  const balanceMultiplierPreview = computed(() => {
    const m = money(form.payment_balance_recharge_multiplier || '1')
    return formatMoney(m.gt(0) ? m : 1)
  })

  const hasFeeRate = computed(() => money(form.payment_recharge_fee_rate).gt(0))
  const feeRatePreview = computed(() => formatMoney(form.payment_recharge_fee_rate || '0'))

  const providerKeyOptions = computed(() => [
    { value: 'easypay', label: t('payment.adminSettings.providerEasypay') },
    { value: 'alipay', label: t('payment.adminSettings.providerAlipay') },
    { value: 'wxpay', label: t('payment.adminSettings.providerWxpay') },
    { value: 'stripe', label: t('payment.adminSettings.providerStripe') },
    { value: 'airwallex', label: t('payment.adminSettings.providerAirwallex') },
  ])

  const enabledProviderKeyOptions = computed(() =>
    providerKeyOptions.value.filter(opt => form.payment_enabled_types.includes(opt.value)),
  )

  const loadBalanceOptions = computed(() => [
    { value: 'round-robin', label: t('payment.adminSettings.strategyRoundRobin') },
    { value: 'least-amount', label: t('payment.adminSettings.strategyLeastAmount') },
  ])

  const cancelRateLimitUnitOptions = computed(() => [
    { value: 'minute', label: t('payment.adminSettings.cancelRateLimitUnitMinute') },
    { value: 'hour', label: t('payment.adminSettings.cancelRateLimitUnitHour') },
    { value: 'day', label: t('payment.adminSettings.cancelRateLimitUnitDay') },
  ])

  const cancelRateLimitModeOptions = computed(() => [
    { value: 'rolling', label: t('payment.adminSettings.cancelRateLimitWindowModeRolling') },
    { value: 'fixed', label: t('payment.adminSettings.cancelRateLimitWindowModeFixed') },
  ])

  function togglePaymentType(type: string): void {
    const idx = form.payment_enabled_types.indexOf(type)
    if (idx >= 0) form.payment_enabled_types.splice(idx, 1)
    else form.payment_enabled_types.push(type)
  }

  // -- Load --
  async function loadConfig() {
    try {
      const res = await adminPaymentAPI.getConfig()
      const cfg = res.data
      if (!cfg) return
      form.payment_enabled = !!cfg.enabled
      form.payment_min_amount = cfg.min_amount && money(cfg.min_amount).gt(0) ? String(cfg.min_amount) : ''
      form.payment_max_amount = cfg.max_amount && money(cfg.max_amount).gt(0) ? String(cfg.max_amount) : ''
      form.payment_daily_limit = cfg.daily_limit && money(cfg.daily_limit).gt(0) ? String(cfg.daily_limit) : ''
      form.payment_order_timeout_minutes = cfg.order_timeout_minutes ?? 30
      form.payment_max_pending_orders = cfg.max_pending_orders ?? 3
      form.payment_balance_disabled = !!cfg.balance_disabled
      form.payment_balance_recharge_multiplier = cfg.balance_recharge_multiplier ? String(cfg.balance_recharge_multiplier) : '1'
      form.payment_recharge_fee_rate = cfg.recharge_fee_rate ? String(cfg.recharge_fee_rate) : '0'
      form.payment_load_balance_strategy = cfg.load_balance_strategy || 'round-robin'
      form.payment_product_name_prefix = cfg.product_name_prefix || ''
      form.payment_product_name_suffix = cfg.product_name_suffix || ''
      form.payment_help_image_url = cfg.help_image_url || ''
      form.payment_help_text = cfg.help_text || ''
      form.payment_cancel_rate_limit_enabled = !!cfg.cancel_rate_limit_enabled
      form.payment_cancel_rate_limit_max = cfg.cancel_rate_limit_max ?? 0
      form.payment_cancel_rate_limit_window = cfg.cancel_rate_limit_window ?? 0
      form.payment_cancel_rate_limit_unit = cfg.cancel_rate_limit_unit || 'minute'
      form.payment_cancel_rate_limit_window_mode = cfg.cancel_rate_limit_window_mode || 'rolling'
      form.payment_visible_method_alipay_enabled = cfg.visible_method_alipay_enabled ?? true
      form.payment_visible_method_alipay_source = cfg.visible_method_alipay_source || 'official'
      form.payment_visible_method_wxpay_enabled = cfg.visible_method_wxpay_enabled ?? true
      form.payment_visible_method_wxpay_source = cfg.visible_method_wxpay_source || 'official'
      form.payment_enabled_types = Array.isArray(cfg.enabled_payment_types) ? [...cfg.enabled_payment_types] : []
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t('common.error')))
    }
  }

  // -- Save --
  async function saveSettings() {
    saving.value = true
    try {
      await adminPaymentAPI.updateConfig({
        enabled: form.payment_enabled,
        min_amount: form.payment_min_amount === '' ? '0' : formatMoney(form.payment_min_amount),
        max_amount: form.payment_max_amount === '' ? '0' : formatMoney(form.payment_max_amount),
        daily_limit: form.payment_daily_limit === '' ? '0' : formatMoney(form.payment_daily_limit),
        order_timeout_minutes: form.payment_order_timeout_minutes,
        max_pending_orders: form.payment_max_pending_orders,
        balance_disabled: form.payment_balance_disabled,
        balance_recharge_multiplier: formatMoney(form.payment_balance_recharge_multiplier || '1'),
        recharge_fee_rate: formatMoney(form.payment_recharge_fee_rate || '0'),
        load_balance_strategy: form.payment_load_balance_strategy,
        product_name_prefix: form.payment_product_name_prefix,
        product_name_suffix: form.payment_product_name_suffix,
        help_image_url: form.payment_help_image_url,
        help_text: form.payment_help_text,
        cancel_rate_limit_enabled: form.payment_cancel_rate_limit_enabled,
        cancel_rate_limit_max: form.payment_cancel_rate_limit_max,
        cancel_rate_limit_window: form.payment_cancel_rate_limit_window,
        cancel_rate_limit_unit: form.payment_cancel_rate_limit_unit,
        cancel_rate_limit_window_mode: form.payment_cancel_rate_limit_window_mode,
        visible_method_alipay_enabled: form.payment_visible_method_alipay_enabled,
        visible_method_alipay_source: form.payment_visible_method_alipay_source,
        visible_method_wxpay_enabled: form.payment_visible_method_wxpay_enabled,
        visible_method_wxpay_source: form.payment_visible_method_wxpay_source,
        enabled_payment_types: [...form.payment_enabled_types],
      })
      appStore.showSuccess(t('common.saved'))
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t('common.error')))
    } finally {
      saving.value = false
    }
  }

  return {
    form,
    loading,
    saving,
    allPaymentTypes,
    hasAnyPaymentTypeEnabled,
    balanceMultiplierPreview,
    hasFeeRate,
    feeRatePreview,
    providerKeyOptions,
    enabledProviderKeyOptions,
    loadBalanceOptions,
    cancelRateLimitUnitOptions,
    cancelRateLimitModeOptions,
    togglePaymentType,
    loadConfig,
    saveSettings,
  }
}