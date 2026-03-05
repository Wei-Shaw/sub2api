<template>
  <div class="flex flex-col gap-1.5">
    <!-- 并发槽位 -->
    <div class="flex items-center gap-1.5">
      <span
        :class="[
          'inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium',
          concurrencyClass
        ]"
      >
        <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" />
        </svg>
        <span class="font-mono">{{ currentConcurrency REDACTEDREDACTED</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">{{ account.concurrency REDACTEDREDACTED</span>
      </span>
    </div>

    <!-- 5h窗口费用限制（仅 Anthropic OAuth/SetupToken 且启用时显示） -->
    <div v-if="showWindowCost" class="flex items-center gap-1">
      <span
        :class="[
          'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium',
          windowCostClass
        ]"
        :title="windowCostTooltip"
      >
        <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v12m-3-2.818l.879.659c1.171.879 3.07.879 4.242 0 1.172-.879 1.172-2.303 0-3.182C13.536 12.219 12.768 12 12 12c-.725 0-1.45-.22-2.003-.659-1.106-.879-1.106-2.303 0-3.182s2.9-.879 4.006 0l.415.33M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <span class="font-mono">${{ formatCost(currentWindowCost) REDACTEDREDACTED</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">${{ formatCost(account.window_cost_limit) REDACTEDREDACTED</span>
      </span>
    </div>

    <!-- 会话数量限制（仅 Anthropic OAuth/SetupToken 且启用时显示） -->
    <div v-if="showSessionLimit" class="flex items-center gap-1">
      <span
        :class="[
          'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium',
          sessionLimitClass
        ]"
        :title="sessionLimitTooltip"
      >
        <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" />
        </svg>
        <span class="font-mono">{{ activeSessions REDACTEDREDACTED</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">{{ account.max_sessions REDACTEDREDACTED</span>
      </span>
    </div>

    <!-- RPM 限制（仅 Anthropic OAuth/SetupToken 且启用时显示） -->
    <div v-if="showRpmLimit" class="flex items-center gap-1">
      <span
        :class="[
          'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium',
          rpmClass
        ]"
        :title="rpmTooltip"
      >
        <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
        </svg>
        <span class="font-mono">{{ currentRPM REDACTEDREDACTED</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">{{ account.base_rpm REDACTEDREDACTED</span>
        <span class="text-[9px] opacity-60">{{ rpmStrategyTag REDACTEDREDACTED</span>
      </span>
    </div>

    <!-- API Key 账号配额限制 -->
    <div v-if="showQuotaLimit" class="flex items-center gap-1">
      <span
        :class="[
          'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium',
          quotaClass
        ]"
        :title="quotaTooltip"
      >
        <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z" />
        </svg>
        <span class="font-mono">${{ formatCost(currentQuotaUsed) REDACTEDREDACTED</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">${{ formatCost(account.quota_limit) REDACTEDREDACTED</span>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { Account REDACTED from '@/types'

const props = defineProps<{
  account: Account
REDACTED>()

const { t REDACTED = useI18n()

// 当前并发数
const currentConcurrency = computed(() => props.account.current_concurrency || 0)

// 是否为 Anthropic OAuth/SetupToken 账号
const isAnthropicOAuthOrSetupToken = computed(() => {
  return (
    props.account.platform === 'anthropic' &&
    (props.account.type === 'oauth' || props.account.type === 'setup-token')
  )
REDACTED)

// 是否显示窗口费用限制
const showWindowCost = computed(() => {
  return (
    isAnthropicOAuthOrSetupToken.value &&
    props.account.window_cost_limit !== undefined &&
    props.account.window_cost_limit !== null &&
    props.account.window_cost_limit > 0
  )
REDACTED)

// 当前窗口费用
const currentWindowCost = computed(() => props.account.current_window_cost ?? 0)

// 是否显示会话限制
const showSessionLimit = computed(() => {
  return (
    isAnthropicOAuthOrSetupToken.value &&
    props.account.max_sessions !== undefined &&
    props.account.max_sessions !== null &&
    props.account.max_sessions > 0
  )
REDACTED)

// 当前活跃会话数
const activeSessions = computed(() => props.account.active_sessions ?? 0)

// 并发状态样式
const concurrencyClass = computed(() => {
  const current = currentConcurrency.value
  const max = props.account.concurrency

  if (current >= max) {
    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  REDACTED
  if (current > 0) {
    return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  REDACTED
  return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
REDACTED)

// 窗口费用状态样式
const windowCostClass = computed(() => {
  if (!showWindowCost.value) return ''

  const current = currentWindowCost.value
  const limit = props.account.window_cost_limit || 0
  const reserve = props.account.window_cost_sticky_reserve || 10

  if (current >= limit + reserve) {
    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  REDACTED
  if (current >= limit) {
    return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
  REDACTED
  if (current >= limit * 0.8) {
    return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  REDACTED
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
REDACTED)

// 窗口费用提示文字
const windowCostTooltip = computed(() => {
  if (!showWindowCost.value) return ''

  const current = currentWindowCost.value
  const limit = props.account.window_cost_limit || 0
  const reserve = props.account.window_cost_sticky_reserve || 10

  if (current >= limit + reserve) {
    return t('admin.accounts.capacity.windowCost.blocked')
  REDACTED
  if (current >= limit) {
    return t('admin.accounts.capacity.windowCost.stickyOnly')
  REDACTED
  return t('admin.accounts.capacity.windowCost.normal')
REDACTED)

// 会话限制状态样式
const sessionLimitClass = computed(() => {
  if (!showSessionLimit.value) return ''

  const current = activeSessions.value
  const max = props.account.max_sessions || 0

  if (current >= max) {
    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  REDACTED
  if (current >= max * 0.8) {
    return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  REDACTED
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
REDACTED)

// 会话限制提示文字
const sessionLimitTooltip = computed(() => {
  if (!showSessionLimit.value) return ''

  const current = activeSessions.value
  const max = props.account.max_sessions || 0
  const idle = props.account.session_idle_timeout_minutes || 5

  if (current >= max) {
    return t('admin.accounts.capacity.sessions.full', { idle REDACTED)
  REDACTED
  return t('admin.accounts.capacity.sessions.normal', { idle REDACTED)
REDACTED)

// 是否显示 RPM 限制
const showRpmLimit = computed(() => {
  return (
    isAnthropicOAuthOrSetupToken.value &&
    props.account.base_rpm !== undefined &&
    props.account.base_rpm !== null &&
    props.account.base_rpm > 0
  )
REDACTED)

// 当前 RPM 计数
const currentRPM = computed(() => props.account.current_rpm ?? 0)

// RPM 策略
const rpmStrategy = computed(() => props.account.rpm_strategy || 'tiered')

// RPM 策略标签
const rpmStrategyTag = computed(() => {
  return rpmStrategy.value === 'sticky_exempt' ? '[S]' : '[T]'
REDACTED)

// RPM buffer 计算（与后端一致：base <= 0 时 buffer 为 0）
const rpmBuffer = computed(() => {
  const base = props.account.base_rpm || 0
  return props.account.rpm_sticky_buffer ?? (base > 0 ? Math.max(1, Math.floor(base / 5)) : 0)
REDACTED)

// RPM 状态样式
const rpmClass = computed(() => {
  if (!showRpmLimit.value) return ''

  const current = currentRPM.value
  const base = props.account.base_rpm ?? 0
  const buffer = rpmBuffer.value

  if (rpmStrategy.value === 'tiered') {
    if (current >= base + buffer) {
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
    REDACTED
    if (current >= base) {
      return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
    REDACTED
  REDACTED else {
    if (current >= base) {
      return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
    REDACTED
  REDACTED
  if (current >= base * 0.8) {
    return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  REDACTED
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
REDACTED)

// RPM 提示文字（增强版：显示策略、区域、缓冲区）
const rpmTooltip = computed(() => {
  if (!showRpmLimit.value) return ''

  const current = currentRPM.value
  const base = props.account.base_rpm ?? 0
  const buffer = rpmBuffer.value

  if (rpmStrategy.value === 'tiered') {
    if (current >= base + buffer) {
      return t('admin.accounts.capacity.rpm.tieredBlocked', { buffer REDACTED)
    REDACTED
    if (current >= base) {
      return t('admin.accounts.capacity.rpm.tieredStickyOnly', { buffer REDACTED)
    REDACTED
    if (current >= base * 0.8) {
      return t('admin.accounts.capacity.rpm.tieredWarning')
    REDACTED
    return t('admin.accounts.capacity.rpm.tieredNormal')
  REDACTED else {
    if (current >= base) {
      return t('admin.accounts.capacity.rpm.stickyExemptOver')
    REDACTED
    if (current >= base * 0.8) {
      return t('admin.accounts.capacity.rpm.stickyExemptWarning')
    REDACTED
    return t('admin.accounts.capacity.rpm.stickyExemptNormal')
  REDACTED
REDACTED)

// 是否显示配额限制（仅 apikey 类型且设置了 quota_limit）
const showQuotaLimit = computed(() => {
  return (
    props.account.type === 'apikey' &&
    props.account.quota_limit !== undefined &&
    props.account.quota_limit !== null &&
    props.account.quota_limit > 0
  )
REDACTED)

// 当前已用配额
const currentQuotaUsed = computed(() => props.account.quota_used ?? 0)

// 配额状态样式
const quotaClass = computed(() => {
  if (!showQuotaLimit.value) return ''

  const used = currentQuotaUsed.value
  const limit = props.account.quota_limit || 0

  if (used >= limit) {
    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  REDACTED
  if (used >= limit * 0.8) {
    return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  REDACTED
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
REDACTED)

// 配额提示文字
const quotaTooltip = computed(() => {
  if (!showQuotaLimit.value) return ''

  const used = currentQuotaUsed.value
  const limit = props.account.quota_limit || 0

  if (used >= limit) {
    return t('admin.accounts.capacity.quota.exceeded')
  REDACTED
  return t('admin.accounts.capacity.quota.normal')
REDACTED)

// 格式化费用显示
const formatCost = (value: number | null | undefined) => {
  if (value === null || value === undefined) return '0'
  return value.toFixed(2)
REDACTED
</script>
