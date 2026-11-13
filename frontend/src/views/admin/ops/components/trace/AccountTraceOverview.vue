<template>
  <div class="space-y-4">
    <!-- 指标卡 -->
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div class="card p-4">
        <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.traceAccount.overview.failures') }}</div>
        <div class="mt-1 text-2xl font-bold text-gray-900 dark:text-white" data-testid="failure-total">{{ summary.total }}</div>
        <div class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ t('admin.traceAccount.overview.failuresHint') }}</div>
      </div>

      <div class="card p-4 sm:col-span-2 lg:col-span-1">
        <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.traceAccount.overview.distribution') }}</div>
        <div class="mt-2 flex h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
          <div
            v-for="k in FAILURE_KINDS"
            v-show="summary.byKind[k] > 0"
            :key="k"
            :class="failureKindStyle(k).bar"
            :style="{ width: `${summary.total ? (summary.byKind[k] / summary.total) * 100 : 0}%` }"
          ></div>
        </div>
        <ul class="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
          <li v-for="k in FAILURE_KINDS" :key="k" class="flex items-center justify-between" :data-testid="`kind-${k}`">
            <span class="flex items-center gap-1.5 text-gray-600 dark:text-gray-300">
              <span :class="['inline-block h-2 w-2 rounded-full', failureKindStyle(k).bar]"></span>
              {{ t(`admin.traceAccount.kinds.${k}`) }}
            </span>
            <span class="font-semibold tabular-nums text-gray-900 dark:text-gray-100" data-testid="kind-count">{{ summary.byKind[k] }}</span>
          </li>
        </ul>
      </div>

      <div class="card p-4">
        <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.traceAccount.overview.lastFailure') }}</div>
        <div class="mt-1 text-base font-semibold text-gray-900 dark:text-white" data-testid="last-failure">
          {{ summary.lastFailureAt ? formatDateTime(summary.lastFailureAt) : t('admin.traceAccount.overview.noFailure') }}
        </div>
        <div v-if="summary.lastFailureAt" class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ formatRelativeTime(summary.lastFailureAt) }}</div>
      </div>

      <div class="card p-4">
        <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.traceAccount.overview.fingerprints') }}</div>
        <div class="mt-1 text-2xl font-bold text-gray-900 dark:text-white" data-testid="fp-count">{{ summary.fingerprints.length }}</div>
        <div class="mt-1 flex flex-wrap gap-1">
          <TraceFingerprintChip v-for="fp in summary.fingerprints" :key="fp" :fp="fp" :title="fpTitle(fp)" />
        </div>
        <div class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ t('admin.traceAccount.overview.fingerprintsHint') }}</div>
      </div>
    </div>

    <!-- 失败尝试列表 -->
    <div class="card overflow-hidden">
      <div class="flex items-center justify-between border-b border-gray-100 px-4 py-2.5 dark:border-dark-700">
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.traceAccount.overview.attemptsTitle') }}</h4>
        <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('admin.traceAccount.overview.attemptsLimitHint', { limit }) }}</span>
      </div>
      <div v-if="failures.length === 0" class="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400" data-testid="attempts-empty">
        {{ t('admin.traceAccount.overview.empty') }}
      </div>
      <div v-else class="max-h-[420px] overflow-auto">
        <table class="min-w-full text-left text-xs">
          <thead class="sticky top-0 bg-gray-50 text-gray-500 dark:bg-dark-800 dark:text-gray-400">
            <tr>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.traceAccount.overview.columns.time') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.traceAccount.overview.columns.requestId') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.traceAccount.overview.columns.status') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.traceAccount.overview.columns.kind') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.traceAccount.overview.columns.errorCode') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.traceAccount.overview.columns.fingerprint') }}</th>
              <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.traceAccount.overview.columns.failover') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="(a, idx) in failures" :key="`${a.created_at}-${idx}`" class="text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-700/50" data-testid="attempt-row">
              <td class="whitespace-nowrap px-3 py-2 tabular-nums">{{ formatDateTime(a.created_at) }}</td>
              <td class="px-3 py-2">
                <span v-if="a.request_id" class="inline-flex items-center gap-1">
                  <code class="max-w-[160px] truncate font-mono text-[11px]" :title="a.request_id">{{ a.request_id }}</code>
                  <button
                    type="button"
                    class="rounded p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-200"
                    :title="t('admin.traceAccount.copy')"
                    @click="copyToClipboard(a.request_id!)"
                  >
                    <Icon name="copy" size="xs" />
                  </button>
                </span>
                <span v-else class="text-gray-400 dark:text-gray-500" :title="t('admin.traceAccount.overview.requestIdMissing')">—</span>
              </td>
              <td class="whitespace-nowrap px-3 py-2 tabular-nums">{{ a.upstream_status ?? '—' }}</td>
              <td class="whitespace-nowrap px-3 py-2">
                <span :class="['rounded px-1.5 py-0.5 text-[11px] font-medium', failureKindStyle(a.failure_kind).badge]">
                  {{ kindLabel(a.failure_kind) }}
                </span>
              </td>
              <td class="px-3 py-2">
                <span class="font-mono text-[11px]" :title="a.error_msg || ''">{{ a.error_code || '—' }}</span>
              </td>
              <td class="whitespace-nowrap px-3 py-2">
                <TraceFingerprintChip :fp="a.credential_fp" :title="a.credential_fp ? fpTitle(a.credential_fp) : ''" />
              </td>
              <td class="whitespace-nowrap px-3 py-2">
                <span v-if="a.retry_from" class="rounded bg-purple-100 px-1.5 py-0.5 text-[11px] text-purple-700 dark:bg-purple-500/20 dark:text-purple-300" data-testid="failover-badge">
                  {{ t('admin.traceAccount.overview.failoverFrom', { id: a.retry_from }) }}
                </span>
                <span v-else class="text-gray-400 dark:text-gray-500">{{ t('admin.traceAccount.overview.firstHop') }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTime, formatRelativeTime } from '@/utils/format'
import type { TraceAttempt } from '@/api/admin/opsTrace'
import TraceFingerprintChip from './TraceFingerprintChip.vue'
import { FAILURE_KINDS, failureKindStyle, isFailure, summarizeAttempts } from './accountTraceUtils'

const props = defineProps<{
  attempts: TraceAttempt[]
  limit: number
  /** 指纹 → 换入时间（来自台账），用于把失败尝试与「换 key」对上 */
  fpSwitchedAt?: Record<string, string>
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const summary = computed(() => summarizeAttempts(props.attempts))
const failures = computed(() => props.attempts.filter(isFailure))

function kindLabel(kind: string) {
  return (FAILURE_KINDS as string[]).includes(kind) ? t(`admin.traceAccount.kinds.${kind}`) : kind
}

function fpTitle(fp: string) {
  const at = props.fpSwitchedAt?.[fp]
  return at ? `${fp} · ${t('admin.traceAccount.overview.fpSwitchedAt', { time: formatDateTime(at) })}` : fp
}
</script>
