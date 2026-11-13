<template>
  <div class="space-y-4" data-testid="request-trace-timeline">
    <!-- Header -->
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="min-w-0">
        <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">
          {{ t('admin.traceRequest.title') }}
        </h3>
        <div class="mt-1 break-all font-mono text-xs text-gray-500 dark:text-gray-400">{{ requestId || '—' }}</div>
      </div>
      <button
        v-if="requestId"
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-bold text-primary-700 hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-60 dark:text-primary-200 dark:hover:bg-dark-700"
        :disabled="loading"
        data-testid="trace-refresh"
        @click="load"
      >
        <Icon name="refresh" size="xs" :stroke-width="2" :class="loading ? 'animate-spin' : ''" />
        {{ t('admin.traceRequest.refresh') }}
      </button>
    </div>

    <!-- No request id -->
    <div
      v-if="!requestId"
      class="rounded-xl bg-gray-50 p-6 text-center text-sm text-gray-500 dark:bg-dark-900 dark:text-gray-400"
      data-testid="trace-no-request-id"
    >
      {{ t('admin.traceRequest.noRequestId') }}
    </div>

    <!-- Loading -->
    <div v-else-if="loading" class="flex items-center justify-center py-12" data-testid="trace-loading">
      <div class="flex flex-col items-center gap-3">
        <div class="h-7 w-7 animate-spin rounded-full border-b-2 border-primary-600"></div>
        <div class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('admin.traceRequest.loading') }}</div>
      </div>
    </div>

    <!-- Not found -->
    <div
      v-else-if="notFound"
      class="rounded-xl bg-gray-50 p-6 text-center dark:bg-dark-900"
      data-testid="trace-empty"
    >
      <Icon name="inbox" size="lg" class="mx-auto text-gray-300 dark:text-gray-600" />
      <div class="mt-2 text-sm font-bold text-gray-700 dark:text-gray-200">{{ t('admin.traceRequest.empty') }}</div>
      <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.traceRequest.emptyHint') }}</div>
    </div>

    <!-- Error -->
    <div
      v-else-if="loadError"
      class="rounded-xl border border-red-200 bg-red-50 p-6 text-center dark:border-red-900/50 dark:bg-red-900/10"
      data-testid="trace-error"
    >
      <Icon name="exclamationCircle" size="lg" class="mx-auto text-red-400" />
      <div class="mt-2 text-sm font-bold text-red-800 dark:text-red-200">{{ t('admin.traceRequest.loadFailed') }}</div>
      <div class="mt-1 break-words text-xs text-red-700 dark:text-red-300">{{ loadError }}</div>
      <button type="button" class="btn btn-secondary btn-sm mt-3" data-testid="trace-retry" @click="load">
        {{ t('admin.traceRequest.retry') }}
      </button>
    </div>

    <!-- Data -->
    <template v-else-if="trace">
      <!-- Summary chips -->
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <span class="rounded-md bg-gray-100 px-2 py-1 font-bold text-gray-700 dark:bg-dark-700 dark:text-gray-200">
          {{ t('admin.traceRequest.summary.attempts', { n: attempts.length }) }}
        </span>
        <span
          v-if="switchCount > 0"
          class="rounded-md bg-indigo-100 px-2 py-1 font-bold text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300"
          data-testid="trace-switch-count"
        >
          {{ t('admin.traceRequest.summary.switches', { n: switchCount }) }}
        </span>
        <span
          v-if="trace.usage"
          class="rounded-md bg-emerald-100 px-2 py-1 font-bold text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
        >
          {{ t('admin.traceRequest.summary.succeeded') }}
        </span>
        <span
          v-else-if="trace.error || attempts.length"
          class="rounded-md bg-red-100 px-2 py-1 font-bold text-red-700 dark:bg-red-900/40 dark:text-red-300"
        >
          {{ t('admin.traceRequest.summary.failed') }}
        </span>
      </div>

      <div
        v-if="!attempts.length"
        class="rounded-xl bg-gray-50 p-4 text-sm text-gray-500 dark:bg-dark-900 dark:text-gray-400"
        data-testid="trace-no-attempts"
      >
        {{ trace.usage ? t('admin.traceRequest.noAttemptsSuccess') : t('admin.traceRequest.noAttempts') }}
      </div>

      <!-- Timeline -->
      <ol v-else class="relative" data-testid="trace-attempts">
        <li v-for="(item, idx) in attemptItems" :key="`${item.attempt.attempt_no}-${idx}`" class="relative">
          <!-- Failover switch marker -->
          <div
            v-if="item.switchedFrom != null"
            class="relative flex items-center gap-3 pb-3 pl-10"
            data-testid="trace-switch-marker"
          >
            <span class="absolute left-[15px] top-0 h-full w-0.5 bg-indigo-300 dark:bg-indigo-700" aria-hidden="true"></span>
            <div
              class="inline-flex items-center gap-2 rounded-lg border border-indigo-200 bg-indigo-50 px-3 py-1.5 text-xs font-bold text-indigo-800 shadow-sm dark:border-indigo-800 dark:bg-indigo-900/30 dark:text-indigo-200"
            >
              <Icon name="swap" size="sm" :stroke-width="2" />
              <span>{{ t('admin.traceRequest.switchFromTo', { from: item.switchedFrom, to: item.attempt.account_id ?? '?' }) }}</span>
            </div>
          </div>

          <div class="relative pb-5 pl-10" data-testid="trace-attempt">
            <!-- rail -->
            <span
              v-if="idx < attemptItems.length - 1 || hasFinal"
              class="absolute left-[15px] top-8 h-[calc(100%-1.5rem)] w-0.5 bg-gray-200 dark:bg-dark-600"
              aria-hidden="true"
            ></span>
            <!-- dot -->
            <span
              :class="[
                'absolute left-0 top-0 flex h-8 w-8 items-center justify-center rounded-full text-xs font-black ring-4 ring-white dark:ring-dark-800',
                kindStyle(item.attempt.failure_kind).dot
              ]"
              :data-kind="normalizeKind(item.attempt.failure_kind)"
            >
              {{ item.attempt.attempt_no }}
            </span>

            <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-xs font-black text-gray-900 dark:text-white">
                  {{ t('admin.traceRequest.attemptNo', { n: item.attempt.attempt_no }) }}
                </span>
                <button
                  v-if="item.attempt.account_id != null"
                  type="button"
                  class="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-0.5 font-mono text-xs font-bold text-gray-800 hover:bg-primary-50 hover:text-primary-700 dark:bg-dark-700 dark:text-gray-100 dark:hover:bg-dark-600 dark:hover:text-primary-300"
                  :title="t('admin.traceRequest.copyAccountId')"
                  data-testid="trace-account-id"
                  @click="copyAccountId(item.attempt.account_id)"
                >
                  {{ t('admin.traceRequest.account', { id: item.attempt.account_id }) }}
                  <Icon name="copy" size="xs" :stroke-width="2" />
                </button>
                <span v-else class="font-mono text-xs text-gray-400">{{ t('admin.traceRequest.accountUnknown') }}</span>
                <span
                  v-if="item.attempt.platform"
                  class="rounded-md bg-gray-100 px-2 py-0.5 text-[11px] font-bold text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                >
                  {{ item.attempt.platform }}
                </span>
                <span
                  :class="['rounded-md px-2 py-0.5 text-[11px] font-black ring-1 ring-inset', kindStyle(item.attempt.failure_kind).badge]"
                  data-testid="trace-failure-kind"
                >
                  {{ kindLabel(item.attempt.failure_kind) }}
                </span>
                <span
                  v-if="item.attempt.upstream_status != null"
                  class="rounded-md bg-gray-900 px-2 py-0.5 font-mono text-[11px] font-black text-white dark:bg-gray-100 dark:text-gray-900"
                  data-testid="trace-upstream-status"
                >
                  HTTP {{ item.attempt.upstream_status }}
                </span>
                <span class="ml-auto text-[11px] text-gray-400">{{ formatDateTime(item.attempt.created_at) }}</span>
              </div>

              <div class="mt-3 grid grid-cols-1 gap-x-4 gap-y-1.5 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2 lg:grid-cols-3">
                <div v-if="item.attempt.error_code">
                  <span class="text-gray-400">{{ t('admin.traceRequest.fields.errorCode') }}:</span>
                  <span class="ml-1 font-mono font-bold text-gray-800 dark:text-gray-100">{{ item.attempt.error_code }}</span>
                </div>
                <div v-if="item.elapsedMs != null">
                  <span class="text-gray-400">{{ t('admin.traceRequest.fields.elapsed') }}:</span>
                  <span class="ml-1 font-mono">{{ formatMs(item.elapsedMs) }}</span>
                </div>
                <div v-if="modelText(item.attempt)">
                  <span class="text-gray-400">{{ t('admin.traceRequest.fields.model') }}:</span>
                  <span class="ml-1 font-mono">{{ modelText(item.attempt) }}</span>
                </div>
                <div v-if="item.attempt.credential_fp">
                  <span class="text-gray-400">{{ t('admin.traceRequest.fields.credentialFp') }}:</span>
                  <span class="ml-1 font-mono" :title="t('admin.traceRequest.credentialFpHint')">{{ item.attempt.credential_fp }}</span>
                </div>
                <div v-if="item.attempt.proxy_id != null">
                  <span class="text-gray-400">{{ t('admin.traceRequest.fields.proxy') }}:</span>
                  <span class="ml-1 font-mono">#{{ item.attempt.proxy_id }}</span>
                </div>
                <div v-if="item.attempt.account_state">
                  <span class="text-gray-400">{{ t('admin.traceRequest.fields.accountState') }}:</span>
                  <span class="ml-1 font-mono">{{ item.attempt.account_state }}</span>
                  <span v-if="item.attempt.cooling_until" class="ml-1 text-gray-400">
                    → {{ formatDateTime(item.attempt.cooling_until) }}
                  </span>
                </div>
                <div v-if="item.attempt.upstream_url" class="sm:col-span-2 lg:col-span-3">
                  <span class="text-gray-400">{{ t('admin.traceRequest.fields.upstreamUrl') }}:</span>
                  <span class="ml-1 break-all font-mono">{{ item.attempt.upstream_url }}</span>
                </div>
              </div>

              <div v-if="item.attempt.error_msg" class="mt-3">
                <div
                  :class="[
                    'whitespace-pre-wrap break-words rounded-lg bg-gray-50 p-2 font-mono text-xs text-gray-800 dark:bg-dark-900 dark:text-gray-100',
                    isExpanded(idx) ? '' : 'line-clamp-2'
                  ]"
                  data-testid="trace-error-msg"
                >{{ item.attempt.error_msg }}</div>
                <button
                  v-if="item.attempt.error_msg.length > ERROR_MSG_FOLD_AT"
                  type="button"
                  class="mt-1 inline-flex items-center gap-1 text-[11px] font-bold text-primary-700 hover:underline dark:text-primary-300"
                  data-testid="trace-error-msg-toggle"
                  @click="toggleExpanded(idx)"
                >
                  <Icon :name="isExpanded(idx) ? 'chevronDown' : 'chevronRight'" size="xs" :stroke-width="2" />
                  {{ isExpanded(idx) ? t('admin.traceRequest.collapse') : t('admin.traceRequest.expand') }}
                </button>
              </div>

              <!-- Status events triggered on this account -->
              <div v-if="item.events.length" class="mt-3 space-y-1.5" data-testid="trace-status-events">
                <div
                  v-for="(ev, evIdx) in item.events"
                  :key="evIdx"
                  class="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-lg border border-dashed border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-700/60 dark:bg-amber-900/10 dark:text-amber-200"
                >
                  <Icon name="exclamationTriangle" size="xs" :stroke-width="2" />
                  <span class="font-bold">{{ statusEventText(ev) }}</span>
                  <span v-if="ev.error_code" class="font-mono">{{ ev.error_code }}</span>
                  <span v-if="ev.reason" class="break-all opacity-80">{{ ev.reason }}</span>
                  <span class="ml-auto opacity-70">{{ sourceLabel(ev.source) }} · {{ formatDateTime(ev.created_at) }}</span>
                </div>
              </div>
            </div>
          </div>
        </li>
      </ol>

      <!-- Orphan status events -->
      <div v-if="orphanEvents.length" class="rounded-xl bg-amber-50 p-4 dark:bg-amber-900/10" data-testid="trace-orphan-events">
        <div class="text-xs font-black uppercase tracking-wider text-amber-900 dark:text-amber-200">
          {{ t('admin.traceRequest.otherStatusEvents') }}
        </div>
        <ul class="mt-2 space-y-1 text-xs text-amber-900 dark:text-amber-200">
          <li v-for="(ev, i) in orphanEvents" :key="i">
            <span class="font-mono font-bold">#{{ ev.account_id }}</span>
            <span class="ml-2">{{ statusEventText(ev) }}</span>
            <span v-if="ev.reason" class="ml-2 opacity-80">{{ ev.reason }}</span>
            <span class="ml-2 opacity-70">{{ formatDateTime(ev.created_at) }}</span>
          </li>
        </ul>
      </div>

      <!-- Final result -->
      <div
        v-if="trace.usage"
        class="rounded-xl border border-emerald-200 bg-emerald-50 p-4 dark:border-emerald-800/60 dark:bg-emerald-900/10"
        data-testid="trace-final-success"
      >
        <div class="flex items-center gap-2 text-sm font-black text-emerald-800 dark:text-emerald-200">
          <Icon name="checkCircle" size="sm" :stroke-width="2" />
          {{ t('admin.traceRequest.final.success') }}
        </div>
        <div class="mt-3 grid grid-cols-2 gap-3 text-xs text-emerald-900 dark:text-emerald-100 sm:grid-cols-3 lg:grid-cols-6">
          <div>
            <div class="text-emerald-600 dark:text-emerald-400">{{ t('admin.traceRequest.final.account') }}</div>
            <div class="font-mono font-bold">#{{ trace.usage.account_id }}</div>
          </div>
          <div class="col-span-2 sm:col-span-1">
            <div class="text-emerald-600 dark:text-emerald-400">{{ t('admin.traceRequest.final.model') }}</div>
            <div class="break-all font-mono font-bold">{{ trace.usage.upstream_model || trace.usage.model }}</div>
          </div>
          <div>
            <div class="text-emerald-600 dark:text-emerald-400">{{ t('admin.traceRequest.final.tokens') }}</div>
            <div class="font-mono font-bold">{{ trace.usage.input_tokens }} / {{ trace.usage.output_tokens }}</div>
          </div>
          <div>
            <div class="text-emerald-600 dark:text-emerald-400">{{ t('admin.traceRequest.final.duration') }}</div>
            <div class="font-mono font-bold">{{ formatMs(trace.usage.duration_ms) }}</div>
          </div>
          <div v-if="trace.usage.first_token_ms">
            <div class="text-emerald-600 dark:text-emerald-400">{{ t('admin.traceRequest.final.firstToken') }}</div>
            <div class="font-mono font-bold">{{ formatMs(trace.usage.first_token_ms) }}</div>
          </div>
          <div>
            <div class="text-emerald-600 dark:text-emerald-400">{{ t('admin.traceRequest.final.cost') }}</div>
            <div class="font-mono font-bold">{{ formatCost(trace.usage.total_cost) }}</div>
          </div>
        </div>
      </div>
      <div
        v-else-if="trace.error"
        class="rounded-xl border border-red-200 bg-red-50 p-4 dark:border-red-900/50 dark:bg-red-900/10"
        data-testid="trace-final-failure"
      >
        <div class="flex flex-wrap items-center gap-2 text-sm font-black text-red-800 dark:text-red-200">
          <Icon name="xCircle" size="sm" :stroke-width="2" />
          {{ t('admin.traceRequest.final.failure') }}
          <span v-if="trace.error.status_code != null" class="rounded-md bg-red-600 px-2 py-0.5 font-mono text-[11px] text-white">
            {{ trace.error.status_code }}
          </span>
          <span
            v-if="trace.error.upstream_status_code != null"
            class="rounded-md bg-red-100 px-2 py-0.5 font-mono text-[11px] text-red-700 dark:bg-red-900/40 dark:text-red-300"
          >
            {{ t('admin.traceRequest.final.upstreamStatus', { code: trace.error.upstream_status_code }) }}
          </span>
        </div>
        <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-red-800 dark:text-red-200">
          <span><span class="opacity-70">{{ t('admin.traceRequest.final.phase') }}:</span> <span class="font-mono">{{ trace.error.error_phase || '—' }}</span></span>
          <span><span class="opacity-70">{{ t('admin.traceRequest.final.type') }}:</span> <span class="font-mono">{{ trace.error.error_type || '—' }}</span></span>
        </div>
        <div v-if="trace.error.error_message" class="mt-2 break-words text-sm font-medium text-red-900 dark:text-red-100">
          {{ trace.error.error_message }}
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { getRequestTrace, type RequestTrace, type TraceAttempt, type TraceStatusEvent } from '@/api/admin/opsTrace'
import { formatDateTime } from '@/utils/format'

