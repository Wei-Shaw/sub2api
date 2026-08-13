<template>
  <component :is="isPopup ? 'div' : AppLayout" :class="isPopup ? 'min-h-screen bg-canvas' : ''">
    <div class="mx-auto max-w-md space-y-4 py-8" :class="isPopup ? 'px-4' : ''">
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
          <h1 class="mt-2 text-lg font-semibold text-ink">{{ t('payment.stripeLoadFailed') }}</h1>
          <p class="mt-1 text-sm text-ink-tertiary">{{ initError }}</p>
        </div>
        <Button
          class="mt-5"
          tone="accent"
          variant="solid"
          size="md"
          @click="router.push('/purchase')"
        >
          {{ t('payment.result.backToRecharge') }}
        </Button>
      </div>

      <template v-else>
        <!--
          The amount, as type.

          This was a `bg-gradient-to-br from-[#635bff] to-[#4f46e5]` banner with
          30px white digits — Stripe's brand gradient painted across the top of
          OUR page. The provider's colour belongs on the provider's button and
          inside the provider's frame, not on the page header; here it just meant
          the loudest thing on screen was a rectangle. The number is now the
          loudest thing because it is the biggest.
        -->
        <section v-if="order" class="rounded border border-line bg-surface px-4 py-3">
          <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('payment.actualPay') }}
          </p>
          <p class="mt-1 flex items-baseline gap-1">
            <span class="text-sm text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
            <span class="font-mono text-2xl font-semibold tabular-nums slashed-zero text-ink">
              <NumCell :value="order.pay_amount" :precision="gatewayPrecision" />
            </span>
          </p>
        </section>

        <!-- WeChat QR, handed to us by Stripe as a data URL -->
        <template v-if="wechatQrUrl">
          <section class="rounded border border-line bg-surface p-5">
            <h1 class="text-sm font-medium text-ink">{{ t('payment.qr.scanWxpay') }}</h1>
            <p class="mt-1 text-xs text-ink-tertiary">{{ t('payment.qr.scanWxpayHint') }}</p>
            <div class="mt-5 flex justify-center">
              <!--
                The 30-line inline WeChat logo path this used to carry is gone;
                `QrFrame` overlays the shared `wxpay.svg` asset instead, and owns
                the reason the QR ground stays white in dark mode.
              -->
              <QrFrame tone="wxpay" :logo="wxpayIcon">
                <img :src="wechatQrUrl" alt="WeChat Pay QR" class="block h-52 w-52" />
              </QrFrame>
            </div>
          </section>
          <div class="rounded border border-line bg-surface-sunken px-4 py-3" aria-live="polite">
            <p class="text-xs text-ink-tertiary">{{ t('payment.qr.waitingPayment') }}</p>
          </div>
        </template>

        <!-- Alipay handoff -->
        <template v-else-if="redirecting">
          <section class="rounded border border-line bg-surface p-5" aria-live="polite" aria-busy="true">
            <p class="flex items-center gap-2 text-2xs font-medium uppercase tracking-[0.08em] text-ink-tertiary">
              <span class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
              {{ t('payment.methods.alipay') }}
            </p>
            <p class="mt-2 text-sm text-ink-secondary">{{ t('payment.qr.payInNewWindowHint') }}</p>
          </section>
        </template>

        <!-- Success -->
        <template v-else-if="stripeSuccess">
          <section class="rounded border border-line bg-surface p-5" aria-live="polite">
            <p class="text-2xs font-medium uppercase tracking-[0.08em] text-success">
              {{ t('payment.status.completed') }}
            </p>
            <h1 class="mt-2 text-lg font-semibold text-ink">{{ t('payment.result.success') }}</h1>
            <p class="mt-1 text-sm text-ink-tertiary">{{ t('payment.stripeSuccessProcessing') }}</p>
          </section>
        </template>

        <!-- Full Payment Element: no method was pre-selected -->
        <template v-else-if="showPaymentElement">
          <!--
            THE DECLARED BOUNDARY.

            `#stripe-payment-element` is a cross-origin iframe. Our stylesheet
            stops at its edge — the font inside falls back to `system-ui`, the
            wallet buttons are Stripe's — so the join can never be seamless.

            Framing and labelling the region is the honest answer. A visible
            hairline with the provider's name reads as "you are now filling in
            the payment processor's form", which is what a checkout page should
            say; a near-match that is half a shade off reads as a rendering bug,
            and on a payment page a rendering bug reads as "is this site
            broken, or is it a phishing page?". Colour, radius and text size do
            cross the boundary, via `stripeAppearance.ts`.
          -->
          <section class="rounded border border-line bg-surface">
            <div class="border-b border-line-subtle px-4 py-2">
              <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                {{ t('payment.methods.stripe') }}
              </p>
            </div>
            <div class="p-4">
              <div id="stripe-payment-element" class="min-h-[200px]"></div>
              <p v-if="stripeError" class="mt-3 text-xs text-danger" aria-live="polite">
                {{ stripeError }}
              </p>
              <!--
                `.btn-stripe` keeps #635BFF: the processor's colour on the
                confirm button is how the user knows who is about to charge
                them. It is the sanctioned exception to the single-accent rule,
                minus the gradient and press-scale it used to carry. The label
                does not change while submitting — the spinner sits beside it —
                because a button that changes width mid-click is the worst
                possible thing to put on a payment page.
              -->
              <button
                class="btn btn-stripe mt-4 w-full"
                :disabled="stripeSubmitting || !stripeReady"
                :aria-busy="stripeSubmitting ? 'true' : undefined"
                @click="handleGenericPay"
              >
                <span v-if="stripeSubmitting" class="spinner h-3.5 w-3.5" aria-hidden="true" />
                {{ t('payment.stripePay') }}
              </button>
            </div>
          </section>
          <Button variant="outline" size="md" block @click="router.push('/purchase')">
            {{ t('payment.result.backToRecharge') }}
          </Button>
        </template>

        <!-- Error outside the Payment Element -->
        <div v-if="stripeError && !showPaymentElement" class="rounded border border-line bg-surface p-5">
          <div aria-live="polite">
            <p class="text-2xs font-medium uppercase tracking-[0.08em] text-danger">
              {{ t('payment.result.failed') }}
            </p>
            <p class="mt-2 text-sm text-ink">{{ stripeError }}</p>
          </div>
          <Button class="mt-5" variant="outline" size="md" block @click="router.push('/purchase')">
            {{ t('payment.result.backToRecharge') }}
          </Button>
        </div>
      </template>
    </div>
  </component>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { usePaymentStore } from '@/stores/payment'
