<template>
  <div class="inline-flex items-center overflow-hidden rounded-md text-xs font-medium">
    <!-- Platform part -->
    <span :class="['inline-flex items-center gap-1 px-2 py-1', platformClass]">
      <PlatformIcon :platform="platform" size="xs" />
      <span>{{ platformLabel REDACTEDREDACTED</span>
    </span>
    <!-- Type part -->
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
      <Icon v-else name="key" size="xs" />
      <span>{{ typeLabel REDACTEDREDACTED</span>
    </span>
    <!-- Plan type part (optional) -->
    <span v-if="planLabel" :class="['inline-flex items-center gap-1 px-1.5 py-1 border-l border-white/20', typeClass]">
      <span>{{ planLabel REDACTEDREDACTED</span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import type { AccountPlatform, AccountType REDACTED from '@/types'
import PlatformIcon from './PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  platform: AccountPlatform
  type: AccountType
  planType?: string
REDACTED

const props = defineProps<Props>()

const platformLabel = computed(() => {
  if (props.platform === 'anthropic') return 'Anthropic'
  if (props.platform === 'openai') return 'OpenAI'
  if (props.platform === 'antigravity') return 'Antigravity'
  if (props.platform === 'sora') return 'Sora'
  return 'Gemini'
REDACTED)

const typeLabel = computed(() => {
  switch (props.type) {
    case 'oauth':
      return 'OAuth'
    case 'setup-token':
      return 'Token'
    case 'apikey':
      return 'Key'
    default:
      return props.type
  REDACTED
REDACTED)

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
    default:
      return props.planType
  REDACTED
REDACTED)

const platformClass = computed(() => {
  if (props.platform === 'anthropic') {
    return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
  REDACTED
  if (props.platform === 'openai') {
    return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
  REDACTED
  if (props.platform === 'antigravity') {
    return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
  REDACTED
  if (props.platform === 'sora') {
    return 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-400'
  REDACTED
  return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
REDACTED)

const typeClass = computed(() => {
  if (props.platform === 'anthropic') {
    return 'bg-orange-100 text-orange-600 dark:bg-orange-900/30 dark:text-orange-400'
  REDACTED
  if (props.platform === 'openai') {
    return 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400'
  REDACTED
  if (props.platform === 'antigravity') {
    return 'bg-purple-100 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400'
  REDACTED
  if (props.platform === 'sora') {
    return 'bg-rose-100 text-rose-600 dark:bg-rose-900/30 dark:text-rose-400'
  REDACTED
  return 'bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
REDACTED)
</script>
