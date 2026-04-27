<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Page header -->
    <div class="flex items-center justify-between gap-3">
      <div class="min-w-0">
        <h1 class="text-xl font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.plugins.title') }}</h1>
        <p class="mt-0.5 truncate text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.plugins.description') }}
        </p>
      </div>
      <button
        class="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
        :disabled="loading"
        @click="reload"
      >
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        {{ t('common.refresh') }}
      </button>
    </div>

    <!-- Load error -->
    <div
      v-if="loadError"
      class="rounded-md border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-300"
    >
      {{ loadError }}
    </div>

    <!-- Loading -->
    <div v-if="loading && plugins.length === 0" class="rounded-md border border-gray-200 bg-white p-6 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800/60">
      {{ t('common.loading') }}
    </div>

    <!-- Whole page empty -->
    <div
      v-else-if="plugins.length === 0"
      class="rounded-md border border-dashed border-gray-300 bg-white/60 p-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:bg-dark-800/40"
    >
      {{ t('admin.plugins.empty') }}
    </div>

    <!-- Mobile selector (md:hidden) -->
    <div v-else class="md:hidden">
      <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
        {{ t('admin.plugins.listTitle') }}
      </label>
      <select
        v-model="selectedName"
        class="block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 shadow-sm focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100"
      >
        <option v-for="p in plugins" :key="p.name" :value="p.name">
          {{ (p.display_name || p.name) }} · {{ stateLabel(p) }}
        </option>
      </select>
    </div>

    <!-- Desktop / tablet split -->
    <div v-if="plugins.length > 0" class="grid grid-cols-1 gap-4 md:grid-cols-4">
      <!-- Left: list (desktop only) -->
      <aside class="hidden md:col-span-1 md:block">
        <div class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-card dark:border-dark-700 dark:bg-dark-800/60">
          <div class="border-b border-gray-100 px-3 py-2 dark:border-dark-700/70">
            <div class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('admin.plugins.listTitle') }}
              <span class="ml-1 text-gray-400 dark:text-gray-500">({{ plugins.length }})</span>
            </div>
            <div v-if="showSearch" class="relative mt-2">
              <Icon name="search" size="sm" class="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-gray-400" />
              <input
                v-model="search"
                type="text"
                :placeholder="t('admin.plugins.searchPlaceholder')"
                class="w-full rounded-md border border-gray-300 bg-white py-1.5 pl-7 pr-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-100"
              />
            </div>
          </div>

          <ul class="max-h-[70vh] overflow-y-auto py-1">
            <li v-if="filteredPlugins.length === 0" class="px-3 py-6 text-center text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.plugins.noResults') }}
            </li>
            <li v-for="p in filteredPlugins" :key="p.name">
              <button
                class="group flex w-full items-center gap-2.5 px-2.5 py-2 text-left transition-colors"
                :class="[
                  selectedName === p.name
                    ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                    : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-dark-700/60'
                ]"
                @click="selectedName = p.name"
              >
                <span
                  class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md"
                  :class="selectedName === p.name
                    ? 'bg-primary-500/15 text-primary-600 dark:text-primary-300'
                    : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400 group-hover:bg-gray-200 dark:group-hover:bg-dark-600'"
                >
                  <Icon name="cube" size="sm" />
                </span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm font-medium">
                    {{ p.display_name || p.name }}
                  </span>
                  <span class="block truncate text-[11px] text-gray-500 dark:text-gray-400">
                    <span v-if="p.version">v{{ p.version }} · </span>{{ p.name }}
                  </span>
                </span>
                <span :class="['inline-block h-2 w-2 shrink-0 rounded-full', dotClass(p.state, p.enabled)]" />
              </button>
            </li>
          </ul>
        </div>
      </aside>

      <!-- Right: detail -->
      <section class="md:col-span-3">
        <template v-if="selected">
          <!-- Overview / hero card -->
          <div class="rounded-xl border border-gray-200 bg-white p-5 shadow-card dark:border-dark-700 dark:bg-dark-800/60">
            <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
              <div class="flex min-w-0 items-start gap-3">
                <span class="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-primary-500/10 text-primary-600 dark:text-primary-300">
                  <Icon name="cube" size="lg" />
                </span>
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-gray-100">
                      {{ selected.display_name || selected.name }}
                    </h2>
                    <span :class="stateBadgeClass(selected.state, selected.enabled)">
                      <span :class="['mr-1 inline-block h-1.5 w-1.5 rounded-full', dotClass(selected.state, selected.enabled)]" />
                      {{ stateLabel(selected) }}
                    </span>
                    <span v-if="selected.builtin" class="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
                      {{ t('admin.plugins.builtinTag') }}
                    </span>
                  </div>
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ selected.name }}<span v-if="selected.version"> · v{{ selected.version }}</span>
                  </p>
                  <p v-if="selected.description" class="mt-2 text-sm text-gray-600 dark:text-gray-300">
                    {{ selected.description }}
                  </p>
                </div>
              </div>

              <!-- Actions -->
              <div class="flex shrink-0 flex-wrap items-center gap-2">
                <button
                  v-if="selected.enabled"
                  class="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
                  :disabled="busy[selected.name]"
                  @click="act(selected.name, 'restart')"
                >
                  <Icon name="refresh" size="xs" :class="busy[selected.name] ? 'animate-spin' : ''" />
                  {{ t('admin.plugins.restart') }}
                </button>
                <button
                  v-if="selected.enabled"
                  class="inline-flex items-center gap-1 rounded-md border border-amber-300 bg-amber-50 px-2.5 py-1.5 text-xs font-medium text-amber-700 hover:bg-amber-100 disabled:opacity-60 dark:border-amber-700 dark:bg-amber-900/20 dark:text-amber-300 dark:hover:bg-amber-900/40"
                  :disabled="busy[selected.name]"
                  @click="act(selected.name, 'disable')"
                >
                  {{ t('admin.plugins.disable') }}
                </button>
                <button
                  v-else
                  class="inline-flex items-center gap-1 rounded-md border border-emerald-300 bg-emerald-50 px-2.5 py-1.5 text-xs font-medium text-emerald-700 hover:bg-emerald-100 disabled:opacity-60 dark:border-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/40"
                  :disabled="busy[selected.name]"
                  @click="act(selected.name, 'enable')"
                >
                  {{ t('admin.plugins.enable') }}
                </button>
              </div>
            </div>

            <!-- Last error -->
            <div
              v-if="selected.last_error"
              class="mt-4 rounded-md border border-red-200 bg-red-50 p-3 text-xs text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300"
            >
              <div class="mb-1 font-semibold">{{ t('admin.plugins.lastError') }}</div>
              <div class="break-all">{{ selected.last_error }}</div>
            </div>
          </div>

          <!-- Runtime / metadata card -->
          <div class="mt-4 rounded-xl border border-gray-200 bg-white p-5 shadow-card dark:border-dark-700 dark:bg-dark-800/60">
            <div class="mb-3 flex items-center gap-2">
              <Icon name="cpu" size="sm" class="text-gray-500 dark:text-gray-400" />
              <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
                {{ t('admin.plugins.metadataTitle') }}
              </h3>
            </div>
            <dl class="grid grid-cols-1 gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
              <div class="flex flex-col gap-0.5">
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.uptime') }}</dt>
                <dd class="text-gray-900 dark:text-gray-100">
                  <span v-if="selected.started_at && !isZeroTime(selected.started_at)">
                    {{ formatTime(selected.started_at) }}
                  </span>
                  <span v-else class="text-gray-400">—</span>
                </dd>
              </div>
              <div class="flex flex-col gap-0.5">
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.restartCount') }}</dt>
                <dd class="text-gray-900 dark:text-gray-100">
                  {{ selected.restart_count ?? 0 }}
                </dd>
              </div>
              <div class="flex flex-col gap-0.5">
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.grpcAddr') }}</dt>
                <dd class="break-all font-mono text-xs text-gray-900 dark:text-gray-100">
                  <span v-if="selected.grpc_addr">{{ selected.grpc_addr }}</span>
                  <span v-else class="text-gray-400">—</span>
                </dd>
              </div>
              <div class="flex flex-col gap-0.5">
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.httpAddr') }}</dt>
                <dd class="break-all font-mono text-xs text-gray-900 dark:text-gray-100">
                  <span v-if="selected.http_addr">{{ selected.http_addr }}</span>
                  <span v-else class="text-gray-400">—</span>
                </dd>
              </div>
              <div class="flex flex-col gap-0.5">
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.version') }}</dt>
                <dd class="text-gray-900 dark:text-gray-100">
                  <span v-if="selected.version">v{{ selected.version }}</span>
                  <span v-else class="text-gray-400">—</span>
                </dd>
              </div>
              <div class="flex flex-col gap-0.5">
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.sortOrder') }}</dt>
                <dd class="text-gray-900 dark:text-gray-100">
                  {{ selected.sort_order ?? 0 }}
                </dd>
              </div>
              <div class="flex flex-col gap-0.5">
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.configKeys') }}</dt>
                <dd class="text-gray-900 dark:text-gray-100">
                  {{ t('admin.plugins.configKeysCount', { count: configEntries.length }) }}
                </dd>
              </div>
              <div class="flex flex-col gap-0.5">
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.builtin') }}</dt>
                <dd class="text-gray-900 dark:text-gray-100">
                  <span v-if="selected.builtin">{{ t('admin.plugins.builtinTag') }}</span>
                  <span v-else>{{ t('admin.plugins.externalTag') }}</span>
                </dd>
              </div>
            </dl>
          </div>

          <!-- Config card (only if there are config entries) -->
          <div v-if="configEntries.length > 0" class="mt-4 rounded-xl border border-gray-200 bg-white p-5 shadow-card dark:border-dark-700 dark:bg-dark-800/60">
            <div class="mb-3 flex items-center gap-2">
              <Icon name="cog" size="sm" class="text-gray-500 dark:text-gray-400" />
              <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
                {{ t('admin.plugins.configTitle') }}
              </h3>
            </div>
            <div class="overflow-hidden rounded-md border border-gray-200 dark:border-dark-700">
              <table class="min-w-full text-sm">
                <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-3 py-2 text-left font-medium">key</th>
                    <th class="px-3 py-2 text-left font-medium">value</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700/70">
                  <tr v-for="entry in configEntries" :key="entry.key">
                    <td class="px-3 py-2 align-top font-mono text-xs text-gray-700 dark:text-gray-200">
                      {{ entry.key }}
                    </td>
                    <td class="px-3 py-2 align-top font-mono text-xs text-gray-600 dark:text-gray-300">
                      <pre class="whitespace-pre-wrap break-all">{{ entry.display }}</pre>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </template>

        <!-- Empty (no selection) -->
        <template v-else>
          <div class="flex h-full min-h-[280px] items-center justify-center rounded-xl border border-dashed border-gray-300 bg-white/60 p-8 dark:border-dark-600 dark:bg-dark-800/40">
            <EmptyState
              :title="t('admin.plugins.selectPrompt')"
              :description="t('admin.plugins.selectPromptDescription')"
            />
          </div>
        </template>
      </section>
    </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { EmptyState, Icon } from '@sub2api/plugin-sdk'
