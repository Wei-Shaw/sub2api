<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="grid gap-3 sm:grid-cols-3">
        <div class="card p-4">
          <div class="text-sm text-gray-500">{{ t('admin.web3Deposits.runtime.title') }}</div>
          <div class="mt-2 flex items-center gap-3">
            <label for="web3-runtime-network" class="shrink-0 text-sm font-medium">{{ t('admin.web3Deposits.runtime.network') }}</label>
            <select id="web3-runtime-network" v-if="runtimes.length" v-model="selectedRuntimeKey" class="input min-w-0 flex-1 text-sm">
              <option v-for="item in runtimes" :key="runtimeKey(item)" :value="runtimeKey(item)">{{ item.network_name || item.network_key }}</option>
            </select>
          </div>
          <div v-if="selectedRuntime" class="mt-2 text-xs text-gray-500">
            <div>{{ t('admin.web3Deposits.runtime.chainId', { id: selectedRuntime.chain_id || '—' }) }}</div>
            <div class="mt-1 flex items-center gap-1 whitespace-nowrap">
              <span>{{ t('admin.web3Deposits.runtime.tokenContract') }}</span>
              <code :title="selectedRuntime.token_contract || undefined">{{ abbreviatedContractAddress(selectedRuntime.token_contract) }}</code>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="text-sm text-gray-500">{{ t('admin.web3Deposits.stats.manualReview') }}</div>
          <div class="mt-1 text-2xl font-semibold">{{ stats.manual_review || 0 }}</div>
        </div>
        <div class="card p-4">
          <div class="text-sm text-gray-500">{{ t('admin.web3Deposits.stats.failed') }}</div>
          <div class="mt-1 text-2xl font-semibold">{{ stats.failed || 0 }}</div>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
        <div class="card p-3">
          <div class="text-xs text-gray-500">{{ t('admin.web3Deposits.runtime.scannerLagLabel') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ selectedRuntime?.scanner_lag_blocks || selectedRuntime?.lag_blocks || '0' }}</div>
        </div>
        <div class="card p-3">
          <div class="text-xs text-gray-500">{{ t('admin.web3Deposits.runtime.finalizerLagLabel') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ selectedRuntime?.finalizer_lag_blocks || '0' }}</div>
        </div>
        <div class="card p-3">
          <div class="text-xs text-gray-500">{{ t('admin.web3Deposits.runtime.latestBlock') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ selectedRuntime?.latest_block || '0' }}</div>
        </div>
        <div class="card p-3">
          <div class="text-xs text-gray-500">{{ t('admin.web3Deposits.runtime.scannedBlock') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ selectedRuntime?.scanned_block || '0' }}</div>
        </div>
        <div class="card p-3">
          <div class="text-xs text-gray-500">{{ t('admin.web3Deposits.runtime.finalizedBlock') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ selectedRuntime?.finalized_block || '0' }}</div>
        </div>
        <div class="card p-3">
          <div class="text-xs text-gray-500">{{ t('admin.web3Deposits.runtime.finalizedCursor') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ selectedRuntime?.finalized_cursor_block || '0' }}</div>
        </div>
      </div>

      <div class="card p-4">
        <div class="flex flex-wrap gap-2">
          <select v-model="status" class="input w-44">
            <option value="">{{ t('admin.web3Deposits.filters.allStatuses') }}</option>
            <option value="manual_review">{{ statusLabel('manual_review') }}</option>
            <option value="failed">{{ statusLabel('failed') }}</option>
            <option value="credited">{{ statusLabel('credited') }}</option>
            <option value="confirming">{{ statusLabel('confirming') }}</option>
          </select>
          <input
            v-model="keyword"
            class="input min-w-64 flex-1"
            :placeholder="t('admin.web3Deposits.filters.transactionOrAddress')"
          />
          <button class="btn btn-primary" @click="search">{{ t('admin.web3Deposits.filters.search') }}</button>
          <button class="btn btn-secondary" @click="showRescan = true">
            {{ t('admin.web3Deposits.filters.boundedRescan') }}
          </button>
        </div>
      </div>

      <div class="card overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead>
            <tr class="text-left text-xs uppercase text-gray-500">
              <th class="p-4">{{ t('admin.web3Deposits.columns.idUser') }}</th>
              <th class="p-4">{{ t('admin.web3Deposits.columns.amount') }}</th>
              <th class="p-4">{{ t('admin.web3Deposits.columns.status') }}</th>
              <th class="p-4">{{ t('admin.web3Deposits.columns.transaction') }}</th>
              <th class="p-4">{{ t('admin.web3Deposits.columns.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in items" :key="item.id">
              <td class="p-4 text-sm">
                #{{ item.id }}
                <div class="text-xs text-gray-500">{{ t('admin.web3Deposits.user', { id: item.user_id }) }}</div>
              </td>
              <td class="p-4 text-sm font-semibold">{{ item.token_amount }} {{ selectedRuntime?.asset_key.toUpperCase() || '' }}</td>
              <td class="p-4 text-sm">
                <span class="rounded-full bg-gray-100 px-2 py-1 dark:bg-dark-700">{{ statusLabel(item.status) }}</span>
                <div v-if="item.failure_reason || item.review_reason" class="mt-1 max-w-xs text-xs text-red-500">
                  {{ reasonLabel(item.failure_reason || item.review_reason || '') }}
                </div>
              </td>
              <td class="p-4 font-mono text-xs">{{ item.tx_hash.slice(0, 12) }}…</td>
              <td class="p-4">
                <div class="flex gap-2">
                  <button v-if="item.status === 'manual_review'" class="text-green-600" @click="approve(item)">
                    {{ t('admin.web3Deposits.actions.approve') }}
                  </button>
                  <button v-if="item.status === 'manual_review'" class="text-gray-600" @click="ignore(item)">
                    {{ t('admin.web3Deposits.actions.ignore') }}
                  </button>
                  <button v-if="item.status === 'failed'" class="text-primary-600" @click="retry(item)">
                    {{ t('admin.web3Deposits.actions.retry') }}
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <Pagination
        v-if="total"
        :page="page"
        :total="total"
        :page-size="pageSize"
        @update:page="nextPage => { page = nextPage; loadTargetData() }"
        @update:pageSize="nextPageSize => { pageSize = nextPageSize; page = 1; loadTargetData() }"
      />

      <div class="card overflow-x-auto">
        <div class="border-b border-gray-200 p-4 dark:border-dark-700">
          <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.web3Deposits.rescanJobs.title') }}</h2>
        </div>
        <div v-if="rescanJobs.length === 0" class="p-8 text-center text-sm text-gray-500">
          {{ t('admin.web3Deposits.rescanJobs.empty') }}
        </div>
        <table v-else class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead>
            <tr class="text-left text-xs uppercase text-gray-500">
              <th class="p-4">{{ t('admin.web3Deposits.rescanJobs.id') }}</th>
              <th class="p-4">{{ t('admin.web3Deposits.rescanJobs.target') }}</th>
              <th class="p-4">{{ t('admin.web3Deposits.rescanJobs.range') }}</th>
              <th class="p-4">{{ t('admin.web3Deposits.rescanJobs.status') }}</th>
              <th class="p-4">{{ t('admin.web3Deposits.rescanJobs.result') }}</th>
              <th class="p-4">{{ t('admin.web3Deposits.rescanJobs.createdAt') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 text-sm dark:divide-dark-700">
            <tr v-for="job in rescanJobs" :key="job.id">
              <td class="p-4 font-mono">#{{ job.id }}</td>
              <td class="p-4">{{ job.network_key }} / {{ job.asset_key }}</td>
              <td class="p-4 font-mono">{{ job.from_block }} – {{ job.to_block }}</td>
              <td class="p-4">{{ rescanJobStatusLabel(job.status) }}</td>
              <td class="p-4">
                <span v-if="job.status === 'succeeded'">{{ t('admin.web3Deposits.rescanJobs.counts', { events: job.event_count, matched: job.matched_count, deposits: job.deposit_count }) }}</span>
                <span v-else-if="job.error_message" class="text-red-600 dark:text-red-400">{{ job.error_message }}</span>
                <span v-else>—</span>
              </td>
              <td class="p-4 text-gray-600 dark:text-gray-300">{{ formatDateTime(job.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <BaseDialog
      :show="showRescan"
      :title="t('admin.web3Deposits.dialogs.rescanTitle')"
      @close="showRescan = false"
    >
      <div class="space-y-3">
        <select v-model="selectedRuntimeKey" class="input w-full">
          <option v-for="item in runtimes" :key="runtimeKey(item)" :value="runtimeKey(item)">{{ item.network_key }} / {{ item.asset_key }}</option>
        </select>
        <input v-model="fromBlock" class="input w-full" :placeholder="t('admin.web3Deposits.dialogs.fromBlock')" />
        <input v-model="toBlock" class="input w-full" :placeholder="t('admin.web3Deposits.dialogs.toBlock')" />
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showRescan = false">{{ t('admin.web3Deposits.dialogs.cancel') }}</button>
        <button class="btn btn-primary" @click="rescan">{{ t('admin.web3Deposits.dialogs.rescan') }}</button>
      </template>
    </BaseDialog>
    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import web3DepositsAPI, { type AdminWeb3Deposit, type Web3DepositRuntime, type Web3RescanJob } from '@/api/admin/web3Deposits'
import { useStepUp, isStepUpCancelled } from '@/composables/useStepUp'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

const statusKeys: Record<string, string> = {
  detected: 'detected',
  confirming: 'confirming',
  ready_to_credit: 'readyToCredit',
  crediting: 'crediting',
  credited: 'credited',
  below_minimum: 'belowMinimum',
  manual_review: 'manualReview',
  orphaned: 'orphaned',
  failed: 'failed',
  ignored: 'ignored',
}

const reasonKeys: Record<string, string> = {
  amount_above_auto_credit_limit: 'amountAboveAutoCreditLimit',
  amount_exceeds_platform_balance: 'amountExceedsPlatformBalance',
  user_missing: 'userMissing',
  user_deleted: 'userDeleted',
  user_inactive: 'userInactive',
  deposit_address_missing: 'depositAddressMissing',
  deposit_address_disabled: 'depositAddressDisabled',
  deposit_address_user_mismatch: 'depositAddressUserMismatch',
  deposit_address_mismatch: 'depositAddressMismatch',
  canonical_block_missing: 'canonicalBlockMissing',
  canonical_block_hash_mismatch: 'canonicalBlockHashMismatch',
  transaction_receipt_missing: 'transactionReceiptMissing',
  transaction_receipt_failed: 'transactionReceiptFailed',
  transaction_receipt_block_hash_mismatch: 'transactionReceiptBlockHashMismatch',
  transfer_log_missing: 'transferLogMissing',
  transfer_token_mismatch: 'transferTokenMismatch',
  transfer_destination_mismatch: 'transferDestinationMismatch',
  transfer_amount_mismatch: 'transferAmountMismatch',
}

const app = useAppStore()
const stepUp = useStepUp()
const { t } = useI18n()
const items = ref<AdminWeb3Deposit[]>([])
const stats = ref<Record<string, number>>({})
const runtimes = ref<Web3DepositRuntime[]>([])
const rescanJobs = ref<Web3RescanJob[]>([])
const selectedRuntimeKey = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const status = ref('')
const keyword = ref('')
const showRescan = ref(false)
const fromBlock = ref('')
const toBlock = ref('')
const selectedRuntime = computed(() => runtimes.value.find(item => runtimeKey(item) === selectedRuntimeKey.value) || runtimes.value[0])

function runtimeKey(item: Web3DepositRuntime) {
  return `${item.network_key}:${item.asset_key}`
}

function abbreviatedContractAddress(value: string) {
  if (!value) return '—'
  if (value.length <= 10) return value
  return `${value.slice(0, 6)}…${value.slice(-4)}`
}

function statusLabel(value: string) {
  const key = statusKeys[value]
  return key
    ? t(`admin.web3Deposits.statuses.${key}`)
    : t('admin.web3Deposits.statuses.unknown', { value: value || '—' })
}

function reasonLabel(reason: string) {
  const key = reasonKeys[reason]
  return key ? t(`admin.web3Deposits.reasons.${key}`) : reason
}

function rescanJobStatusLabel(status: Web3RescanJob['status']) {
  return t(`admin.web3Deposits.rescanJobs.statuses.${status}`)
}

async function load() {
  try {
    const state = await web3DepositsAPI.runtime()
    runtimes.value = state.data.runtimes
    if (!runtimes.value.some(item => runtimeKey(item) === selectedRuntimeKey.value)) {
      selectedRuntimeKey.value = runtimes.value[0] ? runtimeKey(runtimes.value[0]) : ''
    }
    await Promise.all([loadTargetData(), loadRescanJobs()])
  } catch (error) {
    app.showError(extractApiErrorMessage(error, t('admin.web3Deposits.messages.loadFailed')))
  }
}

function selectedTarget() {
  const target = selectedRuntime.value
  return target ? { network_key: target.network_key, asset_key: target.asset_key } : {}
}

async function loadTargetData() {
  const params: Record<string, unknown> = {
    page: page.value,
    page_size: pageSize.value,
    status: status.value,
    ...selectedTarget(),
  }
  if (keyword.value.startsWith('0x') && keyword.value.length === 66) {
    params.tx_hash = keyword.value
  } else if (keyword.value) {
    params.address = keyword.value
  }
  const [list, counts] = await Promise.all([
    web3DepositsAPI.list(params),
    web3DepositsAPI.stats(selectedTarget()),
  ])
  items.value = list.data.items
  total.value = list.data.total
  stats.value = counts.data
}

function search() {
  page.value = 1
  void loadTargetData().catch(error => {
    app.showError(extractApiErrorMessage(error, t('admin.web3Deposits.messages.loadFailed')))
  })
}

async function loadRescanJobs() {
  try {
    rescanJobs.value = (await web3DepositsAPI.listRescanJobs(20, selectedTarget())).data
  } catch (error) {
    app.showError(extractApiErrorMessage(error, t('admin.web3Deposits.messages.loadRescanJobsFailed')))
  }
}

async function run(action: () => Promise<unknown>) {
  try {
    await stepUp.run(action)
    app.showSuccess(t('admin.web3Deposits.messages.operationCompleted'))
    await Promise.all([loadTargetData(), loadRescanJobs()])
  } catch (error) {
    if (!isStepUpCancelled(error)) {
      app.showError(extractApiErrorMessage(error, t('admin.web3Deposits.messages.operationFailed')))
    }
  }
}

function approve(item: AdminWeb3Deposit) {
  if (window.confirm(t('admin.web3Deposits.dialogs.approveConfirm'))) {
    void run(() => web3DepositsAPI.approve(item.id))
  }
}

function ignore(item: AdminWeb3Deposit) {
  const reason = window.prompt(t('admin.web3Deposits.dialogs.ignorePrompt'))
  if (reason) {
    void run(() => web3DepositsAPI.ignore(item.id, reason))
  }
}

function retry(item: AdminWeb3Deposit) {
  if (window.confirm(t('admin.web3Deposits.dialogs.retryConfirm'))) {
    void run(() => web3DepositsAPI.retry(item.id))
  }
}

function rescan() {
  const target = selectedRuntime.value
  if (!target) return
  void run(() => web3DepositsAPI.rescan(target.network_key, target.asset_key, fromBlock.value, toBlock.value)).then(() => {
    showRescan.value = false
  })
}

watch(selectedRuntimeKey, (value, previous) => {
  if (!value || !previous || value === previous) return
  page.value = 1
  void Promise.all([loadTargetData(), loadRescanJobs()]).catch(error => {
    app.showError(extractApiErrorMessage(error, t('admin.web3Deposits.messages.loadFailed')))
  })
})

onMounted(load)
</script>
