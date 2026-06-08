<template>
  <tr class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ ctx.formatDateTime(row.created_at) }}</td>
    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.group_name || '-' }}</td>
    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
      <div>{{ row.user_email || '-' }}</div>
      <div v-if="row.user_id" class="text-xs text-gray-400">UID {{ row.user_id }}</div>
    </td>
    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.api_key_name || '-' }}</td>
    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
      <div>{{ row.endpoint || '-' }}</div>
      <div class="text-xs text-gray-400">{{ row.provider || '-' }} / {{ row.model || '-' }}</div>
    </td>
    <td class="whitespace-nowrap px-5 py-4">
      <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="ctx.resultBadgeClass(row)">
        {{ ctx.resultLabel(row) }}
      </span>
    </td>
    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
      <div>{{ row.highest_category || '-' }}</div>
      <div class="text-xs text-gray-400">{{ ctx.percent(row.highest_score) }}</div>
    </td>
    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
      <div>{{ ctx.violationCountText(row) }}</div>
      <div class="text-xs text-gray-400">
        {{ row.email_sent ? t('admin.riskControl.emailSent') : t('admin.riskControl.emailNotSent') }}
        <span v-if="row.auto_banned"> / {{ t('admin.riskControl.autoBanned') }}</span>
      </div>
      <button
        v-if="ctx.canUnbanRow(row)"
        type="button"
        class="btn-soft-outline-success mt-2 inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60"
        :disabled="ctx.unbanningUserID.value === row.user_id"
        @click="ctx.unbanUser(row)"
      >
        <Icon name="checkCircle" size="xs" :class="ctx.unbanningUserID.value === row.user_id ? 'animate-spin' : ''" />
        {{ ctx.unbanningUserID.value === row.user_id ? t('common.processing') : t('admin.riskControl.unbanUser') }}
      </button>
    </td>
    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
      <div>{{ ctx.latencyText(row.upstream_latency_ms) }}</div>
      <div v-if="row.queue_delay_ms !== null && row.queue_delay_ms !== undefined" class="text-xs text-gray-400">
        {{ t('admin.riskControl.queueDelay', { ms: row.queue_delay_ms }) }}
      </div>
    </td>
    <td class="w-[320px] max-w-sm px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
      <button
        type="button"
        class="group flex w-full min-w-0 items-center gap-2 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
        :title="ctx.inputSummaryText(row)"
        @click="ctx.openInputDetail(row)"
      >
        <span class="min-w-0 flex-1 truncate">{{ ctx.inputSummaryText(row) }}</span>
        <Icon name="eye" size="xs" class="flex-shrink-0 text-gray-300 transition-colors group-hover:text-primary-500 dark:text-gray-500" />
      </button>
    </td>
  </tr>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import type { ContentModerationLog } from './types'
import { RiskControlKey } from './riskControlContext'

defineProps<{ row: ContentModerationLog }>()

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
