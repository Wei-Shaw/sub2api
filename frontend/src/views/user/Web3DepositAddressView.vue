<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <div class="card p-6">
        <div class="mb-5 flex flex-col gap-3 border-b border-gray-100 pb-5 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p class="text-xs font-medium uppercase tracking-wide text-gray-500">{{ t('web3Deposit.web3Balance') }}</p>
            <p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">
              <span v-if="balanceLoading">—</span>
              <template v-else>{{ formattedAvailableBalance }} {{ balanceAssetLabel }}</template>
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button class="btn btn-primary" :disabled="!canTransfer" @click="openTransfer">{{ t('web3Deposit.transferToMain') }}</button>
            <button class="btn btn-secondary" @click="openHistory">{{ t('web3Deposit.history') }}</button>
          </div>
        </div>
        <div v-if="unavailableMessage" role="alert" class="mb-5 rounded-xl border border-amber-200 bg-amber-50 p-4 text-amber-900 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-200">
          <p class="font-semibold">{{ t('web3Deposit.unavailable.title') }}</p>
          <p class="mt-1 text-sm">{{ unavailableMessage }}</p>
        </div>
        <div class="flex flex-col gap-6 lg:flex-row">
          <div class="flex flex-1 flex-col items-center justify-center rounded-2xl bg-gray-50 p-6 dark:bg-dark-800">
            <div v-if="loading" class="py-24 text-sm text-gray-500">{{ t('web3Deposit.loadingAddress') }}</div>
            <template v-else-if="address?.assigned && address.address">
              <img :src="qrCode" :alt="t('web3Deposit.title')" class="h-56 w-56 rounded-xl" />
              <p class="mt-4 break-all text-center font-mono text-sm text-gray-800 dark:text-gray-100">{{ address.address }}</p>
              <button class="btn btn-secondary mt-4" @click="copy(address.address)">{{ t('web3Deposit.copyAddress') }}</button>
            </template>
            <template v-else>
              <Icon name="creditCard" size="xl" class="text-primary-500" />
              <p class="mt-4 text-center text-sm text-gray-600 dark:text-gray-300">
                {{ unavailableMessage || t('web3Deposit.errors.loadAddress') }}
              </p>
            </template>
          </div>

          <div class="flex-1 space-y-4">
            <div>
              <p class="text-xs font-medium uppercase tracking-wide text-gray-500">{{ t('web3Deposit.network') }}</p>
              <select v-if="(config?.networks.length || 0) > 1" v-model="selectedNetworkKey" class="input mt-2 w-full">
                <option v-for="item in config?.networks" :key="item.key" :value="item.key">{{ item.display_name }}</option>
              </select>
              <p v-else class="mt-1 font-semibold text-gray-900 dark:text-white">{{ network?.display_name || '—' }}</p>
              <p class="mt-2 text-sm text-gray-500">{{ t('web3Deposit.chainId', { id: network?.chain_id || '—' }) }}</p>
            </div>
            <div>
              <div class="flex items-center gap-3">
                <p class="shrink-0 text-xs font-medium uppercase tracking-wide text-gray-500">{{ t('web3Deposit.token') }}</p>
                <select v-if="(network?.assets.length || 0) > 1" v-model="selectedAssetKey" class="input min-w-0 flex-1">
                  <option v-for="item in network?.assets" :key="item.key" :value="item.key">{{ item.display_name }}</option>
                </select>
                <p v-else class="min-w-0 font-semibold text-gray-900 dark:text-white">{{ asset?.display_name || '—' }}</p>
              </div>
              <div class="mt-2 flex items-start gap-2">
                <code class="min-w-0 flex-1 break-all rounded-lg bg-gray-100 px-3 py-2 text-xs dark:bg-dark-800">{{ asset?.contract_address }}</code>
                <button class="btn btn-secondary" :disabled="!asset" @click="copy(asset?.contract_address || '')">{{ t('web3Deposit.copy') }}</button>
              </div>
            </div>
            <dl class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800"><dt class="text-gray-500">{{ t('web3Deposit.minimumDeposit') }}</dt><dd class="mt-1 font-semibold">{{ asset?.minimum_deposit || '—' }} {{ asset?.display_name || '' }}</dd></div>
              <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800"><dt class="text-gray-500">{{ t('web3Deposit.automaticCreditLimit') }}</dt><dd class="mt-1 font-semibold">{{ asset?.automatic_credit_limit || '—' }} {{ asset?.display_name || '' }}</dd></div>
              <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800"><dt class="text-gray-500">{{ t('web3Deposit.feeRate') }}</dt><dd class="mt-1 font-semibold">{{ asset ? t('web3Deposit.feeRateValue', { rate: asset.fee_rate }) : '—' }}</dd></div>
              <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800"><dt class="text-gray-500">{{ t('web3Deposit.creditFinality') }}</dt><dd class="mt-1 font-semibold">{{ asset?.credit_finality || '—' }}</dd></div>
            </dl>
          </div>
        </div>
      </div>

      <div class="card border-amber-200 bg-amber-50 p-6 dark:border-amber-800/50 dark:bg-amber-900/20">
        <h2 class="font-semibold text-amber-900 dark:text-amber-200">{{ t('web3Deposit.safetyTitle') }}</h2>
        <ul class="mt-3 list-disc space-y-2 pl-5 text-sm text-amber-800 dark:text-amber-300">
          <li>{{ t('web3Deposit.safetyNetwork') }}</li>
          <li>{{ t('web3Deposit.safetyContract') }}</li>
          <li>{{ t('web3Deposit.safetyFinality') }}</li>
          <li>{{ t('web3Deposit.safetyRefund') }}</li>
        </ul>
      </div>
    </div>

    <BaseDialog :show="showTransfer" :title="t('web3Deposit.transferTitle')" width="narrow" @close="closeTransfer">
      <div class="space-y-4">
        <div>
          <p class="text-sm text-gray-500">{{ t('web3Deposit.availableWeb3Balance') }}</p>
          <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ formattedAvailableBalance }} {{ balanceAssetLabel }}</p>
        </div>
        <label class="block">
          <span class="input-label">{{ t('web3Deposit.transferAmount') }}</span>
          <div class="mt-1 flex gap-2">
            <input v-model.trim="transferAmount" class="input min-w-0 flex-1" type="number" min="0" step="0.00000001" inputmode="decimal" />
            <button class="btn btn-secondary" type="button" @click="fillAllBalance">{{ t('web3Deposit.transferAll') }}</button>
          </div>
        </label>
      </div>
      <template #footer>
        <button class="btn btn-secondary" :disabled="transferring" @click="closeTransfer">{{ t('web3Deposit.cancel') }}</button>
        <button class="btn btn-primary" :disabled="!validTransferAmount || transferring" @click="submitTransfer">
          {{ transferring ? t('web3Deposit.transferring') : t('web3Deposit.confirmTransfer') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import QRCode from 'qrcode'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { web3DepositAPI } from '@/api/web3Deposit'
import type { Web3DepositAddress, Web3DepositConfig, Web3UserBalance } from '@/types/web3Deposit'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractI18nErrorMessage } from '@/utils/apiError'

const appStore = useAppStore()
const authStore = useAuthStore()
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { copyToClipboard: copy } = useClipboard()
const config = ref<Web3DepositConfig>()
const address = ref<Web3DepositAddress>()
const qrCode = ref('')
const loading = ref(true)
const selectedNetworkKey = ref('')
const selectedAssetKey = ref('')
const selectionReady = ref(false)
const balances = ref<Web3UserBalance[]>([])
const balanceLoading = ref(true)
const showTransfer = ref(false)
const transferAmount = ref('')
const transferring = ref(false)
const network = computed(() => config.value?.networks.find(item => item.key === selectedNetworkKey.value) || config.value?.networks[0])
const asset = computed(() => network.value?.assets.find(item => item.key === selectedAssetKey.value) || network.value?.assets[0])
const balanceAssetKey = computed(() => asset.value?.balance_asset_key || 'usdt')
const currentBalance = computed(() => balances.value.find(item => item.asset_key === balanceAssetKey.value))
const balanceAssetLabel = computed(() => balanceAssetKey.value.toUpperCase())
const formattedAvailableBalance = computed(() => Number(currentBalance.value?.available_amount || 0).toFixed(4))
const availableBalanceNumber = computed(() => Number(currentBalance.value?.available_amount || 0))
const canTransfer = computed(() => !balanceLoading.value && availableBalanceNumber.value > 0)
const validTransferAmount = computed(() => {
  const amount = Number(transferAmount.value)
  return Number.isFinite(amount) && amount > 0 && amount <= availableBalanceNumber.value
})

watch(network, value => {
  if (!value?.assets.some(item => item.key === selectedAssetKey.value)) {
    selectedAssetKey.value = value?.assets[0]?.key || ''
  }
})

watch([selectedNetworkKey, selectedAssetKey], ([networkKey, assetKey]) => {
  if (!selectionReady.value || !networkKey || !assetKey) return
  void router.replace({ query: { ...route.query, network: networkKey, asset: assetKey } })
})
const unavailableMessage = computed(() => {
  if (!config.value || config.value.enabled) return ''
  switch (config.value.unavailable_reason) {
    case 'feature_disabled':
      return t('web3Deposit.unavailable.featureDisabled')
    case 'user_entry_disabled':
      return t('web3Deposit.unavailable.userEntryDisabled')
    case 'runtime_unhealthy':
      return t('web3Deposit.unavailable.runtimeUnhealthy')
    default:
      return t('web3Deposit.unavailable.default')
  }
})

async function renderQRCode(value?: string) {
  qrCode.value = value ? await QRCode.toDataURL(value, { width: 320, margin: 2 }) : ''
}

function openHistory() {
  void router.push({
    path: '/web3-deposit/history',
    query: { network: network.value?.key, asset: asset.value?.key },
  })
}

async function loadBalances() {
  balanceLoading.value = true
  try {
    balances.value = (await web3DepositAPI.listBalances()).data
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', t('web3Deposit.errors.loadBalance')))
  } finally {
    balanceLoading.value = false
  }
}

function openTransfer() {
  transferAmount.value = ''
  showTransfer.value = true
}

function closeTransfer() {
  if (transferring.value) return
  showTransfer.value = false
  transferAmount.value = ''
}

function fillAllBalance() {
  transferAmount.value = currentBalance.value?.available_amount || ''
}

async function submitTransfer() {
  if (!validTransferAmount.value || transferring.value) return
  transferring.value = true
  try {
    await web3DepositAPI.transferBalance(balanceAssetKey.value, transferAmount.value, crypto.randomUUID())
    await loadBalances()
    await authStore.refreshUser().catch(() => undefined)
    showTransfer.value = false
    transferAmount.value = ''
    appStore.showSuccess(t('web3Deposit.transferSuccess'))
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', t('web3Deposit.errors.transferFailed')))
  } finally {
    transferring.value = false
  }
}

async function load() {
  loading.value = true
  try {
    const configResponse = await web3DepositAPI.getConfig()
    config.value = configResponse.data
    const requestedNetwork = typeof route.query.network === 'string' ? route.query.network : ''
    selectedNetworkKey.value = config.value.networks.some(item => item.key === requestedNetwork)
      ? requestedNetwork
      : (config.value.networks[0]?.key || '')
    const requestedAsset = typeof route.query.asset === 'string' ? route.query.asset : ''
    selectedAssetKey.value = network.value?.assets.some(item => item.key === requestedAsset)
      ? requestedAsset
      : (network.value?.assets[0]?.key || '')
    await nextTick()
    selectionReady.value = true
    if (config.value.enabled) {
      const addressResponse = await web3DepositAPI.getOrCreateAddress()
      address.value = addressResponse.data
      await renderQRCode(address.value.address)
    }
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', t('web3Deposit.errors.loadAddress')))
  } finally {
    loading.value = false
  }
}

onMounted(() => Promise.all([load(), loadBalances()]))
</script>
