<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import * as api from './api'
import type {
  ConnectionRiskEvent,
  ConnectionRiskEventFilters,
  ConnectionRiskRuntime,
  ConnectionRiskSettings,
} from './types'

const { t } = useI18n()

const activeTab = ref<'events' | 'config' | 'runtime'>('events')
const loading = reactive({ events: false, config: false, runtime: false, action: false })
const error = ref('')
const message = ref('')

const filters = reactive<ConnectionRiskEventFilters>({
  status: 'open',
  severity: '',
  user_id: '',
  api_key_id: '',
  rule: '',
})
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const events = ref<ConnectionRiskEvent[]>([])
const selected = ref<ConnectionRiskEvent | null>(null)

const settings = ref<ConnectionRiskSettings | null>(null)
const runtime = ref<ConnectionRiskRuntime | null>(null)

const severityClass = (s: string) => {
  switch (s) {
    case 'critical':
      return 'bg-red-100 text-red-800 dark:bg-red-950/40 dark:text-red-200'
    case 'high':
      return 'bg-orange-100 text-orange-800 dark:bg-orange-950/40 dark:text-orange-200'
    case 'medium':
      return 'bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-200'
    case 'low':
      return 'bg-sky-100 text-sky-800 dark:bg-sky-950/40 dark:text-sky-200'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200'
  }
}

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

async function loadEvents() {
  loading.events = true
  error.value = ''
  try {
    const res = await api.listEvents(filters, page.value, pageSize.value)
    events.value = res.items ?? []
    total.value = res.total ?? 0
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.loadEvents')
  } finally {
    loading.events = false
  }
}

async function loadConfig() {
  loading.config = true
  try {
    settings.value = await api.getConfig()
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.loadConfig')
  } finally {
    loading.config = false
  }
}

async function loadRuntime() {
  loading.runtime = true
  try {
    runtime.value = await api.getRuntime()
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.loadRuntime')
  } finally {
    loading.runtime = false
  }
}

async function saveConfig() {
  if (!settings.value) return
  loading.action = true
  message.value = ''
  try {
    settings.value = await api.updateConfig(settings.value)
    message.value = t('admin.connectionRisk.messages.saved')
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.saveConfig')
  } finally {
    loading.action = false
  }
}

async function doAction(kind: 'ack' | 'resolve' | 'suppress' | 'delete', id: number) {
  loading.action = true
  error.value = ''
  try {
    if (kind === 'ack') await api.ackEvent(id)
    else if (kind === 'resolve') await api.resolveEvent(id)
    else if (kind === 'suppress') await api.suppressEvent(id)
    else await api.deleteEvent(id)
    message.value = t('admin.connectionRisk.messages.actionOk')
    selected.value = null
    await loadEvents()
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.action')
  } finally {
    loading.action = false
  }
}

async function whitelistFromEvidence() {
  if (!selected.value?.api_key_id) return
  const ips = (selected.value.evidence?.sample_ips as string[] | undefined) || []
  if (!ips.length) {
    error.value = t('admin.connectionRisk.errors.noSampleIPs')
    return
  }
  loading.action = true
  try {
    await api.whitelistIPs(selected.value.api_key_id, ips.slice(0, 10))
    if (selected.value.api_key_id) {
      await api.exemptSubject('k', selected.value.api_key_id, 'whitelist-from-ui')
    }
    message.value = t('admin.connectionRisk.messages.whitelisted')
    await loadEvents()
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.action')
  } finally {
    loading.action = false
  }
}

async function runRetention() {
  loading.action = true
  try {
    const res = await api.runRetention()
    message.value = t('admin.connectionRisk.messages.retention', { count: res.deleted ?? 0 })
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.action')
  } finally {
    loading.action = false
  }
}

function openEvent(ev: ConnectionRiskEvent) {
  selected.value = ev
}

function changePage(p: number) {
  page.value = p
  loadEvents()
}

onMounted(async () => {
  await Promise.all([loadEvents(), loadConfig(), loadRuntime()])
})
</script>