import { apiClient } from '@/api/client'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

interface PluginInfo {
  name: string
  display_name?: string
  version?: string
  description?: string
  enabled: boolean
  sort_order?: number
  state?: string
  grpc_addr?: string
  http_addr?: string
  restart_count?: number
  builtin?: boolean
  started_at?: string
  last_error?: string
  config?: Record<string, unknown>
}

// Plugin lifecycle action keys (must match backend POST endpoints).
type PluginAction = 'enable' | 'disable' | 'restart'

// Threshold above which the search box becomes useful.
const SEARCH_VISIBLE_THRESHOLD = 8

const { t } = useI18n()
const appStore = useAppStore()

const plugins = ref<PluginInfo[]>([])
const loading = ref(false)
const loadError = ref('')
const busy = reactive<Record<string, boolean>>({})

const selectedName = ref<string>('')
const search = ref('')

const showSearch = computed(() => plugins.value.length > SEARCH_VISIBLE_THRESHOLD)

const filteredPlugins = computed<PluginInfo[]>(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return plugins.value
  return plugins.value.filter((p) => {
    const haystack = [p.name, p.display_name, p.description].filter(Boolean).join(' ').toLowerCase()
    return haystack.includes(q)
  })
})

const selected = computed<PluginInfo | undefined>(() =>
  plugins.value.find((p) => p.name === selectedName.value)
)

