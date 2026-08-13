<template>
  <div
    v-if="state?.eligible"
    class="min-w-0 max-w-full space-y-1"
    data-testid="opencode-go-usage-cell"
  >
    <UsageProgressBar
      v-if="snapshot?.data?.rolling"
      :label="t('admin.accounts.opencodeGo.rollingShort')"
      :utilization="snapshot.data.rolling.percent"
      :resets-at="snapshot.data.rolling.resets_at"
      color="indigo"
      data-testid="opencode-go-rolling"
    />
    <UsageProgressBar
      v-if="snapshot?.data?.weekly"
      :label="t('admin.accounts.opencodeGo.weeklyShort')"
      :utilization="snapshot.data.weekly.percent"
      :resets-at="snapshot.data.weekly.resets_at"
      color="emerald"
      data-testid="opencode-go-weekly"
    />
    <UsageProgressBar
      v-if="snapshot?.data?.monthly"
      :label="t('admin.accounts.opencodeGo.monthlyShort')"
      :utilization="snapshot.data.monthly.percent"
      :resets-at="snapshot.data.monthly.resets_at"
      color="amber"
      data-testid="opencode-go-monthly"
    />
    <span
      v-if="snapshot && snapshot.status !== 'ok'"
      class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium"
      :class="snapshot.status === 'unauthorized'
        ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
        : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'"
      data-testid="opencode-go-status-badge"
    >
      {{ statusLabel }}
    </span>
  </div>
  <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const state = computed(() => props.account.opencode_go_usage)
const snapshot = computed(() => state.value?.snapshot)
const statusLabel = computed(() => {
  if (snapshot.value?.status === 'unauthorized') return t('admin.accounts.opencodeGo.unauthorized')
  if (snapshot.value?.status === 'failed') return t('admin.accounts.opencodeGo.failed')
  return t('admin.accounts.opencodeGo.ok')
})
</script>
