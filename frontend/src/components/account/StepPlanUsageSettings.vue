<template>
  <section
    v-if="eligible"
    class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600"
    data-testid="step-plan-usage-settings"
  >
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.stepPlan.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.stepPlan.panelHint') }}
        </p>
      </div>
      <span
        class="whitespace-nowrap rounded px-2 py-1 text-xs font-medium"
        :class="statusBadgeClass"
        data-testid="step-plan-status"
      >
        {{ statusLabel }}
      </span>
    </div>

    <div v-if="loading" class="flex h-20 items-center justify-center text-gray-400">
      <Icon name="refresh" size="sm" class="animate-spin" />
    </div>
    <template v-else>
      <div
        v-if="snapshot"
        class="border-y border-gray-100 py-3 dark:border-dark-700"
        data-testid="step-plan-usage-details"
      >
        <div class="grid grid-cols-[minmax(4rem,auto)_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-xs">
          <template v-if="snapshot.tiers?.length">
            <template v-for="tier in snapshot.tiers" :key="tier.window">
              <span class="text-gray-500 dark:text-gray-400">{{ windowLabel(tier.window) }}</span>
              <span class="break-words text-gray-900 dark:text-white">{{ windowSummary(tier) }}</span>
            </template>
          </template>
          <template v-else>
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stepPlan.windows') }}</span>
            <span class="text-gray-900 dark:text-white">{{ t('admin.accounts.stepPlan.noWindows') }}</span>
          </template>
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stepPlan.status') }}</span>
          <span class="break-words font-medium text-gray-900 dark:text-white">{{ statusLabel }}</span>
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stepPlan.updatedAt') }}</span>
          <span class="break-words text-gray-900 dark:text-white">{{ updatedAtLabel }}</span>
        </div>
        <p
          v-if="snapshot.last_error"
          class="mt-2 break-words border-t border-gray-100 pt-2 text-xs text-amber-700 dark:border-dark-700 dark:text-amber-300"
          data-testid="step-plan-last-error"
        >
          {{ snapshot.last_error }}
        </p>
      </div>
      <p v-else class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.stepPlan.notRefreshedHint') }}
      </p>

      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="!persistedEligible || refreshing || saving"
          data-testid="step-plan-refresh"
          @click="refreshUsage"
        >
          <Icon name="refresh" size="xs" class="mr-1.5" :class="{ 'animate-spin': refreshing }" />
          {{ t('admin.accounts.stepPlan.refreshNow') }}
        </button>
      </div>

      <p
        v-if="errorMessage"
        class="break-words text-xs text-red-600 dark:text-red-400"
        data-testid="step-plan-load-error"
      >
        {{ errorMessage }}
      </p>

      <p
        v-if="!persistedEligible"
        class="break-words text-xs text-amber-700 dark:text-amber-300"
        data-testid="step-plan-save-first"
      >
        {{ t('admin.accounts.stepPlan.saveBeforeRefresh') }}
      </p>

      <div class="flex items-center justify-between gap-4 border-t border-gray-100 pt-4 dark:border-dark-700">
        <div>
          <label class="text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.accounts.stepPlan.autoRefresh') }}
          </label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.stepPlan.autoRefreshHint') }}
          </p>
        </div>
        <Toggle
          :model-value="state?.auto_refresh_enabled ?? true"
          :disabled="!persistedEligible || saving || refreshing"
          data-testid="step-plan-auto-refresh"
          @update:model-value="setAutoRefresh"
        />
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { CNQuotaTier } from '@/api/admin/cnProviders'
import type { Account, StepPlanUsageSnapshot, StepPlanUsageState } from '@/types'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { cnQuotaCellVisible } from './credentialsBuilder'

const props = defineProps<{ account: Account; eligible: boolean }>()
const emit = defineEmits<{ updated: [account: Account] }>()
const { t } = useI18n()

const SNAPSHOT_STALE_MS = 15 * 60 * 1000
const persistedEligible = computed(() => cnQuotaCellVisible(props.account))
const canOperate = computed(() => props.eligible && persistedEligible.value)
const loading = ref(false)
const refreshing = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const state = ref<StepPlanUsageState | null>(initialState(props.account))
const snapshot = computed(() => state.value?.snapshot)
let disposed = false
let loadRequestId = 0
let refreshRequestId = 0
let saveRequestId = 0
let mutationEpoch = 0

