<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <section class="card overflow-hidden">
        <div class="flex items-center justify-between gap-4 bg-gradient-to-br from-primary-500 to-primary-600 px-6 py-7 text-white">
          <div>
            <p class="text-sm text-primary-100">{{ t('redeem.myBalance') }}</p>
            <p class="mt-2 text-4xl font-bold">¥{{ (user?.balance ?? 0).toFixed(2) }}</p>
          </div>
          <Icon name="creditCard" size="xl" class="text-white/90" />
        </div>
      </section>

      <section class="card">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('redeem.balanceHistory') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('redeem.balanceHistoryDescription') }}</p>
        </div>
        <div v-if="loading" class="flex justify-center py-12">
          <Icon name="refresh" size="lg" class="animate-spin text-primary-500" />
        </div>
        <div v-else-if="entries.length === 0" class="py-12 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('redeem.balanceHistoryEmpty') }}
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full text-left text-sm">
            <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-800 dark:text-dark-400">
              <tr>
                <th class="px-6 py-3 font-medium">{{ t('redeem.balanceTime') }}</th>
                <th class="px-6 py-3 font-medium">{{ t('redeem.balanceType') }}</th>
                <th class="px-6 py-3 text-right font-medium">{{ t('redeem.balanceAmount') }}</th>
                <th class="px-6 py-3 text-right font-medium">{{ t('redeem.balanceAfter') }}</th>
                <th class="px-6 py-3 font-medium">{{ t('redeem.balanceReference') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="entry in entries" :key="entry.id" class="text-gray-700 dark:text-gray-300">
                <td class="whitespace-nowrap px-6 py-4">{{ formatDateTime(entry.occurred_at) }}</td>
                <td class="px-6 py-4">{{ typeLabel(entry.type) }}</td>
                <td :class="['whitespace-nowrap px-6 py-4 text-right font-medium', entry.amount >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400']">
                  {{ entry.amount >= 0 ? '+' : '' }}¥{{ entry.amount.toFixed(2) }}
                </td>
                <td class="whitespace-nowrap px-6 py-4 text-right">¥{{ entry.balance_after.toFixed(2) }}</td>
                <td class="max-w-[14rem] truncate px-6 py-4 font-mono text-xs" :title="entry.reference">{{ entry.reference }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="totalPages > 1" class="flex items-center justify-center gap-3 px-6 py-4">
          <button class="btn btn-secondary" :disabled="page <= 1 || loading" @click="load(page - 1)">{{ t('pagination.previous') }}</button>
          <span class="text-sm text-gray-500 dark:text-dark-400">{{ page }} / {{ totalPages }}</span>
          <button class="btn btn-secondary" :disabled="page >= totalPages || loading" @click="load(page + 1)">{{ t('pagination.next') }}</button>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { redeemAPI, type BalanceHistoryItem } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const authStore = useAuthStore()
const user = computed(() => authStore.user)
const entries = ref<BalanceHistoryItem[]>([])
const page = ref(1)
const total = ref(0)
const pageSize = 10
const loading = ref(false)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

const typeLabel = (type: string) => {
  if (type === 'redeem') return t('redeem.balanceTypeRedeem')
  if (type === 'charge') return t('redeem.balanceTypeCharge')
  return t('redeem.balanceTypeRecharge')
}

const load = async (nextPage = 1) => {
  loading.value = true
  try {
    const result = await redeemAPI.getBalanceHistory(nextPage, pageSize)
    entries.value = result.items
    total.value = result.total
    page.value = nextPage
  } finally {
    loading.value = false
  }
}

onMounted(() => load())
</script>
