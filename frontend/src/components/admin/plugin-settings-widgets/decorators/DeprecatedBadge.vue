<template>
  <div class="space-y-1">
    <div class="flex items-center gap-2">
      <span class="line-through opacity-60">
        <slot name="label" />
      </span>
      <span
        class="inline-flex items-center rounded bg-yellow-100 px-1.5 py-0.5 text-xs font-medium text-yellow-800 dark:bg-yellow-900/40 dark:text-yellow-200"
      >
        {{ t('admin.pluginSettings.deprecated') }}
      </span>
    </div>
    <p class="text-xs text-yellow-700 dark:text-yellow-300">
      {{ message }}
    </p>
    <slot />
  </div>
</template>

<script setup lang="ts">
// SETTINGS-V2 decorator — shown when `PropDescriptor.deprecated` is
// non-empty. Wraps the widget with a strikethrough label, a yellow
// "Deprecated" badge, and the deprecation message supplied by the
// plugin author.
import { useI18n } from 'vue-i18n'

defineProps<{ message: string }>()
const { t } = useI18n()
</script>
