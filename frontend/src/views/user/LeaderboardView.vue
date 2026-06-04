<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('leaderboard.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('leaderboard.subtitle') }}
          </p>
        </div>
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="loading"
          @click="loadLeaderboard"
        >
          <RefreshIcon class="h-4 w-4" :class="{ 'animate-spin': loading }" />
          <span>{{ loading ? t('leaderboard.refreshing') : t('leaderboard.refresh') }}</span>
        </button>
      </div>

      <div class="card overflow-hidden">
        <div class="card-header flex items-center justify-between gap-4">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('leaderboard.todayTop') }}
            </h2>
            <p v-if="leaderboard" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ leaderboard.start_date }}
            </p>
          </div>
          <div class="rounded-full bg-primary-50 px-3 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-200">
            Top {{ leaderboard?.limit ?? 5 }}
          </div>
        </div>

        <div v-if="loading && !leaderboard" class="flex items-center justify-center py-16">
          <LoadingSpinner size="lg" />
        </div>

        <div v-else-if="error" class="px-6 py-14 text-center">
          <p class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('leaderboard.failedToLoad') }}
          </p>
          <button type="button" class="btn btn-primary mt-4" @click="loadLeaderboard">
            <RefreshIcon class="h-4 w-4" />
            <span>{{ t('leaderboard.refresh') }}</span>
          </button>
        </div>

        <div v-else-if="!leaderboard?.items.length" class="px-6 py-14 text-center">
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('leaderboard.empty') }}
          </p>
        </div>

        <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
          <div class="grid grid-cols-[72px_1fr_minmax(120px,180px)] gap-3 px-6 py-3 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
            <span>{{ t('leaderboard.rank') }}</span>
            <span>{{ t('leaderboard.user') }}</span>
            <span class="text-right">{{ t('leaderboard.tokensToday') }}</span>
          </div>
          <div
            v-for="item in leaderboard.items"
            :key="item.rank"
            class="grid grid-cols-[72px_1fr_minmax(120px,180px)] items-center gap-3 px-6 py-4"
          >
            <div class="flex items-center gap-2">
              <span
                class="flex h-9 w-9 items-center justify-center rounded-full text-sm font-bold"
                :class="rankClass(item.rank)"
              >
                #{{ item.rank }}
              </span>
            </div>
            <div class="min-w-0">
              <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">
                {{ item.display_name }}
              </p>
            </div>
            <p class="text-right text-sm font-semibold tabular-nums text-gray-900 dark:text-white">
              {{ formatTokens(item.total_tokens) }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { usageAPI, type DailyTokenLeaderboardResponse } from '@/api/usage'

const { t } = useI18n()

const leaderboard = ref<DailyTokenLeaderboardResponse | null>(null)
const loading = ref(false)
const error = ref(false)

const RefreshIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M7.977 14.652H2.985m0 0V9.66m0 4.992 3.181-3.183a8.25 8.25 0 0 1 13.803 3.7'
        })
      ]
    )
}

function formatTokens(value: number): string {
  return new Intl.NumberFormat().format(value)
}

function rankClass(rank: number): string {
  if (rank === 1) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-200'
  if (rank === 2) return 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200'
  if (rank === 3) return 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-200'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
}

async function loadLeaderboard() {
  loading.value = true
  error.value = false
  try {
    leaderboard.value = await usageAPI.getDailyTokenLeaderboard()
  } catch (err) {
    console.error('Failed to load daily token leaderboard:', err)
    error.value = true
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadLeaderboard()
})
</script>
