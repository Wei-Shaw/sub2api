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
      <div
        v-if="loading && plugins.length === 0"
        class="rounded-md border border-gray-200 bg-white p-6 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800/60"
      >
        {{ t('common.loading') }}
      </div>

      <!-- Empty state -->
      <div
        v-else-if="plugins.length === 0"
        class="rounded-xl border border-dashed border-gray-300 bg-white/60 p-8 dark:border-dark-600 dark:bg-dark-800/40"
      >
        <EmptyState :title="t('admin.plugins.empty')" />
      </div>

      <!-- Card grid -->
      <div
        v-else
        class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
      >
        <article
          v-for="p in plugins"
          :key="p.name"
          class="group relative flex flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-card transition-all hover:shadow-card-hover dark:border-dark-700 dark:bg-dark-800/60"
        >
          <!-- Builtin accent strip on the left edge -->
          <span
            v-if="p.builtin"
            class="absolute inset-y-0 left-0 w-1 bg-primary-500"
            aria-hidden="true"
          />

          <div class="flex flex-1 flex-col p-4">
            <!-- Top: icon + display_name + builtin badge -->
            <div class="flex items-start gap-3">
              <span
                class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-500/10 text-primary-600 dark:text-primary-300 [&_svg]:h-6 [&_svg]:w-6"
              >
                <span
                  v-if="p.icon_svg"
                  v-html="sanitizeSvg(p.icon_svg)"
                  aria-hidden="true"
                />
                <Icon v-else name="cube" size="md" />
              </span>
              <div class="min-w-0 flex-1">
                <div class="flex items-start justify-between gap-2">
                  <h2 class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
                    {{ p.display_name || p.name }}
                  </h2>
                  <span
                    v-if="p.builtin"
                    class="inline-flex shrink-0 items-center rounded-full bg-blue-100 px-1.5 py-0.5 text-[10px] font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
                  >
                    {{ t('admin.plugins.builtinTag') }}
                  </span>
                </div>
                <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">
                  {{ p.name }}<span v-if="p.version"> · v{{ p.version }}</span>
                </p>
              </div>
            </div>

            <!-- Status row -->
            <div class="mt-3 flex items-center gap-2 text-xs">
              <span
                :class="['inline-block h-2 w-2 shrink-0 rounded-full', dotClass(p.state, p.enabled)]"
                aria-hidden="true"
              />
              <span class="font-medium text-gray-700 dark:text-gray-200">{{ stateLabel(p) }}</span>
              <span
                v-if="p.enabled && p.started_at && !isZeroTime(p.started_at)"
                class="ml-auto truncate text-gray-500 dark:text-gray-400"
              >
                {{ t('admin.plugins.uptimePrefix') }} {{ formatUptime(p.started_at) }}
              </span>
            </div>

            <!-- Description (line-clamp 2) -->
            <p class="mt-3 line-clamp-2 min-h-[2.5rem] text-xs text-gray-600 dark:text-gray-300">
              <span v-if="p.description">{{ p.description }}</span>
              <span v-else class="text-gray-400 dark:text-gray-500">—</span>
            </p>

            <!-- Last error (compact) -->
            <div
              v-if="p.last_error"
              class="mt-3 flex items-start gap-1.5 rounded-md border border-red-200 bg-red-50 px-2 py-1.5 text-[11px] text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300"
            >
              <Icon name="exclamationCircle" size="xs" class="mt-0.5 shrink-0" />
              <span class="line-clamp-2 break-all">{{ p.last_error }}</span>
            </div>

            <!-- Actions -->
            <div class="mt-4 flex flex-wrap items-center gap-1.5 pt-2">
              <button
                class="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 shadow-sm hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
                @click="openDetail(p.name)"
              >
                <Icon name="infoCircle" size="xs" />
                {{ t('admin.plugins.detailButton') }}
              </button>
              <button
                v-if="p.has_settings === true"
                class="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 shadow-sm hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
                @click="openSettings(p.name)"
              >
                <Icon name="cog" size="xs" />
                {{ t('admin.plugins.settingsButton') }}
              </button>
              <button
                v-if="p.enabled"
                class="inline-flex items-center gap-1 rounded-md border border-amber-300 bg-amber-50 px-2 py-1 text-xs font-medium text-amber-700 hover:bg-amber-100 disabled:opacity-60 dark:border-amber-700 dark:bg-amber-900/20 dark:text-amber-300 dark:hover:bg-amber-900/40"
                :disabled="busy[p.name]"
                @click="act(p.name, 'disable')"
              >
                {{ t('admin.plugins.disable') }}
              </button>
              <button
                v-else
                class="inline-flex items-center gap-1 rounded-md border border-emerald-300 bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 hover:bg-emerald-100 disabled:opacity-60 dark:border-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/40"
                :disabled="busy[p.name]"
                @click="act(p.name, 'enable')"
              >
                {{ t('admin.plugins.enable') }}
              </button>
              <button
                class="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
                :disabled="!p.enabled || busy[p.name]"
                @click="act(p.name, 'restart')"
              >
                <Icon name="refresh" size="xs" :class="busy[p.name] ? 'animate-spin' : ''" />
                {{ t('admin.plugins.restart') }}
              </button>
            </div>
          </div>
        </article>
      </div>
    </div>

    <!-- Detail modal: 详情 + 设置 (tab 切换, 设置按需加载) -->
    <BaseDialog
      v-if="detailPlugin"
      :show="!!detailPlugin"
      :title="detailPlugin.display_name || detailPlugin.name"
      width="wide"
      @close="closeDetail"
    >
      <div class="-mt-2 flex gap-1 border-b border-gray-200 dark:border-dark-700">
        <button
          type="button"
          class="-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors"
          :class="activeTab === 'detail'
            ? 'border-primary-500 text-primary-700 dark:text-primary-300'
            : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="setTab('detail')"
        >
          {{ t('admin.plugins.tabDetail') }}
        </button>
        <button
          v-if="detailPlugin.has_settings"
          type="button"
          class="-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors"
          :class="activeTab === 'settings'
            ? 'border-primary-500 text-primary-700 dark:text-primary-300'
            : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="setTab('settings')"
        >
          {{ t('admin.plugins.tabSettings') }}
        </button>
      </div>

      <div v-if="activeTab === 'settings'" class="pt-4">
        <div v-if="settingsLoading" class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('common.loading') }}
        </div>
        <div
          v-else-if="!settingsInfo"
          class="rounded-md border border-dashed border-gray-300 bg-gray-50/60 p-4 text-sm text-gray-500 dark:border-dark-600 dark:bg-dark-800/40 dark:text-gray-400"
        >
          {{ t('admin.plugins.settingsNotDeclared') }}
        </div>
        <template v-else>
          <!-- SETTINGS-V2 schema version banner (DESIGN §5.5). Older hosts
               that have not yet rolled out W3-A omit the field; we hide
               the badge in that case. -->
          <div
            v-if="settingsInfo.schema_version"
            class="mb-3 inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-mono text-gray-600 dark:bg-dark-700 dark:text-gray-300"
          >
            {{ t('admin.pluginSettings.schemaVersion', { version: settingsInfo.schema_version }) }}
          </div>
          <PluginSettingsForm
            :info="settingsInfo"
            @updated="onSettingsUpdated"
          />
        </template>
      </div>

      <div v-else class="space-y-5 pt-4">
        <!-- Basic info -->
        <section>
          <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ t('admin.plugins.basicInfoTitle') }}
          </h4>
          <dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
            <div class="flex flex-col gap-0.5">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.name') }}</dt>
              <dd class="break-all font-mono text-xs text-gray-900 dark:text-gray-100">{{ detailPlugin.name }}</dd>
            </div>
            <div class="flex flex-col gap-0.5">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.version') }}</dt>
              <dd class="text-gray-900 dark:text-gray-100">
                <span v-if="detailPlugin.version">v{{ detailPlugin.version }}</span>
                <span v-else class="text-gray-400">—</span>
              </dd>
            </div>
            <div class="flex flex-col gap-0.5">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.builtin') }}</dt>
              <dd class="text-gray-900 dark:text-gray-100">
                {{ detailPlugin.builtin ? t('admin.plugins.builtinTag') : t('admin.plugins.externalTag') }}
              </dd>
            </div>
            <div class="flex flex-col gap-0.5">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.sortOrder') }}</dt>
              <dd class="text-gray-900 dark:text-gray-100">{{ detailPlugin.sort_order ?? 0 }}</dd>
            </div>
            <div v-if="detailPlugin.description" class="flex flex-col gap-0.5 sm:col-span-2">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.descriptionLabel') }}</dt>
              <dd class="text-sm text-gray-700 dark:text-gray-300">{{ detailPlugin.description }}</dd>
            </div>
          </dl>
        </section>

        <!-- Runtime info -->
        <section>
          <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ t('admin.plugins.metadataTitle') }}
          </h4>
          <dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
            <div class="flex flex-col gap-0.5">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.state') }}</dt>
              <dd class="flex items-center gap-1.5 text-gray-900 dark:text-gray-100">
                <span :class="['inline-block h-2 w-2 rounded-full', dotClass(detailPlugin.state, detailPlugin.enabled)]" />
                {{ stateLabel(detailPlugin) }}
              </dd>
            </div>
            <div class="flex flex-col gap-0.5">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.uptime') }}</dt>
              <dd class="text-gray-900 dark:text-gray-100">
                <span v-if="detailPlugin.started_at && !isZeroTime(detailPlugin.started_at)">
                  {{ formatUptime(detailPlugin.started_at) }}
                  <span class="ml-1 text-xs text-gray-400">({{ formatTime(detailPlugin.started_at) }})</span>
                </span>
                <span v-else class="text-gray-400">—</span>
              </dd>
            </div>
            <div class="flex flex-col gap-0.5">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.restartCount') }}</dt>
              <dd class="text-gray-900 dark:text-gray-100">{{ detailPlugin.restart_count ?? 0 }}</dd>
            </div>
            <div class="flex flex-col gap-0.5">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.grpcAddr') }}</dt>
              <dd class="break-all font-mono text-xs text-gray-900 dark:text-gray-100">
                <span v-if="detailPlugin.grpc_addr">{{ detailPlugin.grpc_addr }}</span>
                <span v-else class="text-gray-400">—</span>
              </dd>
            </div>
            <div class="flex flex-col gap-0.5 sm:col-span-2">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.httpAddr') }}</dt>
              <dd class="break-all font-mono text-xs text-gray-900 dark:text-gray-100">
                <span v-if="detailPlugin.http_addr">{{ detailPlugin.http_addr }}</span>
                <span v-else class="text-gray-400">—</span>
              </dd>
            </div>
          </dl>
        </section>

        <!-- Last error -->
        <section v-if="detailPlugin.last_error">
          <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-red-600 dark:text-red-400">
            {{ t('admin.plugins.lastError') }}
          </h4>
          <div class="rounded-md border border-red-200 bg-red-50 p-3 text-xs text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
            <div class="break-all font-mono">{{ detailPlugin.last_error }}</div>
          </div>
        </section>

        <!-- Config table -->
        <section v-if="detailConfigEntries.length > 0">
          <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ t('admin.plugins.configTitle') }}
            <span class="ml-1 text-gray-400 dark:text-gray-500">
              ({{ t('admin.plugins.configKeysCount', { count: detailConfigEntries.length }) }})
            </span>
          </h4>
          <div class="overflow-hidden rounded-md border border-gray-200 dark:border-dark-700">
            <table class="min-w-full text-sm">
              <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                <tr>
                  <th class="px-3 py-2 text-left font-medium">key</th>
                  <th class="px-3 py-2 text-left font-medium">value</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700/70">
                <tr v-for="entry in detailConfigEntries" :key="entry.key">
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
        </section>
      </div>

      <template #footer>
        <button
          class="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 shadow-sm hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
          @click="closeDetail"
        >
          {{ t('common.close') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog, EmptyState, Icon } from '@sub2api/plugin-sdk'
import { apiClient } from '@/api/client'
import {
  pluginSettingsApi,
  type PluginSettingsSchemaInfo,
} from '@/api/admin/pluginSettings'
import PluginSettingsForm from '@/components/admin/PluginSettingsForm.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { sanitizeSvg } from '@/utils/sanitize'

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
  /** Manifest-supplied SVG icon markup; empty falls back to the cube icon. */
  icon_svg?: string
  /**
   * True when the plugin manifest declares a non-empty SettingsSchema.
   * The Settings tab is only rendered for plugins that opted in.
   */
  has_settings?: boolean
}

