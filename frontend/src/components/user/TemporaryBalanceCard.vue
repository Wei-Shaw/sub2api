<template>
  <section
    v-if="amount > 0"
    data-testid="temporary-balance-card"
    :data-status="status"
    class="card border border-blue-100 bg-blue-50/70 p-4 dark:border-blue-900/50 dark:bg-blue-950/30"
  >
    <div class="flex items-start gap-3">
      <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/40">
        <Icon name="clock" size="md" class="text-blue-600 dark:text-blue-400" :stroke-width="2" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="text-xs font-medium text-blue-700 dark:text-blue-300">{{ t('dashboard.temporaryBalance') }}</p>
          <span
            v-if="status === 'expired'"
            class="rounded-full bg-red-100 px-2 py-0.5 text-[11px] font-medium text-red-700 dark:bg-red-900/40 dark:text-red-300"
          >{{ t('dashboard.temporaryBalanceExpired') }}</span>
        </div>
        <p class="mt-1 text-xl font-bold" :class="status === 'active' ? 'text-blue-700 dark:text-blue-300' : 'text-gray-500 dark:text-gray-400'">
          ${{ formatAmount(amount) }}
        </p>
        <p v-if="status === 'active' && expiresAt" class="mt-0.5 text-xs text-blue-600 dark:text-blue-400">
          {{ t('dashboard.temporaryBalanceActive', { time: formatExpiry(expiresAt) }) }}
        </p>
        <p v-else-if="status === 'expired'" class="mt-0.5 text-xs text-red-600 dark:text-red-400">
          {{ t('dashboard.temporaryBalanceExpiredAt', { time: expiresAt ? formatExpiry(expiresAt) : '-' }) }}
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'
import { getTemporaryBalanceStatus, type TemporaryBalanceStatus } from '@/utils/temporaryBalance'

const props = defineProps<{
  amount: number
  expiresAt?: string | null
  now?: Date
}>()

const { t } = useI18n()
// Refresh the comparison clock while the card is mounted so a grant switches
// to "expired" even when the user leaves the dashboard open past its deadline.
const currentTime = ref(new Date())
let clockTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  if (!props.now) {
    clockTimer = setInterval(() => { currentTime.value = new Date() }, 30_000)
  }
})
onBeforeUnmount(() => {
  if (clockTimer) clearInterval(clockTimer)
})
const effectiveNow = computed(() => props.now ?? currentTime.value)
const status = computed<TemporaryBalanceStatus>(() =>
  getTemporaryBalanceStatus(
    { temporary_balance: props.amount, temporary_balance_expires_at: props.expiresAt },
    effectiveNow.value
  )
)

const formatAmount = (value: number) => Number(value || 0).toFixed(2)
const formatExpiry = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}
</script>