const configEntries = computed<{ key: string; display: string }[]>(() => {
  const cfg = selected.value?.config
  if (!cfg) return []
  return Object.keys(cfg)
    .sort((a, b) => a.localeCompare(b))
    .map((key) => ({
      key,
      display: formatConfigValue(cfg[key])
    }))
})

watch(plugins, (list) => {
  // After reload: keep current selection if still present, otherwise default to first.
  if (list.length === 0) {
    selectedName.value = ''
    return
  }
  if (!list.some((p) => p.name === selectedName.value)) {
    selectedName.value = list[0].name
  }
})

async function reload() {
  loading.value = true
  loadError.value = ''
  try {
    const resp = await apiClient.get('/admin/plugins')
    const list = (resp.data?.items || []) as PluginInfo[]
    plugins.value = Array.isArray(list) ? list : []
  } catch (err: unknown) {
    loadError.value = extractApiErrorMessage(err, t('common.error'))
  } finally {
    loading.value = false
  }
}

async function act(name: string, action: PluginAction) {
  busy[name] = true
  try {
    await apiClient.post(`/admin/plugins/${name}/${action}`)
    appStore.showSuccess(t(`admin.plugins.${action}Success`, { name }))
    await reload()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busy[name] = false
  }
}

function stateBadgeClass(state?: string, enabled?: boolean): string {
  const base = 'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium'
  if (!enabled) {
    return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
  }
  switch ((state || '').toLowerCase()) {
    case 'running':
      return `${base} bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300`
    case 'starting':
    case 'restarting':
      return `${base} bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300`
    case 'errored':
      return `${base} bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300`
    default:
      return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
  }
}

