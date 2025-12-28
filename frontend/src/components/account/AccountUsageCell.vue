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
        {{ error REDACTEDREDACTED
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

    <!-- Antigravity OAuth accounts: show quota from extra field -->
    <template v-else-if="account.platform === 'antigravity' && account.type === 'oauth'">
      <div v-if="hasAntigravityQuota" class="space-y-1">
        <!-- Gemini 3 Pro -->
        <UsageProgressBar
          v-if="antigravity3ProUsage !== null"
          :label="t('admin.accounts.usageWindow.gemini3Pro')"
          :utilization="antigravity3ProUsage.utilization"
          :resets-at="antigravity3ProUsage.resetTime"
          color="indigo"
        />

        <!-- Gemini 3 Flash -->
        <UsageProgressBar
          v-if="antigravity3FlashUsage !== null"
          :label="t('admin.accounts.usageWindow.gemini3Flash')"
          :utilization="antigravity3FlashUsage.utilization"
          :resets-at="antigravity3FlashUsage.resetTime"
          color="emerald"
        />

        <!-- Gemini 3 Image -->
        <UsageProgressBar
          v-if="antigravity3ImageUsage !== null"
          :label="t('admin.accounts.usageWindow.gemini3Image')"
          :utilization="antigravity3ImageUsage.utilization"
          :resets-at="antigravity3ImageUsage.resetTime"
          color="purple"
        />

        <!-- Claude 4.5 -->
        <UsageProgressBar
          v-if="antigravityClaude45Usage !== null"
          :label="t('admin.accounts.usageWindow.claude45')"
          :utilization="antigravityClaude45Usage.utilization"
          :resets-at="antigravityClaude45Usage.resetTime"
          color="amber"
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
import { ref, computed, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { adminAPI REDACTED from '@/api/admin'
import type { Account, AccountUsageInfo REDACTED from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{
  account: Account
REDACTED>()

const { t REDACTED = useI18n()

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
REDACTED)

// 5h window usage (prefer canonical field)
const codex5hUsedPercent = computed(() => {
  const extra = props.account.extra
  if (!extra) return null

  // Prefer canonical field
  if (extra.codex_5h_used_percent !== undefined) {
    return extra.codex_5h_used_percent
  REDACTED

  // Fallback: detect from legacy fields using window_minutes
  if (
    extra.codex_primary_window_minutes !== undefined &&
    extra.codex_primary_window_minutes <= 360
  ) {
    return extra.codex_primary_used_percent ?? null
  REDACTED
  if (
    extra.codex_secondary_window_minutes !== undefined &&
    extra.codex_secondary_window_minutes <= 360
  ) {
    return extra.codex_secondary_used_percent ?? null
  REDACTED

  // Legacy assumption: secondary = 5h (may be incorrect)
  return extra.codex_secondary_used_percent ?? null
REDACTED)

const codex5hResetAt = computed(() => {
  const extra = props.account.extra
  if (!extra) return null

  // Prefer canonical field
  if (extra.codex_5h_reset_after_seconds !== undefined) {
    const resetTime = new Date(Date.now() + extra.codex_5h_reset_after_seconds * 1000)
    return resetTime.toISOString()
  REDACTED

  // Fallback: detect from legacy fields using window_minutes
  if (
    extra.codex_primary_window_minutes !== undefined &&
    extra.codex_primary_window_minutes <= 360
  ) {
    if (extra.codex_primary_reset_after_seconds !== undefined) {
      const resetTime = new Date(Date.now() + extra.codex_primary_reset_after_seconds * 1000)
      return resetTime.toISOString()
    REDACTED
  REDACTED
  if (
    extra.codex_secondary_window_minutes !== undefined &&
    extra.codex_secondary_window_minutes <= 360
  ) {
    if (extra.codex_secondary_reset_after_seconds !== undefined) {
      const resetTime = new Date(Date.now() + extra.codex_secondary_reset_after_seconds * 1000)
      return resetTime.toISOString()
    REDACTED
  REDACTED

  // Legacy assumption: secondary = 5h
  if (extra.codex_secondary_reset_after_seconds !== undefined) {
    const resetTime = new Date(Date.now() + extra.codex_secondary_reset_after_seconds * 1000)
    return resetTime.toISOString()
  REDACTED

  return null
REDACTED)

// 7d window usage (prefer canonical field)
const codex7dUsedPercent = computed(() => {
  const extra = props.account.extra
  if (!extra) return null

  // Prefer canonical field
  if (extra.codex_7d_used_percent !== undefined) {
    return extra.codex_7d_used_percent
  REDACTED

  // Fallback: detect from legacy fields using window_minutes
  if (
    extra.codex_primary_window_minutes !== undefined &&
    extra.codex_primary_window_minutes >= 10000
  ) {
    return extra.codex_primary_used_percent ?? null
  REDACTED
  if (
    extra.codex_secondary_window_minutes !== undefined &&
    extra.codex_secondary_window_minutes >= 10000
  ) {
    return extra.codex_secondary_used_percent ?? null
  REDACTED

  // Legacy assumption: primary = 7d (may be incorrect)
  return extra.codex_primary_used_percent ?? null
REDACTED)

const codex7dResetAt = computed(() => {
  const extra = props.account.extra
  if (!extra) return null

  // Prefer canonical field
  if (extra.codex_7d_reset_after_seconds !== undefined) {
    const resetTime = new Date(Date.now() + extra.codex_7d_reset_after_seconds * 1000)
    return resetTime.toISOString()
  REDACTED

  // Fallback: detect from legacy fields using window_minutes
  if (
    extra.codex_primary_window_minutes !== undefined &&
    extra.codex_primary_window_minutes >= 10000
  ) {
    if (extra.codex_primary_reset_after_seconds !== undefined) {
      const resetTime = new Date(Date.now() + extra.codex_primary_reset_after_seconds * 1000)
      return resetTime.toISOString()
    REDACTED
  REDACTED
  if (
    extra.codex_secondary_window_minutes !== undefined &&
    extra.codex_secondary_window_minutes >= 10000
  ) {
    if (extra.codex_secondary_reset_after_seconds !== undefined) {
      const resetTime = new Date(Date.now() + extra.codex_secondary_reset_after_seconds * 1000)
      return resetTime.toISOString()
    REDACTED
  REDACTED

  // Legacy assumption: primary = 7d
  if (extra.codex_primary_reset_after_seconds !== undefined) {
    const resetTime = new Date(Date.now() + extra.codex_primary_reset_after_seconds * 1000)
    return resetTime.toISOString()
  REDACTED

  return null
REDACTED)

// Antigravity quota types
interface AntigravityModelQuota {
  remaining: number // 剩余百分比 0-100
  reset_time: string // ISO 8601 重置时间
REDACTED

interface AntigravityQuotaData {
  [model: string]: AntigravityModelQuota
REDACTED

interface AntigravityUsageResult {
  utilization: number
  resetTime: string | null
REDACTED

// Antigravity quota computed properties
const hasAntigravityQuota = computed(() => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  return extra && typeof extra.quota === 'object' && extra.quota !== null
REDACTED)

// 从配额数据中获取使用率（多模型取最低剩余 = 最高使用）
const getAntigravityUsage = (
  modelNames: string[]
): AntigravityUsageResult | null => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (!extra || typeof extra.quota !== 'object' || extra.quota === null) return null

  const quota = extra.quota as AntigravityQuotaData

  let minRemaining = 100
  let earliestReset: string | null = null

  for (const model of modelNames) {
    const modelQuota = quota[model]
    if (!modelQuota) continue

    if (modelQuota.remaining < minRemaining) {
      minRemaining = modelQuota.remaining
    REDACTED
    if (modelQuota.reset_time) {
      if (!earliestReset || modelQuota.reset_time < earliestReset) {
        earliestReset = modelQuota.reset_time
      REDACTED
    REDACTED
  REDACTED

  // 如果没有找到任何匹配的模型
  if (minRemaining === 100 && earliestReset === null) {
    // 检查是否至少有一个模型有数据
    const hasAnyData = modelNames.some((m) => quota[m])
    if (!hasAnyData) return null
  REDACTED

  return {
    utilization: 100 - minRemaining,
    resetTime: earliestReset
  REDACTED
REDACTED

// Gemini 3 Pro: gemini-3-pro-low, gemini-3-pro-high, gemini-3-pro-preview
const antigravity3ProUsage = computed(() =>
  getAntigravityUsage(['gemini-3-pro-low', 'gemini-3-pro-high', 'gemini-3-pro-preview'])
)

// Gemini 3 Flash: gemini-3-flash
const antigravity3FlashUsage = computed(() => getAntigravityUsage(['gemini-3-flash']))

// Gemini 3 Image: gemini-3-pro-image
const antigravity3ImageUsage = computed(() => getAntigravityUsage(['gemini-3-pro-image']))

// Claude 4.5: claude-sonnet-4-5, claude-opus-4-5-thinking
const antigravityClaude45Usage = computed(() =>
  getAntigravityUsage(['claude-sonnet-4-5', 'claude-opus-4-5-thinking'])
)

const loadUsage = async () => {
  // Fetch usage for Anthropic OAuth and Setup Token accounts
  // OpenAI usage comes from account.extra field (updated during forwarding)
  if (props.account.platform !== 'anthropic') return
  if (props.account.type !== 'oauth' && props.account.type !== 'setup-token') return

  loading.value = true
  error.value = null

  try {
    usageInfo.value = await adminAPI.accounts.getUsage(props.account.id)
  REDACTED catch (e: any) {
    error.value = t('common.error')
    console.error('Failed to load usage:', e)
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

onMounted(() => {
  loadUsage()
REDACTED)
</script>
