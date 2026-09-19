<template>
  <section class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="codex-account-ticket-settings">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.stateTicket.title') }}</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stateTicket.description') }}</p>
      </div>
      <Toggle v-model="enabled" :disabled="!status || busy" :aria-label="t('admin.accounts.stateTicket.enable')" data-testid="codex-account-ticket-enabled" />
    </div>
    <p v-if="loading" class="text-xs text-gray-500">{{ t('common.loading') }}</p>
    <template v-if="status">
      <div>
        <label :for="`codex-account-ticket-plan-${accountId}`" class="input-label">{{ t('admin.accounts.stateTicket.plan') }}</label>
        <select :id="`codex-account-ticket-plan-${accountId}`" v-model="ticketPlan" class="input w-full text-sm" :disabled="busy" data-testid="codex-account-ticket-plan">
          <option value="pro">{{ t('admin.accounts.stateTicket.planPro') }}</option>
          <option value="team">{{ t('admin.accounts.stateTicket.planTeam') }}</option>
        </select>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stateTicket.planHint') }}</p>
      </div>
      <p v-if="!status.global_enabled" class="rounded bg-amber-50 p-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200" data-testid="codex-account-ticket-global-off">
        {{ t('admin.accounts.stateTicket.globalOff') }}
        <a href="/admin/settings?tab=gateway" target="_blank" rel="noopener noreferrer" class="font-medium underline">{{ t('admin.accounts.stateTicket.gatewaySettings') }}</a>
      </p>
      <div class="text-xs text-gray-500 dark:text-gray-400" data-testid="codex-account-ticket-global-pool">
        <p v-if="status.proxy_configured">{{ t('admin.accounts.stateTicket.globalPoolConfigured', { address: status.proxy_display }) }}</p>
        <p v-else class="text-amber-700 dark:text-amber-300">{{ t('admin.accounts.stateTicket.globalPoolMissing') }}</p>
        <p>{{ t('admin.accounts.stateTicket.globalPoolHint') }} <a href="/admin/settings?tab=gateway" target="_blank" rel="noopener noreferrer" class="font-medium underline">{{ t('admin.accounts.stateTicket.gatewaySettings') }}</a></p>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stateTicket.model', { model: status.model }) }}</p>
      <div class="flex flex-wrap items-center gap-2 text-sm" aria-live="polite" data-testid="codex-account-ticket-status">
        <span :data-testid="refreshingUsable ? 'codex-account-ticket-refreshing-usable' : undefined" :class="status.state === 'ready' || refreshingUsable ? 'text-emerald-600 dark:text-emerald-400' : status.state === 'error' ? 'text-amber-700 dark:text-amber-300' : 'text-gray-600 dark:text-gray-300'">{{ stateLabel }}</span>
        <span v-if="status.state === 'harvesting' && status.attempts" class="text-xs text-gray-500">{{ t('admin.accounts.stateTicket.attempts', { count: status.attempts }) }}</span>
      </div>
      <div v-if="savedTicket" class="space-y-1 rounded bg-emerald-50 p-2 text-xs text-emerald-800 dark:bg-emerald-900/20 dark:text-emerald-200" data-testid="codex-account-ticket-saved-ticket">
        <p class="font-medium">{{ t('admin.accounts.stateTicket.savedTicket') }}</p>
        <p v-if="capturedAt || expiresAt" class="flex flex-wrap gap-x-3 gap-y-1">
          <span v-if="capturedAt">{{ t('admin.accounts.stateTicket.capturedAt', { time: capturedAt }) }}</span>
          <span v-if="expiresAt">{{ t('admin.accounts.stateTicket.expiresAt', { time: expiresAt }) }}</span>
        </p>
        <p data-testid="codex-account-ticket-saved-ticket-hint">{{ t('admin.accounts.stateTicket.savedTicketHint') }}</p>
      </div>
      <p v-if="usableAfterRefreshFailure" class="text-xs text-amber-700 dark:text-amber-300" data-testid="codex-account-ticket-usable-error">
        {{ t('admin.accounts.stateTicket.refreshFailedUsable') }}
      </p>
      <p v-if="retryAfter" class="text-xs text-gray-600 dark:text-gray-300" data-testid="codex-account-ticket-retry-after">
        {{ t('admin.accounts.stateTicket.retryAfter', { time: retryAfter }) }}
      </p>
      <div class="space-y-1 rounded bg-gray-50 p-2 text-xs dark:bg-dark-700" aria-live="polite" data-testid="codex-account-ticket-watchdog">
        <p class="flex flex-wrap items-center gap-2">
          <span class="font-medium text-gray-700 dark:text-gray-200">{{ t('admin.accounts.stateTicket.watchdog') }}</span>
          <span :class="status.watchdog.enabled ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-500 dark:text-gray-400'" data-testid="codex-account-ticket-watchdog-status">{{ status.watchdog.enabled ? t('admin.accounts.stateTicket.watchdogEnabled') : t('admin.accounts.stateTicket.watchdogDisabled') }}</span>
        </p>
        <p class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stateTicket.watchdogHint') }}</p>
        <p v-if="status.watchdog.trigger_count > 0" class="text-gray-600 dark:text-gray-300" data-testid="codex-account-ticket-watchdog-event">
          {{ t('admin.accounts.stateTicket.watchdogTriggerCount', { count: status.watchdog.trigger_count }) }}
          <span v-if="watchdogLastReason"> · {{ t('admin.accounts.stateTicket.watchdogLastReason', { reason: watchdogLastReason }) }}</span>
          <span v-if="watchdogLastTriggeredAt"> · {{ watchdogLastTriggeredAt }}</span>
        </p>
      </div>
      <p v-if="status.last_error" class="break-words text-xs text-amber-700 dark:text-amber-300" data-testid="codex-account-ticket-error">{{ status.last_error }}</p>
      <p v-if="proxyChanged" class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.accounts.stateTicket.fixedProxyUnsaved') }}</p>
      <p v-else-if="dirty" class="text-xs text-gray-500">{{ t('admin.accounts.stateTicket.unsaved') }}</p>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-primary btn-sm" :disabled="busy || !dirty || proxyChanged || (enabled && !status.proxy_configured)" data-testid="codex-account-ticket-save" @click="save">
          {{ t('admin.accounts.stateTicket.save') }}
        </button>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="busy || dirty || proxyChanged || !status.global_enabled || !status.enabled || !status.proxy_configured || status.state === 'harvesting'" data-testid="codex-account-ticket-harvest" @click="harvest">
          {{ status.state === 'ready' ? t('admin.accounts.stateTicket.reacquire') : t('admin.accounts.stateTicket.acquire') }}
        </button>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stateTicket.failureHint') }}</p>
    </template>
    <p v-if="error" role="alert" class="text-xs text-red-600 dark:text-red-400">{{ error }}</p>
    <p v-if="saved" role="status" class="text-xs text-emerald-600 dark:text-emerald-400">{{ t('admin.accounts.stateTicket.saved') }}</p>
    <button v-if="!status && !loading" type="button" class="btn btn-secondary btn-sm" @click="load(true)">{{ t('admin.accounts.stateTicket.retry') }}</button>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { getCodexAccountTicket, saveCodexAccountTicket, harvestCodexAccountTicket, type CodexAccountTicketStatus, type CodexTicketPlan } from '@/api/admin/codexTickets'

