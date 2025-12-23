<template>
  <div v-if="showUsageWindows">
    <!-- Anthropic OAuth accounts: fetch real usage data -->
    <template v-if="account.platform === 'anthropic' && account.type === 'oauth'">
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
        {{ error }}
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

    <!-- Anthropic Setup Token accounts: show time-based window progress -->
    <template v-else-if="account.platform === 'anthropic' && account.type === 'setup-token'">
      <SetupTokenTimeWindow :account="account" />
    </template>

    <!-- OpenAI accounts: no usage window API, show dash -->
    <template v-else>
      <div class="text-xs text-gray-400">-</div>
    </template>
  </div>

  <!-- Non-OAuth/Setup-Token accounts -->
  <div v-else class="text-xs text-gray-400">
    -
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageInfo } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'
import SetupTokenTimeWindow from './SetupTokenTimeWindow.vue'

const props = defineProps<{
  account: Account
}>()

const loading = ref(false)
const error = ref<string | null>(null)
const usageInfo = ref<AccountUsageInfo | null>(null)

// Show usage windows for OAuth and Setup Token accounts
const showUsageWindows = computed(() =>
  props.account.type === 'oauth' || props.account.type === 'setup-token'
)

const loadUsage = async () => {
  // Only fetch usage for Anthropic OAuth accounts
  // OpenAI doesn't have a usage window API - usage is updated from response headers during forwarding
  if (props.account.platform !== 'anthropic' || props.account.type !== 'oauth') return

  loading.value = true
  error.value = null

  try {
    usageInfo.value = await adminAPI.accounts.getUsage(props.account.id)
  } catch (e: any) {
    error.value = 'Failed'
    console.error('Failed to load usage:', e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadUsage()
})
</script>
