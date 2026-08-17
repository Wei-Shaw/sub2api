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
          :class="{ 'animate-spin': loading }"
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
        {{ balanceLabel }}
      </button>

      <!-- Low balance badge (reactive 402/429 marker or probe-detected) -->
      <span
        v-if="balanceLow"
        class="inline-flex items-center rounded bg-red-100 px-1 py-0.5 text-[10px] font-medium text-red-700 dark:bg-red-900/30 dark:text-red-300"
      >
        {{ t('admin.accounts.cnProviders.balanceLow') }}
      </span>
    </div>

    <div v-if="error" class="truncate text-[10px] text-red-600 dark:text-red-400" :title="error">
      {{ truncatedError }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { CNProviderBalanceEntry, CNProviderBalanceResult } from '@/api/admin/cnProviders'
import type { Account } from '@/types'
import { platformTextClass } from '@/utils/platformColors'
import { cnBalanceCellVisible } from './credentialsBuilder'

const props = defineProps<{
  account: Account
}>()

const { t } = useI18n()

const readMode = (): string => {
  const mode = props.account.credentials?.account_mode
  return typeof mode === 'string' ? mode : ''
}

// 仅 kimi / deepseek payg 账号有公开余额端点（智谱 payg 无）。
const visible = computed(() => cnBalanceCellVisible(props.account.platform, readMode()))

const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<CNProviderBalanceResult | null>(null)

const extraKey = (suffix: string) => `${props.account.platform}_${suffix}`

// 落库快照（后端周期探测/响应式写入 account.Extra）。
const snapshotBalance = computed(() => {
  const v = props.account.extra?.[extraKey('balance')]
  return typeof v === 'number' ? v : null
})
const snapshotCurrency = computed(() => {
  const v = props.account.extra?.[extraKey('balance_currency')]
  return typeof v === 'string' ? v : ''
})
// 多币种快照（后端写 <platform>_balances：[{currency, balance}]，deepseek CNY+USD）。
const snapshotBalances = computed<CNProviderBalanceEntry[]>(() => {
  const v = props.account.extra?.[extraKey('balances')]
  if (!Array.isArray(v)) return []
  return v.flatMap((item): CNProviderBalanceEntry[] => {
    if (!item || typeof item !== 'object') return []
    const { currency, balance } = item as Record<string, unknown>
    if (typeof currency !== 'string' || typeof balance !== 'number') return []
    return [{ currency, balance }]
  })
})
const balanceLow = computed(() => props.account.extra?.[extraKey('balance_low')] === true)

// 优先用探测结果，其次落库快照。多币种返回全部明细，否则主币种单条。
const currentEntries = computed<CNProviderBalanceEntry[]>(() => {
  if (data.value && data.value.success) {
    if (data.value.balances && data.value.balances.length > 0) return data.value.balances
    return [{ currency: data.value.currency || '', balance: data.value.balance }]
  }
  if (snapshotBalances.value.length > 0) return snapshotBalances.value
  if (snapshotBalance.value != null) {
    return [{ currency: snapshotCurrency.value, balance: snapshotBalance.value }]
  }
  return []
})

const formatEntry = (entry: CNProviderBalanceEntry): string => {
  const fixed = entry.balance >= 100 ? entry.balance.toFixed(0) : entry.balance.toFixed(2)
  return `${entry.currency || '¥'} ${fixed}`
}

const balanceLabel = computed(() => {
  if (currentEntries.value.length === 0) {
    return t('admin.accounts.grokBalance')?.trim() || 'Balance'
  }
  return currentEntries.value.map(formatEntry).join(' · ')
})

const extractErrorMessage = (e: unknown): string => {
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string } }
  }
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
}

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}...` : error.value
})

const handleProbe = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  try {
    const result = await adminAPI.cnProviders.queryBalance(props.account.id)
    // 失败时保留快照展示（仅显示错误行），成功才覆盖。
    if (result.success) {
      data.value = result
    } else {
      error.value = result.error || t('common.error')
    }
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.account.id,
  () => {
    data.value = null
    error.value = null
    loading.value = false
  }
)
</script>
