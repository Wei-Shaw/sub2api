<template>
  <div>
    <!--
      A `radiogroup`, not a row of buttons. The old markup carried the selection
      entirely in colour — a per-brand border plus a pastel ground — so a screen
      reader heard N independent buttons and was never told which method was
      active, and an unavailable method was communicated by `opacity-50` alone.
    -->
    <p :id="labelId" class="mb-2 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
      {{ t('payment.paymentMethod') }}
    </p>
    <div
      data-testid="payment-method-grid"
      role="radiogroup"
      :aria-labelledby="labelId"
      class="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4"
    >
      <!--
        Selection is the accent, uniformly. The old selected state used the
        provider's own hue (#02A9F1 for Alipay, #09BB07 for WeChat, #676BE5 for
        Stripe, #FF6B3D for Airwallex) over a pastel ground, so the page carried
        four competing accents and there was no way to tell "this one is chosen"
        from "this one is Alipay". The brand still appears — in the mark, which
        is where a brand belongs.
      -->
      <button
        v-for="method in sortedMethods"
        :key="method.type"
        type="button"
        role="radio"
        :aria-checked="selected === method.type"
        :title="methodLabel(method)"
        :disabled="!method.available"
        :class="[
          'flex h-11 min-w-0 items-center gap-2 rounded border px-2.5 text-left',
          'transition-colors duration-fast ease-out',
          'disabled:cursor-not-allowed',
          !method.available
            ? 'border-line-subtle bg-surface-sunken text-ink-disabled'
            : selected === method.type
              ? 'border-accent bg-accent-tint text-ink'
              : 'border-line bg-surface text-ink hover:border-line-strong hover:bg-surface-hover',
        ]"
        @click="method.available && emit('select', method.type)"
      >
        <img
          :src="methodIcon(method.type)"
          alt=""
          aria-hidden="true"
          class="h-5 w-5 shrink-0 object-contain"
        />
        <span class="flex min-w-0 flex-col">
          <span data-testid="payment-method-label" class="block w-full truncate text-xs font-medium">
            {{ methodLabel(method) }}
          </span>
          <!-- The fee is a number, so it gets tabular figures like every other. -->
          <span
            v-if="method.fee_rate > 0"
            class="block truncate text-2xs text-ink-tertiary"
          >
            {{ t('payment.fee') }}
            <span class="font-mono tabular-nums">{{ method.fee_rate }}%</span>
          </span>
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import { METHOD_ORDER } from './providerConfig'
import paymentIcon from '@/assets/icons/payment.svg'

export interface PaymentMethodOption {
  type: string
  display_name?: string
  fee_rate: number
  available: boolean
}

const props = defineProps<{
  methods: PaymentMethodOption[]
  selected: string
}>()

const emit = defineEmits<{
  select: [type: string]
}>()

const { t } = useI18n()

const labelId = `payment-method-${useId()}`

// Neither provider ships a mark we are licensed to redraw here, so both use
// the generic payment glyph until brand assets are added.
const METHOD_ICONS: Record<string, string> = {
  sepay: paymentIcon,
  nowpayments: paymentIcon,
}

const sortedMethods = computed(() => {
  const order: readonly string[] = METHOD_ORDER
  return [...props.methods].sort((a, b) => {
    const ai = order.indexOf(a.type)
    const bi = order.indexOf(b.type)
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
  })
})

function methodIcon(type: string): string {
  return METHOD_ICONS[type] || paymentIcon
}

function methodLabel(method: PaymentMethodOption): string {
  return method.display_name || t(`payment.methods.${method.type}`, method.type)
}
</script>
