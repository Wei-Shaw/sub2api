<template>
  <div class="space-y-2 rounded-lg border border-gray-200 p-3 text-xs dark:border-dark-600">
    <p class="font-medium">上游明细与钱包 · 每 5 分钟</p>
    <p>明细按北京时间当日零点至当前时间增量读取；成功周期后无本地调用则跳过。钱包独立刷新。</p>
    <p v-if="failed" class="text-amber-600">同步状态暂不可用</p>
    <template v-else-if="state?.configured">
      <p>明细成功同步至：{{ date(state.last_success) }} · 最近检查：{{ date(state.last_attempt) }} · 下次检查：{{ date(state.next_attempt) }}</p>
      <p v-if="state.last_error" class="text-amber-600">{{ state.last_error }}</p>
      <p :title="walletDetails(state, false)">上游钱包：{{ state.wallet_balance == null ? '—' : `$${state.wallet_balance}` }} · 更新：{{ date(state.wallet_checked_at) }}</p>
      <p v-if="state.wallet_error" class="text-amber-600">{{ state.wallet_error }}</p>
    </template>
    <p v-else class="text-gray-500">应用有效面板配置后自动开始；与价格自动同步开关独立。</p>
  </div>
</template>
<script setup lang="ts">
import { useUpstreamActivity, walletDetails } from './activity'
const props = defineProps<{ accountId: number; revision?: number }>()
const { state, failed } = useUpstreamActivity(() => props.accountId, () => props.revision)
const date = (value: string | null) => value ? new Date(value).toLocaleString() : '尚无'
</script>
