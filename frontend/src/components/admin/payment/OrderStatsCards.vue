<template>
  <div class="kpi-grid lg:grid-cols-4">
    <!-- Today Revenue -->
    <div class="kpi-cell">
      <div class="kpi-head">
        <p class="kpi-label">{{ t('payment.admin.todayRevenue') }}</p>
        <Icon name="dollar" size="sm" class="kpi-icon" />
      </div>
      <p v-for="[currency, amount] in sortedAmounts(stats.today_amount)" :key="currency" class="kpi-value">
        {{ formatMoney(currency, amount) }}
      </p>
      <p class="kpi-meta">
        {{ stats.today_count }} {{ t('payment.admin.orders') }}
      </p>
    </div>

    <!-- Total Revenue -->
    <div class="kpi-cell">
      <div class="kpi-head">
        <p class="kpi-label">{{ t('payment.admin.totalRevenue') }}</p>
        <Icon name="creditCard" size="sm" class="kpi-icon" />
      </div>
      <p v-for="[currency, amount] in sortedAmounts(stats.total_amount)" :key="currency" class="kpi-value">
        {{ formatMoney(currency, amount) }}
      </p>
      <p class="kpi-meta">
        {{ stats.total_count }} {{ t('payment.admin.orders') }}
      </p>
    </div>

    <!-- Today Orders -->
    <div class="kpi-cell">
      <div class="kpi-head">
        <p class="kpi-label">{{ t('payment.admin.todayOrders') }}</p>
        <Icon name="chart" size="sm" class="kpi-icon" />
      </div>
      <p class="kpi-value">{{ stats.today_count }}</p>
    </div>

    <!-- Average Amount -->
    <div class="kpi-cell">
      <div class="kpi-head">
        <p class="kpi-label">{{ t('payment.admin.avgAmount') }}</p>
        <Icon name="chart" size="sm" class="kpi-icon" />
      </div>
      <p v-for="[currency, amount] in sortedAmounts(stats.avg_amount)" :key="currency" class="kpi-value">
        {{ formatMoney(currency, amount) }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { CurrencyAmounts, DashboardStats } from '@/types/payment'

const { t } = useI18n()

defineProps<{
  stats: DashboardStats
}>()

function sortedAmounts(amounts: CurrencyAmounts): [string, number][] {
  return Object.entries(amounts).sort(([left], [right]) => left.localeCompare(right))
}

function formatMoney(currency: string, amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(amount)
}
</script>
