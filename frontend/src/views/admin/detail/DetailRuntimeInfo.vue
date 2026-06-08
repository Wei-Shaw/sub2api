<template>
  <section>
    <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
      {{ t('admin.plugins.metadataTitle') }}
    </h4>
    <dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
      <div class="flex flex-col gap-0.5">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.state') }}</dt>
        <dd class="flex items-center gap-1.5 text-gray-900 dark:text-gray-100">
          <span :class="['inline-block h-2 w-2 rounded-full', dotClass(plugin.state, plugin.enabled)]" />
          {{ stateLabel(plugin, t) }}
        </dd>
      </div>
      <div class="flex flex-col gap-0.5">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.uptime') }}</dt>
        <dd class="text-gray-900 dark:text-gray-100">
          <span v-if="plugin.started_at && !isZeroTime(plugin.started_at)">
            {{ formatUptime(plugin.started_at, nowMs) }}
            <span class="ml-1 text-xs text-gray-400">({{ formatTime(plugin.started_at) }})</span>
          </span>
          <span v-else class="text-gray-400">—</span>
        </dd>
      </div>
      <div class="flex flex-col gap-0.5">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.restartCount') }}</dt>
        <dd class="text-gray-900 dark:text-gray-100">{{ plugin.restart_count ?? 0 }}</dd>
      </div>
      <div class="flex flex-col gap-0.5">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.grpcAddr') }}</dt>
        <dd class="break-all font-mono text-xs text-gray-900 dark:text-gray-100">
          <span v-if="plugin.grpc_addr">{{ plugin.grpc_addr }}</span>
          <span v-else class="text-gray-400">—</span>
        </dd>
      </div>
      <div class="flex flex-col gap-0.5 sm:col-span-2">
        <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.plugins.httpAddr') }}</dt>
        <dd class="break-all font-mono text-xs text-gray-900 dark:text-gray-100">
          <span v-if="plugin.http_addr">{{ plugin.http_addr }}</span>
          <span v-else class="text-gray-400">—</span>
        </dd>
      </div>
    </dl>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import {
  type PluginInfo,
  dotClass,
  stateLabel,
  formatUptime,
  formatTime,
  isZeroTime,
} from '../pluginHelpers'

defineProps<{
  plugin: PluginInfo
  nowMs: number
}>()

const { t } = useI18n()
</script>
