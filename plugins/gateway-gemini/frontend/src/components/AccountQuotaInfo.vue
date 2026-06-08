<template>
  <div v-if="shouldShowQuota">
    <!-- First line: Platform + Tier Badge -->
    <div class="mb-1 flex items-center gap-1">
      <span :class="['badge text-xs px-2 py-0.5 rounded font-medium', tierBadgeClass]">
        {{ tierLabel }}
      </span>
    </div>

    <!-- Usage status: unlimited flow or rate limit -->
    <div class="text-xs text-gray-400 dark:text-gray-500">
      <span v-if="!isRateLimited">
        {{ t('admin.accounts.gemini.rateLimit.unlimited') }}
      </span>
      <span
        v-else
        :class="[
          'font-medium',
          isUrgent
            ? 'text-red-600 dark:text-red-400 animate-pulse'
            : 'text-amber-600 dark:text-amber-400'
        ]"
      >
        {{ t('admin.accounts.gemini.rateLimit.limited', { time: resetCountdown }) }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'

/** Minimal Gemini credentials shape. */
interface GeminiCredentials {
  oauth_type?: string
  tier_id?: string
  project_id?: string
}

const props = defineProps<{
  account: {
    platform: string
    type: string
    credentials?: Record<string, unknown>
    rate_limit_reset_at: string | null
    [k: string]: unknown
  }
}>()

const { t } = useI18n()

const now = ref(new Date())
let timer: ReturnType<typeof setInterval> | null = null

const isCodeAssist = computed(() => {
  const creds = props.account.credentials as GeminiCredentials | undefined
  return creds?.oauth_type === 'code_assist' || (!creds?.oauth_type && !!creds?.project_id)
})

const isGoogleOne = computed(() => {
  const creds = props.account.credentials as GeminiCredentials | undefined
  return creds?.oauth_type === 'google_one'
})

const shouldShowQuota = computed(() => {
  return props.account.platform === 'gemini'
})

const tierLabel = computed(() => {
  const creds = props.account.credentials as GeminiCredentials | undefined

  if (isCodeAssist.value) {
    const tier = (creds?.tier_id || '').toString().trim().toLowerCase()
    if (tier === 'gcp_enterprise') return 'GCP Enterprise'
    if (tier === 'gcp_standard') return 'GCP Standard'
    const upper = (creds?.tier_id || '').toString().trim().toUpperCase()
    if (upper.includes('ULTRA') || upper.includes('ENTERPRISE')) return 'GCP Enterprise'
    if (upper) return `GCP ${upper}`
    return 'GCP'
  }

  if (isGoogleOne.value) {
    const tier = (creds?.tier_id || '').toString().trim().toLowerCase()
    if (tier === 'google_ai_ultra') return 'Google AI Ultra'
    if (tier === 'google_ai_pro') return 'Google AI Pro'
    if (tier === 'google_one_free') return 'Google One Free'
    const upper = (creds?.tier_id || '').toString().trim().toUpperCase()
    if (upper === 'AI_PREMIUM') return 'Google AI Pro'
    if (upper === 'GOOGLE_ONE_UNLIMITED') return 'Google AI Ultra'
    if (upper) return `Google One ${upper}`
    return 'Google One'
  }

  const tier = (creds?.tier_id || '').toString().trim().toLowerCase()
  if (tier === 'aistudio_paid') return 'AI Studio Pay-as-you-go'
  if (tier === 'aistudio_free') return 'AI Studio Free Tier'
  return 'AI Studio'
})

const tierBadgeClass = computed(() => {
  const creds = props.account.credentials as GeminiCredentials | undefined

  if (isCodeAssist.value) {
    const tier = (creds?.tier_id || '').toString().trim().toLowerCase()
    if (tier === 'gcp_enterprise') return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    if (tier === 'gcp_standard') return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    const upper = (creds?.tier_id || '').toString().trim().toUpperCase()
    if (upper.includes('ULTRA') || upper.includes('ENTERPRISE')) return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
  }

  if (isGoogleOne.value) {
    const tier = (creds?.tier_id || '').toString().trim().toLowerCase()
    if (tier === 'google_ai_ultra') return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    if (tier === 'google_ai_pro') return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    if (tier === 'google_one_free') return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
    const upper = (creds?.tier_id || '').toString().trim().toUpperCase()
    if (upper === 'GOOGLE_ONE_UNLIMITED') return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    if (upper === 'AI_PREMIUM') return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
  }

  const tier = (creds?.tier_id || '').toString().trim().toLowerCase()
  if (tier === 'aistudio_paid') return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
  if (tier === 'aistudio_free') return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
  return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
})

const isRateLimited = computed(() => {
  if (!props.account.rate_limit_reset_at) return false
  const resetTime = Date.parse(props.account.rate_limit_reset_at)
  if (Number.isNaN(resetTime)) return false
  return resetTime > now.value.getTime()
})

const resetCountdown = computed(() => {
  if (!props.account.rate_limit_reset_at) return ''
  const resetTime = Date.parse(props.account.rate_limit_reset_at)
  if (Number.isNaN(resetTime)) return '-'

  const diffMs = resetTime - now.value.getTime()
  if (diffMs <= 0) return t('admin.accounts.gemini.rateLimit.now')

  const diffSeconds = Math.floor(diffMs / 1000)
  const diffMinutes = Math.floor(diffSeconds / 60)
  const diffHours = Math.floor(diffMinutes / 60)

  if (diffMinutes < 1) return `${diffSeconds}s`
  if (diffHours < 1) {
    const secs = diffSeconds % 60
    return `${diffMinutes}m ${secs}s`
  }
  const mins = diffMinutes % 60
  return `${diffHours}h ${mins}m`
})

const isUrgent = computed(() => {
  if (!props.account.rate_limit_reset_at) return false
  const resetTime = Date.parse(props.account.rate_limit_reset_at)
  if (Number.isNaN(resetTime)) return false
  const diffMs = resetTime - now.value.getTime()
  return diffMs > 0 && diffMs < 60000
})

watch(
  () => isRateLimited.value,
  (limited) => {
    if (limited && !timer) {
      timer = setInterval(() => { now.value = new Date() }, 1000)
    } else if (!limited && timer) {
      clearInterval(timer)
      timer = null
    }
  },
  { immediate: true },
)

onUnmounted(() => {
  if (timer !== null) {
    clearInterval(timer)
    timer = null
  }
})
</script>