const statusLabel = computed(() => {
  if (!snapshot.value) return t('admin.accounts.stepPlan.notRefreshed')
  switch (snapshot.value.status) {
    case 'ok':
      return t('admin.accounts.stepPlan.ok')
    case 'unavailable':
      return t('admin.accounts.stepPlan.unavailable')
    default:
      return t('admin.accounts.stepPlan.failed')
  }
})

const statusBadgeClass = computed(() => {
  if (snapshot.value?.status === 'ok') {
    return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  }
  if (snapshot.value?.status === 'unavailable') {
    return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  }
  if (snapshot.value?.status === 'failed') {
    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  }
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
})

const updatedAtLabel = computed(() => {
  const value = snapshot.value?.fetched_at || snapshot.value?.last_attempt_at
  return formatDate(value)
})

function initialState(account: Account): StepPlanUsageState {
  const rawAutoRefresh = account.extra?.stepfun_usage_auto_refresh
  return {
    account_id: account.id,
    eligible: props.eligible,
    auto_refresh_enabled: typeof rawAutoRefresh === 'boolean' ? rawAutoRefresh : true,
    snapshot: account.extra?.stepfun_usage_snapshot as StepPlanUsageSnapshot | undefined
  }
}

function formatPercent(value: number): string {
  return Number.isFinite(value) ? `${value.toFixed(value % 1 ? 1 : 0)}%` : '-'
}

function formatDate(value?: string): string {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function windowLabel(window: string): string {
  if (window === 'weekly') return t('admin.accounts.stepPlan.windowWeekly')
  if (window === 'monthly') return t('admin.accounts.stepPlan.windowMonthly')
  if (window === '5h') return t('admin.accounts.stepPlan.window5h')
  return window
}

function windowSummary(tier: CNQuotaTier): string {
  const percent = formatPercent(tier.used_percent)
  return tier.reset_at
    ? t('admin.accounts.stepPlan.windowWithReset', { percent, reset: formatDate(tier.reset_at) })
    : percent
}

function snapshotIsStale(): boolean {
  const lastAttempt = snapshot.value?.last_attempt_at
  if (!lastAttempt) return true
  const parsed = new Date(lastAttempt).getTime()
  return Number.isNaN(parsed) || Date.now() - parsed > SNAPSHOT_STALE_MS
}

function freshestSnapshot(
  current: StepPlanUsageSnapshot | undefined,
  next: StepPlanUsageSnapshot | undefined
): StepPlanUsageSnapshot | undefined {
  if (!current) return next
  if (!next) return current
  const currentAt = Date.parse(current.last_attempt_at)
  const nextAt = Date.parse(next.last_attempt_at)
  return Number.isFinite(nextAt) && (!Number.isFinite(currentAt) || nextAt > currentAt) ? next : current
}

function isActiveAccount(accountId: number): boolean {
  return !disposed && canOperate.value && props.account.id === accountId
}

function isCurrentMutation(epoch: number, accountId: number): boolean {
  return epoch === mutationEpoch && isActiveAccount(accountId)
}

async function reloadPersistedState(epoch: number, accountId: number) {
  try {
    const next = await adminAPI.cnProviders.getStepPlanUsageState(accountId)
    if (isCurrentMutation(epoch, accountId)) applyState(next, accountId)
  } catch {
    // Keep the original operation error; this reload is best-effort.
  }
}

function applyState(next: StepPlanUsageState, accountId: number): boolean {
  if (!isActiveAccount(accountId) || next.account_id !== accountId) return false
  state.value = next
  const extra = {
    ...(props.account.extra ?? {}),
    stepfun_usage_auto_refresh: next.auto_refresh_enabled,
    stepfun_usage_snapshot: next.snapshot
  }
  emit('updated', { ...props.account, extra })
  return true
}

function extractErrorMessage(error: unknown): string {
  const value = error as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string } }
  }
  return value?.message || value?.reason || value?.response?.data?.message ||
    value?.response?.data?.error || t('admin.accounts.stepPlan.refreshFailed')
}

