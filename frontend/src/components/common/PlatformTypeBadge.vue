<template>
  <div class="inline-flex flex-col gap-0.5 text-xs font-medium">
    <!-- Row 1: Platform + Type -->
    <div class="inline-flex items-center overflow-hidden rounded-md">
      <span :class="['inline-flex items-center gap-1 px-2 py-1', platformClass]">
        <PlatformIcon :platform="platform" :icon-svg="decl?.icon_svg" size="xs" />
        <span>{{ platformLabel }}</span>
      </span>
      <span :class="['inline-flex items-center gap-1 px-1.5 py-1', typeClass]">
        <!-- OAuth icon -->
        <svg
          v-if="type === 'oauth'"
          class="h-3 w-3"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
          />
        </svg>
        <!-- Setup Token icon -->
        <Icon v-else-if="type === 'setup-token'" name="shield" size="xs" />
        <!-- API Key icon -->
        <Icon v-else-if="type === 'service_account'" name="cloud" size="xs" />
        <Icon v-else name="key" size="xs" />
        <span>{{ typeLabel }}</span>
      </span>
    </div>
    <!-- Row 2: Plan type + Privacy mode (only if either exists) -->
    <div v-if="planLabel || privacyBadge" class="inline-flex items-center overflow-hidden rounded-md">
      <span v-if="planLabel" :class="['inline-flex items-center gap-1 px-1.5 py-1', planBadgeClass]">
        <span>{{ planLabel }}</span>
      </span>
      <span
        v-if="privacyBadge"
        :class="['inline-flex items-center gap-1 px-1.5 py-1', privacyBadge.class]"
        :title="privacyBadge.title"
      >
        <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" :d="privacyBadge.icon" />
        </svg>
        <span>{{ privacyBadge.label }}</span>
      </span>
    </div>
    <!-- Row 3: Subscription expiration (non-free paid accounts only) -->
    <div v-if="expiresLabel" class="text-[10px] leading-tight text-gray-400 dark:text-gray-500 pl-0.5" :title="subscriptionExpiresAt">
      {{ expiresLabel }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountPlatform, AccountType } from '@/types'
import { PlatformIcon } from '@sub2api/plugin-sdk'
import Icon from '@/components/icons/Icon.vue'
import { usePlatforms } from '@/composables/usePlatforms'
import {
  dynamicPlatformBadgeClass,
  dynamicTypeBadgeClass,
} from '@/utils/platformColors'

const { t } = useI18n()
const { getPlatformDecl, getAccountTypeDecl } = usePlatforms()

interface Props {
  platform: AccountPlatform
  type: AccountType
  planType?: string
  privacyMode?: string
  subscriptionExpiresAt?: string
}

const props = defineProps<Props>()

const decl = computed(() => getPlatformDecl(props.platform))
const typeDecl = computed(() => getAccountTypeDecl(props.platform, props.type))

// -- Hardcoded fallback maps (used when plugin API has not loaded yet) --
// TODO: Remove these once plugin API is guaranteed to load before first render

const FALLBACK_LABELS: Record<string, string> = {
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  antigravity: 'Antigravity',
  gemini: 'Gemini',
}

const FALLBACK_PLATFORM_CLASS: Record<string, string> = {
  anthropic: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  openai: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  antigravity: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
  gemini: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
}

const FALLBACK_TYPE_CLASS: Record<string, string> = {
  anthropic: 'bg-orange-100 text-orange-600 dark:bg-orange-900/30 dark:text-orange-400',
  openai: 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400',
  antigravity: 'bg-purple-100 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400',
  gemini: 'bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400',
}

// TODO: Remove these fallback type labels once badge_label is guaranteed from plugin API
const FALLBACK_TYPE_LABELS: Record<string, string> = {
  oauth: 'OAuth',
  'setup-token': 'Token',
  apikey: 'Key',
  bedrock: 'AWS',
  service_account: 'Vertex',
}

const DEFAULT_CLASS = 'bg-slate-100 text-slate-700 dark:bg-slate-900/30 dark:text-slate-400'
const DEFAULT_TYPE_CLASS = 'bg-slate-100 text-slate-600 dark:bg-slate-900/30 dark:text-slate-400'

// -- Computed properties ----------------------------------------------

const platformLabel = computed(() => {
  if (decl.value) return decl.value.display_name
  return FALLBACK_LABELS[props.platform] ?? props.platform ?? 'Unknown'
})