interface Props {
  requestId: string
}

const props = defineProps<Props>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const ERROR_MSG_FOLD_AT = 120

const loading = ref(false)
const notFound = ref(false)
const loadError = ref('')
const trace = ref<RequestTrace | null>(null)
const expanded = ref(new Set<number>())

type KindKey = 'none' | 'auth' | 'rate_limit' | 'overload' | 'transient' | 'client' | 'unknown'

const KIND_STYLES: Record<KindKey, { dot: string; badge: string }> = {
  none: {
    dot: 'bg-emerald-500 text-white',
    badge: 'bg-emerald-50 text-emerald-700 ring-emerald-600/20 dark:bg-emerald-900/30 dark:text-emerald-300 dark:ring-emerald-500/30'
  },
  auth: {
    dot: 'bg-red-500 text-white',
    badge: 'bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-300 dark:ring-red-500/30'
  },
  rate_limit: {
    dot: 'bg-orange-500 text-white',
    badge: 'bg-orange-50 text-orange-700 ring-orange-600/20 dark:bg-orange-900/30 dark:text-orange-300 dark:ring-orange-500/30'
  },
  overload: {
    dot: 'bg-yellow-400 text-yellow-950',
    badge: 'bg-yellow-50 text-yellow-800 ring-yellow-600/30 dark:bg-yellow-900/30 dark:text-yellow-300 dark:ring-yellow-500/30'
  },
  transient: {
    dot: 'bg-yellow-400 text-yellow-950',
    badge: 'bg-yellow-50 text-yellow-800 ring-yellow-600/30 dark:bg-yellow-900/30 dark:text-yellow-300 dark:ring-yellow-500/30'
  },
  client: {
    dot: 'bg-gray-400 text-white dark:bg-gray-500',
    badge: 'bg-gray-100 text-gray-700 ring-gray-500/20 dark:bg-gray-800 dark:text-gray-300 dark:ring-gray-500/30'
  },
  unknown: {
    dot: 'bg-gray-300 text-gray-800 dark:bg-gray-600 dark:text-gray-100',
    badge: 'bg-gray-100 text-gray-600 ring-gray-500/20 dark:bg-gray-800 dark:text-gray-300 dark:ring-gray-500/30'
  }
}

