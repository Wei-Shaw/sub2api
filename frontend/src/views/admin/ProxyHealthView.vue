<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
            {{ t('admin.proxyHealth.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.proxyHealth.description') }}
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center gap-2"
            :disabled="loading"
            @click="loadAll"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('admin.proxyHealth.refresh') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center gap-2"
            :disabled="scanning || loading"
            @click="runScan"
          >
            <Icon name="play" size="sm" :class="scanning ? 'animate-spin' : ''" />
            {{ scanning ? t('admin.proxyHealth.scanRunning') : t('admin.proxyHealth.scan') }}
          </button>
          <RouterLink to="/admin/proxies" class="btn btn-secondary">
            {{ t('admin.proxyHealth.openProxies') }}
          </RouterLink>
        </div>
      </div>

      <div v-if="loading && !runtime" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
          <div
            v-for="item in overviewItems"
            :key="item.key"
            class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          >
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</p>
            <div class="mt-1 flex items-baseline gap-2">
              <p class="text-xl font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
              <span
                v-if="item.badge"
                class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                :class="item.badgeClass"
              >
                {{ item.badge }}
              </span>
            </div>
            <p v-if="item.meta" class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
              {{ item.meta }}
            </p>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <!-- Settings form -->
          <div class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.proxyHealth.configTitle') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.proxyHealth.configHint') }}
              </p>
            </div>
            <div class="space-y-4 p-6">
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded" />
                {{ t('admin.proxyHealth.enabled') }}
              </label>

              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label class="input-label">{{ t('admin.proxyHealth.intervalSec') }}</label>
                  <input v-model.number="form.interval_sec" type="number" min="10" class="input" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.proxyHealth.timeoutMs') }}</label>
                  <input v-model.number="form.timeout_ms" type="number" min="1000" class="input" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.proxyHealth.concurrency') }}</label>
                  <input v-model.number="form.concurrency" type="number" min="1" max="64" class="input" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.proxyHealth.batchSize') }}</label>
                  <input v-model.number="form.batch_size" type="number" min="1" max="500" class="input" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.proxyHealth.failThreshold') }}</label>
                  <input v-model.number="form.fail_threshold" type="number" min="1" class="input" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.proxyHealth.successThreshold') }}</label>
                  <input
                    v-model.number="form.success_threshold"
                    type="number"
                    min="1"
                    class="input"
                  />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.proxyHealth.leaderLockTtl') }}</label>
                  <input
                    v-model.number="form.leader_lock_ttl_sec"
                    type="number"
                    min="10"
                    class="input"
                  />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.proxyHealth.probeScope') }}</label>
                  <select v-model="form.probe_scope" class="input">
                    <option value="group_members">{{ t('admin.proxyHealth.probeScopeGroup') }}</option>
                    <option value="all_active">{{ t('admin.proxyHealth.probeScopeAll') }}</option>
                  </select>
                </div>
                <div>
                  <label class="input-label">{{ t('admin.proxyHealth.probeMode') }}</label>
                  <select v-model="form.probe_mode" class="input">
                    <option value="connectivity">
                      {{ t('admin.proxyHealth.probeModeConnectivity') }}
                    </option>
                    <option value="quality">{{ t('admin.proxyHealth.probeModeQuality') }}</option>
                  </select>
                </div>
              </div>

              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="form.auto_recover" type="checkbox" class="h-4 w-4 rounded" />
                {{ t('admin.proxyHealth.autoRecover') }}
              </label>

              <div>
                <label class="input-label">{{ t('admin.proxyHealth.skipNamePrefix') }}</label>
                <input
                  v-model="skipPrefixText"
                  type="text"
                  class="input"
                  :placeholder="t('admin.proxyHealth.skipNamePrefixHint')"
                />
                <p class="mt-1 text-xs text-gray-500">{{ t('admin.proxyHealth.skipNamePrefixHint') }}</p>
              </div>

              <div class="flex justify-end">
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="saving"
                  @click="saveConfig"
                >
                  {{ t('admin.proxyHealth.save') }}
                </button>
              </div>
            </div>
          </div>

          <!-- Metrics detail -->
          <div class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.proxyHealth.recentTitle') }}
              </h2>
            </div>
            <div class="p-6">
              <div class="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
                <div
                  v-for="m in metricItems"
                  :key="m.key"
                  class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/50"
                >
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ m.label }}</p>
                  <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ m.value }}</p>
                </div>
              </div>

              <div
                v-if="!runtime?.recent_isolated?.length"
                class="py-8 text-center text-sm text-gray-500"
              >
                {{ t('admin.proxyHealth.recentEmpty') }}
              </div>
              <div v-else class="overflow-x-auto">
                <table class="min-w-full text-sm">
                  <thead>
                    <tr class="border-b border-gray-100 text-left text-xs text-gray-500 dark:border-dark-700">
                      <th class="px-2 py-2 font-medium">{{ t('admin.proxyHealth.colName') }}</th>
                      <th class="px-2 py-2 font-medium">{{ t('admin.proxyHealth.colEndpoint') }}</th>
                      <th class="px-2 py-2 font-medium">{{ t('admin.proxyHealth.colStatus') }}</th>
                      <th class="px-2 py-2 font-medium">{{ t('admin.proxyHealth.colFail') }}</th>
                      <th class="px-2 py-2 font-medium">{{ t('admin.proxyHealth.colLast') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="row in runtime.recent_isolated"
                      :key="row.id"
                      class="border-b border-gray-50 dark:border-dark-800"
                    >
                      <td class="px-2 py-2 font-medium text-gray-900 dark:text-gray-100">
                        {{ row.name }}
                      </td>
                      <td class="px-2 py-2 font-mono text-xs text-gray-500">
                        {{ row.protocol }}://{{ row.host }}:{{ row.port }}
                      </td>
                      <td class="px-2 py-2">
                        <span :class="proxyStatusBadgeClass(row.status)">
                          {{ t(proxyStatusLabelKey(row.status)) }}
                        </span>
                      </td>
                      <td class="px-2 py-2">{{ row.health_fail_count }}</td>
                      <td class="px-2 py-2 text-xs text-gray-500">
                        {{ formatUnix(row.last_health_at) }}
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import adminAPI from '@/api/admin'
import type { ProxyHealthRuntime, ProxyHealthSettings } from '@/api/admin/proxies'
import {
  proxyStatusBadgeClass,
  proxyStatusLabelKey
} from '@/utils/proxyStatus'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const scanning = ref(false)
const runtime = ref<ProxyHealthRuntime | null>(null)
const skipPrefixText = ref('warp-')

const form = reactive<ProxyHealthSettings>({
  enabled: false,
  interval_sec: 60,
  timeout_ms: 10000,
  concurrency: 8,
  fail_threshold: 3,
  success_threshold: 2,
  probe_scope: 'group_members',
  auto_recover: true,
  skip_name_prefix: ['warp-'],
  leader_lock_ttl_sec: 50,
  batch_size: 100,
  probe_mode: 'connectivity'
})

let timer: ReturnType<typeof setInterval> | null = null

const overviewItems = computed(() => {
  const r = runtime.value
  const running = !!r?.worker_running
  return [
    {
      key: 'worker',
      label: t('admin.proxyHealth.workerRunning'),
      value: running
        ? t('admin.proxyHealth.workerRunning')
        : t('admin.proxyHealth.workerStopped'),
      badge: running ? 'ON' : 'OFF',
      badgeClass: running
        ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
        : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
      meta: r?.worker_instance_id
        ? `${t('admin.proxyHealth.instanceId')}: ${r.worker_instance_id}`
        : undefined
    },
    {
      key: 'tick',
      label: t('admin.proxyHealth.lastTick'),
      value:
        r?.last_tick_age_sec != null
          ? t('admin.proxyHealth.lastTickAge', { sec: r.last_tick_age_sec })
          : t('admin.proxyHealth.lastTickNever'),
      badge: undefined,
      badgeClass: '',
      meta: r?.yaml_enabled ? t('admin.proxyHealth.yamlEnabled') : undefined
    },
    {
      key: 'isolated',
      label: t('admin.proxyHealth.isolatedCount'),
      value: String(r?.isolated_count ?? 0),
      badge: undefined,
      badgeClass: '',
      meta: undefined
    },
    {
      key: 'ticks',
      label: t('admin.proxyHealth.ticks'),
      value: String(r?.metrics?.ticks ?? 0),
      badge: undefined,
      badgeClass: '',
      meta: undefined
    }
  ]
})

const metricItems = computed(() => {
  const m = runtime.value?.metrics
  return [
    { key: 'probed', label: t('admin.proxyHealth.probedTotal'), value: m?.probed_total ?? 0 },
    { key: 'isolated', label: t('admin.proxyHealth.isolatedTotal'), value: m?.isolated_total ?? 0 },
    { key: 'recovered', label: t('admin.proxyHealth.recoveredTotal'), value: m?.recovered_total ?? 0 },
    { key: 'errors', label: t('admin.proxyHealth.errorTotal'), value: m?.error_total ?? 0 },
    { key: 'skipped', label: t('admin.proxyHealth.skippedTotal'), value: m?.skipped_total ?? 0 },
    { key: 'ticks', label: t('admin.proxyHealth.ticks'), value: m?.ticks ?? 0 }
  ]
})

function applySettings(s: ProxyHealthSettings) {
  Object.assign(form, {
    ...s,
    skip_name_prefix: [...(s.skip_name_prefix || [])]
  })
  skipPrefixText.value = (s.skip_name_prefix || []).join(', ')
}

function formatUnix(ts?: number) {
  if (!ts) return '—'
  try {
    return new Date(ts * 1000).toLocaleString()
  } catch {
    return '—'
  }
}

async function loadAll() {
  loading.value = true
  try {
    const [rt, cfg] = await Promise.all([
      adminAPI.proxies.getHealthRuntime(),
      adminAPI.proxies.getHealthConfig()
    ])
    runtime.value = rt
    applySettings(cfg)
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.proxyHealth.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  saving.value = true
  try {
    const prefixes = skipPrefixText.value
      .split(/[,，\s]+/)
      .map((s) => s.trim())
      .filter(Boolean)
    const payload: ProxyHealthSettings = {
      ...form,
      skip_name_prefix: prefixes.length ? prefixes : ['warp-']
    }
    const saved = await adminAPI.proxies.updateHealthConfig(payload)
    applySettings(saved)
    appStore.showSuccess(t('admin.proxyHealth.saved'))
    await loadAll()
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.proxyHealth.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function runScan() {
  scanning.value = true
  try {
    const res = await adminAPI.proxies.healthScan()
    appStore.showSuccess(
      t('admin.proxyHealth.scanDone', {
        probed: res.probed,
        isolated: res.isolated,
        recovered: res.recovered,
        skipped: res.skipped,
        errors: res.errors
      })
    )
    await loadAll()
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.proxyHealth.scanFailed'))
  } finally {
    scanning.value = false
  }
}

onMounted(() => {
  loadAll()
  timer = setInterval(() => {
    if (!loading.value && !saving.value && !scanning.value) {
      loadAll()
    }
  }, 30000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
