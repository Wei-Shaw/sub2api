<template>
  <div v-if="account.type === 'oauth' || account.type === 'setup-token'">
    <!-- OAuth accounts: fetch real usage data -->
    <template v-if="account.type === 'oauth'">
      <!-- Loading state -->
      <div v-if="loading" class="space-y-1.5">
        <div class="flex items-center gap-1">
          <div class="w-[32px] h-3 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
          <div class="w-8 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full animate-pulse"></div>
          <div class="w-[32px] h-3 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        </div>
        <div class="flex items-center gap-1">
          <div class="w-[32px] h-3 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
          <div class="w-8 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full animate-pulse"></div>
          <div class="w-[32px] h-3 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        </div>
        <div class="flex items-center gap-1">
          <div class="w-[32px] h-3 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
          <div class="w-8 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full animate-pulse"></div>
          <div class="w-[32px] h-3 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        </div>
      </div>

      <!-- Error state -->
      <div v-else-if="error" class="text-xs text-red-500">
        {{ error REDACTEDREDACTED
      </div>

      <!-- Usage data -->
      <div v-else-if="usageInfo" class="space-y-1">
        <!-- 5h Window -->
        <UsageProgressBar
          v-if="usageInfo.five_hour"
          label="5h"
          :utilization="usageInfo.five_hour.utilization"
          :resets-at="usageInfo.five_hour.resets_at"
          :window-stats="usageInfo.five_hour.window_stats"
          color="indigo"
        />

        <!-- 7d Window -->
        <UsageProgressBar
          v-if="usageInfo.seven_day"
          label="7d"
          :utilization="usageInfo.seven_day.utilization"
          :resets-at="usageInfo.seven_day.resets_at"
          color="emerald"
        />

        <!-- 7d Sonnet Window -->
        <UsageProgressBar
          v-if="usageInfo.seven_day_sonnet"
          label="7d S"
          :utilization="usageInfo.seven_day_sonnet.utilization"
          :resets-at="usageInfo.seven_day_sonnet.resets_at"
          color="purple"
        />
      </div>

      <!-- No data yet -->
      <div v-else class="text-xs text-gray-400">
        -
      </div>
    </template>

    <!-- Setup Token accounts: show time-based window progress -->
    <template v-else-if="account.type === 'setup-token'">
      <SetupTokenTimeWindow :account="account" />
    </template>
  </div>

  <!-- Non-OAuth accounts -->
  <div v-else class="text-xs text-gray-400">
    -
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted REDACTED from 'vue'
import { adminAPI REDACTED from '@/api/admin'
import type { Account, AccountUsageInfo REDACTED from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'
import SetupTokenTimeWindow from './SetupTokenTimeWindow.vue'

const props = defineProps<{
  account: Account
REDACTED>()

const loading = ref(false)
const error = ref<string | null>(null)
const usageInfo = ref<AccountUsageInfo | null>(null)

const loadUsage = async () => {
  // Only fetch usage for OAuth accounts (Setup Token uses local time-based calculation)
  if (props.account.type !== 'oauth') return

  loading.value = true
  error.value = null

  try {
    usageInfo.value = await adminAPI.accounts.getUsage(props.account.id)
  REDACTED catch (e: any) {
    error.value = 'Failed'
    console.error('Failed to load usage:', e)
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

onMounted(() => {
  loadUsage()
REDACTED)
</script>
