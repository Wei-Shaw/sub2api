<template>
  <div>
    <!-- Tier badge -->
    <div v-if="tierLabel" class="mb-1 flex items-center gap-1">
      <span
        :class="[
          'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
          tierClass
        ]"
      >
        {{ tierLabel }}
      </span>
      <!-- Ineligible tiers warning icon -->
      <span
        v-if="hasIneligibleTiers"
        class="group relative cursor-help"
      >
        <svg
          class="text-semantic-danger h-3.5 w-3.5"
          fill="currentColor"
          viewBox="0 0 20 20"
        >
          <path
            fill-rule="evenodd"
            d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z"
            clip-rule="evenodd"
          />
        </svg>
        <span
          class="pointer-events-none absolute left-0 top-full z-50 mt-1 w-80 whitespace-normal break-words rounded bg-gray-900 px-3 py-2 text-xs leading-relaxed text-white opacity-0 shadow-lg transition-opacity group-hover:opacity-100 dark:bg-gray-700"
        >
          {{ t('admin.accounts.ineligibleWarning') }}
        </span>
      </span>
    </div>

    <!-- Forbidden state (403) -->
    <div v-if="isForbidden" class="space-y-1">
      <span
        :class="[
          'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
          forbiddenBadgeClass
        ]"
      >
        {{ forbiddenLabel }}
      </span>
      <div v-if="validationURL" class="flex items-center gap-1">
        <a
          :href="validationURL"
          target="_blank"
          rel="noopener noreferrer"
          class="text-semantic-info text-[10px] hover:underline"
          :title="t('admin.accounts.openVerification')"
        >
          {{ t('admin.accounts.openVerification') }}
        </a>
        <button
          type="button"
          class="text-[10px] text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
          :title="t('admin.accounts.copyLink')"
          @click="copyValidationURL"
        >
          {{ linkCopied ? t('admin.accounts.linkCopied') : t('admin.accounts.copyLink') }}
        </button>
      </div>
    </div>

    <!-- Needs reauth (401) -->
    <div v-else-if="needsReauth" class="space-y-1">
      <span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300">
        {{ t('admin.accounts.needsReauth') }}
      </span>
    </div>

    <!-- Degraded error (non-403, non-401) -->
    <div v-else-if="usageInfo?.error" class="space-y-1">
      <span class="badge-warning inline-block rounded px-1.5 py-0.5 text-[10px] font-medium">
        {{ usageErrorLabel }}
      </span>
    </div>

    <!-- Loading state -->
    <div v-else-if="loading" class="space-y-1.5">
      <div class="flex items-center gap-1">
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="text-semantic-danger text-xs">
      {{ error }}
    </div>

    <!-- Usage data from API -->
    <div v-else-if="hasQuotaFromAPI" class="space-y-1">
      <UsageProgressBar
        v-if="proUsage !== null"
        :label="t('admin.accounts.usageWindow.gemini3Pro')"
        :utilization="proUsage.utilization"
        :resets-at="proUsage.resetTime"
        color="indigo"
      />
      <UsageProgressBar
        v-if="flashUsage !== null"
        :label="t('admin.accounts.usageWindow.gemini3Flash')"
        :utilization="flashUsage.utilization"
        :resets-at="flashUsage.resetTime"
        color="emerald"
      />
      <UsageProgressBar
        v-if="imageUsage !== null"
        :label="t('admin.accounts.usageWindow.gemini3Image')"
        :utilization="imageUsage.utilization"
        :resets-at="imageUsage.resetTime"
        color="purple"
      />
      <UsageProgressBar
        v-if="claudeUsage !== null"
        :label="t('admin.accounts.usageWindow.claude')"
        :utilization="claudeUsage.utilization"
        :resets-at="claudeUsage.resetTime"
        color="amber"
      />
      <div v-if="aiCreditsDisplay" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.aiCreditsBalance') }}: {{ aiCreditsDisplay }}
      </div>
    </div>
    <div v-else-if="aiCreditsDisplay" class="text-[10px] text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.aiCreditsBalance') }}: {{ aiCreditsDisplay }}
    </div>
    <div v-else class="text-xs text-gray-400">-</div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { UsageProgressBar } from '@sub2api/plugin-sdk'

/** Minimal model quota shape. */
interface ModelQuota {
  utilization: number
  reset_time: string
}

/** Minimal account usage info for Antigravity display. */
interface AntigravityUsageInfo {
  antigravity_quota?: Record<string, ModelQuota> | null
  ai_credits?: Array<{ credit_type?: string; amount?: number; minimum_balance?: number }> | null
  is_forbidden?: boolean
  forbidden_type?: string
  validation_url?: string
  needs_reauth?: boolean
  error_code?: string
  error?: string
}

