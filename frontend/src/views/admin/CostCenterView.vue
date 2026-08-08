<template>
  <AppLayout>
    <div class="space-y-4">
      <div v-if="summary" class="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-6">
        <div v-for="card in cards" :key="card.key" class="card group relative p-4">
          <p class="text-xs text-gray-500">{{ card.label }}</p>
          <p class="mt-1 text-xl font-semibold" :class="card.class">${{ card.value.toFixed(2) }}</p>
          <div class="pointer-events-none absolute left-3 right-3 top-full z-20 mt-2 rounded-lg bg-gray-900 px-3 py-2 text-xs leading-relaxed text-white opacity-0 shadow-lg transition-opacity group-hover:opacity-100 dark:bg-gray-100 dark:text-gray-900">
            {{ card.description }}
          </div>
        </div>
      </div>
      <div class="card p-4 sm:p-6">
        <div class="flex flex-wrap items-end justify-between gap-4">
          <div class="flex flex-1 flex-wrap items-end gap-4">
            <div class="w-full sm:w-auto sm:min-w-[220px]">
              <label class="input-label">{{ t('admin.dashboard.timeRange') }}</label>
              <DateRangePicker v-model:start-date="filters.start" v-model:end-date="filters.end" @change="onDateRangeChange" />
            </div>
            <div class="w-full sm:w-auto sm:min-w-[220px]">
              <label class="input-label">{{ t('admin.costCenter.model') }}</label>
              <Select v-model="filters.model" :options="modelOptions" searchable @change="load" />
            </div>
            <div class="w-full sm:w-auto sm:min-w-[200px]">
              <label class="input-label">{{ t('admin.costCenter.source') }}</label>
              <Select v-model="filters.source_type" :options="sourceOptions" searchable @change="load" />
            </div>
            <div class="w-full sm:w-auto sm:min-w-[200px]">
              <label class="input-label">{{ t('admin.costCenter.category') }}</label>
              <Select v-model="filters.category" :options="categoryOptions" searchable @change="load" />
            </div>
          </div>
          <div class="flex w-full items-center justify-end gap-3 sm:w-auto">
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
              <Icon name="refresh" size="sm" class="mr-1.5" />{{ t('common.refresh') }}
            </button>
          </div>
        </div>
      </div>
      <div class="card overflow-hidden">
        <div class="border-b border-gray-200 px-4 py-3 font-medium dark:border-dark-600">{{ t('admin.costCenter.events') }}</div>
        <DataTable :columns="eventColumns" :data="events" :loading="loading" row-key="id">
          <template #cell-event_type="{ row }"><span :title="row.event_type">{{ eventTypeLabel(row.event_type) }}</span></template>
          <template #cell-source_type="{ row }"><span :title="row.source_type">{{ sourceLabel(row.source_type) }}</span></template>
          <template #cell-category="{ row }"><span :title="row.category || undefined">{{ categoryLabel(row.category) }}</span></template>
          <template #cell-model="{ row }"><span :title="row.model || undefined">{{ row.model || '-' }}</span></template>
          <template #cell-amount_usd="{ row }"><span class="font-mono" :title="eventHelp(row)">${{ row.amount_usd.toFixed(4) }}</span></template>
          <template #cell-occurred_at="{ value }"><span class="whitespace-nowrap">{{ formatDate(value) }}</span></template>
          <template #cell-note="{ row }"><span class="text-gray-600 dark:text-gray-300">{{ row.note || '-' }}</span></template>
          <template #cell-status="{ row }"><span :title="row.status">{{ statusLabel(row.status) }}</span></template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button v-if="row.status === 'pending'" class="btn btn-sm btn-primary" @click="review(row)">{{ t('admin.costCenter.confirm') }}</button>
              <button v-else-if="row.status === 'settled' && row.source_type !== 'upstream'" class="btn btn-sm btn-secondary" @click="reverse(row)">{{ t('admin.costCenter.reverse') }}</button>
            </div>
          </template>
        </DataTable>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'
import { adminAPI } from '@/api/admin'
import type { CostCenterSummary, CostCenterEvent } from '@/api/admin/costCenter'

