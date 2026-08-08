<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="flex flex-wrap items-end gap-3">
          <label class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.costCenter.start') }}<input v-model="filters.start" type="datetime-local" class="input mt-1" /></label>
          <label class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.costCenter.end') }}<input v-model="filters.end" type="datetime-local" class="input mt-1" /></label>
          <input v-model="filters.platform" class="input" placeholder="Platform" />
          <input v-model="filters.model" class="input" placeholder="Model" />
          <input v-model="filters.source_type" class="input" placeholder="Source" />
          <input v-model="filters.category" class="input" :placeholder="t('admin.costCenter.category')" />
          <button class="btn btn-primary" :disabled="loading" @click="load"><Icon name="refresh" size="sm" />{{ t('common.refresh') }}</button>
        </div>
      </div>
      <div v-if="summary" class="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-6">
        <div v-for="card in cards" :key="card.key" class="card p-4"><p class="text-xs text-gray-500">{{ card.label }}</p><p class="mt-1 text-xl font-semibold" :class="card.class">${{ card.value.toFixed(2) }}</p></div>
      </div>
      <div class="card overflow-hidden"><div class="border-b border-gray-200 px-4 py-3 font-medium dark:border-dark-600">{{ t('admin.costCenter.events') }}</div><div class="overflow-x-auto"><table class="min-w-full text-sm"><thead><tr class="text-left text-xs text-gray-500"><th class="px-4 py-3">{{ t('admin.costCenter.type') }}</th><th class="px-4 py-3">{{ t('admin.costCenter.source') }}</th><th class="px-4 py-3">{{ t('admin.costCenter.category') }}</th><th class="px-4 py-3">USD</th><th class="px-4 py-3">{{ t('admin.costCenter.occurredAt') }}</th><th class="px-4 py-3">{{ t('admin.costCenter.note') }}</th><th class="px-4 py-3">{{ t('admin.costCenter.status') }}</th><th class="px-4 py-3"></th></tr></thead><tbody><tr v-for="event in events" :key="event.id" class="border-t border-gray-100 dark:border-dark-700"><td class="px-4 py-3">{{ event.event_type }}</td><td class="px-4 py-3">{{ event.source_type }}</td><td class="px-4 py-3">{{ event.category || '-' }}</td><td class="px-4 py-3 font-mono">${{ event.amount_usd.toFixed(4) }}</td><td class="px-4 py-3">{{ formatDate(event.occurred_at) }}</td><td class="px-4 py-3">{{ event.note || '-' }}</td><td class="px-4 py-3">{{ event.status }}</td><td class="px-4 py-3"><button v-if="event.status === 'pending'" class="btn btn-sm btn-primary" @click="review(event)">{{ t('admin.costCenter.confirm') }}</button><button v-else-if="event.status === 'settled' && event.source_type !== 'upstream'" class="btn btn-sm btn-secondary" @click="reverse(event)">{{ t('admin.costCenter.reverse') }}</button></td></tr><tr v-if="!events.length"><td colspan="8" class="px-4 py-8 text-center text-gray-500">{{ t('common.noData') }}</td></tr></tbody></table></div></div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { CostCenterSummary, CostCenterEvent } from '@/api/admin/costCenter'

const { t } = useI18n(); const now = new Date(); const prior = new Date(now); prior.setDate(now.getDate() - 30)
const localInput = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
const filters = reactive({ start: localInput(prior), end: localInput(now), platform: '', model: '', source_type: '', category: '' }); const summary = ref<CostCenterSummary | null>(null); const events = ref<CostCenterEvent[]>([]); const loading = ref(false)
const params = () => ({ ...filters, start: new Date(filters.start).toISOString(), end: new Date(filters.end).toISOString(), page: 1, page_size: 100 })
const cards = computed(() => summary.value ? [
  { key: 'cash', label: t('admin.costCenter.cashIncome'), value: summary.value.cash_income, class: 'text-emerald-600' },
  { key: 'realized', label: t('admin.costCenter.realizedIncome'), value: summary.value.realized_income, class: 'text-emerald-600' },
  { key: 'upstream', label: t('admin.costCenter.upstreamCost'), value: summary.value.upstream_cost, class: 'text-red-600' },
  { key: 'expenses', label: t('admin.costCenter.expenses'), value: summary.value.settled_expenses, class: 'text-red-600' },
  { key: 'cashProfit', label: t('admin.costCenter.cashProfit'), value: summary.value.cash_profit, class: 'text-blue-600' },
  { key: 'operatingProfit', label: t('admin.costCenter.operatingProfit'), value: summary.value.operating_profit, class: 'text-blue-600' },
  { key: 'deferred', label: 'Deferred subscription', value: summary.value.deferred_subscription_usd, class: 'text-amber-600' },
  { key: 'expired', label: 'Expired entitlement', value: summary.value.expired_entitlement_usd, class: 'text-amber-600' },
] : [])
const formatDate = (value: string) => new Date(value).toLocaleString()
async function load() { loading.value = true; try { const p = params(); const [s, e] = await Promise.all([adminAPI.costCenter.getSummary(p), adminAPI.costCenter.getEvents(p)]); summary.value = s.data; events.value = e.data.items || [] } finally { loading.value = false } }
async function review(event: CostCenterEvent) { const reason = window.prompt(t('admin.costCenter.reasonPrompt')); if (!reason) return; await adminAPI.costCenter.updateEventStatus(event.id, 'settled', reason); await load() }
async function reverse(event: CostCenterEvent) { const reason = window.prompt(t('admin.costCenter.reasonPrompt')); if (!reason) return; await adminAPI.costCenter.reverseEvent(event.id, reason); await load() }
onMounted(load)
</script>