import { paymentAPI } from '@/api/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import { useTheme } from '@/composables/useTheme'
import {
  currencySymbol,
  normalizePaymentCurrency,
  paymentCurrencyFractionDigits,
} from '@/components/payment/currency'
import { PAYMENT_RECOVERY_STORAGE_KEY, readPaymentRecoverySnapshot } from '@/components/payment/paymentFlow'
import { buildStripeAppearance } from '@/components/payment/stripeAppearance'
import type { PaymentOrder } from '@/types/payment'
import type { Stripe, StripeElements } from '@stripe/stripe-js'
import AppLayout from '@/components/layout/AppLayout.vue'
// Direct paths, never the `components/common` barrel — it pulls `createI18n`
// into the graph and breaks partial `vue-i18n` factory mocks.
import Button from '@/components/common/Button.vue'
import NumCell from '@/components/common/NumCell.vue'
import QrFrame from '@/components/payment/QrFrame.vue'
import wxpayIcon from '@/assets/icons/wxpay.svg'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const paymentStore = usePaymentStore()
const { isDark } = useTheme()

// 弹窗模式：指定支付宝或微信方式时跳过 AppLayout
const isPopup = computed(() => !!route.query.method)

const loading = ref(true)
const initError = ref('')
const stripeError = ref('')
const stripeSubmitting = ref(false)
const stripeSuccess = ref(false)
const stripeReady = ref(false)
const order = ref<PaymentOrder | null>(null)
const currency = ref('CNY')
const wechatQrUrl = ref('')
const redirecting = ref(false)
const showPaymentElement = ref(false)

const gatewayCurrencySymbol = computed(() => currencySymbol(currency.value))
const gatewayPrecision = computed(() => paymentCurrencyFractionDigits(currency.value))

let stripeInstance: Stripe | null = null
let elementsInstance: StripeElements | null = null
let redirectTimer: ReturnType<typeof setTimeout> | null = null

/**
 * Appearance is resolved once, at `elements()` time. Without this, flipping the
 * theme left Stripe's iframe painted for the previous one — a white card in the
 * middle of a near-black page, which is the most obvious way to make an
 * embedded form look like it does not belong to the site it is embedded in.
 */
watch(isDark, (dark) => {
  elementsInstance?.update({ appearance: buildStripeAppearance(dark) })
})

onMounted(async () => {
  const orderId = Number(route.query.order_id)
  const clientSecret = String(route.query.client_secret || '')
  const method = String(route.query.method || '')
  const resumeToken = typeof route.query.resume_token === 'string' ? route.query.resume_token : undefined

  if (!orderId || !clientSecret) {
    loading.value = false
    initError.value = t('payment.stripeMissingParams')
    return
  }

  try {
    if (typeof window !== 'undefined') {
      const restored = readPaymentRecoverySnapshot(
        window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
        { resumeToken },
      )
      if (restored?.orderId === orderId) {
        currency.value = normalizePaymentCurrency(restored.currency)
      }
    }
    const res = await paymentAPI.getOrder(orderId)
    order.value = res.data
    if (res.data.currency) {
      currency.value = normalizePaymentCurrency(res.data.currency)
    }

    await paymentStore.fetchConfig()
    const publishableKey = paymentStore.config?.stripe_publishable_key
    if (!publishableKey) { initError.value = t('payment.stripeNotConfigured'); return }

    const { loadStripe } = await import('@stripe/stripe-js/pure')
    const stripe = await loadStripe(publishableKey)
    if (!stripe) { initError.value = t('payment.stripeLoadFailed'); return }

    stripeInstance = stripe
    loading.value = false

    // 指定方式直接确认，无需渲染完整 Payment Element
    if (method === 'alipay') {
      await confirmAlipay(stripe, clientSecret, orderId)
    } else if (method === 'wechat_pay') {
      await confirmWechatPay(stripe, clientSecret)
    } else {
      // 未指定方式时渲染完整 Payment Element
      showPaymentElement.value = true
      await nextTick()
      mountPaymentElement(stripe, clientSecret)
    }
  } catch (err: unknown) {
    initError.value = extractI18nErrorMessage(err, t, 'payment.errors', t('payment.stripeLoadFailed'))
  } finally {
    loading.value = false
  }
})

