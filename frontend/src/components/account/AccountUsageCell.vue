<template>
  <div v-if="showUsageWindows">
    <!-- Anthropic OAuth and Setup Token accounts: fetch real usage data -->
    <template
      v-if="
        account.platform === 'anthropic' &&
        (account.type === 'oauth' || account.type === 'setup-token')
      "
    >
      <!-- Loading state -->
      <div v-if="loading" class="space-y-1.5">
        <!-- OAuth: 3 rows, Setup Token: 1 row -->
        <div class="flex items-center gap-1">
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        </div>
        <template v-if="account.type === 'oauth'">
          <div class="flex items-center gap-1">
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          </div>
          <div class="flex items-center gap-1">
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
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
      <div v-else class="text-xs text-gray-400">-</div>
    </template>

    <!-- OpenAI OAuth accounts: show Codex usage from extra field -->
    <template v-else-if="account.platform === 'openai' && account.type === 'oauth'">
      <div v-if="hasCodexUsage" class="space-y-1">
        <!-- 5h Window -->
        <UsageProgressBar
          v-if="codex5hUsedPercent !== null"
          label="5h"
          :utilization="codex5hUsedPercent"
          :resets-at="codex5hResetAt"
          color="indigo"
        />

        <!-- 7d Window -->
        <UsageProgressBar
          v-if="codex7dUsedPercent !== null"
          label="7d"
          :utilization="codex7dUsedPercent"
          :resets-at="codex7dResetAt"
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
  <div v-else class="text-xs text-gray-400">-</div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageInfo } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{
  account: Account
}>()

const { t } = useI18n()

const loading = ref(false)
const error = ref<string | null>(null)
const usageInfo = ref<AccountUsageInfo | null>(null)

// Show usage windows for OAuth and Setup Token accounts
const showUsageWindows = computed(
  () => props.account.type === 'oauth' || props.account.type === 'setup-token'
)

// OpenAI Codex usage computed properties
const hasCodexUsage = computed(() => {
  const extra = props.account.extra
  return (
    extra &&
    // Check for new canonical fields first
    (extra.codex_5h_used_percent !== undefined ||
      extra.codex_7d_used_percent !== undefined ||
      // Fallback to legacy fields
      extra.codex_primary_used_percent !== undefined ||
      extra.codex_secondary_used_percent !== undefined)
  )
})

// 5h window usage (prefer canonical field)
const codex5hUsedPercent = computed(() => {
  const extra = props.account.extra
  if (!extra) return null

  // Prefer canonical field
  if (extra.codex_5h_used_percent !== undefined) {
    return extra.codex_5h_used_percent
  }

  // Fallback: detect from legacy fields using window_minutes
  if (
    extra.codex_primary_window_minutes !== undefined &&
    extra.codex_primary_window_minutes <= 360
  ) {
    return extra.codex_primary_used_percent ?? null
  }
  if (
    extra.codex_secondary_window_minutes !== undefined &&
    extra.codex_secondary_window_minutes <= 360
  ) {
    return extra.codex_secondary_used_percent ?? null
  }

  // Legacy assumption: secondary = 5h (may be incorrect)
  return extra.codex_secondary_used_percent ?? null
})

const codex5hResetAt = computed(() => {
  const extra = props.account.extra
  if (!extra) return null

  // Prefer canonical field
  if (extra.codex_5h_reset_after_seconds !== undefined) {
    const resetTime = new Date(Date.now() + extra.codex_5h_reset_after_seconds * 1000)
    return resetTime.toISOString()
  }

  // Fallback: detect from legacy fields using window_minutes
  if (
    extra.codex_primary_window_minutes !== undefined &&
    extra.codex_primary_window_minutes <= 360
  ) {
    if (extra.codex_primary_reset_after_seconds !== undefined) {
      const resetTime = new Date(Date.now() + extra.codex_primary_reset_after_seconds * 1000)
      return resetTime.toISOString()
    }
  }
  if (
    extra.codex_secondary_window_minutes !== undefined &&
    extra.codex_secondary_window_minutes <= 360
  ) {
    if (extra.codex_secondary_reset_after_seconds !== undefined) {
      const resetTime = new Date(Date.now() + extra.codex_secondary_reset_after_seconds * 1000)
      return resetTime.toISOString()
    }
  }

  // Legacy assumption: secondary = 5h
  if (extra.codex_secondary_reset_after_seconds !== undefined) {
    const resetTime = new Date(Date.now() + extra.codex_secondary_reset_after_seconds * 1000)
    return resetTime.toISOString()
  }

  return null
})

// 7d window usage (prefer canonical field)
const codex7dUsedPercent = computed(() => {
  const extra = props.account.extra
  if (!extra) return null

  // Prefer canonical field
  if (extra.codex_7d_used_percent !== undefined) {
    return extra.codex_7d_used_percent
  }

  // Fallback: detect from legacy fields using window_minutes
  if (
    extra.codex_primary_window_minutes !== undefined &&
    extra.codex_primary_window_minutes >= 10000
  ) {
    return extra.codex_primary_used_percent ?? null
  }
  if (
    extra.codex_secondary_window_minutes !== undefined &&
    extra.codex_secondary_window_minutes >= 10000
  ) {
    return extra.codex_secondary_used_percent ?? null
  }

  // Legacy assumption: primary = 7d (may be incorrect)
  return extra.codex_primary_used_percent ?? null
})

const codex7dResetAt = computed(() => {
  const extra = props.account.extra
  if (!extra) return null

  // Prefer canonical field
  if (extra.codex_7d_reset_after_seconds !== undefined) {
    const resetTime = new Date(Date.now() + extra.codex_7d_reset_after_seconds * 1000)
    return resetTime.toISOString()
  }

  // Fallback: detect from legacy fields using window_minutes
  if (
    extra.codex_primary_window_minutes !== undefined &&
    extra.codex_primary_window_minutes >= 10000
  ) {
    if (extra.codex_primary_reset_after_seconds !== undefined) {
      const resetTime = new Date(Date.now() + extra.codex_primary_reset_after_seconds * 1000)
      return resetTime.toISOString()
    }
  }
  if (
    extra.codex_secondary_window_minutes !== undefined &&
    extra.codex_secondary_window_minutes >= 10000
  ) {
    if (extra.codex_secondary_reset_after_seconds !== undefined) {
      const resetTime = new Date(Date.now() + extra.codex_secondary_reset_after_seconds * 1000)
      return resetTime.toISOString()
    }
  }

  // Legacy assumption: primary = 7d
  if (extra.codex_primary_reset_after_seconds !== undefined) {
    const resetTime = new Date(Date.now() + extra.codex_primary_reset_after_seconds * 1000)
    return resetTime.toISOString()
  }

  return null
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
    error.value = t('common.error')
    console.error('Failed to load usage:', e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadUsage()
})
</script>
