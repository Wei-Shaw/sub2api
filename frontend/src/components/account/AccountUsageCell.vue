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
      <!-- 账户类型徽章 -->
      <div v-if="antigravityTierLabel" class="mb-1 flex items-center gap-1">
        <span
          :class="[
            'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
            antigravityTierClass
          ]"
        >
          {{ antigravityTierLabel REDACTEDREDACTED
        </span>
        <!-- 不合格账户警告图标 -->
        <span
          v-if="hasIneligibleTiers"
          class="group relative cursor-help"
        >
          <svg
            class="h-3.5 w-3.5 text-red-500"
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
            {{ t('admin.accounts.ineligibleWarning') REDACTEDREDACTED
          </span>
        </span>
      </div>

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

    <!-- Gemini platform: show quota + local usage window -->
    <template v-else-if="account.platform === 'gemini'">
      <!-- 账户类型徽章 -->
      <div v-if="geminiTierLabel" class="mb-1 flex items-center gap-1">
        <span
          :class="[
            'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
            geminiTierClass
          ]"
        >
          {{ geminiTierLabel REDACTEDREDACTED
        </span>
        <!-- 帮助图标 -->
        <span
          class="group relative cursor-help"
        >
          <svg
            class="h-3.5 w-3.5 text-gray-400 hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-8-3a1 1 0 00-.867.5 1 1 0 11-1.731-1A3 3 0 0113 8a3.001 3.001 0 01-2 2.83V11a1 1 0 11-2 0v-1a1 1 0 011-1 1 1 0 100-2zm0 8a1 1 0 100-2 1 1 0 000 2z"
              clip-rule="evenodd"
            />
          </svg>
          <span
            class="pointer-events-none absolute left-0 top-full z-50 mt-1 w-80 whitespace-normal break-words rounded bg-gray-900 px-3 py-2 text-xs leading-relaxed text-white opacity-0 shadow-lg transition-opacity group-hover:opacity-100 dark:bg-gray-700"
          >
            <div class="font-semibold mb-1">{{ t('admin.accounts.gemini.quotaPolicy.title') REDACTEDREDACTED</div>
            <div class="mb-2 text-gray-300">{{ t('admin.accounts.gemini.quotaPolicy.note') REDACTEDREDACTED</div>
            <div class="space-y-1">
              <div><strong>{{ geminiQuotaPolicyChannel REDACTEDREDACTED:</strong></div>
              <div class="pl-2">• {{ geminiQuotaPolicyLimits REDACTEDREDACTED</div>
              <div class="mt-2">
                <a :href="geminiQuotaPolicyDocsUrl" target="_blank" class="text-blue-400 hover:text-blue-300 underline">
                  {{ t('admin.accounts.gemini.quotaPolicy.columns.docs') REDACTEDREDACTED →
                </a>
              </div>
            </div>
          </span>
        </span>
      </div>

      <div class="space-y-1">
        <div v-if="loading" class="space-y-1">
          <div class="flex items-center gap-1">
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          </div>
        </div>
        <div v-else-if="error" class="text-xs text-red-500">
          {{ error REDACTEDREDACTED
        </div>
        <div v-else-if="geminiUsageAvailable" class="space-y-1">
          <UsageProgressBar
            v-if="usageInfo?.gemini_pro_daily"
            :label="t('admin.accounts.usageWindow.geminiProDaily')"
            :utilization="usageInfo.gemini_pro_daily.utilization"
            :resets-at="usageInfo.gemini_pro_daily.resets_at"
            :window-stats="usageInfo.gemini_pro_daily.window_stats"
            :stats-title="t('admin.accounts.usageWindow.statsTitleDaily')"
            color="indigo"
          />
          <UsageProgressBar
            v-if="usageInfo?.gemini_flash_daily"
            :label="t('admin.accounts.usageWindow.geminiFlashDaily')"
            :utilization="usageInfo.gemini_flash_daily.utilization"
            :resets-at="usageInfo.gemini_flash_daily.resets_at"
            :window-stats="usageInfo.gemini_flash_daily.window_stats"
            :stats-title="t('admin.accounts.usageWindow.statsTitleDaily')"
            color="emerald"
          />
          <p class="mt-1 text-[9px] leading-tight text-gray-400 dark:text-gray-500 italic">
            * {{ t('admin.accounts.gemini.quotaPolicy.simulatedNote') || 'Simulated quota' REDACTEDREDACTED
          </p>
        </div>
      </div>
    </template>

    <!-- Other accounts: no usage window -->
    <template v-else>
      <div class="text-xs text-gray-400">-</div>
    </template>
  </div>

  <!-- Non-OAuth/Setup-Token accounts -->
  <div v-else>
    <!-- Gemini API Key accounts: show quota info -->
    <AccountQuotaInfo v-if="account.platform === 'gemini'" :account="account" />
    <div v-else class="text-xs text-gray-400">-</div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { adminAPI REDACTED from '@/api/admin'
import type { Account, AccountUsageInfo, GeminiCredentials REDACTED from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'
import AccountQuotaInfo from './AccountQuotaInfo.vue'

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

const shouldFetchUsage = computed(() => {
  if (props.account.platform === 'anthropic') {
    return props.account.type === 'oauth' || props.account.type === 'setup-token'
  REDACTED
  if (props.account.platform === 'gemini') {
    return props.account.type === 'oauth'
  REDACTED
  return false
REDACTED)

const geminiUsageAvailable = computed(() => {
  return (
    !!usageInfo.value?.gemini_pro_daily ||
    !!usageInfo.value?.gemini_flash_daily
  )
REDACTED)

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

// Antigravity 账户类型（从 load_code_assist 响应中提取）
const antigravityTier = computed(() => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (!extra) return null

  const loadCodeAssist = extra.load_code_assist as Record<string, unknown> | undefined
  if (!loadCodeAssist) return null

  // 优先取 paidTier，否则取 currentTier
  const paidTier = loadCodeAssist.paidTier as Record<string, unknown> | undefined
  if (paidTier && typeof paidTier.id === 'string') {
    return paidTier.id
  REDACTED

  const currentTier = loadCodeAssist.currentTier as Record<string, unknown> | undefined
  if (currentTier && typeof currentTier.id === 'string') {
    return currentTier.id
  REDACTED

  return null
REDACTED)

// Gemini 账户类型（从 credentials 中提取）
const geminiTier = computed(() => {
  if (props.account.platform !== 'gemini') return null
  const creds = props.account.credentials as GeminiCredentials | undefined
  return creds?.tier_id || null
REDACTED)

// Gemini 是否为 Code Assist OAuth
const isGeminiCodeAssist = computed(() => {
  if (props.account.platform !== 'gemini') return false
  const creds = props.account.credentials as GeminiCredentials | undefined
  return creds?.oauth_type === 'code_assist' || (!creds?.oauth_type && !!creds?.project_id)
REDACTED)

// Gemini 账户类型显示标签
const geminiTierLabel = computed(() => {
  if (!geminiTier.value) return null
  const tierMap: Record<string, string> = {
    LEGACY: t('admin.accounts.tier.free'),
    PRO: t('admin.accounts.tier.pro'),
    ULTRA: t('admin.accounts.tier.ultra')
  REDACTED
  return tierMap[geminiTier.value] || null
REDACTED)

// Gemini 账户类型徽章样式
const geminiTierClass = computed(() => {
  switch (geminiTier.value) {
    case 'LEGACY':
      return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
    case 'PRO':
      return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    case 'ULTRA':
      return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    default:
      return ''
  REDACTED
REDACTED)

// Gemini 配额政策信息
const geminiQuotaPolicyChannel = computed(() => {
  if (isGeminiCodeAssist.value) {
    return t('admin.accounts.gemini.quotaPolicy.rows.cli.channel')
  REDACTED
  return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.channel')
REDACTED)

const geminiQuotaPolicyLimits = computed(() => {
  if (isGeminiCodeAssist.value) {
    if (geminiTier.value === 'PRO' || geminiTier.value === 'ULTRA') {
      return t('admin.accounts.gemini.quotaPolicy.rows.cli.limitsPremium')
    REDACTED
    return t('admin.accounts.gemini.quotaPolicy.rows.cli.limitsFree')
  REDACTED
  // AI Studio - 默认显示免费层限制
  return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsFree')
REDACTED)

const geminiQuotaPolicyDocsUrl = computed(() => {
  if (isGeminiCodeAssist.value) {
    return 'https://cloud.google.com/products/gemini/code-assist#pricing'
  REDACTED
  return 'https://ai.google.dev/pricing'
REDACTED)

// 账户类型显示标签
const antigravityTierLabel = computed(() => {
  switch (antigravityTier.value) {
    case 'free-tier':
      return t('admin.accounts.tier.free')
    case 'g1-pro-tier':
      return t('admin.accounts.tier.pro')
    case 'g1-ultra-tier':
      return t('admin.accounts.tier.ultra')
    default:
      return null
  REDACTED
REDACTED)

// 账户类型徽章样式
const antigravityTierClass = computed(() => {
  switch (antigravityTier.value) {
    case 'free-tier':
      return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
    case 'g1-pro-tier':
      return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    case 'g1-ultra-tier':
      return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    default:
      return ''
  REDACTED
REDACTED)

// 检测账户是否有不合格状态（ineligibleTiers）
const hasIneligibleTiers = computed(() => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (!extra) return false

  const loadCodeAssist = extra.load_code_assist as Record<string, unknown> | undefined
  if (!loadCodeAssist) return false

  const ineligibleTiers = loadCodeAssist.ineligibleTiers as unknown[] | undefined
  return Array.isArray(ineligibleTiers) && ineligibleTiers.length > 0
REDACTED)

const loadUsage = async () => {
  if (!shouldFetchUsage.value) return

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
