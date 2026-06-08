<template>
  <section>
    <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
      {{ t('admin.plugins.permissions.title') }}
    </h4>
    <div
      v-if="!capabilities || capabilities.length === 0"
      class="rounded-md border border-dashed border-gray-300 bg-gray-50/60 px-3 py-2 text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-800/40 dark:text-gray-400"
    >
      {{ t('admin.plugins.permissions.empty') }}
    </div>
    <ul v-else class="space-y-1.5">
      <li
        v-for="cap in capabilities"
        :key="cap"
        class="flex items-center gap-2 text-xs"
      >
        <span
          v-if="capabilityCategory(cap) === 'default'"
          class="inline-flex shrink-0 items-center gap-1 rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300"
        >
          <Icon name="checkCircle" size="xs" />
          {{ t('admin.plugins.permissions.defaultGranted') }}
        </span>
        <span
          v-else
          class="inline-flex shrink-0 items-center gap-1 rounded-full bg-blue-100 px-2 py-0.5 text-[10px] font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
        >
          <Icon name="shield" size="xs" />
          {{ t('admin.plugins.permissions.declared') }}
        </span>
        <code class="break-all font-mono text-[11px] text-gray-700 dark:text-gray-200">{{ cap }}</code>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import { capabilityCategory } from '../pluginHelpers'

defineProps<{
  capabilities?: string[]
}>()

const { t } = useI18n()
</script>
