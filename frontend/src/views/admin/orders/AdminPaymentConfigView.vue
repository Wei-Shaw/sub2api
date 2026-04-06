<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.paymentConfigTitle') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.paymentConfigDesc') }}</p>
        </div>
      </div>

      <!-- Basic Settings Card -->
      <div class="card">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.payment.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.description') }}</p>
        </div>
        <div class="space-y-6 p-6">
          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.settings.payment.enabled') }}</label>
              <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.enabledHint') }}</p>
            </div>
            <Toggle v-model="configForm.enabled" />
          </div>
          <template v-if="configForm.enabled">
            <div class="grid grid-cols-2 gap-4">
              <div><label class="input-label">{{ t('admin.settings.payment.minAmount') }}</label><input v-model.number="configForm.min_amount" type="number" step="0.01" min="0" class="input" /></div>
              <div><label class="input-label">{{ t('admin.settings.payment.maxAmount') }}</label><input v-model.number="configForm.max_amount" type="number" step="0.01" min="0" class="input" /></div>
            </div>
            <div class="grid grid-cols-2 gap-4">
              <div><label class="input-label">{{ t('admin.settings.payment.dailyLimit') }}</label><input v-model.number="configForm.daily_limit" type="number" step="0.01" min="0" class="input" /></div>
              <div><label class="input-label">{{ t('admin.settings.payment.maxPendingOrders') }}</label><input v-model.number="configForm.max_pending_orders" type="number" min="1" class="input" /></div>
            </div>
            <div class="grid grid-cols-2 gap-4">
              <div><label class="input-label">{{ t('admin.settings.payment.orderTimeout') }}</label><input v-model.number="configForm.order_timeout_minutes" type="number" min="1" class="input" /></div>
              <div class="flex items-center gap-2 pt-6">
                <input id="balance-disabled" v-model="configForm.balance_disabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                <label for="balance-disabled" class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.settings.payment.balancePaymentDisabled') }}</label>
              </div>
            </div>
            <div>
              <label class="input-label">{{ t('admin.settings.payment.enabledPaymentTypes') }}</label>
              <input v-model="configForm.enabled_types" type="text" class="input" placeholder="alipay,wxpay,stripe" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.enabledPaymentTypesHint') }}</p>
            </div>
          </template>
          <div class="flex justify-end">
            <button @click="saveConfig" :disabled="configSaving" class="btn btn-primary">
              {{ configSaving ? t('common.saving') : t('common.save') }}
            </button>
          </div>
        </div>
      </div>

      <!-- Provider Management Card -->
      <div v-if="configForm.enabled" class="card">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <div class="flex items-center justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.providerManagement') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.providerManagementDesc') }}</p>
            </div>
            <div class="flex items-center gap-2">
              <button @click="loadProviders" :disabled="providersLoading" class="btn btn-secondary" :title="t('common.refresh')">
                <Icon name="refresh" size="md" :class="providersLoading ? 'animate-spin' : ''" />
              </button>
              <button @click="openCreateDialog" class="btn btn-primary">{{ t('payment.admin.createProvider') }}</button>
            </div>
          </div>
        </div>
        <div class="p-6">
          <!-- Loading -->
          <div v-if="providersLoading && !providers.length" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
          <!-- Provider Cards -->
          <div v-else-if="providers.length" class="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <div v-for="provider in providers" :key="provider.id" class="rounded-lg border border-gray-200 dark:border-dark-600">
              <div class="flex items-start justify-between p-4">
                <div class="flex items-center gap-3">
                  <div :class="['rounded-lg p-2', provider.enabled ? 'bg-green-100 dark:bg-green-900/30' : 'bg-gray-100 dark:bg-dark-700']">
                    <Icon name="server" size="md" :class="provider.enabled ? 'text-green-600 dark:text-green-400' : 'text-gray-400'" :stroke-width="2" />
                  </div>
                  <div>
                    <h3 class="font-medium text-gray-900 dark:text-white">{{ provider.name }}</h3>
                    <p class="text-sm text-gray-500 dark:text-gray-400">{{ providerKeyLabel(provider.provider_key) }}</p>
                  </div>
                </div>
                <span :class="['badge', provider.enabled ? 'badge-success' : 'badge-secondary']">
                  {{ provider.enabled ? t('common.enabled') : t('common.disabled') }}
                </span>
              </div>
              <div class="border-t border-gray-100 px-4 py-3 dark:border-dark-700">
                <div class="flex flex-wrap gap-2 text-xs">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.admin.supportedTypes') }}:</span>
                  <span class="font-medium text-gray-700 dark:text-gray-300">{{ provider.supported_types || '-' }}</span>
                </div>
                <div class="mt-1 flex items-center gap-2 text-xs">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundEnabled') }}:</span>
                  <span :class="provider.refund_enabled ? 'text-green-600' : 'text-gray-400'">{{ provider.refund_enabled ? 'Yes' : 'No' }}</span>
                </div>
              </div>
              <div class="flex items-center justify-end gap-2 border-t border-gray-100 px-4 py-3 dark:border-dark-700">
                <button @click="openEditDialog(provider)" class="btn btn-secondary btn-sm">{{ t('common.edit') }}</button>
                <button @click="confirmDelete(provider)" class="btn btn-sm rounded-md bg-red-50 px-3 py-1.5 text-sm text-red-600 hover:bg-red-100 dark:bg-red-900/20 dark:text-red-400 dark:hover:bg-red-900/30">{{ t('common.delete') }}</button>
              </div>
            </div>
          </div>
          <!-- Empty State -->
          <div v-else class="py-12 text-center">
            <Icon name="server" size="xl" class="mx-auto mb-4 text-gray-300 dark:text-dark-600" />
            <h3 class="text-lg font-medium text-gray-900 dark:text-white">{{ t('payment.admin.noProviders') }}</h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.noProvidersHint') }}</p>
            <button @click="openCreateDialog" class="btn btn-primary mt-4">{{ t('payment.admin.createProvider') }}</button>
          </div>
        </div>
      </div>
    </div>

    <!-- Create/Edit Provider Dialog -->
    <BaseDialog :show="showProviderDialog" :title="editingProvider ? t('payment.admin.editProvider') : t('payment.admin.createProvider')" width="wide" @close="showProviderDialog = false">
      <form id="provider-form" @submit.prevent="handleSaveProvider" class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('payment.admin.providerName') }}</label>
            <input v-model="providerForm.name" type="text" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.providerKey') }}</label>
            <Select v-model="providerForm.provider_key" :options="providerKeyOptions" :disabled="!!editingProvider" />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.supportedTypes') }}</label>
          <input v-model="providerForm.supported_types" type="text" class="input" placeholder="alipay,wxpay" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.supportedTypesHint') }}</p>
        </div>
        <div class="flex items-center gap-4">
          <div class="flex items-center gap-2">
            <input id="prov-enabled" v-model="providerForm.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            <label for="prov-enabled" class="text-sm text-gray-700 dark:text-gray-300">{{ t('common.enabled') }}</label>
          </div>
          <div class="flex items-center gap-2">
            <input id="prov-refund" v-model="providerForm.refund_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            <label for="prov-refund" class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.admin.refundEnabled') }}</label>
          </div>
        </div>

        <!-- Dynamic Config Fields -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.providerConfig') }}</h4>

          <!-- EasyPay -->
          <template v-if="providerForm.provider_key === 'easypay'">
            <div class="grid grid-cols-2 gap-4">
              <div><label class="input-label">PID</label><input v-model="providerConfig.pid" type="text" class="input" /></div>
              <div><label class="input-label">PKey</label><input v-model="providerConfig.pkey" type="password" class="input" /></div>
            </div>
            <div class="mt-3"><label class="input-label">API Base URL</label><input v-model="providerConfig.apiBase" type="url" class="input" /></div>
            <div class="mt-3 grid grid-cols-2 gap-4">
              <div><label class="input-label">Notify URL</label><input v-model="providerConfig.notifyUrl" type="url" class="input" /></div>
              <div><label class="input-label">Return URL</label><input v-model="providerConfig.returnUrl" type="url" class="input" /></div>
            </div>
            <div class="mt-3 grid grid-cols-3 gap-4">
              <div><label class="input-label">CID</label><input v-model="providerConfig.cid" type="text" class="input" /></div>
              <div><label class="input-label">CID Alipay</label><input v-model="providerConfig.cidAlipay" type="text" class="input" /></div>
              <div><label class="input-label">CID Wxpay</label><input v-model="providerConfig.cidWxpay" type="text" class="input" /></div>
            </div>
          </template>

          <!-- Alipay -->
          <template v-else-if="providerForm.provider_key === 'alipay'">
            <div><label class="input-label">App ID</label><input v-model="providerConfig.appId" type="text" class="input" /></div>
            <div class="mt-3"><label class="input-label">Private Key</label><textarea v-model="providerConfig.privateKey" rows="3" class="input font-mono text-xs"></textarea></div>
            <div class="mt-3"><label class="input-label">Public Key</label><textarea v-model="providerConfig.publicKey" rows="3" class="input font-mono text-xs"></textarea></div>
            <div class="mt-3 grid grid-cols-2 gap-4">
              <div><label class="input-label">Notify URL</label><input v-model="providerConfig.notifyUrl" type="url" class="input" /></div>
              <div><label class="input-label">Return URL</label><input v-model="providerConfig.returnUrl" type="url" class="input" /></div>
            </div>
          </template>

          <!-- Wxpay -->
          <template v-else-if="providerForm.provider_key === 'wxpay'">
            <div class="grid grid-cols-2 gap-4">
              <div><label class="input-label">App ID</label><input v-model="providerConfig.appId" type="text" class="input" /></div>
              <div><label class="input-label">Merchant ID</label><input v-model="providerConfig.mchId" type="text" class="input" /></div>
            </div>
            <div class="mt-3"><label class="input-label">Private Key</label><textarea v-model="providerConfig.privateKey" rows="3" class="input font-mono text-xs"></textarea></div>
            <div class="mt-3"><label class="input-label">API V3 Key</label><input v-model="providerConfig.apiV3Key" type="password" class="input" /></div>
            <div class="mt-3"><label class="input-label">Public Key</label><textarea v-model="providerConfig.publicKey" rows="2" class="input font-mono text-xs"></textarea></div>
            <div class="mt-3 grid grid-cols-2 gap-4">
              <div><label class="input-label">Public Key ID</label><input v-model="providerConfig.publicKeyId" type="text" class="input" /></div>
              <div><label class="input-label">Cert Serial</label><input v-model="providerConfig.certSerial" type="text" class="input" /></div>
            </div>
            <div class="mt-3"><label class="input-label">Notify URL</label><input v-model="providerConfig.notifyUrl" type="url" class="input" /></div>
          </template>

          <!-- Stripe -->
          <template v-else-if="providerForm.provider_key === 'stripe'">
            <div><label class="input-label">Secret Key</label><input v-model="providerConfig.secretKey" type="password" class="input" /></div>
            <div class="mt-3"><label class="input-label">Publishable Key</label><input v-model="providerConfig.publishableKey" type="text" class="input" /></div>
            <div class="mt-3"><label class="input-label">Webhook Secret</label><input v-model="providerConfig.webhookSecret" type="password" class="input" /></div>
          </template>

          <div v-else class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.selectProviderKey') }}</div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="showProviderDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="submit" form="provider-form" :disabled="providerSaving" class="btn btn-primary">{{ providerSaving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="showDeleteDialog" :title="t('payment.admin.deleteProvider')" :message="t('payment.admin.deleteProviderConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDeleteProvider" @cancel="showDeleteDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { ProviderInstance } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== Basic Config ====================

