<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <div class="card p-6">
        <div class="mb-5 flex justify-end"><button class="btn btn-secondary" @click="router.push('/web3-deposit/history')">{{ t('web3Deposit.history') }}</button></div>
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
              <p class="mt-4 text-center text-sm text-gray-600 dark:text-gray-300">{{ t('web3Deposit.createHint') }}</p>
              <button class="btn btn-primary mt-5" :disabled="creating || !config?.enabled" @click="createAddress">
                {{ creating ? t('web3Deposit.creating') : t('web3Deposit.createAddress') }}
              </button>
            </template>
          </div>

          <div class="flex-1 space-y-4">
            <div>
              <p class="text-xs font-medium uppercase tracking-wide text-gray-500">{{ t('web3Deposit.network') }}</p>
              <p class="mt-1 font-semibold text-gray-900 dark:text-white">{{ network?.display_name || 'Conflux eSpace' }}</p>
              <p class="text-sm text-gray-500">{{ t('web3Deposit.chainId', { id: network?.chain_id || '1030' }) }}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wide text-gray-500">{{ t('web3Deposit.token') }}</p>
              <p class="mt-1 font-semibold text-gray-900 dark:text-white">{{ asset?.display_name || 'USDT0' }}</p>
              <div class="mt-2 flex items-start gap-2">
                <code class="min-w-0 flex-1 break-all rounded-lg bg-gray-100 px-3 py-2 text-xs dark:bg-dark-800">{{ asset?.contract_address }}</code>
                <button class="btn btn-secondary" :disabled="!asset" @click="copy(asset?.contract_address || '')">{{ t('web3Deposit.copy') }}</button>
              </div>
            </div>
            <dl class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800"><dt class="text-gray-500">{{ t('web3Deposit.minimumDeposit') }}</dt><dd class="mt-1 font-semibold">{{ asset?.minimum_deposit || '1.000000' }} USDT0</dd></div>
              <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800"><dt class="text-gray-500">{{ t('web3Deposit.automaticCreditLimit') }}</dt><dd class="mt-1 font-semibold">{{ asset?.automatic_credit_limit || '10000.000000' }} USDT0</dd></div>
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
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import QRCode from 'qrcode'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { web3DepositAPI } from '@/api/web3Deposit'
import type { Web3DepositAddress, Web3DepositConfig } from '@/types/web3Deposit'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'

const appStore = useAppStore()
const { t } = useI18n()
const router = useRouter()
const { copyToClipboard: copy } = useClipboard()
const config = ref<Web3DepositConfig>()
const address = ref<Web3DepositAddress>()
const qrCode = ref('')
const loading = ref(true)
const creating = ref(false)
const network = computed(() => config.value?.networks[0])
const asset = computed(() => network.value?.assets[0])

async function renderQRCode(value?: string) {
  qrCode.value = value ? await QRCode.toDataURL(value, { width: 320, margin: 2 }) : ''
}

async function load() {
  loading.value = true
  try {
    const [configResponse, addressResponse] = await Promise.all([web3DepositAPI.getConfig(), web3DepositAPI.getAddress()])
    config.value = configResponse.data
    address.value = addressResponse.data
    await renderQRCode(address.value.address)
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', t('web3Deposit.errors.loadAddress')))
  } finally {
    loading.value = false
  }
}

async function createAddress() {
  creating.value = true
  try {
    const response = await web3DepositAPI.getOrCreateAddress()
    address.value = response.data
    await renderQRCode(address.value.address)
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'web3Deposit.errors', t('web3Deposit.errors.createAddress')))
  } finally {
    creating.value = false
  }
}

onMounted(load)
</script>