function normalizeKind(kind: string | undefined | null): KindKey {
  const k = String(kind || '').trim().toLowerCase()
  return (k in KIND_STYLES ? k : 'unknown') as KindKey
}

function kindStyle(kind: string | undefined | null) {
  return KIND_STYLES[normalizeKind(kind)]
}

function kindLabel(kind: string | undefined | null): string {
  const k = normalizeKind(kind)
  if (k === 'unknown') return String(kind || '—')
  return t(`admin.traceRequest.kind.${k}`)
}

const attempts = computed<TraceAttempt[]>(() => {
  const list = trace.value?.attempts || []
  return [...list].sort((a, b) => a.attempt_no - b.attempt_no)
})

/**
 * 把状态事件挂到「同账号的最后一跳」下面；同一账号多跳时取时间上最接近且不晚于事件的那一跳。
 */
const eventAssignment = computed(() => {
  const byIndex = new Map<number, TraceStatusEvent[]>()
  const orphans: TraceStatusEvent[] = []
  const events = trace.value?.status_events || []
  for (const ev of events) {
    const evTime = Date.parse(ev.created_at)
    let target = -1
    attempts.value.forEach((a, idx) => {
      if (a.account_id !== ev.account_id) return
      if (target === -1) {
        target = idx
        return
      }
      const aTime = Date.parse(a.created_at)
      if (!Number.isNaN(evTime) && !Number.isNaN(aTime) && aTime <= evTime + 1000) target = idx
    })
    if (target === -1) {
      orphans.push(ev)
      continue
    }
    const bucket = byIndex.get(target) || []
    bucket.push(ev)
    byIndex.set(target, bucket)
  }
  return { byIndex, orphans }
})

