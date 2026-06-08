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
        <div class="flex shrink-0 items-center gap-2">
          <label class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700">
            <input
              type="checkbox"
              v-model="showUninstalledOnly"
              class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-700"
              @change="reload"
            />
            <span>{{ t('admin.plugins.showUninstalledOnly') }}</span>
          </label>
          <button
            class="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
            :disabled="loading"
            @click="reload"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
        </div>
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
        <PluginCard
          v-for="p in visiblePlugins"
          :key="p.name"
          :plugin="p"
          :is-busy="!!busy[p.name]"
          :now-ms="nowMs"
          @detail="openDetail"
          @settings="openSettings"
          @enable="(name) => act(name, 'enable')"
          @disable="(name) => act(name, 'disable')"
          @restart="(name) => act(name, 'restart')"
          @uninstall="openUninstallDialog"
          @restore="restorePlugin"
          @purge="openPurgeDialog"
          @remove-files="openRemoveFilesDialog"
        />
      </div>
    </div>

    <PluginDetailDialog
      :plugin="detailPlugin"
      :initial-tab="detailInitialTab"
      :now-ms="nowMs"
      @close="closeDetail"
    />

    <PluginPurgeDialogs
      :remove-files-target="removeFilesTarget"
      :uninstall-target="uninstallTarget"
      :purge-target="purgeTarget"
      :purge-name-input="purgeNameInput"
      :purge-error="purgeError"
      :purge-busy="purgeBusy"
      @confirm-remove-files="confirmRemoveFiles"
      @cancel-remove-files="removeFilesTarget = null"
      @confirm-uninstall="confirmUninstall"
      @cancel-uninstall="uninstallTarget = null"
      @confirm-purge="confirmPurge"
      @cancel-purge="closePurgeDialog"
      @update:purge-name-input="purgeNameInput = $event"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { EmptyState, Icon } from '@sub2api/plugin-sdk'
import { apiClient } from '@/api/client'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { PluginInfo, PluginAction } from './pluginHelpers'
import PluginCard from './PluginCard.vue'
import PluginDetailDialog from './PluginDetailDialog.vue'
import PluginPurgeDialogs from './PluginPurgeDialogs.vue'

const UPTIME_TICK_MS = 60_000

const { t } = useI18n()
const appStore = useAppStore()

const plugins = ref<PluginInfo[]>([])
const loading = ref(false)
const loadError = ref('')
const busy = reactive<Record<string, boolean>>({})
const showUninstalledOnly = ref(false)
const nowMs = ref<number>(Date.now())
let uptimeTimer: ReturnType<typeof setInterval> | null = null

// Dialog state
const uninstallTarget = ref<PluginInfo | null>(null)
const removeFilesTarget = ref<PluginInfo | null>(null)
const purgeTarget = ref<PluginInfo | null>(null)
const purgeNameInput = ref('')
const purgeError = ref('')
const purgeBusy = ref(false)

// Detail dialog
const detailName = ref('')
const detailInitialTab = ref<'detail' | 'settings'>('detail')
const detailPlugin = computed<PluginInfo | undefined>(() =>
  plugins.value.find((p) => p.name === detailName.value)
)

const visiblePlugins = computed<PluginInfo[]>(() => {
  if (showUninstalledOnly.value) {
    return plugins.value.filter((p) => !!p.uninstalled_at)
  }
  return plugins.value.filter((p) => !p.uninstalled_at)
})

async function reload() {
  loading.value = true
  loadError.value = ''
  try {
    const params = showUninstalledOnly.value ? { include_uninstalled: true } : undefined
    const resp = await apiClient.get('/admin/plugins', { params })
    const list = (resp.data?.items || []) as PluginInfo[]
    plugins.value = Array.isArray(list) ? list : []
    if (detailName.value && !plugins.value.some((p) => p.name === detailName.value)) {
      detailName.value = ''
    }
  } catch (err: unknown) {
    loadError.value = extractApiErrorMessage(err, t('common.error'))
  } finally {
    loading.value = false
  }
}

function openDetail(name: string) {
  detailName.value = name
  detailInitialTab.value = 'detail'
}

function openSettings(name: string) {
  detailName.value = name
  detailInitialTab.value = 'settings'
}

function closeDetail() {
  detailName.value = ''
}

async function act(name: string, action: PluginAction) {
  busy[name] = true
  try {
    await apiClient.post(`/admin/plugins/${name}/${action}`)
    appStore.showSuccess(t(`admin.plugins.${action}Success`, { name }))
    await reload()
    if (action === 'enable' || action === 'disable') {
      window.location.reload()
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busy[name] = false
  }
}

function openUninstallDialog(p: PluginInfo) {
  uninstallTarget.value = p
}

async function confirmUninstall() {
  const target = uninstallTarget.value
  if (!target) return
  busy[target.name] = true
  try {
    await apiClient.post(`/admin/plugins/${target.name}/uninstall`)
    appStore.showSuccess(t('admin.plugins.uninstallSuccess', { name: target.name }))
    uninstallTarget.value = null
    await reload()
    window.location.reload()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busy[target.name] = false
  }
}

function openRemoveFilesDialog(p: PluginInfo) {
  removeFilesTarget.value = p
}

async function confirmRemoveFiles() {
  const target = removeFilesTarget.value
  if (!target) return
  busy[target.name] = true
  try {
    await apiClient.post(`/admin/plugins/${target.name}/remove-files`)
    appStore.showSuccess(t('admin.plugins.removeFilesSuccess', { name: target.name }))
    removeFilesTarget.value = null
    await reload()
    window.location.reload()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busy[target.name] = false
  }
}

async function restorePlugin(name: string) {
  busy[name] = true
  try {
    await apiClient.post(`/admin/plugins/${name}/install`)
    appStore.showSuccess(t('admin.plugins.restoreSuccess', { name }))
    await reload()
    window.location.reload()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busy[name] = false
  }
}

function openPurgeDialog(p: PluginInfo) {
  purgeTarget.value = p
  purgeNameInput.value = ''
  purgeError.value = ''
  purgeBusy.value = false
}

function closePurgeDialog() {
  purgeTarget.value = null
  purgeNameInput.value = ''
  purgeError.value = ''
  purgeBusy.value = false
}

function purgeErrorMessage(status: number, fallback: string): string {
  if (status === 409) return t('admin.plugins.purgeConfirm.errorMustSoftUninstall')
  if (status === 404) return t('admin.plugins.purgeConfirm.errorNotFound')
  return fallback
}

async function confirmPurge() {
  const target = purgeTarget.value
  if (!target || purgeNameInput.value !== target.name) return
  purgeBusy.value = true
  purgeError.value = ''
  try {
    await apiClient.delete(`/admin/plugins/${target.name}`, {
      params: { purge: true },
      data: { name: target.name },
    })
    appStore.showSuccess(t('admin.plugins.purgeSuccess', { name: target.name }))
    closePurgeDialog()
    await reload()
  } catch (err: unknown) {
    const status = (err as { status?: number })?.status ?? 0
    purgeError.value = purgeErrorMessage(status, extractApiErrorMessage(err, t('common.error')))
  } finally {
    purgeBusy.value = false
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
