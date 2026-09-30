<template>
  <div class="inline-flex flex-col items-start gap-1 text-meta font-medium">
    <!-- Row 1: Platform + Type -->
    <div class="inline-flex items-stretch overflow-hidden rounded-sm border border-border-strong">
      <span :class="['inline-flex items-center gap-1 px-1.5 py-0.5', platformClass]">
        <PlatformIcon :platform="platform" size="xs" />
        <span>{{ platformLabel }}</span>
      </span>
      <span :class="['inline-flex items-center gap-1 border-l border-border-strong px-1.5 py-0.5', typeClass]">
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
    <div v-if="planLabel || privacyBadge" class="inline-flex items-center gap-1">
      <span v-if="planLabel" :class="['inline-flex items-center gap-1 rounded-sm border px-1.5 py-px', planBadgeClass]">
        <GrokFreeIcon
          v-if="isGrokFreePlan"
          data-testid="grok-free-plan-icon"
        />
        <Icon
          v-else-if="planIconName"
          :name="planIconName"
          size="xs"
          data-testid="grok-plan-icon"
          aria-hidden="true"
        />
        <span>{{ planLabel }}</span>
      </span>
      <span
        v-if="privacyBadge"
        :class="['inline-flex items-center gap-1 rounded-sm border px-1.5 py-px', privacyBadge.class]"
        :title="privacyBadge.title"
      >
        <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" :d="privacyBadge.icon" />
        </svg>
        <span>{{ privacyBadge.label }}</span>
      </span>
    </div>
    <!-- Row 3: Subscription expiration (non-free paid accounts only) -->
    <div v-if="expiresLabel" class="text-meta leading-tight tabular-nums text-fg-subtle" :title="subscriptionExpiresAt">
      {{ expiresLabel }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountPlatform, AccountType } from '@/types'
import { platformLabel as sharedPlatformLabel } from '@/utils/platformColors'
import { normalizePlanType, openAIPlanTypeLabel } from '@/utils/planType'
import GrokFreeIcon from './GrokFreeIcon.vue'
import PlatformIcon from './PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

interface Props {
  platform: AccountPlatform
  type: AccountType
  authMode?: string
  planType?: string
  privacyMode?: string
  subscriptionExpiresAt?: string
}

const props = defineProps<Props>()

const platformLabel = computed(() => sharedPlatformLabel(props.platform))

const normalizedAuthMode = computed(() =>
  (props.authMode || '').trim().toLowerCase().replace(/[\s_-]+/g, '')
)

const typeLabel = computed(() => {
  if (props.platform === 'openai' && props.type === 'oauth') {
    if (normalizedAuthMode.value === 'agentidentity') return 'Agent Identity'
    if (normalizedAuthMode.value === 'personalaccesstoken') return 'PAT'
  }
  switch (props.type) {
    case 'oauth':
      return 'OAuth'
    case 'setup-token':
      return 'Token'
    case 'apikey':
      return 'Key'
    case 'bedrock':
      return 'AWS'
    case 'service_account':
      return 'Vertex'
    default:
      return props.type
  }
})

const normalizedPlanType = computed(() => normalizePlanType(props.planType))

const planLabel = computed(() => {
  if (!normalizedPlanType.value) return ''
  // ChatGPT 档位命名（Pro 100 / Pro 200 / Pro 500、Business / Business Premium）只适用于
  // OpenAI：Antigravity 与 Grok 各自的 pro/team 沿用下面的通用标签。
  if (props.platform === 'openai') {
    const label = openAIPlanTypeLabel(props.planType)
    if (label) return label
  }
  switch (normalizedPlanType.value) {
    case 'plus':
      return 'Plus'
    case 'team':
      return 'Team'
    case 'chatgptpro':
    case 'pro':
      return 'Pro'
    case 'free':
    case 'basic':
      return props.platform === 'grok' ? 'Grok Free' : 'Free'
    case 'supergrok':
      return 'SuperGrok'
    case 'supergroklite':
      return 'SuperGrok Lite'
    case 'supergrokplus':
      return 'SuperGrok Plus'
    case 'supergrokheavy':
      return 'SuperGrok Heavy'
    case 'heavy':
      return 'Heavy'
    case 'xbasic':
      return 'X Basic'
    case 'abnormal':
      return t('admin.accounts.subscriptionAbnormal')
    default:
      return props.planType
  }
})

const isGrokFreePlan = computed(() =>
  props.platform === 'grok' &&
  (normalizedPlanType.value === 'free' ||
    normalizedPlanType.value === 'basic' ||
    normalizedPlanType.value === 'xbasic')
)

