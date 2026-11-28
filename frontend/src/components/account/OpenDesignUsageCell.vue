<template>
  <div v-if="state" class="min-w-0 max-w-full space-y-1" data-testid="opendesign-usage">
    <UsageProgressBar
      v-if="state.snapshot?.five_hour"
      label="5h"
      :utilization="usedPercent(state.snapshot.five_hour.remaining_percent)"
      :resets-at="state.snapshot.five_hour.resets_at"
      color="indigo"
    />
    <UsageProgressBar
      v-if="state.snapshot?.seven_day"
      label="7d"
      :utilization="usedPercent(state.snapshot.seven_day.remaining_percent)"
      :resets-at="state.snapshot.seven_day.resets_at"
      color="emerald"
    />
    <div v-if="state.snapshot?.balance_usd != null" class="text-[10px] text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.opendesignUsage.balance', { balance: state.snapshot.balance_usd }) }}
    </div>
    <span
      v-if="refreshFailed || state.status !== 'ok'"
      class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium"
      :class="state.status === 'unauthorized'
        ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
        : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'"
      role="status"
    >
      {{ t(state.status === 'unauthorized' ? 'admin.accounts.opendesignUsage.unauthorized' : 'admin.accounts.opendesignUsage.failed') }}
    </span>
    <div class="flex items-center pt-0.5">
      <UsageQueryButton
        :loading="refreshing"
        data-testid="opendesign-usage-query"
        @click="refresh"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, OpenDesignUsageState } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'
import UsageQueryButton from './UsageQueryButton.vue'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ updated: [state: OpenDesignUsageState] }>()
const { t } = useI18n()
const state = computed(() => props.account.opendesign_usage)
const refreshing = ref(false)
const refreshFailed = ref(false)
let queryVersion = 0
watch([() => props.account.id, () => state.value?.workspace_id], () => {
  queryVersion++
  refreshing.value = false
  refreshFailed.value = false
}, { flush: 'sync' })
watch(() => state.value?.snapshot?.fetched_at, () => { refreshFailed.value = false })
onBeforeUnmount(() => { queryVersion++ })

// 上游返回剩余额度，账号列表统一使用已用百分比。
function usedPercent(remaining: number) {
  return 100 - Math.max(0, Math.min(100, remaining))
}

async function refresh() {
  if (refreshing.value) return
  const version = ++queryVersion
  refreshing.value = true
  refreshFailed.value = false
  try {
    const result = await adminAPI.accounts.refreshOpenDesignUsage(props.account.id)
    if (version === queryVersion) emit('updated', result)
  } catch {
    if (version === queryVersion) refreshFailed.value = true
  } finally {
    if (version === queryVersion) refreshing.value = false
  }
}
</script>
