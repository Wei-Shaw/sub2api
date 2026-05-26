/**
 * Composable: checkout data, method options, amount validation, fee/multiplier calculations.
 *
 * Extracted from PaymentView.vue to keep the view under 300 lines.
 * All money fields are decimal strings; arithmetic goes through `money()`.
 */
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { money, formatMoney, Decimal } from '../utils/decimal'
import { getVisibleMethods } from '../components/payment/paymentFlow'
import { METHOD_ORDER } from '../components/payment/providerConfig'
import { formatPaymentAmount, normalizePaymentCurrency } from '../components/payment/currency'
import type { CheckoutInfoResponse, SubscriptionPlan } from '../types/payment'
import type { PaymentMethodOption } from '../components/payment/PaymentMethodSelector.vue'

export function usePaymentCheckout() {
  const i18n = useI18n()
  const { t } = i18n

  // ── Core state ──────────────────────────────────────────────────
  const checkout = ref<CheckoutInfoResponse>({
    methods: {}, global_min: '0', global_max: '0',
    plans: [], balance_disabled: false, balance_recharge_multiplier: '1',
    recharge_fee_rate: '0', help_text: '', help_image_url: '', stripe_publishable_key: '',
  })
  const amount = ref<number | null>(null)
  const selectedMethod = ref('')
  const selectedPlan = ref<SubscriptionPlan | null>(null)

  // ── Tabs ────────────────────────────────────────────────────────
  const tabs = computed(() => {
    const result: { key: 'recharge' | 'subscription'; label: string }[] = []
    if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
    result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
    return result
  })

  // ── Visible methods / limits ────────────────────────────────────
  const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
  const enabledMethods = computed(() => Object.keys(visibleMethods.value))

  const validAmount = computed<Decimal>(() => money(amount.value))

  const balanceRechargeMultiplier = computed<Decimal>(() => {
    const m = money(checkout.value.balance_recharge_multiplier)
    return m.gt(0) ? m : new Decimal(1)
  })
  const hasMultiplier = computed(() => !balanceRechargeMultiplier.value.equals(1))
  const creditedAmount = computed<Decimal>(() =>
    validAmount.value.mul(balanceRechargeMultiplier.value).toDecimalPlaces(2),
  )

  // ── Amount range ────────────────────────────────────────────────
  const globalMinAmount = computed<Decimal>(() => {
    const limits = Object.values(visibleMethods.value)
    if (limits.length === 0) return new Decimal(0)
    if (limits.some(l => money(l.single_min).lte(0))) return new Decimal(0)
    return limits
      .map(l => money(l.single_min))
      .reduce((acc, v) => (v.lt(acc) ? v : acc), money(limits[0].single_min))
  })
  const globalMaxAmount = computed<Decimal>(() => {
    const limits = Object.values(visibleMethods.value)
    if (limits.length === 0) return new Decimal(0)
    if (limits.some(l => money(l.single_max).lte(0))) return new Decimal(0)
    return limits
      .map(l => money(l.single_max))
      .reduce((acc, v) => (v.gt(acc) ? v : acc), money(limits[0].single_max))
  })
  const globalMinAmountNum = computed(() => globalMinAmount.value.toNumber())
  const globalMaxAmountNum = computed(() => globalMaxAmount.value.toNumber())

  // ── Method-amount fit check ─────────────────────────────────────
  function amountFitsMethod(amt: Decimal | number | string, methodType: string): boolean {
    const amtDec = money(amt)
    if (amtDec.lte(0)) return true
    const ml = visibleMethods.value[methodType]
    if (!ml) return false
    const min = money(ml.single_min)
    const max = money(ml.single_max)
    if (min.gt(0) && amtDec.lt(min)) return false
    if (max.gt(0) && amtDec.gt(max)) return false
    return true
  }

  // ── Selected method / currency ──────────────────────────────────
  const selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])
  const selectedCurrency = computed(() => normalizePaymentCurrency(selectedLimit.value?.currency))
  const localeCode = computed(() => {
    const raw = i18n.locale as unknown
    if (typeof raw === 'string') return raw
    if (raw && typeof raw === 'object' && 'value' in raw) {
      return String((raw as { value?: string }).value || '')
    }
    return undefined
  })

  function formatSelectedPaymentAmount(value: number | Decimal | string): string {
    const num = typeof value === 'number' ? value : Number(String(value))
    return formatPaymentAmount(num, selectedCurrency.value, localeCode.value)
  }

  // ── Fee calculation (recharge) ──────────────────────────────────
  const feeRate = computed<Decimal>(() => money(checkout.value?.recharge_fee_rate))
  const hasFeeRate = computed(() => feeRate.value.gt(0))
  const feeAmount = computed<Decimal>(() => {
    if (feeRate.value.lte(0) || validAmount.value.lte(0)) return new Decimal(0)
    return validAmount.value
      .mul(feeRate.value).div(100)
      .toDecimalPlaces(2, Decimal.ROUND_CEIL)
  })
  const totalAmount = computed<Decimal>(() =>
    feeRate.value.gt(0) && validAmount.value.gt(0)
      ? validAmount.value.plus(feeAmount.value).toDecimalPlaces(2)
      : validAmount.value,
  )

  // ── Fee calculation (subscription) ──────────────────────────────
  const planPrice = computed<Decimal>(() => money(selectedPlan.value?.price))
  const hasPositivePlanPrice = computed(() => planPrice.value.gt(0))
  const subFeeAmount = computed<Decimal>(() => {
    if (feeRate.value.lte(0) || planPrice.value.lte(0)) return new Decimal(0)
    return planPrice.value
      .mul(feeRate.value).div(100)
      .toDecimalPlaces(2, Decimal.ROUND_CEIL)
  })
  const subTotalAmount = computed<Decimal>(() => {
    if (feeRate.value.lte(0) || planPrice.value.lte(0)) return planPrice.value
    return planPrice.value.plus(subFeeAmount.value).toDecimalPlaces(2)
  })

  // ── Method option arrays ────────────────────────────────────────
  const methodOptions = computed<PaymentMethodOption[]>(() =>
    enabledMethods.value.map((type) => {
      const ml = visibleMethods.value[type]
      return {
        type,
        fee_rate: money(ml?.fee_rate).toNumber(),
        available: ml?.available !== false && amountFitsMethod(validAmount.value, type),
      }
    }),
  )
  const subMethodOptions = computed<PaymentMethodOption[]>(() => {
    const price = selectedPlan.value?.price ?? '0'
    return enabledMethods.value.map((type) => {
      const ml = visibleMethods.value[type]
      return {
        type,
        fee_rate: money(ml?.fee_rate).toNumber(),
        available: ml?.available !== false && amountFitsMethod(price, type),
      }
    })
  })

  // ── Validation ──────────────────────────────────────────────────
  const amountError = computed(() => {
    if (validAmount.value.lte(0)) return ''
    if (!enabledMethods.value.some(m => amountFitsMethod(validAmount.value, m))) {
      return t('payment.amountNoMethod')
    }
    const ml = selectedLimit.value
    if (ml) {
      const min = money(ml.single_min)
      const max = money(ml.single_max)
      if (min.gt(0) && validAmount.value.lt(min)) return t('payment.amountTooLow', { min: formatSelectedPaymentAmount(ml.single_min) })
      if (max.gt(0) && validAmount.value.gt(max)) return t('payment.amountTooHigh', { max: formatSelectedPaymentAmount(ml.single_max) })
    }
    return ''
  })

  const canSubmit = computed(() =>
    validAmount.value.gt(0)
    && amountFitsMethod(validAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false,
  )
  const canSubmitSubscription = computed(() =>
    selectedPlan.value !== null
    && amountFitsMethod(selectedPlan.value.price, selectedMethod.value)
    && selectedLimit.value?.available !== false,
  )

  // ── Auto-switch method when amount exceeds limits ───────────────
  watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
    if (amt.lte(0) || amountFitsMethod(amt, method)) return
    const available = enabledMethods.value.find(m => amountFitsMethod(amt, m))
    if (available) selectedMethod.value = available
  })

  // ── Grid class for plan cards ───────────────────────────────────
  const planGridClass = computed(() => {
    const n = checkout.value.plans.length
    if (n <= 2) return 'grid grid-cols-1 gap-5 sm:grid-cols-2'
    return 'grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3'
  })

  // ── Init: set default method from checkout ──────────────────────
  function initDefaultMethod() {
    if (!enabledMethods.value.length) return
    const order: readonly string[] = METHOD_ORDER
    const sorted = [...enabledMethods.value].sort((a, b) => {
      const ai = order.indexOf(a)
      const bi = order.indexOf(b)
      return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
    })
    selectedMethod.value = sorted[0]
  }

  // ── Payment button class ────────────────────────────────────────
  const paymentButtonClass = computed(() => {
    const m = selectedMethod.value
    if (!m) return 'btn-primary'
    if (m.includes('alipay')) return 'btn-alipay'
    if (m.includes('wxpay')) return 'btn-wxpay'
    if (m === 'stripe') return 'btn-stripe'
    if (m === 'airwallex') return 'btn-airwallex'
    return 'btn-primary'
  })

  return {
    checkout,
    amount,
    selectedMethod,
    selectedPlan,
    tabs,
    visibleMethods,
    enabledMethods,
    validAmount,
    balanceRechargeMultiplier,
    hasMultiplier,
    creditedAmount,
    globalMinAmountNum,
    globalMaxAmountNum,
    amountFitsMethod,
    selectedCurrency,
    formatSelectedPaymentAmount,
    feeRate,
    hasFeeRate,
    feeAmount,
    totalAmount,
    planPrice,
    hasPositivePlanPrice,
    subFeeAmount,
    subTotalAmount,
    methodOptions,
    subMethodOptions,
    amountError,
    canSubmit,
    canSubmitSubscription,
    planGridClass,
    initDefaultMethod,
    paymentButtonClass,
    formatMoney,
  }
}
