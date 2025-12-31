<template>
  <div v-if="shouldShowQuota" class="flex items-center gap-2">
    <!-- Tier Badge -->
    <span :class="['badge text-xs px-2 py-0.5 rounded font-medium', tierBadgeClass]">
      {{ tierLabel REDACTEDREDACTED
    </span>

    <!-- 限额文本 -->
    <span class="text-xs text-gray-500 dark:text-gray-400">{{ quotaText REDACTEDREDACTED</span>

    <!-- 二元进度条 -->
    <div class="group/progress relative flex items-center gap-1">
      <div class="h-2 w-20 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
        <div
          :class="[
            'h-full transition-all',
            isRateLimited ? 'bg-red-500' : 'bg-gray-300 dark:bg-gray-600'
          ]"
          :style="{ width: isRateLimited ? '100%' : '0%' REDACTED"
        ></div>
      </div>

      <span
        :class="[
          'text-xs font-medium',
          isRateLimited ? 'text-red-600 dark:text-red-400' : 'text-gray-400 dark:text-gray-500'
        ]"
      >
        {{ isRateLimited ? '100%' : '0%' REDACTEDREDACTED
      </span>

      <!-- 倒计时 -->
      <span
        v-if="isRateLimited"
        :class="[
          'text-xs font-medium',
          isUrgent
            ? 'text-red-600 dark:text-red-400 animate-pulse'
            : 'text-amber-600 dark:text-amber-400'
        ]"
      >
        ⚠️ {{ resetCountdown REDACTEDREDACTED
      </span>

      <!-- Tooltip -->
      <div
        class="pointer-events-none absolute -top-10 left-0 z-10 hidden whitespace-nowrap rounded bg-gray-900 px-2 py-1 text-xs text-white shadow-lg group-hover/progress:block dark:bg-gray-700"
      >
        ⓘ 无法提供实时额度
        <div
          class="absolute left-4 top-full border-4 border-transparent border-t-gray-900 dark:border-t-gray-700"
        ></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onUnmounted REDACTED from 'vue'
import type { Account, GeminiCredentials REDACTED from '@/types'

const props = defineProps<{
  account: Account
REDACTED>()

const now = ref(new Date())
let timer: ReturnType<typeof setInterval> | null = null

// 是否为 Code Assist OAuth
// 判断逻辑与后端保持一致：project_id 存在即为 Code Assist
const isCodeAssist = computed(() => {
  const creds = props.account.credentials as GeminiCredentials | undefined
  // 显式为 code_assist，或 legacy 情况（oauth_type 为空但 project_id 存在）
  return creds?.oauth_type === 'code_assist' || (!creds?.oauth_type && !!creds?.project_id)
REDACTED)

// 是否应该显示配额信息
const shouldShowQuota = computed(() => {
  return props.account.platform === 'gemini'
REDACTED)

// Tier 标签文本
const tierLabel = computed(() => {
  if (isCodeAssist.value) {
    const creds = props.account.credentials as GeminiCredentials | undefined
    const tierMap: Record<string, string> = {
      LEGACY: 'Free',
      PRO: 'Standard',
      ULTRA: 'Enterprise'
    REDACTED
    return tierMap[creds?.tier_id || ''] || 'Unknown'
  REDACTED
  return 'Gemini'
REDACTED)

// Tier Badge 样式
const tierBadgeClass = computed(() => {
  if (!isCodeAssist.value) {
    return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
  REDACTED
  const creds = props.account.credentials as GeminiCredentials | undefined
  const tierColorMap: Record<string, string> = {
    LEGACY: 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-400',
    PRO: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
    ULTRA: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
  REDACTED
  return (
    tierColorMap[creds?.tier_id || ''] ||
    'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-400'
  )
REDACTED)

// 限额文本
const quotaText = computed(() => {
  if (isCodeAssist.value) {
    const creds = props.account.credentials as GeminiCredentials | undefined
    const limitMap: Record<string, string> = {
      LEGACY: '1000/day, 60/min',
      PRO: '1500/day, 120/min',
      ULTRA: '5000/day, 300/min'
    REDACTED
    return limitMap[creds?.tier_id || ''] || '-'
  REDACTED
  return 'RPM/RPD limits'
REDACTED)

// 是否限流
const isRateLimited = computed(() => {
  if (!props.account.rate_limit_reset_at) return false
  const resetTime = Date.parse(props.account.rate_limit_reset_at)
  // 防护：如果日期解析失败（NaN），则认为未限流
  if (Number.isNaN(resetTime)) return false
  return resetTime > now.value.getTime()
REDACTED)

// 倒计时文本
const resetCountdown = computed(() => {
  if (!props.account.rate_limit_reset_at) return ''
  const resetTime = Date.parse(props.account.rate_limit_reset_at)
  // 防护：如果日期解析失败，显示 "-"
  if (Number.isNaN(resetTime)) return '-'

  const diffMs = resetTime - now.value.getTime()
  if (diffMs <= 0) return 'now'

  const diffSeconds = Math.floor(diffMs / 1000)
  const diffMinutes = Math.floor(diffSeconds / 60)
  const diffHours = Math.floor(diffMinutes / 60)

  if (diffMinutes < 1) return `${diffSecondsREDACTEDs`
  if (diffHours < 1) {
    const secs = diffSeconds % 60
    return `${diffMinutesREDACTEDm ${secsREDACTEDs`
  REDACTED
  const mins = diffMinutes % 60
  return `${diffHoursREDACTEDh ${minsREDACTEDm`
REDACTED)

// 是否紧急（< 1分钟）
const isUrgent = computed(() => {
  if (!props.account.rate_limit_reset_at) return false
  const resetTime = Date.parse(props.account.rate_limit_reset_at)
  // 防护：如果日期解析失败，返回 false
  if (Number.isNaN(resetTime)) return false

  const diffMs = resetTime - now.value.getTime()
  return diffMs > 0 && diffMs < 60000
REDACTED)

// 监听限流状态，动态启动/停止定时器
watch(
  () => isRateLimited.value,
  (limited) => {
    if (limited && !timer) {
      // 进入限流状态，启动定时器
      timer = setInterval(() => {
        now.value = new Date()
      REDACTED, 1000)
    REDACTED else if (!limited && timer) {
      // 解除限流，停止定时器
      clearInterval(timer)
      timer = null
    REDACTED
  REDACTED,
  { immediate: true REDACTED // 立即执行，确保挂载时已限流的情况也能启动定时器
)

onUnmounted(() => {
  if (timer !== null) {
    clearInterval(timer)
    timer = null
  REDACTED
REDACTED)
</script>
