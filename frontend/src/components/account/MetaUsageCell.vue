<template>
  <div class="min-w-0 max-w-full space-y-1" data-testid="meta-usage-cell">
    <template v-if="hasSample && usage">
      <UsageProgressBar
        v-if="usage.five_hour && (usage.five_hour.utilization > 0 || usage.five_hour.resets_at)"
        label="5h"
        :utilization="usage.five_hour.utilization"
        :resets-at="usage.five_hour.resets_at"
        :window-stats="usage.five_hour.window_stats"
        color="indigo"
        data-testid="meta-usage-window"
      />
      <UsageProgressBar
        v-if="usage.seven_day"
        label="7d"
        :utilization="usage.seven_day.utilization"
        :resets-at="usage.seven_day.resets_at"
        color="emerald"
        data-testid="meta-usage-weekly"
      />
    </template>
    <div
      v-else
      class="text-xs text-gray-400"
      :title="t('admin.accounts.metaUsage.noSample')"
    >-</div>
    <div class="flex items-center gap-1 pt-0.5">
      <span class="text-[9px] italic text-gray-400 dark:text-gray-500">
        {{ t('admin.accounts.usageWindow.passiveSampled') }}
      </span>
      <span
        v-if="usage?.subscription_tier_raw"
        class="rounded bg-indigo-50 px-1 py-0.5 text-[10px] text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300"
        :title="t('admin.accounts.metaUsage.tier')"
      >
        {{ usage.subscription_tier_raw }}
      </span>
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading"
        data-testid="meta-usage-refresh"
        @click="loadUsage"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': loading }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
        {{ t('admin.accounts.usageWindow.activeQuery') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageInfo } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const hasSample = computed(() =>
  Boolean(
    (usage.value?.five_hour &&
      (usage.value.five_hour.utilization > 0 || usage.value.five_hour.resets_at)) ||
    usage.value?.seven_day
  )
)
const usage = ref<AccountUsageInfo | undefined>(undefined)
const loading = ref(false)

const loadUsage = async () => {
  if (loading.value) return
  loading.value = true
  try {
    usage.value = await adminAPI.accounts.getUsage(props.account.id, 'passive')
  } catch (error) {
    console.error('Failed to load Meta usage:', error)
  } finally {
    loading.value = false
  }
}

onMounted(loadUsage)

watch(() => props.account.id, loadUsage)
</script>
