<template>
  <div>
    <div v-if="sorted.length === 0" class="card px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400" data-testid="status-empty">
      {{ t('admin.traceAccount.status.empty') }}
    </div>
    <ol v-else class="relative ml-3 border-l border-gray-200 dark:border-dark-600">
      <li v-for="(ev, idx) in sorted" :key="`${ev.created_at}-${idx}`" class="mb-5 ml-5" data-testid="status-event">
        <span :class="['absolute -left-[7px] mt-1.5 h-3.5 w-3.5 rounded-full ring-4 ring-white dark:ring-dark-800', dotClass(ev.to_state)]"></span>
        <div class="card p-3">
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ stateTitle(ev.to_state) }}</span>
            <span class="text-xs tabular-nums text-gray-500 dark:text-gray-400">{{ formatDateTime(ev.created_at) }}</span>
            <span class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-700 dark:bg-dark-600 dark:text-gray-300" data-testid="status-source">
              {{ sourceText(ev.source, 'label') }}
            </span>
          </div>

          <div v-if="ev.to_state === 'cooling' && ev.cooling_until" class="mt-1.5 text-xs text-orange-700 dark:text-orange-300">
            {{ t('admin.traceAccount.status.coolingUntil', { time: formatDateTime(ev.cooling_until) }) }}
            <template v-if="durationMinutes(ev.created_at, ev.cooling_until) !== null">
              （{{ t('admin.traceAccount.status.coolingDuration', { minutes: durationMinutes(ev.created_at, ev.cooling_until) }) }}）
            </template>
          </div>

          <div class="mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-600 dark:text-gray-300">
            <span v-if="ev.from_state">{{ t('admin.traceAccount.status.fromState', { state: ev.from_state }) }}</span>
            <span v-if="ev.reason">{{ t('admin.traceAccount.status.reason', { reason: ev.reason }) }}</span>
            <span v-if="ev.error_code" class="font-mono">{{ t('admin.traceAccount.status.errorCode', { code: ev.error_code }) }}</span>
          </div>

          <div v-if="sourceText(ev.source, 'desc')" class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ sourceText(ev.source, 'desc') }}</div>
          <div class="mt-1 text-xs text-gray-700 dark:text-gray-200">
            <span class="font-medium">{{ t('admin.traceAccount.status.owner') }}</span>{{ sourceText(ev.source, 'owner') }}
          </div>

          <div v-if="ev.request_id" class="mt-1.5 flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t('admin.traceAccount.status.triggeredBy') }}:</span>
            <code class="font-mono text-[11px] text-gray-700 dark:text-gray-200">{{ ev.request_id }}</code>
            <button
              type="button"
              class="rounded p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-200"
              :title="t('admin.traceAccount.copy')"
              @click="copyToClipboard(ev.request_id!)"
            >
              <Icon name="copy" size="xs" />
            </button>
          </div>
        </div>
      </li>
    </ol>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTime } from '@/utils/format'
import type { TraceStatusEvent } from '@/api/admin/opsTrace'
import { durationMinutes } from './accountTraceUtils'

const props = defineProps<{ events: TraceStatusEvent[] }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const KNOWN_SOURCES = ['classify', 'refresh', 'manual', 'expiry', 'scheduled_test']

const sorted = computed(() =>
  [...props.events].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
)

function stateTitle(state: string) {
  if (state === 'cooling') return t('admin.traceAccount.status.enterCooling')
  if (state === 'invalid') return t('admin.traceAccount.status.enterInvalid')
  return t('admin.traceAccount.status.enterState', { state })
}

function dotClass(state: string) {
  if (state === 'invalid') return 'bg-red-500'
  if (state === 'cooling') return 'bg-orange-500'
  if (state === 'active') return 'bg-emerald-500'
  return 'bg-gray-400'
}

function sourceText(source: string, part: 'label' | 'desc' | 'owner') {
  const key = KNOWN_SOURCES.includes(source) ? source : 'unknown'
  if (key === 'unknown' && part === 'desc') return ''
  return t(`admin.traceAccount.status.sources.${key}.${part}`, { source })
}
</script>
