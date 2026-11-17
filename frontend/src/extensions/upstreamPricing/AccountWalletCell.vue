<template>
  <div class="text-xs" :title="walletDetails(state, failed)">
    <span v-if="state?.wallet_balance != null" class="whitespace-nowrap font-medium tabular-nums" :class="Number(state.wallet_balance) < 0 ? 'text-red-600' : 'text-gray-700 dark:text-gray-300'">${{ state.wallet_balance }}</span>
    <span v-else class="text-gray-400">—</span>
    <span v-if="state?.wallet_balance != null && (failed || state.wallet_error || stale)" class="ml-1 text-amber-600">旧值</span>
    <p v-if="enabled && state?.configured && !state.wallet_checked_at && !state.wallet_error" class="mt-0.5 text-gray-400">等待首次同步</p>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useUpstreamActivity, walletDetails } from './activity'
const props = defineProps<{ accountId: number; enabled: boolean; refreshToken?: number }>()
const { state, failed } = useUpstreamActivity(() => props.enabled ? props.accountId : 0, () => props.refreshToken)
const stale = computed(() => !!state.value?.wallet_checked_at && Date.now() - Date.parse(state.value.wallet_checked_at) > 10 * 60000)
</script>