const props = defineProps<{
  account: { extra?: Record<string, unknown>; [k: string]: unknown }
  usageInfo: AntigravityUsageInfo | null
  loading: boolean
  error: string | null
}>()

const { t } = useI18n()

// ===== Tier detection =====

const tierFromExtra = computed(() => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (!extra) return null
  const lca = extra.load_code_assist as Record<string, unknown> | undefined
  if (!lca) return null
  const paid = lca.paidTier as Record<string, unknown> | undefined
  if (paid && typeof paid.id === 'string') return paid.id
  const cur = lca.currentTier as Record<string, unknown> | undefined
  if (cur && typeof cur.id === 'string') return cur.id
  return null
})

const tierLabel = computed(() => {
  switch (tierFromExtra.value) {
    case 'free-tier': return t('admin.accounts.tier.free')
    case 'g1-pro-tier': return t('admin.accounts.tier.pro')
    case 'g1-ultra-tier': return t('admin.accounts.tier.ultra')
    default: return null
  }
})

const tierClass = computed(() => {
  switch (tierFromExtra.value) {
    case 'free-tier':
      return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
    case 'g1-pro-tier':
      return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    case 'g1-ultra-tier':
      return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    default: return ''
  }
})

const hasIneligibleTiers = computed(() => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (!extra) return false
  const lca = extra.load_code_assist as Record<string, unknown> | undefined
  if (!lca) return false
  const arr = lca.ineligibleTiers as unknown[] | undefined
  return Array.isArray(arr) && arr.length > 0
})

// ===== Forbidden / reauth / error states =====

const isForbidden = computed(() => !!props.usageInfo?.is_forbidden)
const forbiddenType = computed(() => props.usageInfo?.forbidden_type || 'forbidden')
const validationURL = computed(() => props.usageInfo?.validation_url || '')
const needsReauth = computed(() => !!props.usageInfo?.needs_reauth)

const usageErrorLabel = computed(() => {
  const code = props.usageInfo?.error_code
  if (code === 'rate_limited') return t('admin.accounts.rateLimited')
  return t('admin.accounts.usageError')
})

const forbiddenLabel = computed(() => {
  switch (forbiddenType.value) {
    case 'validation': return t('admin.accounts.forbiddenValidation')
    case 'violation': return t('admin.accounts.forbiddenViolation')
    default: return t('admin.accounts.forbidden')
  }
})

const forbiddenBadgeClass = computed(() => {
  if (forbiddenType.value === 'validation') {
    return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/40 dark:text-yellow-300'
  }
  return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
})

const linkCopied = ref(false)
const copyValidationURL = async () => {
  if (!validationURL.value) return
  try {
    await navigator.clipboard.writeText(validationURL.value)
    linkCopied.value = true
    setTimeout(() => { linkCopied.value = false }, 2000)
  } catch { /* fallback: ignore */ }
}

// ===== Quota from API =====

interface QuotaUsageResult {
  utilization: number
  resetTime: string | null
}

const hasQuotaFromAPI = computed(() => {
  const q = props.usageInfo?.antigravity_quota
  return q && Object.keys(q).length > 0
})

const getQuotaUsage = (modelNames: string[]): QuotaUsageResult | null => {
  const quota = props.usageInfo?.antigravity_quota
  if (!quota) return null

  let maxUtil = 0
  let earliestReset: string | null = null

  for (const model of modelNames) {
    const mq = quota[model]
    if (!mq) continue
    if (mq.utilization > maxUtil) maxUtil = mq.utilization
    if (mq.reset_time) {
      if (!earliestReset || mq.reset_time < earliestReset) earliestReset = mq.reset_time
    }
  }

  if (maxUtil === 0 && earliestReset === null) {
    if (!modelNames.some((m) => quota[m])) return null
  }
  return { utilization: maxUtil, resetTime: earliestReset }
}

const proUsage = computed(() =>
  getQuotaUsage(['gemini-3-pro-low', 'gemini-3-pro-high', 'gemini-3-pro-preview'])
)
const flashUsage = computed(() => getQuotaUsage(['gemini-3-flash']))
const imageUsage = computed(() =>
  getQuotaUsage(['gemini-2.5-flash-image', 'gemini-3.1-flash-image', 'gemini-3-pro-image'])
)
const claudeUsage = computed(() =>
  getQuotaUsage([
    'claude-sonnet-4-5', 'claude-opus-4-5-thinking',
    'claude-sonnet-4-6', 'claude-opus-4-6', 'claude-opus-4-6-thinking',
  ])
)

const aiCreditsDisplay = computed(() => {
  const credits = props.usageInfo?.ai_credits
  if (!credits || credits.length === 0) return null
  const total = credits.reduce((sum, c) => sum + (c.amount ?? 0), 0)
  if (total <= 0) return null
  return total.toFixed(0)
})
</script>