const configSaving = ref(false)
const configForm = reactive({
  enabled: false,
  min_amount: 1,
  max_amount: 10000,
  daily_limit: 50000,
  max_pending_orders: 3,
  order_timeout_minutes: 30,
  balance_disabled: false,
  enabled_types: '',
})

async function loadConfig() {
  try {
    const res = await adminPaymentAPI.getConfig()
    const cfg = res.data
    configForm.enabled = cfg.enabled
    configForm.min_amount = cfg.min_amount
    configForm.max_amount = cfg.max_amount
    configForm.daily_limit = cfg.daily_limit
    configForm.max_pending_orders = cfg.max_pending_orders
    configForm.order_timeout_minutes = cfg.order_timeout_minutes
    configForm.balance_disabled = cfg.balance_disabled
    configForm.enabled_types = (cfg.enabled_payment_types || []).join(',')
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  }
}

async function saveConfig() {
  configSaving.value = true
  try {
    const enabledTypes = configForm.enabled_types
      ? configForm.enabled_types.split(',').map((s: string) => s.trim()).filter(Boolean)
      : []
    await adminPaymentAPI.updateConfig({
      enabled: configForm.enabled,
      min_amount: configForm.min_amount,
      max_amount: configForm.max_amount,
      daily_limit: configForm.daily_limit,
      max_pending_orders: configForm.max_pending_orders,
      order_timeout_minutes: configForm.order_timeout_minutes,
      balance_disabled: configForm.balance_disabled,
      enabled_payment_types: enabledTypes,
    })
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  } finally {
    configSaving.value = false
  }
}