const typeLabel = computed(() => {
  // Prefer badge_label from AccountTypeDeclaration (plugin-driven)
  if (typeDecl.value?.badge_label) return typeDecl.value.badge_label
  // Then try display_name from AccountTypeDeclaration
  if (typeDecl.value?.display_name) return typeDecl.value.display_name
  // Fallback to hardcoded labels for startup resilience
  return FALLBACK_TYPE_LABELS[props.type] ?? props.type
})

const planLabel = computed(() => {
  if (!props.planType) return ''
  const lower = props.planType.toLowerCase()
  switch (lower) {
    case 'plus':
      return 'Plus'
    case 'team':
      return 'Team'
    case 'chatgptpro':
    case 'pro':
      return 'Pro'
    case 'free':
      return 'Free'
    case 'abnormal':
      return t('admin.accounts.subscriptionAbnormal')
    default:
      return props.planType
  }
})

const platformClass = computed(() => {
  if (decl.value?.theme_color) return dynamicPlatformBadgeClass(decl.value.theme_color)
  return FALLBACK_PLATFORM_CLASS[props.platform] ?? DEFAULT_CLASS
})

const typeClass = computed(() => {
  if (decl.value?.theme_color) return dynamicTypeBadgeClass(decl.value.theme_color)
  return FALLBACK_TYPE_CLASS[props.platform] ?? DEFAULT_TYPE_CLASS
})

const planBadgeClass = computed(() => {
  if (props.planType && props.planType.toLowerCase() === 'abnormal') {
    return 'bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400'
  }
  return typeClass.value
})

// Subscription expiration label (non-free only)
const expiresLabel = computed(() => {
  if (!props.subscriptionExpiresAt || !props.planType) return ''
  if (props.planType.toLowerCase() === 'free') return ''
  try {
    const d = new Date(props.subscriptionExpiresAt)
    if (isNaN(d.getTime())) return ''
    const yyyy = d.getFullYear()
    const mm = String(d.getMonth() + 1).padStart(2, '0')
    const dd = String(d.getDate()).padStart(2, '0')
    return `${t('admin.accounts.subscriptionExpires')} ${yyyy}-${mm}-${dd}`
  } catch {
    return ''
  }
})

// -- Privacy badge ----------------------------------------------------
// Reads from plugin declaration privacy_states when available,
// falls back to hardcoded values for builtin platforms.

const SHIELD_CHECK = 'M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z'
const SHIELD_X = 'M12 9v3.75m0-10.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285zM12 18h.008v.008H12V18z'

const PRIVACY_COLOR_MAP: Record<string, string> = {
  green: 'bg-green-100 text-green-600 dark:bg-green-900/30 dark:text-green-400',
  yellow: 'bg-yellow-100 text-yellow-600 dark:bg-yellow-900/30 dark:text-yellow-400',
  red: 'bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400',
}

const privacyBadge = computed(() => {
  if (props.type !== 'oauth' || !props.privacyMode) return null

  // Try plugin declaration first
  const privacyStates = decl.value?.privacy_states
  if (privacyStates?.length) {
    const state = privacyStates.find(s => s.value === props.privacyMode)
    if (!state) return null
    const icon = state.is_set ? SHIELD_CHECK : SHIELD_X
    const colorClass = PRIVACY_COLOR_MAP[state.badge_color ?? '']
      ?? (state.is_set ? PRIVACY_COLOR_MAP.green : PRIVACY_COLOR_MAP.red)
    return { label: state.display_name, icon, title: state.display_name, class: colorClass }
  }

  // TODO: Remove hardcoded fallback once all platforms declare privacy_states via plugin API
  if (props.platform !== 'openai' && props.platform !== 'antigravity') return null

  switch (props.privacyMode) {
    case 'training_off':
      return { label: 'Private', icon: SHIELD_CHECK, title: t('admin.accounts.privacyTrainingOff'), class: PRIVACY_COLOR_MAP.green }
    case 'training_set_cf_blocked':
      return { label: 'CF', icon: SHIELD_X, title: t('admin.accounts.privacyCfBlocked'), class: PRIVACY_COLOR_MAP.yellow }
    case 'training_set_failed':
      return { label: 'Fail', icon: SHIELD_X, title: t('admin.accounts.privacyFailed'), class: PRIVACY_COLOR_MAP.red }
    case 'privacy_set':
      return { label: 'Private', icon: SHIELD_CHECK, title: t('admin.accounts.privacyAntigravitySet'), class: PRIVACY_COLOR_MAP.green }
    case 'privacy_set_failed':
      return { label: 'Fail', icon: SHIELD_X, title: t('admin.accounts.privacyAntigravityFailed'), class: PRIVACY_COLOR_MAP.red }
    default:
      return null
  }
})
</script>