const orphanEvents = computed(() => eventAssignment.value.orphans)

const attemptItems = computed(() =>
  attempts.value.map((attempt, idx) => {
    const prev = idx > 0 ? attempts.value[idx - 1] : null
    let elapsedMs: number | null = null
    if (prev) {
      const d = Date.parse(attempt.created_at) - Date.parse(prev.created_at)
      if (Number.isFinite(d) && d >= 0) elapsedMs = d
    }
    const switchedFrom =
      attempt.retry_from != null && attempt.retry_from !== attempt.account_id ? attempt.retry_from : null
    return {
      attempt,
      elapsedMs,
      switchedFrom,
      events: eventAssignment.value.byIndex.get(idx) || []
    }
  })
)

const switchCount = computed(() => attemptItems.value.filter((i) => i.switchedFrom != null).length)

const hasFinal = computed(() => !!(trace.value?.usage || trace.value?.error))

function modelText(a: TraceAttempt): string {
  const req = String(a.model_req || '').trim()
  const up = String(a.model_upstream || '').trim()
  if (req && up && req !== up) return `${req} → ${up}`
  return up || req
}

function statusEventText(ev: TraceStatusEvent): string {
  const to = String(ev.to_state || '').toLowerCase()
  if (to === 'cooling' && ev.cooling_until) {
    return t('admin.traceRequest.statusEvent.coolingUntil', { id: ev.account_id, until: formatDateTime(ev.cooling_until) })
  }
  if (to === 'invalid') return t('admin.traceRequest.statusEvent.invalid', { id: ev.account_id })
  return t('admin.traceRequest.statusEvent.changed', {
    id: ev.account_id,
    from: ev.from_state || '—',
    to: ev.to_state || '—'
  })
}