// ==================== Provider Management ====================

const providersLoading = ref(false)
const providerSaving = ref(false)
const providers = ref<ProviderInstance[]>([])
const showProviderDialog = ref(false)
const showDeleteDialog = ref(false)
const editingProvider = ref<ProviderInstance | null>(null)
const deletingProviderId = ref<number | null>(null)

const providerForm = reactive({
  name: '',
  provider_key: 'easypay',
  supported_types: '',
  enabled: true,
  refund_enabled: false,
})

const providerConfig = reactive({
  pid: '', pkey: '', apiBase: '', notifyUrl: '', returnUrl: '',
  cid: '', cidAlipay: '', cidWxpay: '',
  appId: '', privateKey: '', publicKey: '',
  mchId: '', apiV3Key: '', publicKeyId: '', certSerial: '',
  secretKey: '', publishableKey: '', webhookSecret: '',
})

const providerKeyOptions = [
  { value: 'easypay', label: 'EasyPay' },
  { value: 'alipay', label: 'Alipay (Direct)' },
  { value: 'wxpay', label: 'WeChat Pay (Direct)' },
  { value: 'stripe', label: 'Stripe' },
]

function providerKeyLabel(key: string): string {
  const opt = providerKeyOptions.find(o => o.value === key)
  return opt ? opt.label : key
}

