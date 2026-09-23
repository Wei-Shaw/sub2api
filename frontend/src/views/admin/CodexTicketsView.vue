<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6" data-testid="ticket-management">
      <section class="relative overflow-hidden rounded-2xl bg-gradient-to-br from-slate-900 via-slate-900 to-teal-950 p-6 text-white shadow-sm sm:p-8">
        <div class="pointer-events-none absolute -right-16 -top-28 h-80 w-80 rounded-full border border-white/5 bg-white/[0.025]"></div>
        <div class="relative flex flex-col justify-between gap-6 sm:flex-row sm:items-center">
          <div class="min-w-0">
            <div class="mb-4 flex items-center gap-3">
              <span class="flex h-10 w-10 items-center justify-center rounded-xl bg-white/10"><Icon name="bolt" size="md" /></span>
              <span class="inline-flex items-center gap-2 rounded-full border border-white/15 px-3 py-1 text-xs font-medium text-slate-200" data-testid="ticket-enabled-status">
                <span class="h-1.5 w-1.5 rounded-full" :class="runtime?.enabled ? 'bg-emerald-400' : 'bg-slate-400'"></span>
                {{ t(`${prefix}.${!runtime ? 'ticketLoading' : runtime.enabled ? 'ticketEnabled' : 'ticketPaused'}`) }}
              </span>
            </div>
            <h2 class="text-2xl font-semibold tracking-tight sm:text-3xl">{{ t(`${prefix}.ticketHeroTitle`) }}</h2>
            <p class="mt-2 max-w-xl text-sm leading-6 text-slate-300">{{ t(`${prefix}.ticketHeroDescription`) }}</p>
            <div v-if="runtime" class="mt-4 flex flex-wrap gap-2">
              <span v-for="model in runtime.models" :key="model" class="rounded-md bg-white/5 px-2.5 py-1 font-mono text-xs text-slate-300">{{ model }}</span>
            </div>
          </div>
          <div class="flex shrink-0 items-center justify-between gap-5 rounded-xl border border-white/10 bg-white/5 px-4 py-3 sm:flex-col sm:items-end">
            <span id="ticket-switch-label" class="text-sm font-medium text-slate-200">{{ t(`${prefix}.ticketSwitch`) }}</span>
            <Toggle id="ticket-master-switch" :model-value="runtime?.enabled ?? false" :disabled="!runtime || loading || saving || savingProxy || actionBusy" aria-labelledby="ticket-switch-label" class="disabled:cursor-wait disabled:opacity-50" @update:model-value="setEnabled" />
          </div>
        </div>
      </section>

      <div v-if="error" role="alert" class="flex items-center justify-between gap-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-900/20 dark:text-red-300">
        <span>{{ error }}</span>
        <button type="button" class="shrink-0 font-medium underline" :disabled="loading" @click="loadOverview">{{ t(`${prefix}.poolRefresh`) }}</button>
      </div>
      <div class="grid grid-cols-2 gap-3 xl:grid-cols-4 sm:gap-4">
        <div v-for="metric in metrics" :key="metric.label" class="min-w-0 rounded-2xl border border-gray-200/80 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-5">
          <div class="flex items-center justify-between gap-2">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400 sm:text-sm">{{ metric.label }}</p>
            <Icon :name="metric.icon" size="sm" class="shrink-0 text-gray-400" />
          </div>
          <p class="mt-3 text-2xl font-semibold tracking-tight text-gray-900 dark:text-white sm:text-3xl" :data-testid="metric.testId">{{ metric.value }}</p>
          <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ metric.hint }}</p>
        </div>
      </div>

      <div class="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
        <section class="min-w-0 overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="border-b border-gray-100 p-5 dark:border-dark-700 sm:p-6">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t(`${prefix}.ticketSupply`) }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t(`${prefix}.ticketSupplyDescription`) }}</p>
            <div class="mt-5 inline-flex max-w-full gap-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-800" role="tablist" :aria-label="t(`${prefix}.ticketSupply`)" @keydown.left.prevent="switchTab" @keydown.right.prevent="switchTab">
              <button v-for="item in tabs" :id="`ticket-tab-${item.id}`" :key="item.id" type="button" role="tab" :aria-selected="tab === item.id" :aria-controls="`ticket-panel-${item.id}`" :tabindex="tab === item.id ? 0 : -1" :disabled="actionBusy" class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50" :class="tab === item.id ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-white'" @click="tab = item.id">
                <Icon :name="item.icon" size="sm" />{{ t(`${prefix}.${item.label}`) }}
              </button>
            </div>
          </div>
          <div class="p-5 sm:p-6">
            <p v-if="pool?.managed && pool.count === 0" role="status" class="mb-4 rounded-xl bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">{{ t(`${prefix}.poolEmpty`) }}</p>
            <div v-show="tab === 'api'" id="ticket-panel-api" role="tabpanel" aria-labelledby="ticket-tab-api" tabindex="0">
              <OnesProxyImport embedded @imported="onImported" @saved="onProviderSaved" @busy-change="setActionBusy" />
            </div>
            <div v-show="tab === 'manual'" id="ticket-panel-manual" role="tabpanel" aria-labelledby="ticket-tab-manual" tabindex="0">
              <TicketProxyPool embedded @imported="onImported" @busy-change="setActionBusy" />
            </div>
          </div>
        </section>

        <aside class="space-y-4">
          <section class="rounded-2xl border border-gray-200/80 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-6">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t(`${prefix}.ticketRules`) }}</h2>
            <ol class="mt-5 space-y-6">
              <li v-for="(rule, index) in rules" :key="rule.title" class="flex gap-3">
                <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-xs font-semibold text-gray-500 dark:bg-dark-800 dark:text-gray-400">{{ String(index + 1).padStart(2, '0') }}</span>
                <div><p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ rule.title }}</p><p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ rule.hint }}</p></div>
              </li>
            </ol>
            <div class="mt-6 flex gap-2.5 rounded-xl bg-teal-50 p-3 text-xs leading-5 text-teal-800 dark:bg-teal-950/50 dark:text-teal-200"><Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0" /><span>{{ t(`${prefix}.ticketBackground`) }}</span></div>
          </section>
          <details class="rounded-2xl border border-gray-200/80 bg-white p-5 dark:border-dark-700 dark:bg-dark-900">
            <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-200">{{ t(`${prefix}.ticketAdvanced`) }}</summary>
            <p class="mt-3 text-xs leading-5 text-gray-500">{{ t(`${prefix}.ticketAdvancedHint`) }}</p>
            <label for="ticket-single-proxy" class="sr-only">{{ t(`${prefix}.ticketAdvanced`) }}</label>
            <input id="ticket-single-proxy" v-model="proxyInput" type="password" class="input mt-3 w-full text-sm" autocomplete="new-password" :placeholder="t(`${prefix}.${runtime?.harvest_proxy_configured ? 'apiSecretSaved' : 'ticketAdvancedPlaceholder'}`)" :disabled="savingProxy" />
            <button type="button" class="btn btn-secondary mt-3 w-full text-sm" :disabled="!runtime || loading || savingProxy || saving || actionBusy" @click="saveProxy">{{ t(`${prefix}.ticketSaveProxy`) }}</button>
            <p v-if="proxyMessage" class="mt-2 text-xs text-emerald-600" role="status">{{ proxyMessage }}</p>
            <p v-if="proxyError" class="mt-2 text-xs text-red-600" role="alert">{{ proxyError }}</p>
          </details>
        </aside>
      </div>
      <div class="flex flex-wrap items-center justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ updatedAt ? t(`${prefix}.ticketUpdated`, { time: updatedAt }) : t(`${prefix}.ticketLoading`) }}</span>
        <button type="button" class="inline-flex items-center gap-2 rounded-lg px-3 py-2 hover:bg-gray-100 disabled:opacity-50 dark:hover:bg-dark-800" :disabled="loading || saving || savingProxy || actionBusy" @click="loadOverview"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />{{ t(`${prefix}.poolRefresh`) }}</button>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import type { TicketHarvestStatus, TicketProxyPoolStatus, TicketProxyProviderStatus } from '@/api/admin/settings'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import OnesProxyImport from '@/views/admin/settings/OnesProxyImport.vue'