const { t } = useI18n(); const now = new Date(); const prior = new Date(now); prior.setDate(now.getDate() - 30)
const dateOnly = (d: Date) => d.toISOString().slice(0, 10)
const filters = reactive({ start: dateOnly(prior), end: dateOnly(now), model: '', source_type: '', category: '' }); const summary = ref<CostCenterSummary | null>(null); const events = ref<CostCenterEvent[]>([]); const loading = ref(false)
const knownModels = ref<string[]>([])
const knownCategories = ref<string[]>([])
const rangeStart = (value: string) => new Date(`${value}T00:00:00`).toISOString()
const rangeEnd = (value: string) => new Date(new Date(`${value}T00:00:00`).getTime() + 24 * 60 * 60 * 1000).toISOString()
const params = () => ({ ...filters, start: rangeStart(filters.start), end: rangeEnd(filters.end), page: 1, page_size: 100 })
const cards = computed(() => summary.value ? [
  { key: 'cash', label: t('admin.costCenter.cashIncome'), description: t('admin.costCenter.cashIncomeHelp'), value: summary.value.cash_income, class: 'text-emerald-600' },
  { key: 'realized', label: t('admin.costCenter.realizedIncome'), description: t('admin.costCenter.realizedIncomeHelp'), value: summary.value.realized_income, class: 'text-emerald-600' },
  { key: 'upstream', label: t('admin.costCenter.upstreamCost'), description: t('admin.costCenter.upstreamCostHelp'), value: summary.value.upstream_cost, class: 'text-red-600' },
  { key: 'expenses', label: t('admin.costCenter.expenses'), description: t('admin.costCenter.expensesHelp'), value: summary.value.settled_expenses, class: 'text-red-600' },
  { key: 'cashProfit', label: t('admin.costCenter.cashProfit'), description: t('admin.costCenter.cashProfitHelp'), value: summary.value.cash_profit, class: 'text-blue-600' },
  { key: 'operatingProfit', label: t('admin.costCenter.operatingProfit'), description: t('admin.costCenter.operatingProfitHelp'), value: summary.value.operating_profit, class: 'text-blue-600' },
  { key: 'deferred', label: t('admin.costCenter.deferredSubscription'), description: t('admin.costCenter.deferredSubscriptionHelp'), value: summary.value.deferred_subscription_usd, class: 'text-amber-600' },
  { key: 'expired', label: t('admin.costCenter.expiredEntitlement'), description: t('admin.costCenter.expiredEntitlementHelp'), value: summary.value.expired_entitlement_usd, class: 'text-amber-600' },
] : [])
const formatDate = (value: string) => new Date(value).toLocaleString()
const translateEnum = (path: string, value: string) => {
  const translated = t(`${path}.${value}`)
  return translated === `${path}.${value}` ? value : translated
}
const toOptions = (values: string[], path: string) =>
  [...new Set(values.filter(Boolean))]
    .sort()
    .map(value => ({ value, label: translateEnum(path, value) }))
const modelOptions = computed(() =>
  [{ value: '', label: t('admin.costCenter.allModels') }, ...toOptions([...knownModels.value, filters.model], 'admin.costCenter.models')]
)
const sourceOptions = computed(() =>
  [{ value: '', label: t('admin.costCenter.allSources') }, ...toOptions(
    [
      'payment_order',
      'paid_balance',
      'subscription',
      'recharge_bonus',
      'admin_grant',
      'affiliate_grant',
      'upstream',
      'recurring',
      'account',
      'manual',
      'reversal',
      'unknown',
      filters.source_type
    ],
    'admin.costCenter.sources'
  )]
)
const categoryOptions = computed(() =>
  [{ value: '', label: t('admin.costCenter.allCategories') }, ...toOptions(
    [
      ...knownCategories.value,
      'balance',
      'subscription',
      'account_setup',
      'account_renewal',
      'account_recharge',
      'account_expense',
      'token_consumption',
      'upstream_usage',
      'proxy',
      'server',
      'database',
      'bandwidth',
      'payment_fee',
      'rebate',
      'refund_loss',
      'other',
      filters.category
    ],
    'admin.costCenter.categories'
  )]
)
const eventColumns = computed<Column[]>(() => [
  { key: 'event_type', label: t('admin.costCenter.type') },
  { key: 'source_type', label: t('admin.costCenter.source') },
  { key: 'category', label: t('admin.costCenter.category') },
  { key: 'model', label: t('admin.costCenter.model') },
  { key: 'amount_usd', label: 'USD' },
  { key: 'occurred_at', label: t('admin.costCenter.occurredAt') },
  { key: 'note', label: t('admin.costCenter.note') },
  { key: 'status', label: t('admin.costCenter.status') },
  { key: 'actions', label: t('common.actions') }
])
const eventTypeLabel = (value: string) => translateEnum('admin.costCenter.eventTypes', value)
const sourceLabel = (value: string) => translateEnum('admin.costCenter.sources', value)
const statusLabel = (value: string) => translateEnum('admin.costCenter.statuses', value)
const categoryLabel = (value: string) => value ? translateEnum('admin.costCenter.categories', value) : '-'
const eventHelp = (event: CostCenterEvent) => t(`admin.costCenter.eventHelp.${event.event_type}`, { source: event.source_type })
async function load() { loading.value = true; try { const p = params(); const [s, e] = await Promise.all([adminAPI.costCenter.getSummary(p), adminAPI.costCenter.getEvents(p)]); summary.value = s.data; events.value = e.data.items || []; knownModels.value = [...new Set([...knownModels.value, ...events.value.map(event => event.model || '')].filter(Boolean))]; knownCategories.value = [...new Set([...knownCategories.value, ...events.value.map(event => event.category || '')].filter(Boolean))] } finally { loading.value = false } }
function onDateRangeChange(range: { startDate: string; endDate: string }) { filters.start = range.startDate; filters.end = range.endDate; load() }
async function review(event: CostCenterEvent) { const reason = window.prompt(t('admin.costCenter.reasonPrompt')); if (!reason) return; await adminAPI.costCenter.updateEventStatus(event.id, 'settled', reason); await load() }
async function reverse(event: CostCenterEvent) { const reason = window.prompt(t('admin.costCenter.reasonPrompt')); if (!reason) return; await adminAPI.costCenter.reverseEvent(event.id, reason); await load() }
onMounted(load)
</script>
