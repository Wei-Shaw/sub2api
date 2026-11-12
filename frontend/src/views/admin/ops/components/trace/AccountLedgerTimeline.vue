<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.traceAccount.ledger.total', { count: events.length }) }}
        <template v-if="hideTokenRefresh && hiddenCount > 0">
          · <span data-testid="hidden-count">{{ t('admin.traceAccount.ledger.hiddenCount', { count: hiddenCount }) }}</span>
        </template>
      </div>
      <label class="flex cursor-pointer select-none items-center gap-2 text-xs text-gray-700 dark:text-gray-300">
        <input v-model="hideTokenRefresh" type="checkbox" class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-700" data-testid="toggle-token-refresh" />
        {{ t('admin.traceAccount.ledger.hideTokenRefresh') }}
      </label>
    </div>

    <!-- 空台账 -->
    <div v-if="events.length === 0" class="card px-6 py-10 text-center" data-testid="ledger-empty">
      <Icon name="document" size="lg" class="mx-auto text-gray-300 dark:text-dark-500" />
      <div class="mt-2 text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('admin.traceAccount.ledger.emptyTitle') }}</div>
      <p class="mx-auto mt-1 max-w-xl text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('admin.traceAccount.ledger.emptyDesc') }}</p>
    </div>
    <div v-else-if="visible.length === 0" class="card px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400" data-testid="ledger-all-hidden">
      {{ t('admin.traceAccount.ledger.allHidden') }}
    </div>

    <ol v-else class="relative ml-3 border-l border-gray-200 dark:border-dark-600">
      <li v-for="ev in visible" :key="ev.id" class="mb-4 ml-5" data-testid="ledger-event" :data-type="ev.event_type">
        <span :class="['absolute -left-[7px] mt-2 h-3.5 w-3.5 rounded-full ring-4 ring-white dark:ring-dark-800', eventTone(ev.event_type)]"></span>
        <div
          :class="[
            'card p-3',
            credentialChanged(ev) ? 'ring-2 ring-amber-400 dark:ring-amber-500/70' : ''
          ]"
          :data-credential-changed="credentialChanged(ev) ? 'true' : undefined"
        >
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span class="text-sm font-semibold text-gray-900 dark:text-white" data-testid="event-label">{{ eventLabel(ev.event_type) }}</span>
            <span class="text-xs tabular-nums text-gray-500 dark:text-gray-400">{{ formatDateTime(ev.created_at) }}</span>
            <span v-if="ev.status_after" class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.traceAccount.ledger.statusAfter', { status: ev.status_after }) }}</span>
            <span v-if="ev.source && ev.source !== 'db'" class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.traceAccount.ledger.source', { source: ev.source }) }}</span>
            <span v-if="ev.operator_id" class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.traceAccount.ledger.operator', { id: ev.operator_id }) }}</span>
          </div>

          <!-- 凭据指纹变化高亮 -->
          <div
            v-if="credentialChanged(ev)"
            class="mt-2 rounded-lg bg-amber-50 px-2.5 py-2 text-xs text-amber-900 dark:bg-amber-500/10 dark:text-amber-200"
            data-testid="credential-highlight"
          >
            <div class="flex flex-wrap items-center gap-1.5">
              <Icon name="key" size="xs" />
              <span class="font-semibold">{{ t('admin.traceAccount.ledger.credentialChanged') }}</span>
              <TraceFingerprintChip :fp="credentialFpPair(ev).from" strike />
              <span>→</span>
              <TraceFingerprintChip :fp="credentialFpPair(ev).to" />
            </div>
            <div v-if="credentialFpPair(ev).to" class="mt-1" data-testid="credential-failures">
              <span v-if="failureCount(credentialFpPair(ev).to) > 0" class="font-semibold text-red-600 dark:text-red-400">
                {{ t('admin.traceAccount.ledger.credentialFailures', { count: failureCount(credentialFpPair(ev).to) }) }}
              </span>
              <span v-else class="text-amber-700/80 dark:text-amber-300/80">{{ t('admin.traceAccount.ledger.credentialNoFailures') }}</span>
            </div>
          </div>

          <!-- changes：字段：旧值 → 新值 -->
          <div v-if="ev.event_type === 'created'" class="mt-1.5 text-[11px] font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
            {{ t('admin.traceAccount.ledger.snapshot') }}
          </div>
          <ul class="mt-1.5 space-y-1 text-xs">
            <li v-for="row in visibleRows(ev)" :key="row.field" class="flex flex-wrap items-baseline gap-x-1.5" data-testid="change-row" :data-field="row.field">
              <span class="text-gray-500 dark:text-gray-400">{{ fieldLabel(row.field) }}：</span>
              <template v-if="!row.isSnapshot">
                <span class="text-gray-500 line-through decoration-gray-300 dark:text-gray-400 dark:decoration-gray-600" data-testid="change-from">{{ formatValue(row.field, row.from) }}</span>
                <span class="text-gray-400">→</span>
              </template>
              <span class="font-medium text-gray-900 dark:text-gray-100" data-testid="change-to">{{ formatValue(row.field, row.to) }}</span>
              <span
                v-if="row.field === 'expires_at' && !row.isSnapshot && expiryDeltaDays(ev) !== null && expiryDeltaDays(ev) !== 0"
                :class="[
                  'rounded px-1.5 py-0.5 text-[11px] font-medium',
                  expiryDeltaDays(ev)! > 0
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300'
                    : 'bg-orange-100 text-orange-700 dark:bg-orange-500/20 dark:text-orange-300'
                ]"
                data-testid="expiry-delta"
              >
                {{
                  expiryDeltaDays(ev)! > 0
                    ? t('admin.traceAccount.ledger.extended', { days: expiryDeltaDays(ev) })
                    : t('admin.traceAccount.ledger.shortened', { days: Math.abs(expiryDeltaDays(ev)!) })
                }}
              </span>
            </li>
          </ul>
        </div>
      </li>
    </ol>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import { formatDateTime } from '@/utils/format'
