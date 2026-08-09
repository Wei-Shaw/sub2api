<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="grid gap-3 sm:grid-cols-3">
        <div class="card p-4">
          <div class="text-sm text-gray-500">{{ t('admin.web3Deposits.runtime.title') }}</div>
          <div class="mt-1 font-semibold">
            {{ runtimeStateLabel }}
          </div>
          <div class="mt-2 grid gap-1 text-xs text-gray-500">
            <div>{{ t('admin.web3Deposits.runtime.scannerLag', { blocks: selectedRuntime?.scanner_lag_blocks || selectedRuntime?.lag_blocks || '0' }) }}</div>
            <div>{{ t('admin.web3Deposits.runtime.finalizerLag', { blocks: selectedRuntime?.finalizer_lag_blocks || '0' }) }}</div>
            <div>{{ t('admin.web3Deposits.runtime.heights', { latest: selectedRuntime?.latest_block || '0', scanned: selectedRuntime?.scanned_block || '0', finalized: selectedRuntime?.finalized_block || '0', finalizedCursor: selectedRuntime?.finalized_cursor_block || '0' }) }}</div>
          </div>
          <select v-if="runtimes.length > 1" v-model="selectedRuntimeKey" class="input mt-2 w-full text-sm">
            <option v-for="item in runtimes" :key="runtimeKey(item)" :value="runtimeKey(item)">{{ item.network_key }} / {{ item.asset_key }}</option>
          </select>
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
          <button class="btn btn-primary" @click="load">{{ t('admin.web3Deposits.filters.search') }}</button>
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
              <td class="p-4 text-sm font-semibold">{{ item.token_amount }} USDT0</td>
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
        @update:page="nextPage => { page = nextPage; load() }"
        @update:pageSize="nextPageSize => { pageSize = nextPageSize; page = 1; load() }"
      />
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
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import web3DepositsAPI, { type AdminWeb3Deposit, type Web3DepositRuntime } from '@/api/admin/web3Deposits'
import { useStepUp, isStepUpCancelled } from '@/composables/useStepUp'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

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

const runtimeStateKeys: Record<string, string> = {
  disabled: 'disabled',
  standby: 'standby',
  leader: 'leader',
  unhealthy: 'unhealthy',
  stopped: 'stopped',
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
const runtimeStateLabel = computed(() => {
  const state = selectedRuntime.value?.state || ''
  const key = runtimeStateKeys[state]
  return key
    ? t(`admin.web3Deposits.runtimeStates.${key}`)
    : t('admin.web3Deposits.runtimeStates.unknown', { value: state || '—' })
})

function runtimeKey(item: Web3DepositRuntime) {
  return `${item.network_key}:${item.asset_key}`
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

async function load() {
  try {
    const params: Record<string, unknown> = {
      page: page.value,
      page_size: pageSize.value,
      status: status.value,
    }
    if (keyword.value.startsWith('0x') && keyword.value.length === 66) {
      params.tx_hash = keyword.value
    } else if (keyword.value) {
      params.address = keyword.value
    }
    const [list, counts, state] = await Promise.all([
      web3DepositsAPI.list(params),
      web3DepositsAPI.stats(),
      web3DepositsAPI.runtime(),
    ])
    items.value = list.data.items
    total.value = list.data.total
    stats.value = counts.data
    runtimes.value = state.data.runtimes
    if (!runtimes.value.some(item => runtimeKey(item) === selectedRuntimeKey.value)) {
      selectedRuntimeKey.value = runtimes.value[0] ? runtimeKey(runtimes.value[0]) : ''
    }
  } catch (error) {
    app.showError(extractApiErrorMessage(error, t('admin.web3Deposits.messages.loadFailed')))
  }
}

async function run(action: () => Promise<unknown>) {
  try {
    await stepUp.run(action)
    app.showSuccess(t('admin.web3Deposits.messages.operationCompleted'))
    await load()
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

onMounted(load)
</script>