async function loadProviders() {
  providersLoading.value = true
  try {
    const res = await adminPaymentAPI.getProviders()
    providers.value = res.data || []
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  } finally { providersLoading.value = false }
}

function resetProviderForm() {
  providerForm.name = ''; providerForm.provider_key = 'easypay'; providerForm.supported_types = ''
  providerForm.enabled = true; providerForm.refund_enabled = false
  Object.keys(providerConfig).forEach(k => { (providerConfig as any)[k] = '' })
}

function openCreateDialog() {
  editingProvider.value = null
  resetProviderForm()
  showProviderDialog.value = true
}

function openEditDialog(provider: ProviderInstance) {
  editingProvider.value = provider
  providerForm.name = provider.name
  providerForm.provider_key = provider.provider_key
  providerForm.supported_types = provider.supported_types
  providerForm.enabled = provider.enabled
  providerForm.refund_enabled = provider.refund_enabled
  Object.keys(providerConfig).forEach(k => { (providerConfig as any)[k] = '' })
  showProviderDialog.value = true
}

async function handleSaveProvider() {
  providerSaving.value = true
  try {
    const data = { ...providerForm, config: { ...providerConfig } } as any
    if (editingProvider.value) {
      await adminPaymentAPI.updateProvider(editingProvider.value.id, data)
    } else {
      await adminPaymentAPI.createProvider(data)
    }
    appStore.showSuccess(t('common.saved'))
    showProviderDialog.value = false
    loadProviders()
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  } finally { providerSaving.value = false }
}

function confirmDelete(provider: ProviderInstance) {
  deletingProviderId.value = provider.id
  showDeleteDialog.value = true
}

async function handleDeleteProvider() {
  if (!deletingProviderId.value) return
  try {
    await adminPaymentAPI.deleteProvider(deletingProviderId.value)
    appStore.showSuccess(t('common.deleted'))
    showDeleteDialog.value = false
    loadProviders()
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  }
}

onMounted(() => {
  loadConfig()
  loadProviders()
})
</script>
