<template>
  <AppLayout>
    <div class="space-y-4">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold">{{ t('admin.plugins.title') }}</h1>
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.plugins.description') }}
        </p>
      </div>
      <button
        class="rounded-md border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-100 dark:border-gray-600 dark:hover:bg-gray-700"
        :disabled="loading"
        @click="reload"
      >
        {{ t('common.refresh') }}
      </button>
    </div>

    <div v-if="loadError" class="rounded-md border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-300">
      {{ loadError }}
    </div>

    <div v-if="loading && plugins.length === 0" class="text-sm text-gray-500">{{ t('common.loading') }}</div>

    <div v-else-if="plugins.length === 0" class="rounded-md border border-dashed border-gray-300 p-8 text-center text-gray-500 dark:border-gray-600">
      {{ t('admin.plugins.empty') }}
    </div>

    <div v-else class="overflow-hidden rounded-md border border-gray-200 dark:border-gray-700">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-gray-700">
        <thead class="bg-gray-50 dark:bg-gray-800/50">
          <tr>
            <th class="px-4 py-2 text-left">{{ t('admin.plugins.name') }}</th>
            <th class="px-4 py-2 text-left">{{ t('admin.plugins.state') }}</th>
            <th class="px-4 py-2 text-left">{{ t('admin.plugins.builtin') }}</th>
            <th class="px-4 py-2 text-left">{{ t('admin.plugins.uptime') }}</th>
            <th class="px-4 py-2 text-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
          <tr v-for="p in plugins" :key="p.name">
            <td class="px-4 py-2 align-top">
              <div class="font-medium">{{ p.display_name || p.name }}</div>
              <div class="text-xs text-gray-500">{{ p.name }} <span v-if="p.version">· v{{ p.version }}</span></div>
              <div v-if="p.description" class="mt-1 text-xs text-gray-500">{{ p.description }}</div>
            </td>
            <td class="px-4 py-2 align-top">
              <span :class="stateBadgeClass(p.state)">{{ p.state || '—' }}</span>
              <div v-if="p.last_error" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ p.last_error }}</div>
            </td>
            <td class="px-4 py-2 align-top">
              <span v-if="p.builtin" class="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
                {{ t('admin.plugins.builtin') }}
              </span>
              <span v-else class="text-xs text-gray-500">—</span>
            </td>
            <td class="px-4 py-2 align-top text-xs text-gray-500">
              <span v-if="p.started_at">{{ formatTime(p.started_at) }}</span>
              <span v-else>—</span>
            </td>
            <td class="px-4 py-2 align-top text-right">
              <button
                v-if="p.enabled"
                class="rounded-md border border-gray-300 px-2 py-1 text-xs hover:bg-gray-100 dark:border-gray-600 dark:hover:bg-gray-700"
                :disabled="busy[p.name]"
                @click="act(p.name, 'restart')"
              >
                {{ t('admin.plugins.restart') }}
              </button>
              <button
                v-if="p.enabled"
                class="ml-2 rounded-md border border-amber-300 px-2 py-1 text-xs text-amber-700 hover:bg-amber-50 dark:border-amber-700 dark:text-amber-300 dark:hover:bg-amber-900/30"
                :disabled="busy[p.name]"
                @click="act(p.name, 'disable')"
              >
                {{ t('admin.plugins.disable') }}
              </button>
              <button
                v-else
                class="rounded-md border border-emerald-300 px-2 py-1 text-xs text-emerald-700 hover:bg-emerald-50 dark:border-emerald-700 dark:text-emerald-300 dark:hover:bg-emerald-900/30"
                :disabled="busy[p.name]"
                @click="act(p.name, 'enable')"
              >
                {{ t('admin.plugins.enable') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()
const appStore = useAppStore()

const plugins = ref<PluginInfo[]>([])
const loading = ref(false)
const loadError = ref('')
const busy = reactive<Record<string, boolean>>({})

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

async function act(name: string, action: 'enable' | 'disable' | 'restart') {
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

function stateBadgeClass(state?: string): string {
  const base = 'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium'
  switch ((state || '').toLowerCase()) {
    case 'running':
      return `${base} bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300`
    case 'starting':
    case 'restarting':
      return `${base} bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300`
    case 'errored':
      return `${base} bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300`
    default:
      return `${base} bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300`
  }
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

onMounted(reload)
</script>
