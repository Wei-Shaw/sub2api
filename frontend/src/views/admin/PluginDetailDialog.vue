<template>
  <BaseDialog
    v-if="plugin"
    :show="!!plugin"
    :title="plugin.display_name || plugin.name"
    width="wide"
    @close="$emit('close')"
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
        v-if="plugin.has_settings"
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

    <!-- Settings tab -->
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

    <!-- Detail tab -->
    <div v-else class="space-y-5 pt-4">
      <DetailBasicInfo :plugin="plugin" />
      <DetailRuntimeInfo :plugin="plugin" :now-ms="nowMs" />

      <!-- Last error -->
      <section v-if="plugin.last_error">
        <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-red-600 dark:text-red-400">
          {{ t('admin.plugins.lastError') }}
        </h4>
        <div class="rounded-md border border-red-200 bg-red-50 p-3 text-xs text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
          <div class="break-all font-mono">{{ plugin.last_error }}</div>
        </div>
      </section>

      <DetailCapabilities :capabilities="plugin.capabilities" />
      <DetailConfigTable :config="plugin.config" />
    </div>

    <template #footer>
      <button
        class="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 shadow-sm hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
        @click="$emit('close')"
      >
        {{ t('common.close') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog } from '@sub2api/plugin-sdk'
import {
  pluginSettingsApi,
  type PluginSettingsSchemaInfo,
} from '@/api/admin/pluginSettings'
import PluginSettingsForm from '@/components/admin/PluginSettingsForm.vue'
import type { PluginInfo } from './pluginHelpers'
import DetailBasicInfo from './detail/DetailBasicInfo.vue'
import DetailRuntimeInfo from './detail/DetailRuntimeInfo.vue'
import DetailCapabilities from './detail/DetailCapabilities.vue'
import DetailConfigTable from './detail/DetailConfigTable.vue'

type DetailTab = 'detail' | 'settings'

const props = defineProps<{
  plugin: PluginInfo | undefined
  initialTab?: DetailTab
  nowMs: number
}>()

defineEmits<{
  close: []
}>()

const { t } = useI18n()

const activeTab = ref<DetailTab>(props.initialTab ?? 'detail')
const settingsLoading = ref(false)
const settingsInfo = ref<PluginSettingsSchemaInfo | null>(null)

function setTab(tab: DetailTab) {
  activeTab.value = tab
  if (tab === 'settings' && props.plugin) {
    void loadSettings(props.plugin.name)
  }
}

async function loadSettings(name: string) {
  settingsLoading.value = true
  try {
    settingsInfo.value = await pluginSettingsApi.get(name)
  } catch {
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

watch(
  () => props.plugin?.name,
  (newName, oldName) => {
    if (newName !== oldName) {
      activeTab.value = props.initialTab ?? 'detail'
      settingsInfo.value = null
      if (props.initialTab === 'settings' && newName) {
        void loadSettings(newName)
      }
    }
  },
)
</script>
