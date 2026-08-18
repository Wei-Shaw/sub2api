<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.runResultTitle')"
    width="normal"
    @close="$emit('close')"
  >
    <div class="space-y-2">
      <div
        v-for="r in results"
        :key="r.model"
        class="flex items-center justify-between rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
      >
        <div class="flex flex-col">
          <span class="font-medium text-gray-900 dark:text-white">{{ formatMonitorModel(r.model) REDACTEDREDACTED</span>
          <span v-if="r.message" class="text-xs text-gray-500 dark:text-gray-400">{{ r.message REDACTEDREDACTED</span>
          <MonitorQuotaView :snapshot="r.quota" class="mt-1" />
        </div>
        <div class="flex items-center gap-2">
          <span
            class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px]"
            :class="statusBadgeClass(r.status)"
          >
            {{ statusLabel(r.status) REDACTEDREDACTED
          </span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatLatency(r.latency_ms) REDACTEDREDACTED ms</span>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end">
        <button @click="$emit('close')" class="btn btn-primary">
          {{ t('common.close') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import type { CheckResult REDACTED from '@/api/admin/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'
import { useChannelMonitorFormat REDACTED from '@/composables/useChannelMonitorFormat'

defineProps<{
  show: boolean
  results: CheckResult[]
REDACTED>()

defineEmits<{
  (e: 'close'): void
REDACTED>()

const { t REDACTED = useI18n()
const { statusLabel, statusBadgeClass, formatLatency, formatMonitorModel REDACTED = useChannelMonitorFormat()
</script>
