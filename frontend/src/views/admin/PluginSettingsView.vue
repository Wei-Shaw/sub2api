<template>
  <div class="space-y-6 p-6">
    <header>
      <h1 class="text-xl font-semibold text-gray-900 dark:text-gray-100">
        {{ t('admin.pluginSettings.title') }}
      </h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.pluginSettings.description') }}
      </p>
    </header>

    <div v-if="loading" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="plugins.length === 0" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.pluginSettings.noPlugins') }}
    </div>

    <div v-else class="grid grid-cols-1 gap-6 lg:grid-cols-[200px_1fr]">
      <!-- Tab list -->
      <nav class="space-y-1">
        <button
          v-for="name in plugins"
          :key="name"
          type="button"
          class="block w-full rounded-md px-3 py-2 text-left text-sm"
          :class="[
            activePlugin === name
              ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/40 dark:text-blue-200'
              : 'text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700'
          ]"
          @click="selectPlugin(name)"
        >
          {{ name }}
        </button>
      </nav>

      <!-- Form panel -->
      <section>
        <div v-if="loadingDetail" class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('common.loading') }}
        </div>
        <PluginSettingsForm
          v-else-if="currentInfo"
          :info="currentInfo"
          @updated="onUpdated"
        />
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import {
  pluginSettingsApi,
  type PluginSettingsSchemaInfo
} from '@/api/admin/pluginSettings'
import PluginSettingsForm from '@/components/admin/PluginSettingsForm.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const loadingDetail = ref(false)
const plugins = ref<string[]>([])
const activePlugin = ref<string | null>(null)
const currentInfo = ref<PluginSettingsSchemaInfo | null>(null)

async function loadList() {
  loading.value = true
  try {
    plugins.value = await pluginSettingsApi.list()
    if (plugins.value.length > 0 && !activePlugin.value) {
      await selectPlugin(plugins.value[0])
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

async function selectPlugin(name: string) {
  activePlugin.value = name
  loadingDetail.value = true
  try {
    currentInfo.value = await pluginSettingsApi.get(name)
  } catch (err: unknown) {
    currentInfo.value = null
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loadingDetail.value = false
  }
}

function onUpdated(key: string, value: unknown) {
  if (currentInfo.value) {
    currentInfo.value.values[key] = value
  }
}

onMounted(loadList)
</script>
