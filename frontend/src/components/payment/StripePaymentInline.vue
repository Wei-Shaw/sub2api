<template>
  <div class="space-y-4">
    <div v-if="loading" class="rounded border border-line bg-surface p-5" aria-live="polite">
      <p class="flex items-center gap-2 text-2xs font-medium uppercase tracking-[0.08em] text-ink-tertiary">
        <span class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
        {{ t('common.processing') }}
      </p>
    </div>

    <div v-else-if="initError" class="rounded border border-line bg-surface p-5">
      <div aria-live="polite">
        <p class="text-2xs font-medium uppercase tracking-[0.08em] text-danger">
          {{ t('payment.result.failed') }}
        </p>
        <p class="mt-2 text-sm text-ink">{{ initError }}</p>
      </div>
      <Button class="mt-5" variant="outline" size="md" @click="$emit('back')">
        {{ t('payment.result.backToRecharge') }}
      </Button>
    </div>

    <!-- Success -->
    <template v-else-if="success">
      <section class="rounded border border-line bg-surface p-5">
        <div aria-live="polite">
          <p class="text-2xs font-medium uppercase tracking-[0.08em] text-success">
            {{ t('payment.status.completed') }}
          </p>
          <h2 class="mt-2 text-lg font-semibold text-ink">{{ t('payment.result.success') }}</h2>
        </div>
        <dl class="mt-5 divide-y divide-line-subtle border-y border-line-subtle text-xs">
          <div class="flex items-baseline justify-between gap-4 py-1.5">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.orderId') }}</dt>
            <!-- Identifier, not quantity: `NumCell` would print `#1,234`. -->
            <dd class="font-mono tabular-nums slashed-zero text-ink">#{{ orderId }}</dd>
          </div>
          <div v-if="amount > 0" class="flex items-baseline justify-between gap-4 py-1.5">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.amount') }}</dt>
            <dd class="inline-flex items-baseline justify-end gap-0.5">
              <span class="text-2xs text-ink-tertiary">{{ creditedAmountSymbol }}</span>
              <NumCell :value="amount" :precision="2" />
            </dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 py-1.5">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.payAmount') }}</dt>
            <dd class="inline-flex items-baseline justify-end gap-0.5">
              <span class="text-2xs text-ink-tertiary">{{ paymentAmountSymbol }}</span>
              <NumCell :value="payAmount" :precision="paymentPrecision" />
            </dd>
          </div>
        </dl>
        <Button class="mt-5" tone="accent" variant="solid" size="md" @click="$emit('done')">
          {{ t('common.confirm') }}
        </Button>
      </section>
    </template>

    <template v-else>
      <!--
        The amount, as type rather than as a coloured banner.

        This used to be a `bg-gradient-to-br from-[#635bff] to-[#4f46e5]` block
        with white 30px digits — Stripe's brand gradient painted across the top
        of OUR page, which is precisely the seam this rewrite is supposed to
        close. Stripe's brand belongs on the Stripe button and inside Stripe's
        own frame, not on the page header. The number is the loudest thing here
        now because it is the largest, not because it sits on purple.
      -->
      <section class="rounded border border-line bg-surface px-4 py-3">
        <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
          {{ t('payment.actualPay') }}
        </p>
        <p class="mt-1 flex items-baseline gap-1">
          <span class="text-sm text-ink-tertiary">{{ paymentAmountSymbol }}</span>
          <span class="font-mono text-2xl font-semibold tabular-nums slashed-zero text-ink">
            <NumCell :value="payAmount" :precision="paymentPrecision" />
          </span>
        </p>
      </section>

      <!--
        THE DECLARED BOUNDARY.

        Everything inside `#stripe-payment-element` is a cross-origin iframe that
        Stripe styles itself. Our CSS stops at the frame edge, so the join can
        never be perfect — the font falls back to `system-ui` in there, and the
        wallet buttons are Stripe's.

        Rather than pretend otherwise and end up with a near-match that reads as
        a rendering bug, the region is FRAMED and LABELLED with the provider's
        name. A visible boundary around a third party's form is what a bank
        statement or a checkout page does; it reads as intent. The colours,
        radius and text size still cross the boundary via the Appearance API
        (see `stripeAppearance.ts`), which is what keeps it from looking foreign.
      -->
      <section class="rounded border border-line bg-surface">
        <div class="border-b border-line-subtle px-4 py-2">
          <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('payment.methods.stripe') }}
          </p>
        </div>
        <div class="p-4">
          <div ref="stripeMount" class="min-h-[200px]"></div>
          <p v-if="error" class="mt-3 text-xs text-danger" aria-live="polite">{{ error }}</p>
          <!--
            `.btn-stripe` keeps #635BFF. The provider's colour on the confirm
            button is how the user knows which processor is about to be charged;
            it is the sanctioned exception to the single-accent rule, minus the
            gradient and the press-scale it used to carry.
          -->
          <button
            class="btn btn-stripe mt-4 w-full"
            :disabled="submitting || !ready"
            :aria-busy="submitting ? 'true' : undefined"
            @click="handlePay"
          >
            <span v-if="submitting" class="spinner h-3.5 w-3.5" aria-hidden="true" />
            {{ t('payment.stripePay') }}
          </button>
        </div>
      </section>

      <Button variant="outline" size="md" block :loading="cancelling" @click="handleCancel">
        {{ t('payment.qr.cancelOrder') }}
      </Button>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { paymentAPI } from '@/api/payment'
