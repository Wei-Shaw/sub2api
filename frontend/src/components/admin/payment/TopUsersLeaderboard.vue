<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('payment.admin.topUsers') }}
    </h3>
    <div
      v-if="!hasUsers(props.users)"
      class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('payment.admin.noData') }}
    </div>
    <div v-else class="space-y-2">
      <div v-for="[currency, currencyUsers] in sortedUsers(props.users)" :key="currency" class="space-y-2">
        <p class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ currency }}</p>
        <div
          v-for="(user, idx) in currencyUsers"
          :key="user.user_id"
          class="flex items-center justify-between rounded-lg px-3 py-2 hover:bg-gray-50 dark:hover:bg-dark-700"
        >
          <div class="flex items-center gap-3">
            <span
              :class="[
                'flex h-6 w-6 items-center justify-center rounded-full text-xs font-bold',
                rankClass(idx),
              ]"
            >
              {{ idx + 1 }}
            </span>
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ user.email }}</span>
          </div>
          <span class="text-sm font-medium text-gray-900 dark:text-white">
            {{ formatMoney(currency, user.amount) }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { TopUserPaymentStats } from '@/types/payment'

const { t } = useI18n()

const props = defineProps<{
  users: Record<string, TopUserPaymentStats[]>
}>()

function rankClass(idx: number): string {
  if (idx === 0) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  if (idx === 1) return 'bg-gray-200 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
  if (idx === 2) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
  return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
}

function hasUsers(usersByCurrency: Record<string, TopUserPaymentStats[]>): boolean {
  return Object.values(usersByCurrency).some(users => users.length > 0)
}

function sortedUsers(usersByCurrency: Record<string, TopUserPaymentStats[]>): [string, TopUserPaymentStats[]][] {
  return Object.entries(usersByCurrency).sort(([left], [right]) => left.localeCompare(right))
}

function formatMoney(currency: string, amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(amount)
}
</script>