// Plugin lifecycle action keys (must match backend POST endpoints).
type PluginAction = 'enable' | 'disable' | 'restart'

// Refresh "now" reference every minute so card uptime stays current without
// hammering the layout. We only re-render when the displayed value changes.
const UPTIME_TICK_MS = 60_000

const { t } = useI18n()
const appStore = useAppStore()

const plugins = ref<PluginInfo[]>([])
const loading = ref(false)
const loadError = ref('')
const busy = reactive<Record<string, boolean>>({})

// Detail modal: open by plugin name; computed reference reads from `plugins`.
const detailName = ref<string>('')
const detailPlugin = computed<PluginInfo | undefined>(() =>
  plugins.value.find((p) => p.name === detailName.value)
)

// Used by formatUptime to compute "now - started_at". Updated on a timer so
// uptime labels do not go stale while the page is open.
const nowMs = ref<number>(Date.now())
let uptimeTimer: ReturnType<typeof setInterval> | null = null

// Detail dialog tab state. Settings 是 W3 SettingsExtension 暴露的可调字段,
// 没声明 schema 的插件 (即 list API 不返回的) 显示"未声明可配置项"空态.
type DetailTab = 'detail' | 'settings'
const activeTab = ref<DetailTab>('detail')
const settingsLoading = ref(false)
const settingsInfo = ref<PluginSettingsSchemaInfo | null>(null)

