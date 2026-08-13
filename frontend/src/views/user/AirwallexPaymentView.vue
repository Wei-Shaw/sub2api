<template>
  <AppLayout>
    <div class="mx-auto max-w-md space-y-6 py-8">
      <div v-if="errorMessage" class="rounded border border-line bg-surface p-5">
        <!--
          The failure is TEXT, in the semantic tone, inside a live region. It used
          to be a 64px circle in `bg-red-100` holding a 32px glyph, with the
          actual reason in small gray type underneath — the decoration was the
          loudest element and the information was the quietest.
        -->
        <div aria-live="polite">
          <p class="text-2xs font-medium uppercase tracking-[0.08em] text-danger">
            {{ t('payment.result.failed') }}
          </p>
          <h1 class="mt-2 text-lg font-semibold text-ink">{{ t('payment.airwallexLoadFailed') }}</h1>
          <p class="mt-1 text-sm text-ink-tertiary">{{ errorMessage }}</p>
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

      <!--
        THE DECLARED BOUNDARY.

        Airwallex exposes far less theming surface than Stripe: this flow hands
        off to an Airwallex-hosted checkout, and once `redirectToCheckout` fires,
        every pixel belongs to them. There is no Appearance API to push tokens
        through, so a seam is unavoidable.

        The response is to PUBLISH the seam rather than to half-hide it: a
        hairline panel with the provider's name on it, and our own chrome
        stopping visibly at its edge. A boundary that looks deliberate reads as
        "you are being handed to the payment provider now"; a near-match that is
        half a shade and two pixels of radius off reads as a rendering bug, and
        on a payment page a rendering bug reads as "is this site broken, or is
        this a phishing page?".
      -->
      <section v-else class="rounded border border-line bg-surface">
        <div class="flex items-center justify-between gap-3 border-b border-line-subtle px-4 py-2">
          <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('payment.methods.airwallex') }}
          </p>
          <span class="text-2xs text-ink-disabled">{{ t('payment.airwallexPay') }}</span>
        </div>
        <div class="p-5" aria-live="polite" aria-busy="true">
          <p class="flex items-center gap-2 text-2xs font-medium uppercase tracking-[0.08em] text-ink-tertiary">
            <span class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
            {{ t('common.processing') }}
          </p>
          <p class="mt-2 text-sm text-ink-secondary">{{ t('payment.qr.payInNewWindowHint') }}</p>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
// Direct path, never the `components/common` barrel — it pulls `createI18n`
// into the graph and breaks partial `vue-i18n` factory mocks.
import Button from '@/components/common/Button.vue'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()

/**
 * `loading` is gone: the template has exactly two states, "handing off" and
 * "failed", and `loading` only ever distinguished two spinners from each other.
 * The SDK either redirects the tab away or sets `errorMessage`.
 */
const errorMessage = ref('')

function queryString(key: string): string {
  const value = route.query[key]
  if (Array.isArray(value)) return value[0] || ''
  return typeof value === 'string' ? value : ''
}

function buildSuccessUrl(snapshot: PaymentRecoverySnapshot): string {
  const url = new URL('/payment/result', window.location.origin)
  const orderId = queryString('order_id')
  const outTradeNo = queryString('out_trade_no')
  const resumeToken = queryString('resume_token')

  if (orderId || snapshot.orderId > 0) url.searchParams.set('order_id', orderId || String(snapshot.orderId))
  if (outTradeNo || snapshot.outTradeNo) url.searchParams.set('out_trade_no', outTradeNo || snapshot.outTradeNo)
  if (resumeToken || snapshot.resumeToken) url.searchParams.set('resume_token', resumeToken || snapshot.resumeToken)
  return url.toString()
}

function restoreAirwallexSnapshot(): PaymentRecoverySnapshot | null {
  if (typeof window === 'undefined') {
    return null
  }

  const orderId = Number(queryString('order_id')) || 0
  const outTradeNo = queryString('out_trade_no')
  const resumeToken = queryString('resume_token')
  const snapshot = readPaymentRecoverySnapshot(
    window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
    resumeToken ? { resumeToken } : {},
  )

  if (!snapshot || snapshot.paymentType !== 'airwallex') {
    return null
  }
  if (orderId > 0 && snapshot.orderId !== orderId) {
    return null
  }
  if (outTradeNo && snapshot.outTradeNo !== outTradeNo) {
    return null
  }
  if (!snapshot.intentId || !snapshot.clientSecret) {
    return null
  }
  return snapshot
}

onMounted(async () => {
  const snapshot = restoreAirwallexSnapshot()
  const checkoutLocale = locale.value.toLowerCase().startsWith('zh') ? 'zh' : 'en'

  if (!snapshot) {
    errorMessage.value = t('payment.airwallexMissingParams')
    return
  }

  try {
    const airwallex = await import('@airwallex/components-sdk')
    const result = await airwallex.init({
      env: snapshot.paymentEnv === 'prod' ? 'prod' : 'demo',
      enabledElements: ['payments'],
      locale: checkoutLocale,
    })

    const checkoutOptions = {
      intent_id: snapshot.intentId,
      client_secret: snapshot.clientSecret,
      currency: snapshot.currency || 'CNY',
      country_code: snapshot.countryCode || 'CN',
      successUrl: buildSuccessUrl(snapshot),
    }
    if (!result.payments) {
      throw new Error(t('payment.airwallexLoadFailed'))
    }
    const redirectResult = result.payments.redirectToCheckout(checkoutOptions)

    if (typeof redirectResult === 'string' && redirectResult) {
      window.location.assign(redirectResult)
    }
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error && err.message
      ? err.message
      : t('payment.airwallexLoadFailed')
  }
})
</script>
