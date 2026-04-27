<template>
  <p class="mt-1 text-xs text-orange-600 dark:text-orange-400">
    {{ t('admin.pluginSettings.requiresReload') }}
  </p>
</template>

<script setup lang="ts">
// SETTINGS-V2 decorator — placed below the widget when
// `PropDescriptor.requiresReload === true`. Tells the admin that
// saving will trigger a plugin reload (host re-instantiates the
// settings store; in-flight requests survive the swap).
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>