import { useAppStore } from '@/stores'
import { useTheme } from '@/composables/useTheme'
import { getPaymentPopupFeatures } from '@/components/payment/providerConfig'
import { currencySymbol, paymentCurrencyFractionDigits } from '@/components/payment/currency'
import { buildStripeAppearance } from '@/components/payment/stripeAppearance'
import type { Stripe, StripeElements } from '@stripe/stripe-js'
// Direct paths, never the `components/common` barrel — it pulls `createI18n`
// into the graph and breaks partial `vue-i18n` factory mocks.
import Button from '@/components/common/Button.vue'
import NumCell from '@/components/common/NumCell.vue'

// Stripe payment methods that open a popup (redirect or QR code)
const POPUP_METHODS = new Set(['alipay', 'wechat_pay'])

const props = defineProps<{
  orderId: number
  amount: number
  clientSecret: string
  orderType?: 'balance' | 'subscription'
  publishableKey: string
  payAmount: number
  currency?: string
}>()

const emit = defineEmits<{ success: []; done: []; back: []; redirect: [orderId: number, payUrl: string] }>()

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const { isDark } = useTheme()

const stripeMount = ref<HTMLElement | null>(null)
const loading = ref(true)
const initError = ref('')
const error = ref('')
const submitting = ref(false)
const cancelling = ref(false)
const success = ref(false)
const ready = ref(false)
const selectedType = ref('')
const creditedAmountSymbol = currencySymbol('USD')
const paymentAmountSymbol = computed(() => currencySymbol(props.currency))
const paymentPrecision = computed(() => paymentCurrencyFractionDigits(props.currency))

let stripeInstance: Stripe | null = null
let elementsInstance: StripeElements | null = null

/**
 * Appearance is resolved once, when `elements()` is called. Without this watch,
 * toggling the theme left Stripe's iframe painted for the old one — a white
 * card sitting in the middle of a near-black page, which is the single most
 * obvious "this widget is not part of the app" artefact there is.
 */
watch(isDark, (dark) => {
  elementsInstance?.update({ appearance: buildStripeAppearance(dark) })
})

onMounted(async () => {
  try {
    const { loadStripe } = await import('@stripe/stripe-js/pure')
    const stripe = await loadStripe(props.publishableKey)
    if (!stripe) { initError.value = t('payment.stripeLoadFailed'); return }

    stripeInstance = stripe
    loading.value = false
    await nextTick()
    if (!stripeMount.value) return

    const elements = stripe.elements({
      clientSecret: props.clientSecret,
      appearance: buildStripeAppearance(isDark.value),
    })
    elementsInstance = elements
    const paymentElement = elements.create('payment', {
      layout: 'tabs',
      paymentMethodOrder: ['alipay', 'wechat_pay', 'card', 'link'],
    } as Record<string, unknown>)
    paymentElement.mount(stripeMount.value)
    paymentElement.on('ready', () => { ready.value = true })
    paymentElement.on('change', (event: { value: { type: string } }) => {
      selectedType.value = event.value.type
    })
  } catch (err: unknown) {
    initError.value = extractI18nErrorMessage(err, t, 'payment.errors', t('payment.stripeLoadFailed'))
  } finally {
    loading.value = false
  }
})

async function handlePay() {
  if (!stripeInstance || !elementsInstance || submitting.value) return

  // Alipay / WeChat Pay: open popup for redirect or QR display
  if (POPUP_METHODS.has(selectedType.value)) {
    const popupUrl = router.resolve({
      path: '/payment/stripe-popup',
      query: {
        order_id: String(props.orderId),
        method: selectedType.value,
        amount: String(props.payAmount),
        /*
         * The popup used to receive an amount with no currency and hardcoded a
         * `¥` next to it, so a Stripe order settled in USD or HKD showed the
         * user a yuan sign on the screen that confirms what they are paying.
         * The param is optional on the reading side and falls back to CNY, so
         * any popup URL already in flight renders exactly as it did before.
         */
        currency: props.currency || undefined,
      },
    }).href
    const popup = window.open(popupUrl, 'paymentPopup', getPaymentPopupFeatures())

    const onReady = (event: MessageEvent) => {
      if (event.source !== popup || event.data?.type !== 'STRIPE_POPUP_READY') return
      window.removeEventListener('message', onReady)
      popup?.postMessage({
        type: 'STRIPE_POPUP_INIT',
        clientSecret: props.clientSecret,
        publishableKey: props.publishableKey,
      }, window.location.origin)
    }
    window.addEventListener('message', onReady)

    emit('redirect', props.orderId, popupUrl)
    return
  }

  // Card / Link: confirm inline
  submitting.value = true
  error.value = ''
  try {
    const { error: stripeError } = await stripeInstance.confirmPayment({
      elements: elementsInstance,
      confirmParams: {
        return_url: window.location.origin + '/payment/result?order_id=' + props.orderId + '&status=success',
      },
      redirect: 'if_required',
    })
    if (stripeError) {
      error.value = stripeError.message || t('payment.result.failed')
    } else {
      success.value = true
      emit('success')
    }
  } catch (err: unknown) {
    error.value = extractI18nErrorMessage(err, t, 'payment.errors', t('payment.result.failed'))
  } finally {
    submitting.value = false
  }
}

async function handleCancel() {
  if (!props.orderId || cancelling.value) return
  cancelling.value = true
  try {
    await paymentAPI.cancelOrder(props.orderId)
    emit('back')
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    cancelling.value = false
  }
}
</script>
