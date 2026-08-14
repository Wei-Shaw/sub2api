<template>
  <!--
    The QR code is the fast path, not the only one: a payer on a desktop, or on
    a banking app whose scanner refuses a screen, has to be able to type the
    transfer in by hand. Every field the bank form asks for is here, and each is
    one tap to copy — the transfer content especially, because SePay matches the
    payment by that string alone and a mistyped order code lands as an
    unmatched credit.
  -->
  <section v-if="rows.length" class="rounded border border-line bg-surface p-5">
    <h3 class="text-sm font-medium text-ink">{{ t('payment.qr.transfer.title') }}</h3>
    <p class="mt-1 text-xs text-ink-tertiary">{{ t('payment.qr.transfer.hint') }}</p>

    <dl class="mt-4 divide-y divide-line-subtle border-y border-line-subtle text-xs">
      <div
        v-for="row in rows"
        :key="row.key"
        class="flex items-baseline justify-between gap-3 py-2"
      >
        <dt class="shrink-0 text-ink-tertiary">{{ row.label }}</dt>
        <dd class="flex min-w-0 items-baseline justify-end gap-2">
          <span
            class="min-w-0 break-all text-right font-mono slashed-zero"
            :class="row.emphasis ? 'font-semibold text-ink' : 'text-ink'"
            :data-testid="`transfer-${row.key}`"
          >{{ row.display }}</span>
          <Button
            size="xs"
            variant="ghost"
            class="shrink-0 self-center"
            :aria-label="t('common.copy') + ' ' + row.label"
            :data-testid="`copy-transfer-${row.key}`"
            @click="copyRow(row)"
          >
            <template #icon>
              <Icon :name="copiedKey === row.key ? 'check' : 'copy'" size="xs" />
            </template>
          </Button>
        </dd>
      </div>
    </dl>

    <!--
      The copy result is a toast, which a screen reader in a form context does
      not necessarily get. This is the redundant channel for it.
    -->
    <p class="sr-only" role="status">{{ copiedKey ? t('common.copied') : '' }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
// Direct paths, never the `components/common` barrel: it drags `createI18n`
// into the graph and breaks specs that mock `vue-i18n` with a partial factory.
import Button from '@/components/common/Button.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import {
  formatPaymentAmount,
  paymentCurrencyFractionDigits,
} from '@/components/payment/currency'
import type { BankTransferInfo } from '@/types/payment'

const props = defineProps<{
  transfer?: BankTransferInfo | null
  /** Gateway currency of the order; the transfer amount is denominated in it. */
  currency?: string
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

interface TransferRow {
  key: string
  label: string
  /** What the payer reads. */
  display: string
  /** What lands on the clipboard — never the formatted form. */
  copyValue: string
  emphasis?: boolean
}

const copiedKey = ref('')

/**
 * The provider hands the amount over as a minor-unit integer string. Dong is a
 * zero-decimal currency so the two coincide today, but dividing by the
 * currency's own scale keeps this honest if a second bank-transfer provider
 * ever settles in something else.
 */
const amountRow = computed<TransferRow | null>(() => {
  const raw = (props.transfer?.amount || '').trim()
  if (!raw) return null
  const minor = Number(raw)
  if (!Number.isFinite(minor)) return null
  const scale = 10 ** paymentCurrencyFractionDigits(props.currency)
  const major = minor / scale
  return {
    key: 'amount',
    label: t('payment.qr.transfer.amount'),
    display: formatPaymentAmount(major, props.currency),
    // Bank forms reject grouping separators and currency signs.
    copyValue: String(major),
    emphasis: true,
  }
})

const rows = computed<TransferRow[]>(() => {
  const transfer = props.transfer
  if (!transfer) return []

  const out: TransferRow[] = []
  const push = (key: string, label: string, value: string | undefined, emphasis = false) => {
    const trimmed = (value || '').trim()
    if (!trimmed) return
    out.push({ key, label, display: trimmed, copyValue: trimmed, emphasis })
  }

  push('bank', t('payment.qr.transfer.bank'), transfer.bank_code || transfer.bank_bin)
  push('account', t('payment.qr.transfer.accountNumber'), transfer.account_number, true)
  push('name', t('payment.qr.transfer.accountName'), transfer.account_name)

  const amount = amountRow.value
  if (amount) out.push(amount)

  push('content', t('payment.qr.transfer.content'), transfer.content, true)

  return out
})

async function copyRow(row: TransferRow): Promise<void> {
  if (await copyToClipboard(row.copyValue)) {
    copiedKey.value = row.key
    setTimeout(() => {
      if (copiedKey.value === row.key) copiedKey.value = ''
    }, 2000)
  }
}
</script>
