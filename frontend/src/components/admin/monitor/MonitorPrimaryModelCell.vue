<template>
  <div class="flex flex-col gap-0.5">
    <div class="flex items-center gap-2">
      <!-- 纯配额模式主模型是占位符 "quota"（数据源是账号不是模型），展示层替换为本地化标签 -->
      <span class="text-sm text-gray-900 dark:text-gray-100">{{ formatMonitorModel(row.primary_model) REDACTEDREDACTED</span>
      <HelpTooltip>
      <template #trigger>
        <span
          class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium"
          :class="statusBadgeClass(row.primary_status)"
        >
          {{ statusLabel(row.primary_status) REDACTEDREDACTED
        </span>
      </template>
      <div class="space-y-2">
        <div class="text-xs font-semibold text-gray-100">
          {{ formatMonitorModel(row.primary_model) REDACTEDREDACTED
          <span
            class="ml-1 inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium"
            :class="statusBadgeClass(row.primary_status)"
          >
            {{ statusLabel(row.primary_status) REDACTEDREDACTED
          </span>
        </div>
        <div v-if="(row.extra_models?.length ?? 0) === 0" class="text-[11px] text-gray-300">
          {{ t('monitorCommon.extraModelsEmpty') REDACTEDREDACTED
        </div>
        <div v-else class="space-y-1">
          <div class="text-[11px] font-semibold uppercase tracking-wide text-gray-400">
            {{ t('monitorCommon.extraModelsHeader') REDACTEDREDACTED
          </div>
          <table class="w-full text-left text-[11px]">
            <thead>
              <tr class="text-gray-400">
                <th class="py-0.5 pr-2 font-medium">{{ t('admin.channelMonitor.columns.primaryModel') REDACTEDREDACTED</th>
                <th class="py-0.5 pr-2 font-medium">{{ t('admin.channelMonitor.columns.actions') REDACTEDREDACTED</th>
                <th class="py-0.5 font-medium">{{ t('admin.channelMonitor.columns.latency') REDACTEDREDACTED</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="m in (row.extra_models_status || [])" :key="m.model">
                <td class="py-0.5 pr-2 text-gray-100">{{ m.model REDACTEDREDACTED</td>
                <td class="py-0.5 pr-2">
                  <span
                    class="inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px]"
                    :class="statusBadgeClass(m.status)"
                  >
                    {{ statusLabel(m.status) REDACTEDREDACTED
                  </span>
                </td>
                <td class="py-0.5 text-gray-100">{{ formatLatency(m.latency_ms) REDACTEDREDACTED</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      </HelpTooltip>
    </div>
    <!-- 配额模式监控：主模型行内联展示最新用量/余额快照（管理端不受用户端开关限制） -->
    <MonitorQuotaView :snapshot="row.latest_quota" />
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import type { ChannelMonitor REDACTED from '@/api/admin/channelMonitor'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'
import { useChannelMonitorFormat REDACTED from '@/composables/useChannelMonitorFormat'

defineProps<{
  row: ChannelMonitor
REDACTED>()

const { t REDACTED = useI18n()
const { statusLabel, statusBadgeClass, formatLatency, formatMonitorModel REDACTED = useChannelMonitorFormat()
</script>
