<template>
  <section>
    <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
      {{ t('admin.plugins.basicInfoTitle') }}
    </h4>
    <dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
      <div class="flex flex-col gap-0.5">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.name') }}</dt>
        <dd class="break-all font-mono text-xs text-gray-900 dark:text-gray-100">{{ plugin.name }}</dd>
      </div>
      <div class="flex flex-col gap-0.5">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.version') }}</dt>
        <dd class="text-gray-900 dark:text-gray-100">
          <span v-if="plugin.version">v{{ plugin.version }}</span>
          <span v-else class="text-gray-400">—</span>
        </dd>
      </div>
      <div class="flex flex-col gap-0.5">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.builtin') }}</dt>
        <dd class="text-gray-900 dark:text-gray-100">
          {{ plugin.builtin ? t('admin.plugins.builtinTag') : t('admin.plugins.externalTag') }}
        </dd>
      </div>
      <div class="flex flex-col gap-0.5">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.sortOrder') }}</dt>
        <dd class="text-gray-900 dark:text-gray-100">{{ plugin.sort_order ?? 0 }}</dd>
      </div>
      <div v-if="plugin.description" class="flex flex-col gap-0.5 sm:col-span-2">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.descriptionLabel') }}</dt>
        <dd class="text-sm text-gray-700 dark:text-gray-300">{{ plugin.description }}</dd>
      </div>
    </dl>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { PluginInfo } from '../pluginHelpers'

defineProps<{ plugin: PluginInfo }>()

const { t } = useI18n()
</script>
