<template>
  <span v-if="proxy?.id"
    class="inline-flex items-center rounded-md bg-primary-50 px-2 py-1 text-xs font-semibold text-primary-700 ring-1 ring-inset ring-primary-600/20 dark:bg-primary-500/10 dark:text-primary-300 dark:ring-primary-400/20"
    data-testid="usage-proxy">
    {{ proxy.name || `#${proxy.id}` }}
  </span>
  <span v-else class="text-xs text-gray-500 dark:text-gray-400">
    {{ t(proxy ? 'admin.usage.proxyDirect' : 'admin.usage.proxyNotRecorded') }}
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { AdminUsageLog } from '@/types'

defineProps<{ proxy: AdminUsageLog['proxy_snapshot'] }>()
const { t } = useI18n()
</script>
