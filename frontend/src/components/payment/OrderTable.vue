<template>
  <DataTable :columns="columns" :data="orders" :loading="loading">
    <template #cell-id="{ value }">
      <span class="font-mono text-meta text-fg-muted">#{{ value }}</span>
    </template>
    <template #cell-out_trade_no="{ value }">
      <span class="font-mono text-meta text-fg">{{ value }}</span>
    </template>
    <template v-if="showUser" #cell-user_email="{ value, row }">
      <div class="text-body">
        <span class="text-fg">{{ value || row.user_name || '#' + row.user_id }}</span>
        <span v-if="row.user_notes" class="ml-1 text-meta text-fg-subtle">({{ row.user_notes }})</span>
      </div>
    </template>
    <template #cell-pay_amount="{ value, row }">
      <div class="text-right tabular-nums">
        <span class="font-semibold text-fg">{{ formatOrderAmount(value, row.currency) }}</span>
        <span v-if="row.fee_rate > 0" class="ml-1 text-meta text-fg-subtle" :title="t('payment.orders.fee') + ': ' + row.fee_rate + '%'">
          ({{ t('payment.orders.fee') }} {{ row.fee_rate }}%)
        </span>
        <div v-if="row.amount !== row.pay_amount || (row.bonus_amount ?? 0) > 0" class="text-meta text-fg-muted">
          {{ t('payment.orders.creditedAmount') }}: {{ formatCreditedAmount(row.amount) }}
          <span v-if="(row.bonus_amount ?? 0) > 0" class="ml-1 text-accent-strong">
            ({{ t('payment.orders.bonusIncluded', { amount: formatCreditedAmount(row.bonus_amount ?? 0) }) }})
          </span>
        </div>
      </div>
    </template>
    <template #cell-payment_type="{ value }">
      <span class="text-fg">{{ t('payment.methods.' + value, value) }}</span>
    </template>
    <template #cell-status="{ value }">
      <OrderStatusBadge :status="value" />
    </template>
    <template #cell-created_at="{ value }">
      <span class="text-meta tabular-nums text-fg-muted">{{ formatDate(value) }}</span>
    </template>
    <template #cell-actions="{ row }">
      <slot name="actions" :row="row" />
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import type { Column } from '@/components/common/types'
import DataTable from '@/components/common/DataTable.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { formatPaymentAmount } from '@/components/payment/currency'

const i18n = useI18n()
const { t } = i18n
// i18n.locale 在部分测试替身里不存在；格式化不该因为拿不到语言而炸掉整块渲染。
const localeCode = computed(() => String(i18n.locale?.value ?? 'en'))

const props = defineProps<{
  orders: PaymentOrder[]
  loading: boolean
  showUser?: boolean
}>()

function formatDate(dateStr: string) { return new Date(dateStr).toLocaleString() }

// 支付金额按订单币种格式化：VND 等零小数币种不能出现 .00。
function formatOrderAmount(value: number, currency?: string | null): string {
  return formatPaymentAmount(value, currency, localeCode.value)
}

// 入账余额始终是 USD，与网关币种无关。
function formatCreditedAmount(value: number): string {
  return formatPaymentAmount(value, 'USD', localeCode.value)
}

const columns = computed((): Column[] => {
  const cols: Column[] = [
    { key: 'id', label: t('payment.orders.orderId') },
    { key: 'out_trade_no', label: t('payment.orders.orderNo') },
  ]
  if (props.showUser) {
    cols.push({ key: 'user_email', label: t('payment.admin.colUser') })
  }
  cols.push(
    { key: 'pay_amount', label: t('payment.orders.payAmount'), class: 'text-right' },
    { key: 'payment_type', label: t('payment.orders.paymentMethod') },
    { key: 'status', label: t('payment.orders.status') },
    { key: 'created_at', label: t('payment.orders.createdAt') },
    { key: 'actions', label: t('common.actions') },
  )
  return cols
})
</script>