const planIconName = computed<'bolt' | null>(() => {
  if (props.platform !== 'grok') return null
  // Paid Grok tiers (SuperGrok / Heavy) share the bolt mark; free uses GrokFreeIcon.
  if (
    normalizedPlanType.value === 'supergrok' ||
    normalizedPlanType.value === 'supergrokheavy' ||
    normalizedPlanType.value === 'heavy' ||
    normalizedPlanType.value.includes('heavy')
  ) {
    return 'bolt'
  }
  return null
})

// 平台由品牌图标区分；印章本身只用中性票面色，避免把 success/warning/danger 当装饰色
const platformClass = computed(() => 'bg-surface text-fg')

const typeClass = computed(() => 'bg-surface-sunken text-fg-muted')

// 套餐印章：free=灰、plus/SuperGrok=浅蓝底、team=蓝色描边、pro=警示色、异常=危险色，互相可分
const neutralPlanClass = 'border-border-strong bg-surface-sunken text-fg-muted'
const planBadgeClass = computed(() => {
  if (normalizedPlanType.value === 'abnormal') {
    return 'border-danger/60 bg-danger-weak text-danger-strong'
  }
  if (
    normalizedPlanType.value === 'free' ||
    normalizedPlanType.value === 'basic' ||
    normalizedPlanType.value === 'xbasic'
  ) {
    return neutralPlanClass
  }
  if (props.platform === 'grok' && normalizedPlanType.value) {
    // Heavy / SuperGrok Heavy → neutral
    if (normalizedPlanType.value.includes('heavy')) {
      return neutralPlanClass
    }
    // SuperGrok → accent
    if (normalizedPlanType.value.includes('supergrok')) {
      return 'border-accent/60 bg-accent-weak text-accent-strong'
    }
    // Any other non-free Grok plan (future tiers) → warning so it still stands out
    return 'border-warning/60 bg-warning-weak text-warning-strong'
  }
  if (normalizedPlanType.value === 'plus') {
    return 'border-accent/60 bg-accent-weak text-accent-strong'
  }
  if (normalizedPlanType.value === 'team' || normalizedPlanType.value === 'selfservebusinessprolite' ||
    (props.platform === 'openai' && ['selfservebusinessusagebased', 'business', 'enterprise', 'ent26', 'enterprisecbpautomation', 'enterprisecbpusagebased', 'edu', 'eduplus', 'edupro'].includes(normalizedPlanType.value))) {
    return 'border-accent bg-surface text-accent-strong'
  }
  if (
    normalizedPlanType.value === 'pro' ||
    normalizedPlanType.value === 'chatgptpro' ||
    normalizedPlanType.value === 'prolite' ||
    (props.platform === 'openai' && normalizedPlanType.value === 'promax')
  ) {
    return 'border-warning/60 bg-warning-weak text-warning-strong'
  }
  return 'border-border-strong bg-surface text-fg-muted'
})

// Subscription expiration label (non-free only)
const expiresLabel = computed(() => {
  if (!props.subscriptionExpiresAt || !props.planType) return ''
  if (
    normalizedPlanType.value === 'free' ||
    normalizedPlanType.value === 'basic' ||
    normalizedPlanType.value === 'xbasic'
  ) return ''
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

// Privacy badge — shows different states for OpenAI/Antigravity OAuth privacy setting
const privacyBadge = computed(() => {
  if (props.type !== 'oauth' || !props.privacyMode) return null
  // 支持 OpenAI 和 Antigravity 平台
  if (props.platform !== 'openai' && props.platform !== 'antigravity') return null

  const shieldCheck = 'M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z'
  const shieldX = 'M12 9v3.75m0-10.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285zM12 18h.008v.008H12V18z'
  switch (props.privacyMode) {
    // OpenAI states
    case 'training_off':
      return { label: 'Private', icon: shieldCheck, title: t('admin.accounts.privacyTrainingOff'), class: 'border-success/60 bg-success-weak text-success-strong' }
    case 'training_set_cf_blocked':
      return { label: 'CF', icon: shieldX, title: t('admin.accounts.privacyCfBlocked'), class: 'border-warning/60 bg-warning-weak text-warning-strong' }
    case 'training_set_failed':
      return { label: 'Fail', icon: shieldX, title: t('admin.accounts.privacyFailed'), class: 'border-danger/60 bg-danger-weak text-danger-strong' }
    // Antigravity states
    case 'privacy_set':
      return { label: 'Private', icon: shieldCheck, title: t('admin.accounts.privacyAntigravitySet'), class: 'border-success/60 bg-success-weak text-success-strong' }
    case 'privacy_set_failed':
      return { label: 'Fail', icon: shieldX, title: t('admin.accounts.privacyAntigravityFailed'), class: 'border-danger/60 bg-danger-weak text-danger-strong' }
    default:
      return null
  }
})
</script>