const props = defineProps<{ accountId: number; visible: boolean; proxyChanged?: boolean }>()
const { t, locale } = useI18n()
const status = ref<CodexAccountTicketStatus | null>(null)
const enabled = ref(false)
const ticketPlan = ref<CodexTicketPlan>('pro')
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const saved = ref(false)
let generation = 0
let revision = 0
let timer: ReturnType<typeof setTimeout> | undefined
const dirty = computed(() => !!status.value && (enabled.value !== status.value.enabled || ticketPlan.value !== status.value.ticket_plan))
const hasUsableTicket = computed(() => status.value?.ticket_usable === true)
const refreshingUsable = computed(() => status.value?.state === 'harvesting' && hasUsableTicket.value)
const savedTicket = computed(() => hasUsableTicket.value && !!(capturedAt.value || expiresAt.value))
const usableAfterRefreshFailure = computed(() => hasUsableTicket.value && !!status.value?.last_error)
const remainingTime = computed(() => formatRemaining(status.value?.remaining_seconds ?? 0))
const capturedAt = computed(() => formatLocalDate(status.value?.captured_at))
const expiresAt = computed(() => formatLocalDate(status.value?.expires_at))
const retryAfter = computed(() => formatLocalDate(status.value?.retry_after))
const stateLabel = computed(() => {
  if (!status.value) return ''
  if (refreshingUsable.value) return t('admin.accounts.stateTicket.refreshing', { time: remainingTime.value })
  if (status.value.state === 'ready') {
    return t('admin.accounts.stateTicket.ready', { time: remainingTime.value })
  }
  return t(`admin.accounts.stateTicket.states.${status.value.state}`)
})
const watchdogLastReason = computed(() => {
  const reason = status.value?.watchdog.last_reason
  if (reason === 'model_mismatch') return t('admin.accounts.stateTicket.watchdogModelMismatch')
  if (reason === 'state_312') return t('admin.accounts.stateTicket.watchdogState312')
  return ''
})
const watchdogLastTriggeredAt = computed(() => {
  return formatLocalDate(status.value?.watchdog.last_triggered_at)
})

