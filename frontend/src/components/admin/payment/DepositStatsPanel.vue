<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CurrencyAmounts, DepositEventStat, DepositRecipientStat, DepositStats } from '@/types/payment'

const props = defineProps<{
  stats?: DepositStats | null
}>()

const { t } = useI18n()

const depositSourceRows = computed(() => props.stats?.by_source ?? [])
const depositRecentEvents = computed(() => props.stats?.recent_events ?? [])
const depositTopRecipients = computed(() => props.stats?.top_recipients ?? [])

const CURRENCY_SYMBOLS: Record<string, string> = {
  USD: '$', VND: '₫', CNY: '¥', EUR: '€', GBP: '£',
}

function formatPaymentAmount(value: number, currency: string): string {
  const symbol = CURRENCY_SYMBOLS[currency] || currency + ' '
  if (currency === 'VND') {
    return symbol + Math.round(value).toLocaleString()
  }
  return symbol + value.toFixed(2)
}

function formatLedgerAmount(value: number, currency = 'USD'): string {
  return formatPaymentAmount(value, currency || 'USD')
}

function formatCurrencyAmounts(amounts: CurrencyAmounts): string {
  const entries = Object.entries(amounts).sort(([left], [right]) => left.localeCompare(right))
  if (entries.length === 0) return formatLedgerAmount(0)
  return entries.map(([currency, amount]) => formatLedgerAmount(amount, currency)).join(' · ')
}

function formatCredits(value: number): string {
  return Math.round(value).toLocaleString()
}

function formatDateTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

function sourceLabel(source: string): string {
  return t('payment.admin.depositSourceLabels.' + source, source)
}

function displayUser(user: Pick<DepositEventStat | DepositRecipientStat, 'email' | 'username' | 'user_id'>): string {
  return user.email || user.username || `#${user.user_id}`
}

function sourceBadgeClass(source: string): string {
  const base = 'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium'
  const colors: Record<string, string> = {
    paid_balance_order: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    paid_subscription_order: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    redeem_balance: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
    redeem_subscription: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
    redeem_affiliate_balance: 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300',
    admin_balance_adjustment: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    manual_subscription_assignment: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
    auto_subscription_assignment: 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300',
  }
  return `${base} ${colors[source] || 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'}`
}
</script>

