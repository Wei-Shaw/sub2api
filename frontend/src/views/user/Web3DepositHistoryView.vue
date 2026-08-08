<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="flex items-center justify-between gap-3">
          <div>
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">Web3 deposit history</h1>
            <p class="text-sm text-gray-500">USDT0 deposits detected for your assigned address.</p>
          </div>
          <div class="flex gap-2">
            <button class="btn btn-secondary" :disabled="loading" @click="loadDeposits">Refresh</button>
            <button class="btn btn-primary" @click="router.push('/web3-deposit')">Deposit address</button>
          </div>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div v-if="loading" class="p-12 text-center text-sm text-gray-500">Loading deposits…</div>
        <div v-else-if="deposits.length === 0" class="p-12 text-center text-sm text-gray-500">No Web3 deposits detected yet.</div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr class="text-left text-xs font-medium uppercase tracking-wide text-gray-500">
                <th class="px-5 py-3">Amount</th>
                <th class="px-5 py-3">Status</th>
                <th class="px-5 py-3">Transaction</th>
                <th class="px-5 py-3">Detected</th>
                <th class="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="deposit in deposits" :key="deposit.id" class="text-sm">
                <td class="px-5 py-4"><span class="font-semibold">{{ deposit.token_amount }} USDT0</span><div v-if="deposit.credited_amount" class="text-xs text-gray-500">{{ deposit.credited_amount }} USD credited</div></td>
                <td class="px-5 py-4"><span :class="statusClass(deposit.status)" class="inline-flex rounded-full px-2.5 py-1 text-xs font-medium">{{ statusLabel(deposit.status) }}</span></td>
                <td class="px-5 py-4 font-mono text-xs">{{ shortHash(deposit.tx_hash) }}</td>
                <td class="px-5 py-4 text-gray-600 dark:text-gray-300">{{ formatDateTime(deposit.detected_at) }}</td>
                <td class="px-5 py-4 text-right"><button class="text-primary-600 hover:text-primary-700" @click="showDetail(deposit.id)">Details</button></td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="changePage" @update:pageSize="changePageSize" />
    </div>

    <BaseDialog :show="!!selected" title="Deposit details" width="wide" @close="selected = undefined">
      <dl v-if="selected" class="grid gap-4 text-sm sm:grid-cols-2">
        <div><dt class="text-gray-500">Status</dt><dd class="mt-1 font-medium">{{ statusLabel(selected.status) }}</dd></div>
        <div><dt class="text-gray-500">Amount</dt><dd class="mt-1 font-medium">{{ selected.token_amount }} USDT0</dd></div>
        <div><dt class="text-gray-500">Credited amount</dt><dd class="mt-1 font-medium">{{ selected.credited_amount ? `${selected.credited_amount} USD` : '—' }}</dd></div>
        <div><dt class="text-gray-500">Chain ID</dt><dd class="mt-1 font-mono">{{ selected.chain_id }}</dd></div>
        <div class="sm:col-span-2"><dt class="text-gray-500">Transaction hash</dt><dd class="mt-1 break-all font-mono text-xs">{{ selected.tx_hash }}</dd></div>
        <div class="sm:col-span-2"><dt class="text-gray-500">Token contract</dt><dd class="mt-1 break-all font-mono text-xs">{{ selected.token_contract }}</dd></div>
        <div><dt class="text-gray-500">Detected</dt><dd class="mt-1">{{ formatDateTime(selected.detected_at) }}</dd></div>
        <div><dt class="text-gray-500">Finalized</dt><dd class="mt-1">{{ selected.finalized_at ? formatDateTime(selected.finalized_at) : '—' }}</dd></div>
        <div><dt class="text-gray-500">Credited</dt><dd class="mt-1">{{ selected.credited_at ? formatDateTime(selected.credited_at) : '—' }}</dd></div>
        <div><dt class="text-gray-500">Block</dt><dd class="mt-1 font-mono">{{ selected.block_number }}</dd></div>
      </dl>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import { web3DepositAPI } from '@/api/web3Deposit'
import type { Web3DepositRecord, Web3DepositStatus } from '@/types/web3Deposit'
import { formatDateTime } from '@/utils/format'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const loading = ref(false)
const deposits = ref<Web3DepositRecord[]>([])
const selected = ref<Web3DepositRecord>()
const pagination = reactive({ page: 1, page_size: 20, total: 0 })

const labels: Record<Web3DepositStatus, string> = { confirming: 'Confirming', credited: 'Credited', below_minimum: 'Below minimum', under_review: 'Under review', failed: 'Failed' }
const classes: Record<Web3DepositStatus, string> = {
  confirming: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
  credited: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
  below_minimum: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  under_review: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
  failed: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
}

const statusLabel = (status: Web3DepositStatus) => labels[status]
const statusClass = (status: Web3DepositStatus) => classes[status]
const shortHash = (hash: string) => `${hash.slice(0, 10)}…${hash.slice(-8)}`

async function loadDeposits() {
  loading.value = true
  try {
    const response = await web3DepositAPI.listDeposits({ page: pagination.page, page_size: pagination.page_size })
    deposits.value = response.data.items
    pagination.total = response.data.total
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', 'Failed to load deposit history'))
  } finally {
    loading.value = false
  }
}

async function showDetail(id: number) {
  try {
    selected.value = (await web3DepositAPI.getDeposit(id)).data
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', 'Failed to load deposit details'))
  }
}

function changePage(page: number) { pagination.page = page; loadDeposits() }
function changePageSize(pageSize: number) { pagination.page_size = pageSize; pagination.page = 1; loadDeposits() }
onMounted(loadDeposits)
</script>