function sourceLabel(source: string): string {
  const s = String(source || '').toLowerCase()
  const known = ['classify', 'refresh', 'manual', 'expiry', 'scheduled_test']
  return known.includes(s) ? t(`admin.traceRequest.source.${s}`) : source || '—'
}

function formatMs(ms: number | null | undefined): string {
  if (ms == null || !Number.isFinite(ms)) return '—'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function formatCost(cost: number | null | undefined): string {
  if (cost == null || !Number.isFinite(cost)) return '—'
  return `$${cost < 0.01 && cost > 0 ? cost.toFixed(6) : cost.toFixed(4)}`
}

function isExpanded(idx: number) {
  return expanded.value.has(idx)
}

function toggleExpanded(idx: number) {
  const next = new Set(expanded.value)
  if (next.has(idx)) next.delete(idx)
  else next.add(idx)
  expanded.value = next
}

function copyAccountId(id: number) {
  void copyToClipboard(String(id), t('admin.traceRequest.copiedAccountId', { id }))
}

function isNotFoundError(err: any): boolean {
  return err?.status === 404 || err?.code === 'TRACE_NOT_FOUND' || err?.reason === 'TRACE_NOT_FOUND'
}

let seq = 0

async function load() {
  const id = String(props.requestId || '').trim()
  const current = ++seq
  trace.value = null
  notFound.value = false
  loadError.value = ''
  expanded.value = new Set()
  if (!id) {
    loading.value = false
    return
  }
  loading.value = true
  try {
    const res = await getRequestTrace(id)
    if (current !== seq) return
    trace.value = res
  } catch (err: any) {
    if (current !== seq) return
    if (isNotFoundError(err)) {
      notFound.value = true
    } else {
      loadError.value = String(err?.message || t('admin.traceRequest.loadFailed'))
    }
  } finally {
    if (current === seq) loading.value = false
  }
}

watch(() => props.requestId, load, { immediate: true })

defineExpose({ reload: load })
</script>
