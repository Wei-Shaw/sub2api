<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium transition-colors',
      badgeClass
    ]"
  >
    <!-- Platform logo -->
    <PlatformIcon v-if="platform" :platform="platform" size="sm" />
    <!-- Group name -->
    <span class="truncate">{{ name REDACTEDREDACTED</span>
    <!-- Right side label -->
    <span v-if="showLabel" :class="labelClass">
      <template v-if="hasCustomRate">
        <!-- 原倍率删除线 + 专属倍率高亮 -->
        <span class="line-through opacity-50 mr-0.5">{{ rateMultiplier REDACTEDREDACTEDx</span>
        <span class="font-bold">{{ userRateMultiplier REDACTEDREDACTEDx</span>
      </template>
      <template v-else>
        {{ labelText REDACTEDREDACTED
      </template>
    </span>
    <span v-if="hasPeakRate" :class="peakRateClass" :title="peakRateTitle">
      {{ peakRateText REDACTEDREDACTED
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { SubscriptionType, GroupPlatform REDACTED from '@/types'
import { useAppStore REDACTED from '@/stores/app'
import { formatPeakRateWindow, serverTimezoneLabel REDACTED from '@/utils/peak-rate'
import PlatformIcon from './PlatformIcon.vue'

interface Props {
  name: string
  platform?: GroupPlatform
  subscriptionType?: SubscriptionType
  rateMultiplier?: number
  userRateMultiplier?: number | null // 用户专属倍率
  peakRateEnabled?: boolean
  peakStart?: string
  peakEnd?: string
  peakRateMultiplier?: number
  showRate?: boolean
  daysRemaining?: number | null // 剩余天数（订阅类型时使用）
  /**
   * 订阅分组默认在右侧 label 展示"订阅"或剩余天数；
   * 开启后订阅分组也改为显示倍率（保留订阅主题色 label，配合可用渠道这类
   * 只关心费率、不关心有效期的场景）。
   */
  alwaysShowRate?: boolean
REDACTED

const props = withDefaults(defineProps<Props>(), {
  subscriptionType: 'standard',
  showRate: true,
  daysRemaining: null,
  userRateMultiplier: null,
  peakRateEnabled: false,
  alwaysShowRate: false
REDACTED)

const { t REDACTED = useI18n()

const isSubscription = computed(() => props.subscriptionType === 'subscription')

// 是否有专属倍率（且与默认倍率不同）
const hasCustomRate = computed(() => {
  return (
    props.userRateMultiplier !== null &&
    props.userRateMultiplier !== undefined &&
    props.rateMultiplier !== undefined &&
    props.userRateMultiplier !== props.rateMultiplier
  )
REDACTED)

const appStore = useAppStore()

const hasPeakRate = computed(() => {
  return Boolean(props.showRate && props.peakRateEnabled && props.peakStart && props.peakEnd)
REDACTED)

const peakRateText = computed(() => {
  return formatPeakRateWindow(
    {
      peak_rate_enabled: props.peakRateEnabled,
      peak_start: props.peakStart,
      peak_end: props.peakEnd,
      peak_rate_multiplier: props.peakRateMultiplier
    REDACTED,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
REDACTED)

const peakRateTitle = computed(() => {
  return t('common.peakRateTooltip', { window: peakRateText.value REDACTED)
REDACTED)

// 是否显示右侧标签
const showLabel = computed(() => {
  if (!props.showRate) return false
  // 订阅类型：显示天数或"订阅"
  if (isSubscription.value) return true
  // 标准类型：显示倍率（包括专属倍率）
  return props.rateMultiplier !== undefined || hasCustomRate.value
REDACTED)

// Label text
const labelText = computed(() => {
  const rateLabel = props.rateMultiplier !== undefined ? `${props.rateMultiplierREDACTEDx` : ''
  if (isSubscription.value && !props.alwaysShowRate) {
    // 如果有剩余天数，显示天数
    if (props.daysRemaining !== null && props.daysRemaining !== undefined) {
      if (props.daysRemaining <= 0) {
        return t('admin.users.expired')
      REDACTED
      return t('admin.users.daysRemaining', { days: props.daysRemaining REDACTED)
    REDACTED
    // 否则显示"订阅"
    return t('groups.subscription')
  REDACTED
  return rateLabel
REDACTED)

// Label style based on type and days remaining
const labelClass = computed(() => {
  const base = 'px-1.5 py-0.5 rounded text-[10px] font-semibold'

  if (!isSubscription.value) {
    // Standard: subtle background (不再为专属倍率使用不同的背景色)
    return `${baseREDACTED bg-black/10 dark:bg-white/10`
  REDACTED

  // 订阅类型：根据剩余天数显示不同颜色
  if (props.daysRemaining !== null && props.daysRemaining !== undefined) {
    if (props.daysRemaining <= 0 || props.daysRemaining <= 3) {
      // 已过期或紧急（<=3天）：红色
      return `${baseREDACTED bg-red-200/80 text-red-800 dark:bg-red-800/50 dark:text-red-300`
    REDACTED
    if (props.daysRemaining <= 7) {
      // 警告（<=7天）：橙色
      return `${baseREDACTED bg-amber-200/80 text-amber-800 dark:bg-amber-800/50 dark:text-amber-300`
    REDACTED
  REDACTED

  // 正常状态或无天数：根据平台显示主题色
  if (props.platform === 'anthropic') {
    return `${baseREDACTED bg-orange-200/60 text-orange-800 dark:bg-orange-800/40 dark:text-orange-300`
  REDACTED
  if (props.platform === 'openai') {
    return `${baseREDACTED bg-emerald-200/60 text-emerald-800 dark:bg-emerald-800/40 dark:text-emerald-300`
  REDACTED
  if (props.platform === 'gemini') {
    return `${baseREDACTED bg-blue-200/60 text-blue-800 dark:bg-blue-800/40 dark:text-blue-300`
  REDACTED
  if (props.platform === 'antigravity') {
    return `${baseREDACTED bg-purple-200/60 text-purple-800 dark:bg-purple-800/40 dark:text-purple-300`
  REDACTED
  if (props.platform === 'grok') {
    return `${baseREDACTED bg-zinc-300/70 text-zinc-800 dark:bg-zinc-700/60 dark:text-zinc-200`
  REDACTED
  return `${baseREDACTED bg-violet-200/60 text-violet-800 dark:bg-violet-800/40 dark:text-violet-300`
REDACTED)

const peakRateClass = computed(() => {
  return 'px-1.5 py-0.5 rounded text-[10px] font-semibold bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
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
  if (props.platform === 'gemini') {
    return isSubscription.value
      ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
      : 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-400'
  REDACTED
  if (props.platform === 'antigravity') {
    return isSubscription.value
      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
      : 'bg-fuchsia-50 text-fuchsia-700 dark:bg-fuchsia-900/20 dark:text-fuchsia-400'
  REDACTED
  if (props.platform === 'grok') {
    return isSubscription.value
      ? 'bg-zinc-200 text-zinc-800 dark:bg-zinc-700 dark:text-zinc-100'
      : 'bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200'
  REDACTED
  // Fallback: original colors
  return isSubscription.value
    ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
    : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
REDACTED)
</script>
