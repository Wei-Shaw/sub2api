<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-5">
      <div v-if="loading" class="flex min-h-[460px] items-center justify-center">
        <LoadingSpinner />
      </div>

      <template v-else-if="status">
        <div
          role="note"
          class="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-amber-900 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-100"
        >
          <Icon name="sparkles" size="sm" class="mt-0.5 flex-shrink-0 text-amber-600 dark:text-amber-400" />
          <div>
            <p class="text-sm font-semibold">{{ t('checkIn.redeemLuckTitle') }}</p>
          </div>
        </div>

        <section class="card overflow-hidden">
          <div class="grid min-h-[330px] lg:grid-cols-[minmax(0,1.25fr)_minmax(280px,0.75fr)]">
            <div class="flex flex-col justify-between border-b border-gray-100 p-6 dark:border-dark-700 sm:p-8 lg:border-b-0 lg:border-r">
              <div class="flex items-start justify-between gap-4">
                <div>
                  <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
                    {{ status.server_date }} · {{ status.server_timezone }}
                  </p>
                  <h1 class="mt-2 text-2xl font-semibold tracking-normal text-gray-950 dark:text-white">
                    {{ t('checkIn.title') }}
                  </h1>
                </div>
                <span
                  class="inline-flex h-11 w-11 items-center justify-center rounded-lg"
                  :class="status.checked_today ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400' : 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400'"
                >
                  <Icon :name="status.checked_today ? 'checkCircle' : 'gift'" size="lg" />
                </span>
              </div>

              <div v-if="!status.config.enabled" class="py-10">
                <p class="text-lg font-medium text-gray-900 dark:text-white">{{ t('checkIn.disabled') }}</p>
              </div>

              <div v-else class="py-8">
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ status.checked_today ? t('checkIn.todayReceived') : t('checkIn.maxReward') }}
                </p>
                <p class="mt-2 text-5xl font-semibold tracking-normal text-gray-950 dark:text-white">
                  <template v-if="status.checked_today">+{{ formatAmount(status.today_reward) }}</template>
                  <template v-else>10$</template>
                </p>
                <p v-if="status.checked_today" class="mt-2 text-sm font-medium text-emerald-600 dark:text-emerald-400">
                  {{ t('checkIn.creditUnit') }}
                </p>
              </div>

              <button
                type="button"
                class="inline-flex h-12 w-full items-center justify-center gap-2 rounded-md bg-gray-950 px-5 text-sm font-semibold text-white transition-colors hover:bg-gray-800 disabled:cursor-not-allowed disabled:bg-gray-300 dark:bg-white dark:text-gray-950 dark:hover:bg-gray-100 dark:disabled:bg-dark-600 dark:disabled:text-dark-400 sm:w-auto"
                :disabled="submitting || status.checked_today || !status.config.enabled"
                @click="handleCheckIn"
              >
                <LoadingSpinner v-if="submitting" size="sm" />
                <Icon v-else :name="status.checked_today ? 'check' : 'gift'" size="sm" />
                {{ status.checked_today ? t('checkIn.checked') : t('checkIn.action') }}
              </button>
            </div>

            <div class="flex flex-col justify-center bg-gray-50/70 p-6 dark:bg-dark-800/40 sm:p-8">
              <div>
                <p class="text-xs font-medium uppercase text-gray-400">{{ t('checkIn.totalReward') }}</p>
                <p class="mt-2 text-2xl font-semibold text-gray-950 dark:text-white">
                  {{ formatAmount(status.total_reward) }}
                </p>
              </div>
            </div>
          </div>
        </section>

        <section class="card overflow-hidden">
          <div class="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-dark-700">
            <h2 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('checkIn.history') }}</h2>
            <span class="text-xs text-gray-400">{{ status.recent_checkins.length }}</span>
          </div>
          <div v-if="status.recent_checkins.length" class="divide-y divide-gray-100 dark:divide-dark-700">
            <div
              v-for="item in status.recent_checkins"
              :key="item.id"
              class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-4 px-5 py-4"
            >
              <div class="flex min-w-0 items-center gap-3">
                <span class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-md bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  <Icon name="calendar" size="sm" />
                </span>
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ item.date }}</p>
                </div>
              </div>
              <p class="text-sm font-semibold text-emerald-600 dark:text-emerald-400">
                +{{ formatAmount(item.reward) }}
              </p>
            </div>
          </div>
          <EmptyState v-else :title="t('checkIn.noHistory')">
            <template #icon>
              <Icon name="calendar" size="xl" />
            </template>
          </EmptyState>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import checkInAPI, { type CheckInStatus } from '@/api/checkin'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const loading = ref(true)
const submitting = ref(false)
const status = ref<CheckInStatus | null>(null)

function formatAmount(value: number): string {
  return Number(value || 0).toFixed(2)
}

async function loadStatus(): Promise<void> {
  status.value = await checkInAPI.getStatus()
}

async function handleCheckIn(): Promise<void> {
  if (!status.value || status.value.checked_today || !status.value.config.enabled) return
  submitting.value = true
  try {
    const result = await checkInAPI.checkIn()
    await Promise.all([loadStatus(), authStore.refreshUser()])
    if (result.already_checked) {
      appStore.showWarning(t('checkIn.alreadyChecked'))
      return
    }
    appStore.showSuccess(t('checkIn.success', { amount: formatAmount(result.record.reward) }))
  } catch (error: any) {
    appStore.showError(error?.message || t('checkIn.failed'))
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  try {
    await loadStatus()
  } catch (error: any) {
    appStore.showError(error?.message || t('checkIn.failed'))
  } finally {
    loading.value = false
  }
})
</script>
