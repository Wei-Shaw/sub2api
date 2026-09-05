<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('web3Deposit.historyTitle') }}</h1>
            <p class="text-sm text-gray-500">{{ t('web3Deposit.historyHint') }}</p>
          </div>
          <div class="flex gap-2">
            <button class="btn btn-secondary" :disabled="loading" @click="loadDeposits">{{ t('web3Deposit.refresh') }}</button>
            <button class="btn btn-primary" @click="openDepositPage">{{ t('web3Deposit.title') }}</button>
          </div>
        </div>
        <div v-if="config?.enabled" class="mt-4 grid gap-3 sm:grid-cols-2">
          <label class="text-sm text-gray-600 dark:text-gray-300">
            <span class="mb-1 block">{{ t('web3Deposit.network') }}</span>
            <select v-model="selectedNetworkKey" class="input w-full">
              <option v-for="item in config.networks" :key="item.key" :value="item.key">{{ item.display_name }}</option>
            </select>
          </label>
          <label class="text-sm text-gray-600 dark:text-gray-300">
            <span class="mb-1 block">{{ t('web3Deposit.token') }}</span>
            <select v-model="selectedAssetKey" class="input w-full">
              <option v-for="item in network?.assets || []" :key="item.key" :value="item.key">{{ item.display_name }}</option>
            </select>
          </label>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div v-if="loading" class="p-12 text-center text-sm text-gray-500">{{ t('web3Deposit.loadingHistory') }}</div>
        <div v-else-if="deposits.length === 0" class="p-12 text-center text-sm text-gray-500">{{ t('web3Deposit.emptyHistory') }}</div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr class="text-left text-xs font-medium uppercase tracking-wide text-gray-500">
                <th class="px-5 py-3">{{ t('web3Deposit.amount') }}</th>
                <th class="px-5 py-3">{{ t('web3Deposit.status') }}</th>
                <th class="px-5 py-3">{{ t('web3Deposit.transaction') }}</th>
                <th class="px-5 py-3">{{ t('web3Deposit.detected') }}</th>
                <th class="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="deposit in deposits" :key="deposit.id" class="text-sm">
                <td class="px-5 py-4"><span class="font-semibold">{{ deposit.token_amount }} {{ asset?.display_name || '' }}</span><div v-if="deposit.credited_amount" class="text-xs text-gray-500">{{ t('web3Deposit.creditedUsd', { amount: deposit.credited_amount }) }}</div></td>
                <td class="px-5 py-4"><span :class="statusClass(deposit.status)" class="inline-flex rounded-full px-2.5 py-1 text-xs font-medium">{{ statusLabel(deposit.status) }}</span></td>
                <td class="px-5 py-4 font-mono text-xs">{{ shortHash(deposit.tx_hash) }}</td>
                <td class="px-5 py-4 text-gray-600 dark:text-gray-300">{{ formatDateTime(deposit.detected_at) }}</td>
                <td class="px-5 py-4 text-right"><button class="text-primary-600 hover:text-primary-700" @click="showDetail(deposit.id)">{{ t('web3Deposit.details') }}</button></td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="changePage" @update:pageSize="changePageSize" />
    </div>

    <BaseDialog :show="!!selected" :title="t('web3Deposit.detailTitle')" width="wide" @close="selected = undefined">
      <dl v-if="selected" class="grid gap-4 text-sm sm:grid-cols-2">
        <div><dt class="text-gray-500">{{ t('web3Deposit.status') }}</dt><dd class="mt-1 font-medium">{{ statusLabel(selected.status) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('web3Deposit.amount') }}</dt><dd class="mt-1 font-medium">{{ selected.token_amount }} {{ asset?.display_name || '' }}</dd></div>
        <div><dt class="text-gray-500">{{ t('web3Deposit.creditedAmount') }}</dt><dd class="mt-1 font-medium">{{ selected.credited_amount ? `${selected.credited_amount} USD` : '—' }}</dd></div>
        <div><dt class="text-gray-500">Chain ID</dt><dd class="mt-1 font-mono">{{ selected.chain_id }}</dd></div>
        <div class="sm:col-span-2"><dt class="text-gray-500">{{ t('web3Deposit.transactionHash') }}</dt><dd class="mt-1 break-all font-mono text-xs">{{ selected.tx_hash }}</dd></div>
        <div class="sm:col-span-2"><dt class="text-gray-500">{{ t('web3Deposit.tokenContract') }}</dt><dd class="mt-1 break-all font-mono text-xs">{{ selected.token_contract }}</dd></div>
        <div><dt class="text-gray-500">{{ t('web3Deposit.detected') }}</dt><dd class="mt-1">{{ formatDateTime(selected.detected_at) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('web3Deposit.finalized') }}</dt><dd class="mt-1">{{ selected.finalized_at ? formatDateTime(selected.finalized_at) : '—' }}</dd></div>
        <div><dt class="text-gray-500">{{ t('web3Deposit.credited') }}</dt><dd class="mt-1">{{ selected.credited_at ? formatDateTime(selected.credited_at) : '—' }}</dd></div>
        <div><dt class="text-gray-500">{{ t('web3Deposit.block') }}</dt><dd class="mt-1 font-mono">{{ selected.block_number }}</dd></div>
      </dl>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import { web3DepositAPI } from '@/api/web3Deposit'
