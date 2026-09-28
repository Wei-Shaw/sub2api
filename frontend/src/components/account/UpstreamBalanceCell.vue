<template>
  <div v-if="enabled" class="space-y-1" data-testid="upstream-balance-cell">
    <div class="flex flex-wrap items-center gap-1.5">
      <span
        class="text-[10px] font-medium leading-4 text-emerald-700 dark:text-emerald-300"
        :title="sourceLabel"
      >
        {{ balanceLabel }}
      </span>
      <span
        class="rounded bg-gray-100 px-1 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-600 dark:text-gray-300"
      >
        {{ sourceShort }}
      </span>
    </div>
    <div class="flex flex-wrap items-center gap-1.5">
      <button
        type="button"
        data-testid="upstream-balance-probe"
        class="inline-flex items-center gap-0.5 whitespace-nowrap rounded px-1.5 py-0.5 text-[10px] font-medium leading-4 text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading"
        :title="t('admin.accounts.upstreamBalance.probeTooltip')"
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
        {{ t('admin.accounts.upstreamBalance.probe') }}
      </button>
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
import type { UpstreamBalanceProbeResult } from '@/api/admin/accounts'
import type { Account } from '@/types'
import { accountBalanceProbeSource } from './credentialsBuilder'

const props = defineProps<{
  account: Account
}>()

const { t } = useI18n()
const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<UpstreamBalanceProbeResult | null>(null)

const source = computed(() => accountBalanceProbeSource(props.account))
const enabled = computed(() => source.value === 'sub2api' || source.value === 'newapi')

const snapshotBalance = computed(() => {
  const v = props.account.extra?.upstream_balance
  return typeof v === 'number' ? v : null
})
const snapshotCurrency = computed(() => {
  const v = props.account.extra?.upstream_balance_currency
  return typeof v === 'string' ? v : 'USD'
})
const snapshotSource = computed(() => {
  const v = props.account.extra?.upstream_balance_source
  return typeof v === 'string' ? v : source.value
})

const currentBalance = computed(() => {
  if (data.value?.success) return data.value.balance
  return snapshotBalance.value
})
const currentCurrency = computed(() => {
  if (data.value?.success) return data.value.currency || 'USD'
  return snapshotCurrency.value || 'USD'
})
const currentSource = computed(() => {
  if (data.value?.success) return data.value.source
  return snapshotSource.value
})

const sourceShort = computed(() => {
  if (currentSource.value === 'newapi') return 'NewAPI'
  if (currentSource.value === 'sub2api') return 'Sub2API'
  return source.value === 'newapi' ? 'NewAPI' : 'Sub2API'
})
const sourceLabel = computed(() =>
  currentSource.value === 'newapi'
    ? t('admin.accounts.upstreamBalance.sourceNewAPI')
    : t('admin.accounts.upstreamBalance.sourceSub2API')
)

const balanceLabel = computed(() => {
  if (currentBalance.value == null) return t('admin.accounts.upstreamBalance.placeholder')
  const value = currentBalance.value
  const fixed = value >= 100 ? value.toFixed(0) : value.toFixed(2)
  return `${currentCurrency.value} ${fixed}`
})

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}...` : error.value
})

const handleProbe = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  try {
    const result = await adminAPI.accounts.probeUpstreamBalance(props.account.id)
    if (result.success) {
      data.value = result
    } else {
      error.value = result.error || t('common.error')
    }
  } catch (e) {
    const err = e as { message?: string; response?: { data?: { message?: string } } }
    error.value = err.message || err.response?.data?.message || t('common.error')
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
