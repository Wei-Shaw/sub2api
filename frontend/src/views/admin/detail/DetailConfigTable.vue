<template>
  <section v-if="entries.length > 0">
    <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
      {{ t('admin.plugins.configTitle') }}
      <span class="ml-1 text-gray-400 dark:text-gray-500">
        ({{ t('admin.plugins.configKeysCount', { count: entries.length }) }})
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
          <tr v-for="entry in entries" :key="entry.key">
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
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatConfigValue } from '../pluginHelpers'

const props = defineProps<{
  config?: Record<string, unknown>
}>()

const { t } = useI18n()

const entries = computed<{ key: string; display: string }[]>(() => {
  const cfg = props.config
  if (!cfg) return []
  return Object.keys(cfg)
    .sort((a, b) => a.localeCompare(b))
    .map((key) => ({
      key,
      display: formatConfigValue(cfg[key])
    }))
})
</script>