async function confirmAlipay(stripe: Stripe, clientSecret: string, orderId: number) {
  redirecting.value = true
  const returnUrl = window.location.origin + '/payment/result?order_id=' + orderId + '&status=success'
  const { error } = await stripe.confirmAlipayPayment(clientSecret, { return_url: returnUrl })
  if (error) {
    redirecting.value = false
    stripeError.value = error.message || t('payment.result.failed')
  }
  // 无错误时 Stripe 会自动跳转
}

async function confirmWechatPay(stripe: Stripe, clientSecret: string) {
  const { paymentIntent, error } = await (stripe as Stripe & {
    confirmWechatPayPayment: (cs: string, opts: Record<string, unknown>) => Promise<{ paymentIntent?: { status: string; next_action?: { wechat_pay_display_qr_code?: { image_data_url?: string } } }; error?: { message?: string } }>
  }).confirmWechatPayPayment(clientSecret, {
    payment_method_options: { wechat_pay: { client: isMobileDevice() ? 'mobile_web' : 'web' } },
  })

  if (error) {
    stripeError.value = error.message || t('payment.result.failed')
    return
  }

  // 从 next_action 中提取二维码
  const qrData = paymentIntent?.next_action?.wechat_pay_display_qr_code?.image_data_url
  if (qrData) {
    wechatQrUrl.value = qrData
    // 轮询支付完成状态
    startPolling()
  } else if (paymentIntent?.status === 'succeeded') {
    stripeSuccess.value = true
    scheduleClose()
  } else {
    stripeError.value = t('payment.result.failed')
  }
}

function mountPaymentElement(stripe: Stripe, clientSecret: string) {
  const elements = stripe.elements({
    clientSecret,
    /*
     * Was `{ theme, variables: { borderRadius: '8px' } }` — an 8px radius in a
     * system whose ceiling is 4px, so the payment form was visibly rounder than
     * every control around it, and every other value was Stripe's default.
     */
    appearance: buildStripeAppearance(isDark.value),
  })
  elementsInstance = elements
  const paymentElement = elements.create('payment', {
    layout: 'tabs',
    paymentMethodOrder: ['alipay', 'wechat_pay', 'card', 'link'],
  } as Record<string, unknown>)
  paymentElement.mount('#stripe-payment-element')
  paymentElement.on('ready', () => { stripeReady.value = true })
}

async function handleGenericPay() {
  if (!stripeInstance || !elementsInstance || stripeSubmitting.value) return
  stripeSubmitting.value = true
  stripeError.value = ''
  try {
    const { error } = await stripeInstance.confirmPayment({
      elements: elementsInstance,
      confirmParams: {
        return_url: window.location.origin + '/payment/result?order_id=' + route.query.order_id + '&status=success',
      },
      redirect: 'if_required',
    })
    if (error) {
      stripeError.value = error.message || t('payment.result.failed')
    } else {
      stripeSuccess.value = true
      scheduleClose()
    }
  } catch (err: unknown) {
    stripeError.value = extractI18nErrorMessage(err, t, 'payment.errors', t('payment.result.failed'))
  } finally {
    stripeSubmitting.value = false
  }
}

let pollTimer: ReturnType<typeof setInterval> | null = null

function startPolling() {
  const orderId = Number(route.query.order_id)
  if (!orderId) return
  pollTimer = setInterval(async () => {
    const o = await paymentStore.pollOrderStatus(orderId)
    if (!o) return
    if (o.status === 'COMPLETED' || o.status === 'PAID') {
      if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
      stripeSuccess.value = true
      wechatQrUrl.value = ''
      scheduleClose()
    }
  }, 3000)
}

function scheduleClose() {
  if (window.opener) {
    redirectTimer = setTimeout(() => { window.close() }, 2000)
  } else {
    redirectTimer = setTimeout(() => {
      router.push({ path: '/payment/result', query: { order_id: String(route.query.order_id || ''), status: 'success' } })
    }, 2000)
  }
}

onUnmounted(() => {
  if (redirectTimer) clearTimeout(redirectTimer)
  if (pollTimer) clearInterval(pollTimer)
})
</script>