function formatRemaining(seconds: number) {
  const total = Math.max(0, Math.floor(seconds))
  return `${Math.floor(total / 60)}m ${String(total % 60).padStart(2, '0')}s`
}

function formatLocalDate(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString(locale.value, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}
watch([enabled, ticketPlan], () => { saved.value = false }, { flush: 'sync' })

async function load(initial = false) {
  const currentGeneration = generation
  const currentRevision = revision
  if (initial) loading.value = true
  try {
    const next = await getCodexAccountTicket(props.accountId)
    if (generation !== currentGeneration || revision !== currentRevision || !props.visible) return
    // Polling updates status only; it must never overwrite an in-progress edit.
    status.value = next
    if (initial) {
      enabled.value = next.enabled
      ticketPlan.value = next.ticket_plan
    }
    error.value = ''
  } catch {
    if (generation === currentGeneration && revision === currentRevision) error.value = t('admin.accounts.stateTicket.loadFailed')
  } finally {
    if (generation === currentGeneration) loading.value = false
  }
}

function schedulePoll() {
  timer = setTimeout(async () => {
    const currentGeneration = generation
    if (!props.visible) return
    if (!busy.value) await load()
    if (generation === currentGeneration && props.visible) schedulePoll()
  }, 3000)
}

async function save() {
  if (!status.value || busy.value || props.proxyChanged) return
  busy.value = true
  saved.value = false
  error.value = ''
  revision++
  const currentGeneration = generation
  try {
    const next = await saveCodexAccountTicket(props.accountId, {
      enabled: enabled.value,
      ticket_plan: ticketPlan.value
    })
    if (generation !== currentGeneration) return
    status.value = next
    enabled.value = next.enabled
    ticketPlan.value = next.ticket_plan
    // Keep success feedback after the draft watchers have cleared the old message.
    saved.value = true
  } catch {
    if (generation === currentGeneration) error.value = t('admin.accounts.stateTicket.saveFailed')
  } finally {
    if (generation === currentGeneration) busy.value = false
  }
}

async function harvest() {
  if (busy.value || dirty.value || props.proxyChanged || !status.value?.global_enabled || !status.value.enabled) return
  busy.value = true
  saved.value = false
  error.value = ''
  revision++
  const currentGeneration = generation
  try {
    const next = await harvestCodexAccountTicket(props.accountId)
    if (generation === currentGeneration) status.value = next
  } catch {
    if (generation === currentGeneration) error.value = t('admin.accounts.stateTicket.harvestFailed')
  } finally {
    if (generation === currentGeneration) busy.value = false
  }
}

watch(() => [props.accountId, props.visible] as const, async () => {
  generation++
  const currentGeneration = generation
  clearTimeout(timer)
  status.value = null
  enabled.value = false
  ticketPlan.value = 'pro'
  error.value = ''
  saved.value = false
  busy.value = false
  loading.value = false
  if (!props.visible) return
  await load(true)
  if (generation === currentGeneration && props.visible) schedulePoll()
}, { immediate: true })

onBeforeUnmount(() => { generation++; clearTimeout(timer) })
</script>
