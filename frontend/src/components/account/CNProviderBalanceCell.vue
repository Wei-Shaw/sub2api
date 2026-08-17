<template>
  <div v-if="visible" class="space-y-1">
    <div class="flex flex-wrap items-center gap-1.5">
      <button
        type="button"
        :class="[
          'inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-dark-600',
          platformTextClass(account.platform)
        ]"
        :disabled="loading"
        @click="handleProbe"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': loading REDACTED"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
        {{ balanceLabel REDACTEDREDACTED
      </button>

      <!-- Low balance badge (reactive 402/429 marker or probe-detected) -->
      <span
        v-if="balanceLow"
        class="inline-flex items-center rounded bg-red-100 px-1 py-0.5 text-[10px] font-medium text-red-700 dark:bg-red-900/30 dark:text-red-300"
      >
        {{ t('admin.accounts.cnProviders.balanceLow') REDACTEDREDACTED
      </span>
    </div>

    <div v-if="error" class="truncate text-[10px] text-red-600 dark:text-red-400" :title="error">
      {{ truncatedError REDACTEDREDACTED
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { adminAPI REDACTED from '@/api/admin'
import type { CNProviderBalanceEntry, CNProviderBalanceResult REDACTED from '@/api/admin/cnProviders'
import type { Account REDACTED from '@/types'
import { platformTextClass REDACTED from '@/utils/platformColors'
import { cnBalanceCellVisible REDACTED from './credentialsBuilder'

const props = defineProps<{
  account: Account
REDACTED>()

const { t REDACTED = useI18n()

const readMode = (): string => {
  const mode = props.account.credentials?.account_mode
  return typeof mode === 'string' ? mode : ''
REDACTED

// 仅 kimi / deepseek payg 账号有公开余额端点（智谱 payg 无）。
const visible = computed(() => cnBalanceCellVisible(props.account.platform, readMode()))

const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<CNProviderBalanceResult | null>(null)

const extraKey = (suffix: string) => `${props.account.platformREDACTED_${suffixREDACTED`

// 落库快照（后端周期探测/响应式写入 account.Extra）。
const snapshotBalance = computed(() => {
  const v = props.account.extra?.[extraKey('balance')]
  return typeof v === 'number' ? v : null
REDACTED)
const snapshotCurrency = computed(() => {
  const v = props.account.extra?.[extraKey('balance_currency')]
  return typeof v === 'string' ? v : ''
REDACTED)
// 多币种快照（后端写 <platform>_balances：[{currency, balanceREDACTED]，deepseek CNY+USD）。
const snapshotBalances = computed<CNProviderBalanceEntry[]>(() => {
  const v = props.account.extra?.[extraKey('balances')]
  if (!Array.isArray(v)) return []
  return v.flatMap((item): CNProviderBalanceEntry[] => {
    if (!item || typeof item !== 'object') return []
    const { currency, balance REDACTED = item as Record<string, unknown>
    if (typeof currency !== 'string' || typeof balance !== 'number') return []
    return [{ currency, balance REDACTED]
  REDACTED)
REDACTED)
const balanceLow = computed(() => props.account.extra?.[extraKey('balance_low')] === true)

// 优先用探测结果，其次落库快照。多币种返回全部明细，否则主币种单条。
const currentEntries = computed<CNProviderBalanceEntry[]>(() => {
  if (data.value && data.value.success) {
    if (data.value.balances && data.value.balances.length > 0) return data.value.balances
    return [{ currency: data.value.currency || '', balance: data.value.balance REDACTED]
  REDACTED
  if (snapshotBalances.value.length > 0) return snapshotBalances.value
  if (snapshotBalance.value != null) {
    return [{ currency: snapshotCurrency.value, balance: snapshotBalance.value REDACTED]
  REDACTED
  return []
REDACTED)

const formatEntry = (entry: CNProviderBalanceEntry): string => {
  const fixed = entry.balance >= 100 ? entry.balance.toFixed(0) : entry.balance.toFixed(2)
  return `${entry.currency || '¥'REDACTED ${fixedREDACTED`
REDACTED

const balanceLabel = computed(() => {
  if (currentEntries.value.length === 0) {
    // 修复:此前误引 admin.accounts.grokBalance(实际嵌套在 usageWindow 下),
    // 未命中时渲染原始 key。CN 供应商使用自己的占位键。
    return t('admin.accounts.cnProviders.balance')
  REDACTED
  return currentEntries.value.map(formatEntry).join(' · ')
REDACTED)

const extractErrorMessage = (e: unknown): string => {
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string REDACTED REDACTED
  REDACTED
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
REDACTED

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)REDACTED...` : error.value
REDACTED)

const handleProbe = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  try {
    const result = await adminAPI.cnProviders.queryBalance(props.account.id)
    // 失败时保留快照展示（仅显示错误行），成功才覆盖。
    if (result.success) {
      data.value = result
    REDACTED else {
      error.value = result.error || t('common.error')
    REDACTED
  REDACTED catch (e) {
    error.value = extractErrorMessage(e)
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

watch(
  () => props.account.id,
  () => {
    data.value = null
    error.value = null
    loading.value = false
  REDACTED
)
</script>