<template>
  <section class="space-y-4">
    <div>
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.depositOverview') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.depositOverviewDesc') }}</p>
    </div>
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-5">
      <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('payment.admin.totalDepositEvents') }}</p>
        <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ stats?.total_events ?? 0 }}</p>
      </div>
      <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('payment.admin.depositLedgerAmount') }}</p>
        <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatCurrencyAmounts(stats?.total_ledger_amount ?? {}) }}</p>
      </div>
      <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('payment.admin.depositCredits') }}</p>
        <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatCredits(stats?.total_credits ?? 0) }}</p>
      </div>
      <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('payment.admin.depositPackageAssignments') }}</p>
        <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ stats?.subscription_assignments ?? 0 }}</p>
      </div>
      <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('payment.admin.depositAdminAutoBreakdown') }}</p>
        <p class="mt-2 text-sm font-medium text-gray-900 dark:text-white">
          {{ t('payment.admin.adminAdjustments') }}: {{ stats?.admin_adjustments ?? 0 }}
        </p>
        <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">
          {{ t('payment.admin.manualAssignments') }}: {{ stats?.manual_assignments ?? 0 }} · {{ t('payment.admin.autoAssignments') }}: {{ stats?.auto_assignments ?? 0 }}
        </p>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-3">
      <div class="card p-4 xl:col-span-1">
        <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.depositSources') }}</h3>
        <div v-if="!depositSourceRows.length" class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.noData') }}</div>
        <div v-else class="space-y-3">
          <div v-for="source in depositSourceRows" :key="source.source" class="rounded-lg border border-gray-100 p-3 dark:border-dark-700">
            <div class="flex items-center justify-between gap-3">
              <span class="text-sm font-medium text-gray-900 dark:text-white">{{ sourceLabel(source.source) }}</span>
              <span :class="sourceBadgeClass(source.source)">{{ source.count }}</span>
            </div>
            <div class="mt-2 grid grid-cols-2 gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span>{{ t('payment.admin.depositLedgerAmount') }}: {{ formatCurrencyAmounts(source.ledger_amount) }}</span>
              <span>{{ t('payment.admin.depositCredits') }}: {{ formatCredits(source.credits) }}</span>
              <span>{{ t('payment.admin.depositPackageAssignments') }}: {{ source.subscription_assignments }}</span>
              <span>{{ t('payment.admin.lastDeposit') }}: {{ formatDateTime(source.last_deposit_at) }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="card p-4 xl:col-span-2">
        <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.recentDeposits') }}</h3>
        <div v-if="!depositRecentEvents.length" class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.noData') }}</div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="text-left text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">
              <tr>
                <th class="py-2 pr-4 font-medium">{{ t('payment.admin.colUser') }}</th>
                <th class="py-2 pr-4 font-medium">{{ t('payment.admin.colSource') }}</th>
                <th class="py-2 pr-4 font-medium">{{ t('payment.admin.colAmount') }}</th>
                <th class="py-2 pr-4 font-medium">{{ t('payment.admin.colPackage') }}</th>
                <th class="py-2 pr-4 font-medium">{{ t('payment.admin.operator') }}</th>
                <th class="py-2 font-medium">{{ t('payment.admin.lastDeposit') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="event in depositRecentEvents" :key="event.reference_type + ':' + event.reference_id" class="text-gray-700 dark:text-gray-300">
                <td class="py-3 pr-4">
                  <div class="font-medium text-gray-900 dark:text-white">{{ displayUser(event) }}</div>
                  <div class="text-xs text-gray-500">ID {{ event.user_id }}</div>
                </td>
                <td class="py-3 pr-4">
                  <span :class="sourceBadgeClass(event.source)">{{ sourceLabel(event.source) }}</span>
                  <div v-if="event.payment_type" class="mt-1 text-xs text-gray-500">{{ t('payment.methods.' + event.payment_type, event.payment_type) }}</div>
                </td>
                <td class="py-3 pr-4">
                  <div>{{ formatLedgerAmount(event.ledger_amount, event.currency) }}</div>
                  <div class="text-xs text-gray-500">{{ formatCredits(event.credits) }}</div>
                </td>
                <td class="py-3 pr-4">
                  <div v-if="event.subscription_assignments">{{ event.group_name || event.group_id || t('payment.admin.unknownPackage') }}</div>
                  <div v-else class="text-gray-400">—</div>
                  <div v-if="event.validity_days" class="text-xs text-gray-500">{{ event.validity_days }} {{ t('payment.admin.days') }}</div>
                </td>
                <td class="py-3 pr-4">{{ event.operator_email || (event.operator_id ? `#${event.operator_id}` : '—') }}</td>
                <td class="py-3">{{ formatDateTime(event.occurred_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div class="card p-4">
      <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.depositRecipients') }}</h3>
      <div v-if="!depositTopRecipients.length" class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.noData') }}</div>
      <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        <div v-for="recipient in depositTopRecipients" :key="recipient.user_id" class="rounded-lg border border-gray-100 p-3 dark:border-dark-700">
          <div class="flex items-start justify-between gap-3">
            <div>
              <div class="text-sm font-medium text-gray-900 dark:text-white">{{ displayUser(recipient) }}</div>
              <div class="text-xs text-gray-500">ID {{ recipient.user_id }}</div>
            </div>
            <span :class="sourceBadgeClass(recipient.last_source || '')">{{ recipient.count }}</span>
          </div>
          <div class="mt-3 grid grid-cols-2 gap-2 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t('payment.admin.depositLedgerAmount') }}: {{ formatCurrencyAmounts(recipient.ledger_amount) }}</span>
            <span>{{ t('payment.admin.depositCredits') }}: {{ formatCredits(recipient.credits) }}</span>
            <span>{{ t('payment.admin.depositPackageAssignments') }}: {{ recipient.subscription_assignments }}</span>
            <span>{{ t('payment.admin.lastDeposit') }}: {{ formatDateTime(recipient.last_deposit_at) }}</span>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
