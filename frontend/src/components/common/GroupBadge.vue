<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-xs font-medium transition-colors',
      badgeClass
    ]"
  >
    <!-- Platform logo -->
    <PlatformIcon v-if="platform" :platform="platform" size="sm" />
    <!-- Group name -->
    <span class="truncate">{{ name REDACTEDREDACTED</span>
    <!-- Right side label: subscription shows "订阅", standard shows rate multiplier -->
    <span
      v-if="showRate"
      :class="labelClass"
    >
      {{ labelText REDACTEDREDACTED
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { SubscriptionType, GroupPlatform REDACTED from '@/types'
import PlatformIcon from './PlatformIcon.vue'

interface Props {
  name: string
  platform?: GroupPlatform
  subscriptionType?: SubscriptionType
  rateMultiplier?: number
  showRate?: boolean
REDACTED

const props = withDefaults(defineProps<Props>(), {
  subscriptionType: 'standard',
  showRate: true
REDACTED)

const { t REDACTED = useI18n()

const isSubscription = computed(() => props.subscriptionType === 'subscription')

// Label text: subscription shows localized text, standard shows rate
const labelText = computed(() => {
  if (isSubscription.value) {
    return t('groups.subscription')
  REDACTED
  return props.rateMultiplier !== undefined ? `${props.rateMultiplierREDACTEDx` : ''
REDACTED)

// Label style based on type
const labelClass = computed(() => {
  const base = 'px-1.5 py-0.5 rounded text-[10px] font-semibold'
  if (isSubscription.value) {
    // Subscription: more prominent style with border
    if (props.platform === 'anthropic') {
      return `${baseREDACTED bg-orange-200/60 text-orange-800 dark:bg-orange-800/40 dark:text-orange-300`
    REDACTED else if (props.platform === 'openai') {
      return `${baseREDACTED bg-emerald-200/60 text-emerald-800 dark:bg-emerald-800/40 dark:text-emerald-300`
    REDACTED
    return `${baseREDACTED bg-violet-200/60 text-violet-800 dark:bg-violet-800/40 dark:text-violet-300`
  REDACTED
  // Standard: subtle background
  return `${baseREDACTED bg-black/10 dark:bg-white/10`
REDACTED)

// Badge color based on platform and subscription type
const badgeClass = computed(() => {
  if (props.platform === 'anthropic') {
    // Claude: orange theme
    return isSubscription.value
      ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
      : 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400'
  REDACTED else if (props.platform === 'openai') {
    // OpenAI: green theme
    return isSubscription.value
      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
      : 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400'
  REDACTED
  // Fallback: original colors
  return isSubscription.value
    ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
    : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
REDACTED)
</script>