const detailConfigEntries = computed<{ key: string; display: string }[]>(() => {
  const cfg = detailPlugin.value?.config
  if (!cfg) return []
  return Object.keys(cfg)
    .sort((a, b) => a.localeCompare(b))
    .map((key) => ({
      key,
      display: formatConfigValue(cfg[key])
    }))
})

async function reload() {
  loading.value = true
  loadError.value = ''
  try {
    const resp = await apiClient.get('/admin/plugins')
    const list = (resp.data?.items || []) as PluginInfo[]
    plugins.value = Array.isArray(list) ? list : []
    // If the detail modal is open and the plugin disappeared, close it.
    if (detailName.value && !plugins.value.some((p) => p.name === detailName.value)) {
      detailName.value = ''
    }
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
    // enable/disable 改变 active plugin 集合 — sidebar 菜单 + router 路由都基于
    // 启动时 SSR 注入的 window.__PLUGIN_MANIFESTS__, 不响应后续变更. 强制全页
    // reload 让浏览器重新拿到新 manifest, 即时反映启用/禁用结果. restart 不影响
    // manifest 内容, 不需要 reload.
    if (action === 'enable' || action === 'disable') {
      window.location.reload()
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busy[name] = false
  }
}

function openDetail(name: string) {
  detailName.value = name
  activeTab.value = 'detail'
  settingsInfo.value = null
}

// Open detail modal pre-focused on the Settings tab. Equivalent to clicking
// "详情" then switching tabs, but done atomically so the user lands on the
// settings form directly. has_settings should be true at the call site
// (button is hidden otherwise); we still defensively load — if schema is
// missing the form falls back to the "settingsNotDeclared" empty state.
function openSettings(name: string) {
  detailName.value = name
  activeTab.value = 'settings'
  settingsInfo.value = null
  void loadSettings(name)
}

function closeDetail() {
  detailName.value = ''
  settingsInfo.value = null
}

function setTab(tab: DetailTab) {
  activeTab.value = tab
  if (tab === 'settings' && detailName.value) {
    void loadSettings(detailName.value)
  }
}

async function loadSettings(name: string) {
  settingsLoading.value = true
  try {
    settingsInfo.value = await pluginSettingsApi.get(name)
  } catch {
    // 404 / 权限不足等都按"未声明" 处理. 真实错误会通过浏览器控制台和 toast
    // 路径暴露 (apiClient 拦截器), 这里只用 null 触发 fallback UI.
    settingsInfo.value = null
  } finally {
    settingsLoading.value = false
  }
}

function onSettingsUpdated(key: string, value: unknown) {
  if (settingsInfo.value) {
    settingsInfo.value.values[key] = value
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
    case '':
    case undefined:
      return t('admin.plugins.stateUnknown')
    case 'errored':
      return t('admin.plugins.stateErrored')
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

// Format duration "now - started_at" as "2h 30m" / "5m" / "10s" / "3d 2h".
// Returns "—" placeholder for invalid input.
function formatUptime(iso: string): string {
  const ts = Date.parse(iso)
  if (!Number.isFinite(ts)) return '—'
  let secs = Math.max(0, Math.floor((nowMs.value - ts) / 1000))
  const days = Math.floor(secs / 86400)
  secs -= days * 86400
  const hours = Math.floor(secs / 3600)
  secs -= hours * 3600
  const mins = Math.floor(secs / 60)
  secs -= mins * 60
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  if (mins > 0) return `${mins}m`
  return `${secs}s`
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

onMounted(() => {
  reload()
  uptimeTimer = setInterval(() => {
    nowMs.value = Date.now()
  }, UPTIME_TICK_MS)
})

onUnmounted(() => {
  if (uptimeTimer) {
    clearInterval(uptimeTimer)
    uptimeTimer = null
  }
})
</script>
