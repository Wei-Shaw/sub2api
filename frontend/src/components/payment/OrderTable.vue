<template>
  <!--
    A hand-rolled `.table` rather than `DataTable`.

    `DataTable` is the legacy generic grid: 56px rows, `py-4` cells, a zebra-ish
    gray header and a card-per-row mobile mode. None of that is reachable from
    the outside, and none of it matches the data-surface rules this table has to
    follow — 32px rows (44px below the desktop breakpoint), a 34px header on
    `--ds-surface-sunken` closed by a `--ds-line-strong` rule, a
    `--ds-line-subtle` separator on every row, hover that moves the ground and
    nothing else. `.table` in style.css already encodes all of that, and none of
    the grid's own features (sort, virtualization, selection) were ever used
    here: orders arrive server-paginated at 20 rows.
  -->
  <div class="overflow-x-auto rounded border border-line bg-surface">
    <table class="table min-w-[56rem]">
      <thead>
        <tr>
          <th scope="col" class="is-numeric">{{ t('payment.orders.orderId') }}</th>
          <th scope="col">{{ t('payment.orders.orderNo') }}</th>
          <th v-if="showUser" scope="col">{{ t('payment.admin.colUser') }}</th>
          <th scope="col" class="is-numeric">{{ t('payment.orders.payAmount') }}</th>
          <th scope="col">{{ t('payment.orders.paymentMethod') }}</th>
          <th scope="col">{{ t('payment.orders.status') }}</th>
          <th scope="col" class="is-numeric">{{ t('payment.orders.createdAt') }}</th>
          <th scope="col" class="text-right">{{ t('common.actions') }}</th>
        </tr>
      </thead>

      <tbody>
        <!-- Flat dim bars, no shimmer sweep and no animation on a table row. -->
        <template v-if="loading">
          <tr v-for="i in 5" :key="`skeleton-${i}`">
            <td v-for="column in columnCount" :key="column">
              <div class="skeleton h-3 w-full"></div>
            </td>
          </tr>
        </template>

        <tr v-else-if="orders.length === 0">
          <td :colspan="columnCount" class="text-center text-xs text-ink-tertiary">
            {{ t('payment.orders.empty') }}
          </td>
        </tr>

        <template v-else>
          <tr v-for="row in orders" :key="row.id">
            <td class="is-numeric text-ink">#{{ row.id }}</td>
            <td class="whitespace-nowrap font-mono text-xs text-ink">{{ row.out_trade_no }}</td>
            <td v-if="showUser" class="min-w-0">
              <span class="text-ink">{{ row.user_email || row.user_name || '#' + row.user_id }}</span>
              <span v-if="row.user_notes" class="ml-1 text-2xs text-ink-tertiary">
                ({{ row.user_notes }})
              </span>
            </td>
            <td class="is-numeric">
              <span class="inline-flex items-baseline justify-end gap-0.5">
                <span class="text-2xs text-ink-tertiary">{{ paymentAmountSymbol(row) }}</span>
                <NumCell :value="row.pay_amount" :precision="2" />
              </span>
              <!--
                The fee and the credited amount are captions on the paid amount,
                not columns of their own — they exist for at most a fraction of
                the rows.
              -->
              <span
                v-if="row.fee_rate > 0"
                class="ml-1 text-2xs text-ink-tertiary"
                :title="t('payment.orders.fee') + ': ' + row.fee_rate + '%'"
              >({{ t('payment.orders.fee') }} {{ row.fee_rate }}%)</span>
              <div v-if="row.amount !== row.pay_amount" class="text-2xs text-ink-tertiary">
                {{ t('payment.orders.creditedAmount') }}:
                <span class="inline-flex items-baseline gap-0.5">
                  <span>{{ creditedAmountSymbol }}</span>
                  <NumCell :value="row.amount" :precision="2" />
                </span>
              </div>
            </td>
            <td class="whitespace-nowrap">{{ t('payment.methods.' + row.payment_type, row.payment_type) }}</td>
            <td class="whitespace-nowrap">
              <!--
                Dot plus a written label. Colour is never the only channel, and the
                row itself is never tinted: a table where every row carries a
                status ground has no signal left for the rows that matter.
              -->
              <StatusDot
                :tone="statusTone(row.status)"
                :label="statusLabel(row.status)"
                :muted="statusTone(row.status) === 'neutral'"
              />
            </td>
            <td class="is-numeric whitespace-nowrap">{{ formatDate(row.created_at) }}</td>
            <td class="text-right">
              <slot name="actions" :row="row" />
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import NumCell from '@/components/common/NumCell.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import type { Tone } from '@/components/common/primitives'
import { currencySymbol } from '@/components/payment/currency'
import type { OrderStatus, PaymentOrder } from '@/types/payment'

const { t } = useI18n()

/**
 * The admin listing joins the buyer onto the order. These were reached through
 * `DataTable`'s `any` row before, which is why they were never declared.
 */
type OrderRow = PaymentOrder & {
  user_email?: string
  user_name?: string
  user_notes?: string
}

const props = defineProps<{
  orders: OrderRow[]
  loading: boolean
  showUser?: boolean
}>()

function formatDate(dateStr: string) { return new Date(dateStr).toLocaleString() }

const creditedAmountSymbol = currencySymbol('USD')

function paymentAmountSymbol(order: PaymentOrder): string {
  return currencySymbol(order.currency)
}

const columnCount = computed(() => (props.showUser ? 8 : 7))

/**
 * Status tones.
 *
 * `accent` is absent on purpose: it means interactive or selected in this
 * system and must never signal state. Everything that is simply *over* —
 * expired, cancelled, refunded — stays neutral and muted, so the handful of
 * rows that actually need attention are the only coloured things on screen.
 */
const STATUS_TONE: Record<OrderStatus, Tone> = {
  PENDING: 'warn',
  PAID: 'info',
  RECHARGING: 'info',
  COMPLETED: 'success',
  EXPIRED: 'neutral',
  CANCELLED: 'neutral',
  FAILED: 'danger',
}

const STATUS_KEY: Record<OrderStatus, string> = {
  PENDING: 'payment.status.pending',
  PAID: 'payment.status.paid',
  RECHARGING: 'payment.status.recharging',
  COMPLETED: 'payment.status.completed',
  EXPIRED: 'payment.status.expired',
  CANCELLED: 'payment.status.cancelled',
  FAILED: 'payment.status.failed',
}

function statusTone(status: OrderStatus): Tone {
  return STATUS_TONE[status] ?? 'neutral'
}

/** An unmapped status prints its raw value: more diagnostic than "unknown". */
function statusLabel(status: OrderStatus): string {
  const key = STATUS_KEY[status]
  return key ? t(key) : String(status)
}
</script>
