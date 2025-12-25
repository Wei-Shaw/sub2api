<template>
  <div v-if="showUsageWindows">
    <!-- Anthropic OAuth and Setup Token accounts: fetch real usage data -->
    <template v-if="account.platform === 'anthropic' && (account.type === 'oauth' || account.type === 'setup-token')">
      <!-- Loading state -->
      <div v-if="loading" class="space-y-1.5">
        <!-- OAuth: 3 rows, Setup Token: 1 row -->
        <div class="flex items-center gap-1">
          <div class="w-[32px] h-3 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
          <div class="w-8 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full animate-pulse"></div>
          <div class="w-[32px] h-3 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        </div>
        <template v-if="account.type === 'oauth'">
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
        </template>
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

        <!-- 7d Window (OAuth only) -->
        <UsageProgressBar
          v-if="usageInfo.seven_day"
          label="7d"
          :utilization="usageInfo.seven_day.utilization"
          :resets-at="usageInfo.seven_day.resets_at"
          color="emerald"
        />

        <!-- 7d Sonnet Window (OAuth only) -->
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

    <!-- OpenAI OAuth accounts: show Codex usage from extra field -->
    <template v-else-if="account.platform === 'openai' && account.type === 'oauth'">
      <div v-if="hasCodexUsage" class="space-y-1">
        <!-- 5h Window (Secondary) -->
        <UsageProgressBar
          v-if="codexSecondaryUsedPercent !== null"
          label="5h"
          :utilization="codexSecondaryUsedPercent"
          :resets-at="codexSecondaryResetAt"
          color="indigo"
        />

        <!-- Weekly Window (Primary) -->
        <UsageProgressBar
          v-if="codexPrimaryUsedPercent !== null"
          label="7d"
          :utilization="codexPrimaryUsedPercent"
          :resets-at="codexPrimaryResetAt"
          color="emerald"
        />
      </div>
      <div v-else class="text-xs text-gray-400">-</div>
    </template>

    <!-- Other accounts: no usage window -->
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

// OpenAI Codex usage computed properties
const hasCodexUsage = computed(() => {
  const extra = props.account.extra
  return extra && (
    extra.codex_primary_used_percent !== undefined ||
    extra.codex_secondary_used_percent !== undefined
  )
})

const codexPrimaryUsedPercent = computed(() => {
  const extra = props.account.extra
  if (!extra || extra.codex_primary_used_percent === undefined) return null
  return extra.codex_primary_used_percent
})

const codexSecondaryUsedPercent = computed(() => {
  const extra = props.account.extra
  if (!extra || extra.codex_secondary_used_percent === undefined) return null
  return extra.codex_secondary_used_percent
})

const codexPrimaryResetAt = computed(() => {
  const extra = props.account.extra
  if (!extra || extra.codex_primary_reset_after_seconds === undefined) return null
  const resetTime = new Date(Date.now() + extra.codex_primary_reset_after_seconds * 1000)
  return resetTime.toISOString()
})

const codexSecondaryResetAt = computed(() => {
  const extra = props.account.extra
  if (!extra || extra.codex_secondary_reset_after_seconds === undefined) return null
  const resetTime = new Date(Date.now() + extra.codex_secondary_reset_after_seconds * 1000)
  return resetTime.toISOString()
})

const loadUsage = async () => {
  // Fetch usage for Anthropic OAuth and Setup Token accounts
  // OpenAI usage comes from account.extra field (updated during forwarding)
  if (props.account.platform !== 'anthropic') return
  if (props.account.type !== 'oauth' && props.account.type !== 'setup-token') return

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