import type { AccountLifecycleEvent } from '@/api/admin/opsTrace'
import TraceFingerprintChip from './TraceFingerprintChip.vue'
import {
  KNOWN_EVENT_TYPES,
  credentialChanged,
  credentialFpPair,
  eventTone,
  expiryDeltaDays,
  isDateField,
  parseChanges,
  type ChangeRow
} from './accountTraceUtils'

const props = defineProps<{
  events: AccountLifecycleEvent[]
  /** 指纹 → 概览时间窗内的失败次数 */
  failuresByFp?: Record<string, number>
}>()

const { t } = useI18n()
const hideTokenRefresh = ref(true)

const sorted = computed(() =>
  [...props.events].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime() || b.id - a.id)
)
const hiddenCount = computed(() => props.events.filter((e) => e.event_type === 'token_refreshed').length)
const visible = computed(() =>
  hideTokenRefresh.value ? sorted.value.filter((e) => e.event_type !== 'token_refreshed') : sorted.value
)

const KNOWN_FIELDS = [
  'status', 'schedulable', 'expires_at', 'auto_pause_on_expired', 'deleted_at', 'proxy_id', 'concurrency',
  'priority', 'rate_multiplier', 'name', 'credential_fp', 'platform', 'type'
]

/** 凭据指纹已在高亮块里展示，不再重复一行 */
function visibleRows(ev: AccountLifecycleEvent): ChangeRow[] {
  const rows = parseChanges(ev)
  return credentialChanged(ev) ? rows.filter((r) => r.field !== 'credential_fp') : rows
}

function eventLabel(type: string) {
  return (KNOWN_EVENT_TYPES as readonly string[]).includes(type) ? t(`admin.traceAccount.ledger.events.${type}`) : type
}

function fieldLabel(field: string) {
  if (field.startsWith('extra.')) return t('admin.traceAccount.ledger.fields.extraPrefix', { key: field.slice(6) })
  return KNOWN_FIELDS.includes(field) ? t(`admin.traceAccount.ledger.fields.${field}`) : field
}

function formatValue(field: string, v: unknown): string {
  if (v === null || v === undefined || v === '') return t('admin.traceAccount.ledger.emptyValue')
  if (typeof v === 'boolean') return v ? t('admin.traceAccount.ledger.yes') : t('admin.traceAccount.ledger.no')
  if (typeof v === 'string' && isDateField(field)) return formatDateTime(v) || v
  if (typeof v === 'object') {
    if (Object.keys(v as object).length === 0) return t('admin.traceAccount.ledger.emptyValue')
    return JSON.stringify(v)
  }
  return String(v)
}

function failureCount(fp: string | null): number {
  if (!fp) return 0
  return props.failuresByFp?.[fp] ?? 0
}
</script>