import TicketProxyPool from '@/views/admin/settings/TicketProxyPool.vue'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t, locale } = useI18n()
const prefix = 'admin.settings.gatewayForwarding'
const runtime = ref<TicketHarvestStatus | null>(null)
const pool = ref<TicketProxyPoolStatus | null>(null)
const provider = ref<TicketProxyProviderStatus | null>(null)
const loading = ref(false)
const saving = ref(false)
const savingProxy = ref(false)
const actionBusy = ref(false)
const error = ref('')
const updatedAt = ref('')
const proxyInput = ref('')
const proxyMessage = ref('')
const proxyError = ref('')
const tab = ref<'api' | 'manual'>('api')
const tabs = [{ id: 'api', label: 'ticketApiTab', icon: 'bolt' }, { id: 'manual', label: 'ticketManualTab', icon: 'upload' }] as const
const minutes = computed(() => runtime.value ? Math.ceil(runtime.value.refresh_before_seconds / 60) : '—')
const number = (value: number | undefined) => value === undefined ? '—' : new Intl.NumberFormat(locale.value).format(value)
const metrics = computed(() => [
  { label: t(`${prefix}.ticketStock`), value: number(pool.value?.count), hint: t(`${prefix}.ticketStockHint`), icon: 'server', testId: 'ticket-pool-count' },
  { label: t(`${prefix}.ticketRemoved`), value: number(pool.value?.removed_count), hint: t(`${prefix}.ticketRemovedHint`), icon: 'shield', testId: 'ticket-removed-count' },
  { label: t(`${prefix}.ticketRenewal`), value: runtime.value ? t(`${prefix}.ticketMinutes`, { minutes: minutes.value }) : '—', hint: t(`${prefix}.ticketRenewalHint`), icon: 'clock', testId: 'ticket-refresh-minutes' },
  { label: t(`${prefix}.ticketRefill`), value: provider.value ? t(`${prefix}.${provider.value.auto_refill ? 'ticketEnabled' : 'ticketPaused'}`) : '—', hint: t(`${prefix}.ticketRefillHint`), icon: 'refresh', testId: 'ticket-refill-status' },
] as const)
const rules = computed(() => [
  { title: t(`${prefix}.ticketRenewRule`, { minutes: minutes.value }), hint: t(`${prefix}.ticketRenewRuleHint`) },
  { title: t(`${prefix}.ticketRemoveRule`, { count: pool.value?.failure_limit ?? '—' }), hint: t(`${prefix}.ticketRemoveRuleHint`) },
  { title: t(`${prefix}.ticketRefillRule`), hint: t(`${prefix}.${provider.value?.auto_refill ? 'ticketRefillRuleHint' : 'ticketRefillDisabledHint'}`) },
])
let timer: ReturnType<typeof setInterval> | undefined
let disposed = false
let revision = 0
function stamp() { updatedAt.value = new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date()) }
async function loadOverview() {
  if (loading.value || saving.value || savingProxy.value || actionBusy.value || disposed) return
  loading.value = true
  const requestRevision = revision
  try {
    const [policy, poolState, supplier] = await Promise.all([adminAPI.settings.getTicketHarvestStatus(), adminAPI.settings.getTicketProxyPool(), adminAPI.settings.getTicketProxyProvider()])
    if (disposed || requestRevision !== revision) return
    runtime.value = policy; pool.value = poolState; provider.value = supplier
    error.value = ''; stamp()
  } catch (err) { if (!disposed) error.value = extractApiErrorMessage(err, t(`${prefix}.ticketLoadError`)) }
  finally { loading.value = false }
}
async function setEnabled(enabled: boolean) {
  if (!runtime.value || saving.value || savingProxy.value || loading.value || actionBusy.value) return
  saving.value = true; error.value = ''
  try {
    const updated = await adminAPI.settings.updateSettings({ openai_codex_ticket_enabled: enabled })
    if (!disposed && runtime.value) { runtime.value.enabled = updated.openai_codex_ticket_enabled; stamp() }
  } catch (err) { if (!disposed) error.value = extractApiErrorMessage(err, t(`${prefix}.ticketSaveError`)) }
  finally { saving.value = false }
}
async function saveProxy() {
  if (!runtime.value || loading.value || savingProxy.value || saving.value || actionBusy.value) return
  savingProxy.value = true; proxyError.value = ''; proxyMessage.value = ''
  try {
    const updated = await adminAPI.settings.updateSettings({ openai_codex_ticket_harvest_proxy_url: proxyInput.value.trim() })
    if (!disposed && runtime.value) {
      runtime.value.harvest_proxy_configured = updated.openai_codex_ticket_harvest_proxy_configured
      proxyInput.value = ''; proxyMessage.value = t(`${prefix}.ticketSaved`)
    }
  } catch (err) { if (!disposed) proxyError.value = extractApiErrorMessage(err, t(`${prefix}.ticketSaveError`)) }
  finally { savingProxy.value = false }
}
function setActionBusy(busy: boolean) { actionBusy.value = busy; if (busy) revision++ }
function onProviderSaved(result: TicketProxyProviderStatus) { revision++; provider.value = result; stamp() }
function onImported(result: TicketProxyPoolStatus) { revision++; pool.value = result; stamp() }
async function switchTab() {
  if (actionBusy.value) return
  tab.value = tab.value === 'api' ? 'manual' : 'api'
  await nextTick()
  document.getElementById(`ticket-tab-${tab.value}`)?.focus()
}
onMounted(() => { void loadOverview(); timer = setInterval(() => { void loadOverview() }, 15000) })
onUnmounted(() => { disposed = true; if (timer) clearInterval(timer) })
</script>
