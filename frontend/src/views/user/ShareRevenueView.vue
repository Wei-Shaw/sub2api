<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <template v-else-if="summary">
        <div
          v-if="!summary.enabled"
          class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-200"
        >
          {{ t('shareRevenue.disabledHint') }}
        </div>

        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">
              {{ t('shareRevenue.stats.totalEarned') }}
            </p>
            <p class="mt-2 text-2xl font-semibold text-emerald-600 dark:text-emerald-400">
              {{ formatCurrency(summary.total_earned) }}
            </p>
            <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">
              {{ t('shareRevenue.stats.totalEarnedHint') }}
            </p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">
              {{ t('shareRevenue.stats.records') }}
            </p>
            <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">
              {{ summary.total_records }}
            </p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">
              {{ t('shareRevenue.stats.userPct') }}
            </p>
            <p class="mt-2 text-2xl font-semibold text-primary-600 dark:text-primary-400">
              {{ formatPct(summary.user_pct) }}%
            </p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">
              {{ t('shareRevenue.stats.split') }}
            </p>
            <p class="mt-2 text-sm font-medium text-gray-800 dark:text-gray-200">
              {{ t('shareRevenue.stats.splitDetail', {
                invite: formatPct(summary.invite_pct),
                user: formatPct(summary.user_pct),
                platform: formatPct(summary.platform_pct)
              }) }}
            </p>
          </div>
        </div>

        <div class="card overflow-hidden">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('shareRevenue.ledgersTitle') }}
            </h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
              {{ t('shareRevenue.ledgersHint') }}
            </p>
          </div>

          <div v-if="listLoading" class="flex justify-center py-10">
            <div
              class="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
            ></div>
          </div>

          <div v-else-if="items.length === 0" class="px-6 py-10 text-center text-sm text-gray-500">
            {{ t('shareRevenue.empty') }}
          </div>

          <div v-else class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/50">
                <tr>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                    {{ t('shareRevenue.columns.time') }}
                  </th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                    {{ t('shareRevenue.columns.account') }}
                  </th>
                  <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">
                    {{ t('shareRevenue.columns.total') }}
                  </th>
                  <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">
                    {{ t('shareRevenue.columns.earned') }}
                  </th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                    {{ t('shareRevenue.columns.request') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="row in items" :key="row.id" class="bg-white dark:bg-dark-900">
                  <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-700 dark:text-gray-300">
                    {{ formatTime(row.created_at) }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-700 dark:text-gray-300">
                    #{{ row.account_id || '—' }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-right text-sm text-gray-700 dark:text-gray-300">
                    {{ formatCurrency(row.total_cost) }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-right text-sm font-medium text-emerald-600 dark:text-emerald-400">
                    +{{ formatCurrency(row.user_amount) }}
                  </td>
                  <td class="max-w-[12rem] truncate px-4 py-3 text-xs text-gray-500" :title="row.request_id">
                    {{ row.request_id }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div
            v-if="total > pageSize"
            class="flex items-center justify-between border-t border-gray-100 px-4 py-3 dark:border-dark-700"
          >
            <p class="text-sm text-gray-500">
              {{ t('shareRevenue.pageInfo', { page, pages, total }) }}
            </p>
            <div class="flex gap-2">
              <button class="btn btn-secondary btn-sm" :disabled="page <= 1" @click="goPage(page - 1)">
                {{ t('shareRevenue.prev') }}
              </button>
              <button
                class="btn btn-secondary btn-sm"
                :disabled="page >= pages"
                @click="goPage(page + 1)"
              >
                {{ t('shareRevenue.next') }}
              </button>
            </div>
          </div>
        </div>
      </template>

      <div v-else class="card p-8 text-center text-sm text-gray-500">
        {{ t('shareRevenue.loadFailed') }}
        <button class="btn btn-secondary btn-sm mt-3" @click="loadAll">{{ t('shareRevenue.retry') }}</button>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import shareRevenueAPI, {
  type ShareRevenueLedgerItem,
  type ShareRevenueSummary
} from '@/api/shareRevenue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const listLoading = ref(false)
const summary = ref<ShareRevenueSummary | null>(null)
const items = ref<ShareRevenueLedgerItem[]>([])
const page = ref(1)
const pageSize = 20
const total = ref(0)

const pages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

function formatCurrency(v: number): string {
  return Number(v || 0).toFixed(4)
}

function formatPct(v: number): string {
  return Number(v || 0).toFixed(1)
}

function formatTime(iso: string): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

async function loadSummary() {
  summary.value = await shareRevenueAPI.getSummary()
}

async function loadLedgers() {
  listLoading.value = true
  try {
    const res = await shareRevenueAPI.listLedgers(page.value, pageSize)
    items.value = res.items || []
    total.value = res.total || 0
  } finally {
    listLoading.value = false
  }
}

async function loadAll() {
  loading.value = true
  try {
    await loadSummary()
    await loadLedgers()
  } catch (e: unknown) {
    appStore.showError(extractApiErrorMessage(e, t('shareRevenue.loadFailed')))
    summary.value = null
  } finally {
    loading.value = false
  }
}

async function goPage(p: number) {
  page.value = p
  await loadLedgers()
}

onMounted(loadAll)
</script>
