<template>
  <div v-if="visible" class="space-y-1">
    <div class="flex flex-wrap items-center gap-1.5">
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-cyan-700 transition-colors hover:bg-cyan-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-cyan-300 dark:hover:bg-cyan-900/30"
        :disabled="loading"
        :title="t('admin.accounts.usageWindow.grokProbeTooltip')"
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
        {{ loading ? t('admin.accounts.usageWindow.grokProbing') : t('admin.accounts.usageWindow.grokProbe') }}
      </button>
    </div>

    <div v-if="error" class="truncate text-[10px] text-red-600 dark:text-red-400" :title="error">
      {{ truncatedError }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { GrokBillingSummary, GrokQuotaProbeResult } from '@/api/admin/grok'
import type { Account } from '@/types'

const props = defineProps<{
  account: Account
  /** Prefill from passive /usage payload when available */
  initialBilling?: GrokBillingSummary | null
  /** Auto-run billing probe when no billing snapshot is present */
  autoProbe?: boolean
}>()

const emit = defineEmits<{
  (e: 'updated', result: GrokQuotaProbeResult): void
}>()

const { t } = useI18n()

const visible = computed(() => props.account.platform === 'grok' && props.account.type === 'oauth')
const loading = ref(false)
const error = ref<string | null>(null)
const probedOnce = ref(false)

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

const hasBilling = (billing?: GrokBillingSummary | null) => {
  if (!billing) return false
  return (
    billing.usage_percent != null ||
    billing.used_percent != null ||
    billing.monthly_limit_cents != null ||
    !!billing.period_end ||
    !!billing.billing_period_end ||
    (billing.on_demand_cap_cents != null && billing.on_demand_cap_cents > 0)
  )
}

const handleProbe = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  try {
    const result = await adminAPI.grok.queryQuota(props.account.id)
    emit('updated', result)
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    loading.value = false
    probedOnce.value = true
  }
}

onMounted(() => {
  if (!props.autoProbe) return
  if (hasBilling(props.initialBilling)) return
  void handleProbe()
})

watch(
  () => props.account.id,
  () => {
    error.value = null
    loading.value = false
    probedOnce.value = false
    if (props.autoProbe && !hasBilling(props.initialBilling)) {
      void handleProbe()
    }
  }
)
</script>