import type { Web3DepositConfig, Web3DepositRecord, Web3DepositStatus } from '@/types/web3Deposit'
import { formatDateTime } from '@/utils/format'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const loading = ref(false)
const deposits = ref<Web3DepositRecord[]>([])
const selected = ref<Web3DepositRecord>()
const config = ref<Web3DepositConfig>()
const selectedNetworkKey = ref('')
const selectedAssetKey = ref('')
const selectionReady = ref(false)
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const network = computed(() => config.value?.networks.find(item => item.key === selectedNetworkKey.value) || config.value?.networks[0])
const asset = computed(() => network.value?.assets.find(item => item.key === selectedAssetKey.value) || network.value?.assets[0])

const classes: Record<Web3DepositStatus, string> = {
  confirming: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
  credited: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
  below_minimum: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  under_review: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
  failed: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
}

const statusLabel = (status: Web3DepositStatus) => t(`web3Deposit.statuses.${status}`)
const statusClass = (status: Web3DepositStatus) => classes[status]
const shortHash = (hash: string) => `${hash.slice(0, 10)}…${hash.slice(-8)}`

async function loadDeposits() {
  loading.value = true
  try {
    const response = await web3DepositAPI.listDeposits({
      page: pagination.page,
      page_size: pagination.page_size,
      network_key: network.value?.key,
      asset_key: asset.value?.key,
    })
    deposits.value = response.data.items
    pagination.total = response.data.total
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', t('web3Deposit.errors.loadHistory')))
  } finally {
    loading.value = false
  }
}

async function showDetail(id: number) {
  try {
    selected.value = (await web3DepositAPI.getDeposit(id)).data
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', t('web3Deposit.errors.loadDetail')))
  }
}

function changePage(page: number) { pagination.page = page; loadDeposits() }
function changePageSize(pageSize: number) { pagination.page_size = pageSize; pagination.page = 1; loadDeposits() }

function openDepositPage() {
  void router.push({
    path: '/web3-deposit',
    query: { network: network.value?.key, asset: asset.value?.key },
  })
}

watch(network, value => {
  if (!value?.assets.some(item => item.key === selectedAssetKey.value)) {
    selectedAssetKey.value = value?.assets[0]?.key || ''
  }
})

watch([selectedNetworkKey, selectedAssetKey], ([networkKey, assetKey]) => {
  if (!selectionReady.value || !networkKey || !assetKey) return
  pagination.page = 1
  selected.value = undefined
  void router.replace({ query: { ...route.query, network: networkKey, asset: assetKey } })
  void loadDeposits()
})

onMounted(async () => {
  try {
    config.value = (await web3DepositAPI.getConfig()).data
    const requestedNetwork = typeof route.query.network === 'string' ? route.query.network : ''
    selectedNetworkKey.value = config.value.networks.some(item => item.key === requestedNetwork) ? requestedNetwork : (config.value.networks[0]?.key || '')
    const requestedAsset = typeof route.query.asset === 'string' ? route.query.asset : ''
    selectedAssetKey.value = network.value?.assets.some(item => item.key === requestedAsset) ? requestedAsset : (network.value?.assets[0]?.key || '')
    await nextTick()
    selectionReady.value = true
    await loadDeposits()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', t('web3Deposit.errors.loadHistory')))
  }
})
</script>
