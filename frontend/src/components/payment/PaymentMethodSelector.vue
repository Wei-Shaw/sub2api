<template>
  <div>
    <label class="input-label">
      {{ t('payment.paymentMethod') }}
    </label>
    <!-- Ruled rows: one line per payment method -->
    <div
      data-testid="payment-method-grid"
      class="divide-y divide-border border border-border"
    >
      <button
        v-for="method in sortedMethods"
        :key="method.type"
        type="button"
        :title="methodLabel(method)"
        :disabled="!method.available"
        :class="[
          'relative flex min-h-[52px] w-full min-w-0 items-center gap-3 px-3 py-2 text-left transition-colors',
          !method.available
            ? 'cursor-not-allowed bg-surface-sunken text-fg-subtle opacity-60'
            : selected === method.type
              ? 'bg-accent-weak text-accent-strong ring-1 ring-inset ring-accent'
              : 'bg-surface text-fg hover:bg-accent-weak/50',
        ]"
        @click="method.available && emit('select', method.type)"
      >
        <span
          aria-hidden="true"
          :class="[
            'flex h-4 w-4 shrink-0 items-center justify-center border-2',
            selected === method.type && method.available ? 'border-accent' : 'border-border-strong',
          ]"
        >
          <span v-if="selected === method.type && method.available" class="h-2 w-2 bg-accent" />
        </span>
        <img :src="methodIcon(method.type)" :alt="methodLabel(method)" class="h-6 w-6 shrink-0 object-contain" />
        <span data-testid="payment-method-label" class="block min-w-0 flex-1 truncate text-body font-semibold">
          {{ methodLabel(method) }}
        </span>
        <span
          v-if="method.fee_rate > 0"
          class="shrink-0 text-meta tabular-nums text-fg-muted"
        >
          {{ t('payment.fee') }} {{ method.fee_rate }}%
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { GPMPAY_BANK_TRANSFER, METHOD_ORDER, SEPAY_BANK_TRANSFER } from './providerConfig'
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

// SePay ships no per-method marks, so every method uses the neutral payment
// glyph rather than a borrowed brand icon.
const METHOD_ICONS: Record<string, string> = {
  [SEPAY_BANK_TRANSFER]: paymentIcon,
  [GPMPAY_BANK_TRANSFER]: paymentIcon,
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
