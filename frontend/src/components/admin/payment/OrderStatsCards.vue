<template>
  <div class="space-y-4">
    <!-- Revenue by Currency -->
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      <div v-for="revenue in stats.revenue_by_currency" :key="revenue.currency" class="card p-4">
        <div class="flex items-center gap-3">
          <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30">
            <Icon name="dollar" size="md" class="text-green-600 dark:text-green-400" :stroke-width="2" />
          </div>
          <div class="min-w-0">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('payment.admin.revenue') }} ({{ revenue.currency }})
            </p>
            <p class="text-xl font-bold text-gray-900 dark:text-white">
              {{ formatMoney(revenue.currency, revenue.total_amount) }}
            </p>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('payment.admin.today') }}: {{ formatMoney(revenue.currency, revenue.today_amount) }}
              · {{ revenue.today_count }}/{{ revenue.total_count }} {{ t('payment.admin.orders') }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- Summary -->
    <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <div class="card p-4">
        <div class="flex items-center gap-3">
          <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30">
            <Icon name="chart" size="md" class="text-purple-600 dark:text-purple-400" :stroke-width="2" />
          </div>
          <div>
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.todayOrders') }}</p>
            <p class="text-xl font-bold text-gray-900 dark:text-white">{{ stats.today_count }}</p>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <div class="flex items-center gap-3">
          <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
            <Icon name="creditCard" size="md" class="text-blue-600 dark:text-blue-400" :stroke-width="2" />
          </div>
          <div>
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.totalOrders') }}</p>
            <p class="text-xl font-bold text-gray-900 dark:text-white">{{ stats.total_count }}</p>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <div class="flex items-center gap-3">
          <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30">
            <Icon name="chart" size="md" class="text-amber-600 dark:text-amber-400" :stroke-width="2" />
          </div>
          <div>
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.pendingOrders') }}</p>
            <p class="text-xl font-bold text-gray-900 dark:text-white">{{ stats.pending_orders }}</p>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <div class="flex items-center gap-3">
          <div class="rounded-lg bg-teal-100 p-2 dark:bg-teal-900/30">
            <Icon name="chart" size="md" class="text-teal-600 dark:text-teal-400" :stroke-width="2" />
          </div>
          <div>
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.avgAmount') }}</p>
            <p
              v-for="[currency, amount] in sortedAmounts(stats.avg_amount)"
              :key="currency"
              class="text-xl font-bold text-gray-900 dark:text-white"
            >
              {{ formatMoney(currency, amount) }}
            </p>
          </div>
        </div>
      </div>
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
