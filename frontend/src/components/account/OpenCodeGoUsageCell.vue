<template>
  <div
    v-if="state?.eligible"
    class="min-w-0 max-w-full space-y-1"
    data-testid="opencode-go-usage-cell"
  >
    <UsageProgressBar
      v-if="snapshot?.data?.rolling"
      :label="t('admin.accounts.opencodeGo.rollingShort')"
      :utilization="snapshot.data.rolling.percent"
      :resets-at="snapshot.data.rolling.resets_at"
      color="indigo"
      data-testid="opencode-go-rolling"
    />
    <UsageProgressBar
      v-if="snapshot?.data?.weekly"
      :label="t('admin.accounts.opencodeGo.weeklyShort')"
      :utilization="snapshot.data.weekly.percent"
      :resets-at="snapshot.data.weekly.resets_at"
      color="emerald"
      data-testid="opencode-go-weekly"
    />
    <UsageProgressBar
      v-if="snapshot?.data?.monthly"
      :label="t('admin.accounts.opencodeGo.monthlyShort')"
      :utilization="snapshot.data.monthly.percent"
      :resets-at="snapshot.data.monthly.resets_at"
      color="amber"
      data-testid="opencode-go-monthly"
    />
    <span
      v-if="snapshot && snapshot.status !== 'ok'"
      class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium"
      :class="snapshot.status === 'unauthorized'
        ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
        : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'"
      data-testid="opencode-go-status-badge"
    >
      {{ statusLabel }}
    </span>
    <div class="flex items-center pt-0.5">
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="refreshing"
        data-testid="opencode-go-usage-query"
        @click="refreshUsage"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': refreshing }"
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
  <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, OpenCodeGoUsageState } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ updated: [state: OpenCodeGoUsageState] }>()
const { t } = useI18n()
const state = ref(props.account.opencode_go_usage)
const refreshing = ref(false)
const snapshot = computed(() => state.value?.snapshot)
const statusLabel = computed(() => {
  if (snapshot.value?.status === 'unauthorized') return t('admin.accounts.opencodeGo.unauthorized')
  if (snapshot.value?.status === 'failed') return t('admin.accounts.opencodeGo.failed')
  return t('admin.accounts.opencodeGo.ok')
})

watch(() => props.account.opencode_go_usage, (next) => {
  state.value = next
})

const refreshUsage = async () => {
  if (refreshing.value) return
  refreshing.value = true
  try {
    const next = await adminAPI.accounts.refreshOpenCodeGoUsage(props.account.id)
    state.value = next
    emit('updated', next)
  } catch (error) {
    console.error('Failed to refresh OpenCode Go usage:', error)
  } finally {
    refreshing.value = false
  }
}
</script>
