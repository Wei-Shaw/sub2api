<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.runResultTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <div v-if="results.length === 0" class="text-sm text-gray-500">
      {{ t('common.noData') }}
    </div>
    <div v-else class="space-y-3">
      <div
        v-for="r in results"
        :key="r.model"
        class="rounded-md border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="mb-1 flex items-center justify-between">
          <span class="font-medium">{{ r.model }}</span>
          <span
            class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
            :class="statusBadgeClass(r.status)"
          >
            {{ statusLabel(r.status) }}
          </span>
        </div>
        <div class="grid grid-cols-2 gap-2 text-xs text-gray-600 dark:text-gray-400">
          <div>
            {{ t('monitorCommon.dialogLatency') }}:
            <span class="font-mono">{{ formatLatency(r.latency_ms) }}</span>
          </div>
          <div>
            {{ t('monitorCommon.endpointPing') }}:
            <span class="font-mono">{{ formatLatency(r.ping_latency_ms) }}</span>
          </div>
        </div>
        <div v-if="r.message" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ r.message }}
        </div>
      </div>
    </div>

    <template #footer>
      <button type="button" @click="$emit('close')" class="btn btn-primary">
        {{ t('common.close') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
/**
 * V5 W7 — Run-now 结果弹窗 (与 09fd83ab host 版逻辑等价, 仅替换 import).
 */
import { useI18n } from 'vue-i18n'
import { BaseDialog } from '@sub2api/plugin-sdk'
import type { CheckResult } from '../../../api/admin/channelMonitor'
import { useChannelMonitorFormat } from '../../../composables/useChannelMonitorFormat'

defineProps<{
  show: boolean
  results: CheckResult[]
}>()

defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const { statusBadgeClass, statusLabel, formatLatency } = useChannelMonitorFormat()
</script>