async function loadState() {
  if (!canOperate.value) return
  const accountId = props.account.id
  const requestId = ++loadRequestId
  loading.value = true
  errorMessage.value = ''
  try {
    const next = await adminAPI.cnProviders.getStepPlanUsageState(accountId)
    if (requestId !== loadRequestId || !applyState(next, accountId)) return
    if (snapshotIsStale()) await refreshUsage()
  } catch (error) {
    if (requestId === loadRequestId && isActiveAccount(accountId)) {
      errorMessage.value = extractErrorMessage(error)
    }
  } finally {
    if (requestId === loadRequestId && isActiveAccount(accountId)) loading.value = false
  }
}

async function refreshUsage() {
  if (!canOperate.value || refreshing.value || saving.value) return
  const accountId = props.account.id
  const requestId = ++refreshRequestId
  const epoch = ++mutationEpoch
  refreshing.value = true
  errorMessage.value = ''
  try {
    await adminAPI.cnProviders.queryQuota(accountId)
    if (requestId !== refreshRequestId || !isCurrentMutation(epoch, accountId)) return
    const next = await adminAPI.cnProviders.getStepPlanUsageState(accountId)
    if (requestId === refreshRequestId && isCurrentMutation(epoch, accountId)) {
      applyState(next, accountId)
    }
  } catch (error) {
    if (requestId === refreshRequestId && isCurrentMutation(epoch, accountId)) {
      errorMessage.value = extractErrorMessage(error)
      await reloadPersistedState(epoch, accountId)
    }
  } finally {
    if (requestId === refreshRequestId && isActiveAccount(accountId)) refreshing.value = false
  }
}

async function setAutoRefresh(enabled: boolean) {
  if (!canOperate.value || saving.value || refreshing.value) return
  const accountId = props.account.id
  const requestId = ++saveRequestId
  const epoch = ++mutationEpoch
  saving.value = true
  errorMessage.value = ''
  try {
    const next = await adminAPI.cnProviders.setStepPlanUsageAutoRefresh(accountId, enabled)
    if (requestId === saveRequestId && isCurrentMutation(epoch, accountId)) {
      const current = state.value
      if (current) {
        // The PUT is authoritative only for the toggle. A concurrent periodic
        // probe must not be rolled back by the snapshot read before that write.
        applyState({
          ...current,
          account_id: next.account_id,
          eligible: next.eligible,
          auto_refresh_enabled: next.auto_refresh_enabled,
          snapshot: freshestSnapshot(current.snapshot, next.snapshot)
        }, accountId)
      }
    }
  } catch (error) {
    if (requestId === saveRequestId && isCurrentMutation(epoch, accountId)) {
      errorMessage.value = extractErrorMessage(error)
    }
  } finally {
    if (requestId === saveRequestId && isActiveAccount(accountId)) saving.value = false
  }
}

watch(() => props.account.id, () => {
  loadRequestId += 1
  refreshRequestId += 1
  saveRequestId += 1
  mutationEpoch += 1
  loading.value = false
  refreshing.value = false
  saving.value = false
  state.value = initialState(props.account)
  errorMessage.value = ''
  void loadState()
})

watch(() => props.eligible, (eligible) => {
  loadRequestId += 1
  refreshRequestId += 1
  saveRequestId += 1
  mutationEpoch += 1
  loading.value = false
  refreshing.value = false
  saving.value = false
  state.value = initialState(props.account)
  errorMessage.value = ''
  if (eligible) void loadState()
})

watch(persistedEligible, (eligible) => {
  loadRequestId += 1
  refreshRequestId += 1
  saveRequestId += 1
  mutationEpoch += 1
  loading.value = false
  refreshing.value = false
  saving.value = false
  state.value = initialState(props.account)
  errorMessage.value = ''
  if (eligible && props.eligible) void loadState()
})

onMounted(loadState)

onUnmounted(() => {
  disposed = true
  loadRequestId += 1
  refreshRequestId += 1
  saveRequestId += 1
  mutationEpoch += 1
})
</script>
