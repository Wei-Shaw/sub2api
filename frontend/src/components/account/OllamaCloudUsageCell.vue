<template>
  <div
    v-if="state?.eligible"
    class="min-w-0 max-w-full space-y-1"
    data-testid="ollama-cloud-usage-cell"
  >
    <div v-if="queryError" class="text-[10px] text-amber-600" :title="queryError">
      {{ t('admin.accounts.usageWindow.showingLastData') }}
    </div>
    <UsageProgressBar
      v-if="snapshot?.data?.five_hour"
      label="5h"
      :utilization="snapshot.data.five_hour.used_percent"
      :resets-at="snapshot.data.five_hour.reset_at"
      color="indigo"
      data-testid="ollama-cloud-five-hour"
    />
    <UsageProgressBar
      v-if="snapshot?.data?.seven_day"
      label="7d"
      :utilization="snapshot.data.seven_day.used_percent"
      :resets-at="snapshot.data.seven_day.reset_at"
      color="emerald"
      data-testid="ollama-cloud-seven-day"
    />
    <div v-if="state.configured" class="flex items-center pt-0.5">
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="refreshing"
        data-testid="ollama-cloud-usage-query"
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
import { useAccountQuotaRefresh, quotaSnapshotTime } from '@/utils/accountQuotaRefresh'
import type { Account, OllamaCloudUsageState } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ updated: [state: OllamaCloudUsageState] }>()
const { t } = useI18n()
const state = ref(props.account.ollama_cloud_usage)
const quotaQuery = useAccountQuotaRefresh(() => props.account)
const refreshing = ref(false)
const queryError = computed(() => quotaQuery.state.value?.error)
const snapshot = computed(() => state.value?.snapshot)

watch(() => props.account.ollama_cloud_usage, (next) => {
  if (!quotaQuery.state.value?.data || (
    !quotaQuery.state.value.loading && next?.snapshot?.status === 'ok' &&
    quotaSnapshotTime(next.snapshot.fetched_at) > quotaSnapshotTime(state.value?.snapshot?.fetched_at)
  )) state.value = next
})

const refreshUsage = async () => {
  await quotaQuery.refresh()
}
watch(quotaQuery.state, (next) => {
  if (!next) return
  refreshing.value = next.loading
}, { deep: true, immediate: true })
watch(() => quotaQuery.state.value?.data?.ollama, (next) => {
  if (!next) return
  state.value = next
  if (!quotaQuery.provided) emit('updated', next)
}, { immediate: true })

</script>
