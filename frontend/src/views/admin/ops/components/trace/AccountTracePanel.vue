<template>
  <BaseDialog
    :show="show"
    :title="account ? t('admin.traceAccount.title', { name: account.name }) : ''"
    width="full"
    @close="emit('close')"
  >
    <!-- 固定最小高度：切标签 / 时间窗时弹窗不随内容高度上下跳动 -->
    <div v-if="account" class="min-h-[60vh] space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.traceAccount.subtitle', { id: account.id, platform: account.platform || '-' }) }}
        </div>
        <!-- 台账是完整历史，不受时间窗影响；用 invisible 占位，避免标签栏跳动 -->
        <div :class="['flex items-center gap-2', tab === 'ledger' ? 'invisible' : '']" :aria-hidden="tab === 'ledger'">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.traceAccount.range.label') }}</span>
          <div class="inline-flex overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
            <button
              v-for="opt in RANGE_OPTIONS"
              :key="opt.hours"
              type="button"
              :class="[
                'px-3 py-1 text-xs font-medium transition-colors',
                hours === opt.hours
                  ? 'bg-primary-600 text-white'
                  : 'bg-white text-gray-600 hover:bg-gray-50 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'
              ]"
              :data-testid="`range-${opt.hours}`"
              @click="hours = opt.hours"
            >
              {{ t(opt.label) }}
            </button>
          </div>
        </div>
      </div>

      <!-- Tabs -->
      <div class="flex gap-1 border-b border-gray-200 dark:border-dark-600">
        <button
          v-for="item in TABS"
          :key="item"
          type="button"
          :class="[
            '-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors',
            tab === item
              ? 'border-primary-600 text-primary-600 dark:border-primary-400 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'
          ]"
          :data-testid="`tab-${item}`"
          @click="tab = item"
        >
          {{ t(`admin.traceAccount.tabs.${item}`) }}
          <span
            v-if="item === 'status' && trace && trace.status_events.length"
            class="ml-1 rounded-full bg-orange-100 px-1.5 text-[11px] text-orange-700 dark:bg-orange-500/20 dark:text-orange-300"
          >{{ trace.status_events.length }}</span>
        </button>
      </div>

      <!-- 概览 / 调度状态 共用 trace 请求 -->
      <template v-if="tab === 'overview' || tab === 'status'">
        <div v-if="traceLoading && !trace" class="flex items-center justify-center py-16"><LoadingSpinner /></div>
        <div v-else-if="traceError" class="card flex flex-col items-center gap-3 px-4 py-10 text-sm text-red-600 dark:text-red-400" data-testid="trace-error">
          {{ t('admin.traceAccount.loadFailed', { message: traceError }) }}
          <button type="button" class="btn btn-secondary btn-sm" @click="loadTrace">{{ t('admin.traceAccount.retry') }}</button>
        </div>
        <div v-else-if="trace" :class="['transition-opacity', traceLoading ? 'opacity-60' : '']">
          <AccountTraceOverview v-if="tab === 'overview'" :attempts="trace.attempts" :limit="TRACE_LIMIT" :fp-switched-at="fpSwitchedAt" />
          <AccountStatusTimeline v-else :events="trace.status_events" />
        </div>
      </template>

      <!-- 订阅台账 -->
      <template v-else>
        <div v-if="ledgerLoading && !ledger" class="flex items-center justify-center py-16"><LoadingSpinner /></div>
        <div v-else-if="ledgerError" class="card flex flex-col items-center gap-3 px-4 py-10 text-sm text-red-600 dark:text-red-400" data-testid="ledger-error">
          {{ t('admin.traceAccount.loadFailed', { message: ledgerError }) }}
          <button type="button" class="btn btn-secondary btn-sm" @click="loadLedger">{{ t('admin.traceAccount.retry') }}</button>
        </div>
        <AccountLedgerTimeline v-else-if="ledger" :events="ledger.events" :failures-by-fp="failuresByFp" />
      </template>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { getAccountLedger, getAccountTrace, type AccountLedger, type AccountTrace } from '@/api/admin/opsTrace'
import AccountTraceOverview from './AccountTraceOverview.vue'
import AccountStatusTimeline from './AccountStatusTimeline.vue'
import AccountLedgerTimeline from './AccountLedgerTimeline.vue'
import { credentialChanged, credentialFpPair, failuresByFingerprint } from './accountTraceUtils'

type TabKey = 'overview' | 'status' | 'ledger'

const props = defineProps<{
  show: boolean
  account: { id: number; name: string; platform?: string } | null
}>()
const emit = defineEmits<{ (e: 'close'): void }>()
const { t } = useI18n()

const TABS: TabKey[] = ['overview', 'status', 'ledger']
const RANGE_OPTIONS = [
  { hours: 1, label: 'admin.traceAccount.range.h1' },
  { hours: 24, label: 'admin.traceAccount.range.h24' },
  { hours: 168, label: 'admin.traceAccount.range.d7' }
]
const TRACE_LIMIT = 200
const LEDGER_LIMIT = 500

const tab = ref<TabKey>('overview')
const hours = ref(24)

const trace = ref<AccountTrace | null>(null)
const traceLoading = ref(false)
const traceError = ref('')
const ledger = ref<AccountLedger | null>(null)
const ledgerLoading = ref(false)
const ledgerError = ref('')

let traceSeq = 0
let ledgerSeq = 0

function errorMessage(err: unknown): string {
  const e = err as { message?: string; response?: { data?: { message?: string } } }
  return e?.response?.data?.message || e?.message || String(err)
}

async function loadTrace() {
  if (!props.account) return
  const seq = ++traceSeq
  traceLoading.value = true
  traceError.value = ''
  try {
    const data = await getAccountTrace(props.account.id, { hours: hours.value, limit: TRACE_LIMIT })
    if (seq !== traceSeq) return
    trace.value = { ...data, attempts: data.attempts ?? [], status_events: data.status_events ?? [] }
  } catch (err) {
    if (seq !== traceSeq) return
    traceError.value = errorMessage(err)
  } finally {
    if (seq === traceSeq) traceLoading.value = false
  }
}

async function loadLedger() {
  if (!props.account) return
  const seq = ++ledgerSeq
  ledgerLoading.value = true
  ledgerError.value = ''
  try {
    const data = await getAccountLedger(props.account.id, { limit: LEDGER_LIMIT })
    if (seq !== ledgerSeq) return
    ledger.value = { ...data, events: data.events ?? [] }
  } catch (err) {
    if (seq !== ledgerSeq) return
    ledgerError.value = errorMessage(err)
  } finally {
    if (seq === ledgerSeq) ledgerLoading.value = false
  }
}

/** 台账里每个新指纹的换入时间（取最近一次），供概览 tooltip 对照 */
const fpSwitchedAt = computed<Record<string, string>>(() => {
  const out: Record<string, string> = {}
  for (const ev of ledger.value?.events ?? []) {
    if (!credentialChanged(ev)) continue
    const to = credentialFpPair(ev).to
    if (!to) continue
    if (!out[to] || new Date(ev.created_at) > new Date(out[to])) out[to] = ev.created_at
  }
  return out
})

const failuresByFp = computed(() => failuresByFingerprint(trace.value?.attempts ?? []))

watch(
  () => [props.show, props.account?.id] as const,
  ([visible]) => {
    if (!visible || !props.account) return
    tab.value = 'overview'
    hours.value = 24
    trace.value = null
    ledger.value = null
    loadTrace()
    loadLedger()
  },
  { immediate: true }
)

watch(hours, () => {
  if (props.show && props.account) loadTrace()
})
</script>
