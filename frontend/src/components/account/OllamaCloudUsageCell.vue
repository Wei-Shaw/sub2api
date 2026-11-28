<template>
  <div
    v-if="state?.eligible"
    class="min-w-0 max-w-full space-y-1"
    data-testid="ollama-cloud-usage-cell"
  >
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
      <UsageQueryButton
        :loading="refreshing"
        data-testid="ollama-cloud-usage-query"
        @click="refreshUsage"
      />
    </div>
  </div>
  <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { adminAPI } from '@/api/admin'
import type { Account, OllamaCloudUsageState } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'
import UsageQueryButton from './UsageQueryButton.vue'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ updated: [state: OllamaCloudUsageState] }>()
const state = ref(props.account.ollama_cloud_usage)
const refreshing = ref(false)
const snapshot = computed(() => state.value?.snapshot)

watch(() => props.account.ollama_cloud_usage, (next) => {
  state.value = next
})

const refreshUsage = async () => {
  if (refreshing.value) return
  refreshing.value = true
  try {
    const next = await adminAPI.accounts.refreshOllamaCloudUsage(props.account.id)
    state.value = next
    emit('updated', next)
  } catch (error) {
    console.error('Failed to refresh Ollama Cloud usage:', error)
  } finally {
    refreshing.value = false
  }
}
</script>
