<template>
  <div
    v-if="state?.eligible"
    class="min-w-0 max-w-full space-y-1"
    data-testid="ollama-cloud-usage-cell"
  >
    <!-- legacy 模式：官方 5h / 7d 滚动窗口。credits 账号的官方 settings 页也会
         夹带窗口数据，但与月度信用池语义不符，按 mode 不渲染以免误导运营。 -->
    <template v-if="!isCredits">
      <UsageProgressBar
        v-if="snapshot?.data?.five_hour"
        label="5h"
        :utilization="snapshot.data.five_hour.used_percent"
        :resets-at="snapshot.data.five_hour.reset_at"
        color="indigo"
        data-testid="ollama-cloud-five-hour"
      />
      <UsageProgressBar
        v-if="snapshot?.data?.seven_day"
        label="7d"
        :utilization="snapshot.data.seven_day.used_percent"
        :resets-at="snapshot.data.seven_day.reset_at"
        color="emerald"
        data-testid="ollama-cloud-seven-day"
      />
    </template>
    <!-- credits 模式：月度信用池已用进度（(pool-balance)/pool，clamp 0-100）。
         monthly_credit_usd 未录入或余额解析失败时不渲染，只显示原始余额文本。 -->
    <UsageProgressBar
      v-if="creditsPoolUtilization !== null"
      :label="t('admin.accounts.ollamaCloud.monthlyPoolShort')"
      :utilization="creditsPoolUtilization"
      color="amber"
      data-testid="ollama-cloud-monthly-pool"
    />
    <div
      v-if="balanceText"
      class="truncate text-[10px] text-gray-500 dark:text-gray-400"
      data-testid="ollama-cloud-balance"
    >
      {{ balanceLabel }}
    </div>
    <!-- credits 模式标注快照新鲜度（余额是采样值，不是实时值）。 -->
    <div
      v-if="isCredits && snapshotTimeLabel"
      class="truncate text-[10px] text-gray-400 dark:text-dark-500"
      data-testid="ollama-cloud-snapshot-time"
    >
      {{ snapshotTimeLabel }}
    </div>
    <div v-if="state.configured" class="flex items-center pt-0.5">
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="refreshing"
        data-testid="ollama-cloud-usage-query"
        @click="refreshUsage"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': refreshing }"
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
        {{ t('admin.accounts.usageWindow.activeQuery') }}
      </button>
    </div>
  </div>
  <!-- 不合格：展示后端原因码的可解释提示，而不是空白占位。 -->
  <span
    v-else-if="eligibleReasonLabel"
    class="inline-block max-w-full truncate text-[10px] text-amber-600 dark:text-amber-400"
    :title="eligibleReasonLabel"
    data-testid="ollama-cloud-usage-ineligible"
  >{{ eligibleReasonLabel }}</span>
  <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, OllamaCloudUsageState } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ updated: [state: OllamaCloudUsageState] }>()
const { t } = useI18n()
const state = ref(props.account.ollama_cloud_usage)
const refreshing = ref(false)
const snapshot = computed(() => state.value?.snapshot)
const isCredits = computed(() => state.value?.mode === 'ollama_credits')

// balance 是官方文本（如 "$12.00"），派生池进度需要数值：剥掉货币符号与
// 千分位后 parseFloat；解析不了返回 null（回落为只显示原始文本）。
const balanceNumber = computed<number | null>(() => {
  const raw = snapshot.value?.data?.balance
  if (!raw) return null
  const parsed = Number.parseFloat(raw.replace(/[^0-9.]/g, ''))
  return Number.isFinite(parsed) ? parsed : null
})

// credits 模式的月度信用池已用百分比：(pool - balance) / pool，clamp 0-100。
const creditsPoolUtilization = computed<number | null>(() => {
  const pool = state.value?.monthly_credit_usd
  if (!isCredits.value || !pool || pool <= 0 || balanceNumber.value === null) return null
  const used = ((pool - balanceNumber.value) / pool) * 100
  return Math.min(Math.max(used, 0), 100)
})

const balanceText = computed(() => snapshot.value?.data?.balance ?? '')
const balanceLabel = computed(() => {
  const prefix = `${t('admin.accounts.ollamaCloud.balance')}: ${balanceText.value}`
  const pool = state.value?.monthly_credit_usd
  return isCredits.value && pool && pool > 0 ? `${prefix} / $${pool}` : prefix
})

const eligibleReasonLabel = computed(() => {
  const reason = state.value?.eligible_reason
  return reason ? t(`admin.accounts.ollamaCloud.eligibleReason.${reason}`) : ''
})

const snapshotTimeLabel = computed(() => {
  const raw = snapshot.value?.fetched_at
  if (!raw) return ''
  const date = new Date(raw)
  const formatted = Number.isNaN(date.getTime()) ? raw : date.toLocaleString()
  return `${t('admin.accounts.ollamaCloud.updatedAt')}: ${formatted}`
})

watch(() => props.account.ollama_cloud_usage, (next) => {
  state.value = next
})

const refreshUsage = async () => {
  if (refreshing.value) return
  refreshing.value = true
  try {
    const next = await adminAPI.accounts.refreshOllamaCloudUsage(props.account.id)
    state.value = next
    emit('updated', next)
  } catch (error) {
    console.error('Failed to refresh Ollama Cloud usage:', error)
  } finally {
    refreshing.value = false
  }
}
</script>