<template>
  <AppLayout>
    <div class="mx-auto max-w-[1400px] pb-10">
      <header class="mb-6">
        <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-400">
          {{ t('nav.securityAudit') }}
        </p>
        <h1 class="mt-1 text-2xl font-semibold text-gray-950 dark:text-white">
          {{ t('admin.connectionRisk.title') }}
        </h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-300">
          {{ t('admin.connectionRisk.description') }}
        </p>
      </header>

      <div v-if="error" role="alert" class="mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
        {{ error }}
        <button type="button" class="ml-3 underline" @click="error = ''">×</button>
      </div>
      <div v-if="message" role="status" class="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-200">
        {{ message }}
        <button type="button" class="ml-3 underline" @click="message = ''">×</button>
      </div>

      <div class="mb-4 tabs inline-flex" role="tablist">
        <button type="button" class="tab" :class="{ 'tab-active': activeTab === 'events' }" @click="activeTab = 'events'">
          {{ t('admin.connectionRisk.tabs.events') }}
        </button>
        <button type="button" class="tab" :class="{ 'tab-active': activeTab === 'config' }" @click="activeTab = 'config'; loadConfig()">
          {{ t('admin.connectionRisk.tabs.config') }}
        </button>
        <button type="button" class="tab" :class="{ 'tab-active': activeTab === 'runtime' }" @click="activeTab = 'runtime'; loadRuntime()">
          {{ t('admin.connectionRisk.tabs.runtime') }}
        </button>
      </div>

      <!-- Events -->
      <section v-show="activeTab === 'events'" class="card p-4 sm:p-6">
        <div class="mb-4 flex flex-wrap gap-3">
          <select v-model="filters.status" class="input input-sm w-36">
            <option value="">{{ t('admin.connectionRisk.filters.allStatus') }}</option>
            <option value="open">open</option>
            <option value="acknowledged">acknowledged</option>
            <option value="resolved">resolved</option>
            <option value="suppressed">suppressed</option>
          </select>
          <select v-model="filters.severity" class="input input-sm w-36">
            <option value="">{{ t('admin.connectionRisk.filters.allSeverity') }}</option>
            <option value="critical">critical</option>
            <option value="high">high</option>
            <option value="medium">medium</option>
            <option value="low">low</option>
          </select>
          <input v-model="filters.user_id" class="input input-sm w-28" :placeholder="t('admin.connectionRisk.filters.userId')" />
          <input v-model="filters.api_key_id" class="input input-sm w-28" :placeholder="t('admin.connectionRisk.filters.keyId')" />
          <input v-model="filters.rule" class="input input-sm w-24" placeholder="R1" />
          <button type="button" class="btn btn-primary btn-sm" :disabled="loading.events" @click="page = 1; loadEvents()">
            {{ t('common.search') }}
          </button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading.action" @click="runRetention">
            {{ t('admin.connectionRisk.actions.runRetention') }}
          </button>
        </div>

        <div v-if="loading.events" class="py-10 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full text-left text-sm">
            <thead class="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-dark-700">
              <tr>
                <th class="px-2 py-2">ID</th>
                <th class="px-2 py-2">{{ t('admin.connectionRisk.columns.severity') }}</th>
                <th class="px-2 py-2">{{ t('admin.connectionRisk.columns.score') }}</th>
                <th class="px-2 py-2">{{ t('admin.connectionRisk.columns.subject') }}</th>
                <th class="px-2 py-2">{{ t('admin.connectionRisk.columns.rules') }}</th>
                <th class="px-2 py-2">{{ t('admin.connectionRisk.columns.status') }}</th>
                <th class="px-2 py-2">{{ t('admin.connectionRisk.columns.lastSeen') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="ev in events"
                :key="ev.id"
                class="cursor-pointer border-b border-gray-100 hover:bg-gray-50 dark:border-dark-800 dark:hover:bg-dark-800/50"
                @click="openEvent(ev)"
              >
                <td class="px-2 py-2 font-mono text-xs">{{ ev.id }}</td>
                <td class="px-2 py-2">
                  <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="severityClass(ev.severity)">{{ ev.severity }}</span>
                </td>
                <td class="px-2 py-2">{{ ev.score?.toFixed?.(1) ?? ev.score }}</td>
                <td class="px-2 py-2 text-xs">
                  <div>key #{{ ev.api_key_id }} <span class="text-gray-400">{{ ev.api_key_prefix }}</span></div>
                  <div class="text-gray-500">user #{{ ev.user_id }}</div>
                </td>
                <td class="px-2 py-2 font-mono text-xs">{{ (ev.rules_fired || []).map((r) => r.rule_id).join(', ') }}</td>
                <td class="px-2 py-2">{{ ev.status }}</td>
                <td class="px-2 py-2 text-xs text-gray-500">{{ ev.last_seen_at }}</td>
              </tr>
              <tr v-if="!events.length">
                <td colspan="7" class="px-2 py-8 text-center text-gray-500">{{ t('admin.connectionRisk.empty') }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="mt-4 flex items-center justify-between text-sm">
          <span class="text-gray-500">{{ t('common.total') }}: {{ total }}</span>
          <div class="flex gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="page <= 1" @click="changePage(page - 1)">‹</button>
            <span>{{ page }} / {{ totalPages }}</span>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="page >= totalPages" @click="changePage(page + 1)">›</button>
          </div>
        </div>
      </section>

      <!-- Detail drawer -->
      <div v-if="selected" class="fixed inset-0 z-40 flex justify-end bg-black/30" @click.self="selected = null">
        <aside class="h-full w-full max-w-lg overflow-y-auto bg-white p-6 shadow-xl dark:bg-dark-900">
          <div class="mb-4 flex items-start justify-between">
            <div>
              <h2 class="text-lg font-semibold">{{ selected.title || `#${selected.id}` }}</h2>
              <p class="mt-1 text-sm text-gray-500">{{ selected.summary }}</p>
            </div>
            <button type="button" class="btn btn-ghost btn-sm" @click="selected = null">×</button>
          </div>
          <dl class="space-y-2 text-sm">
            <div><dt class="text-gray-500">severity</dt><dd>{{ selected.severity }} / {{ selected.score }}</dd></div>
            <div><dt class="text-gray-500">status</dt><dd>{{ selected.status }}</dd></div>
            <div><dt class="text-gray-500">rules</dt><dd class="font-mono text-xs">{{ JSON.stringify(selected.rules_fired, null, 2) }}</dd></div>
            <div><dt class="text-gray-500">evidence</dt><dd class="font-mono text-xs whitespace-pre-wrap">{{ JSON.stringify(selected.evidence, null, 2) }}</dd></div>
          </dl>
          <div class="mt-6 flex flex-wrap gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loading.action" @click="doAction('ack', selected.id)">{{ t('admin.connectionRisk.actions.ack') }}</button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loading.action" @click="doAction('resolve', selected.id)">{{ t('admin.connectionRisk.actions.resolve') }}</button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loading.action" @click="doAction('suppress', selected.id)">{{ t('admin.connectionRisk.actions.suppress') }}</button>
            <button type="button" class="btn btn-primary btn-sm" :disabled="loading.action || !selected.api_key_id" @click="whitelistFromEvidence">{{ t('admin.connectionRisk.actions.whitelist') }}</button>
            <button type="button" class="btn btn-danger btn-sm" :disabled="loading.action" @click="doAction('delete', selected.id)">{{ t('common.delete') }}</button>
          </div>
        </aside>
      </div>

      <!-- Config -->
      <section v-show="activeTab === 'config'" class="card space-y-4 p-4 sm:p-6">
        <div v-if="loading.config" class="py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
        <template v-else-if="settings">
          <label class="flex items-center gap-2 text-sm"><input v-model="settings.enabled" type="checkbox" /> {{ t('admin.connectionRisk.config.enabled') }}</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="settings.emit_enabled" type="checkbox" /> {{ t('admin.connectionRisk.config.emitEnabled') }}</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="settings.worker_enabled" type="checkbox" /> {{ t('admin.connectionRisk.config.workerEnabled') }}</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="settings.include_read_only_endpoints" type="checkbox" /> {{ t('admin.connectionRisk.config.includeReadOnly') }}</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="settings.r7_include_admin_actors" type="checkbox" /> {{ t('admin.connectionRisk.config.r7Admin') }}</label>
          <label v-if="settings.actions" class="flex items-center gap-2 text-sm"><input v-model="settings.actions.soft_throttle_enabled" type="checkbox" /> {{ t('admin.connectionRisk.config.softThrottle') }}</label>
          <label v-if="settings.actions" class="flex items-center gap-2 text-sm"><input v-model="settings.actions.auto_disable_enabled" type="checkbox" /> {{ t('admin.connectionRisk.config.autoDisable') }}</label>
          <div class="grid gap-3 sm:grid-cols-2">
            <label class="text-sm">{{ t('admin.connectionRisk.config.sampleRate') }}
              <input v-model.number="settings.emit_sample_rate_evidence" type="number" min="0" max="1" step="0.05" class="input input-sm mt-1 w-full" />
            </label>
            <label class="text-sm">{{ t('admin.connectionRisk.config.workerInterval') }}
              <input v-model.number="settings.worker_interval_seconds" type="number" min="60" max="300" class="input input-sm mt-1 w-full" />
            </label>
            <label v-if="settings.actions" class="text-sm">{{ t('admin.connectionRisk.config.throttleRpm') }}
              <input v-model.number="settings.actions.throttle_abs_rpm" type="number" min="0" class="input input-sm mt-1 w-full" />
            </label>
            <label class="text-sm">{{ t('admin.connectionRisk.config.retentionDays') }}
              <input v-model.number="settings.retention_days" type="number" min="1" class="input input-sm mt-1 w-full" />
            </label>
          </div>
          <p class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.connectionRisk.config.yamlHint') }}</p>
          <button type="button" class="btn btn-primary" :disabled="loading.action" @click="saveConfig">{{ t('common.save') }}</button>
        </template>
      </section>

      <!-- Runtime -->
      <section v-show="activeTab === 'runtime'" class="card p-4 sm:p-6">
        <div class="mb-3 flex justify-between">
          <h2 class="font-semibold">{{ t('admin.connectionRisk.tabs.runtime') }}</h2>
          <button type="button" class="btn btn-secondary btn-sm" @click="loadRuntime">{{ t('common.refresh') }}</button>
        </div>
        <pre class="overflow-x-auto rounded-lg bg-gray-50 p-4 text-xs dark:bg-dark-800">{{ JSON.stringify(runtime, null, 2) }}</pre>
      </section>
    </div>
  </AppLayout>
</template>