function dotClass(state?: string, enabled?: boolean): string {
  if (!enabled) return 'bg-gray-400 dark:bg-gray-500'
  switch ((state || '').toLowerCase()) {
    case 'running':
      return 'bg-emerald-500'
    case 'starting':
    case 'restarting':
      return 'bg-amber-500'
    case 'errored':
      return 'bg-red-500'
    default:
      return 'bg-gray-400 dark:bg-gray-500'
  }
}

function stateLabel(p: PluginInfo): string {
  if (!p.enabled) return t('admin.plugins.stateDisabled')
  switch ((p.state || '').toLowerCase()) {
    case 'running':
      return t('admin.plugins.stateRunning')
    case 'starting':
      return t('admin.plugins.stateStarting')
    case 'restarting':
      return t('admin.plugins.stateRestarting')
    case 'errored':
      return t('admin.plugins.stateErrored')
    case '':
    case undefined:
      return t('admin.plugins.stateUnknown')
    default:
      return p.state || t('admin.plugins.stateUnknown')
  }
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

// Backend may serialize a zero time.Time as "0001-01-01T00:00:00Z" when omitempty
// fails to fire; treat it as "no start time".
function isZeroTime(iso: string): boolean {
  return iso.startsWith('0001-01-01')
}

function formatConfigValue(v: unknown): string {
  if (v === null || v === undefined) return '—'
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

onMounted(reload)
</script>
