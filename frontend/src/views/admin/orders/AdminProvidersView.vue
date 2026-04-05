<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.providerManagement') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.providerManagementDesc') }}</p>
        </div>
        <div class="flex items-center gap-2">
          <button @click="loadProviders" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button @click="openCreateDialog" class="btn btn-primary">{{ t('payment.admin.createProvider') }}</button>
        </div>
      </div>

      <!-- Loading -->
      <div v-if="loading && !providers.length" class="flex items-center justify-center py-12"><LoadingSpinner /></div>

      <!-- Provider Cards -->
      <div v-else-if="providers.length" class="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <div v-for="provider in providers" :key="provider.id" class="card">
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
            <div class="flex items-center gap-1">
              <span :class="['badge', provider.enabled ? 'badge-success' : 'badge-secondary']">
                {{ provider.enabled ? t('common.enabled') : t('common.disabled') }}
              </span>
            </div>
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
      <div v-else class="card p-12 text-center">
        <Icon name="server" size="xl" class="mx-auto mb-4 text-gray-300 dark:text-dark-600" />
        <h3 class="text-lg font-medium text-gray-900 dark:text-white">{{ t('payment.admin.noProviders') }}</h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.noProvidersHint') }}</p>
        <button @click="openCreateDialog" class="btn btn-primary mt-4">{{ t('payment.admin.createProvider') }}</button>
      </div>
    </div>

    <!-- Create/Edit Dialog -->
    <BaseDialog :show="showDialog" :title="editingProvider ? t('payment.admin.editProvider') : t('payment.admin.createProvider')" width="wide" @close="showDialog = false">
      <form id="provider-form" @submit.prevent="handleSave" class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('payment.admin.providerName') }}</label>
            <input v-model="form.name" type="text" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.providerKey') }}</label>
            <Select v-model="form.provider_key" :options="providerKeyOptions" :disabled="!!editingProvider" />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.supportedTypes') }}</label>
          <input v-model="form.supported_types" type="text" class="input" placeholder="alipay,wxpay" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.supportedTypesHint') }}</p>
        </div>
        <div class="flex items-center gap-4">
          <div class="flex items-center gap-2">
            <input id="prov-enabled" v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            <label for="prov-enabled" class="text-sm text-gray-700 dark:text-gray-300">{{ t('common.enabled') }}</label>
          </div>
          <div class="flex items-center gap-2">
            <input id="prov-refund" v-model="form.refund_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            <label for="prov-refund" class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.admin.refundEnabled') }}</label>
          </div>
        </div>

        <!-- Dynamic Config Fields -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.providerConfig') }}</h4>

          <!-- EasyPay Fields -->
          <template v-if="form.provider_key === 'easypay'">
            <div class="grid grid-cols-2 gap-4">
              <div><label class="input-label">PID</label><input v-model="config.pid" type="text" class="input" /></div>
              <div><label class="input-label">PKey</label><input v-model="config.pkey" type="password" class="input" /></div>
            </div>
            <div class="mt-3"><label class="input-label">API Base URL</label><input v-model="config.apiBase" type="url" class="input" /></div>
            <div class="mt-3 grid grid-cols-2 gap-4">
              <div><label class="input-label">Notify URL</label><input v-model="config.notifyUrl" type="url" class="input" /></div>
              <div><label class="input-label">Return URL</label><input v-model="config.returnUrl" type="url" class="input" /></div>
            </div>
            <div class="mt-3 grid grid-cols-3 gap-4">
              <div><label class="input-label">CID</label><input v-model="config.cid" type="text" class="input" /></div>
              <div><label class="input-label">CID Alipay</label><input v-model="config.cidAlipay" type="text" class="input" /></div>
              <div><label class="input-label">CID Wxpay</label><input v-model="config.cidWxpay" type="text" class="input" /></div>
            </div>
          </template>

          <!-- Alipay Fields -->
          <template v-else-if="form.provider_key === 'alipay'">
            <div><label class="input-label">App ID</label><input v-model="config.appId" type="text" class="input" /></div>
            <div class="mt-3"><label class="input-label">Private Key</label><textarea v-model="config.privateKey" rows="3" class="input font-mono text-xs"></textarea></div>
            <div class="mt-3"><label class="input-label">Public Key</label><textarea v-model="config.publicKey" rows="3" class="input font-mono text-xs"></textarea></div>
            <div class="mt-3 grid grid-cols-2 gap-4">
              <div><label class="input-label">Notify URL</label><input v-model="config.notifyUrl" type="url" class="input" /></div>
              <div><label class="input-label">Return URL</label><input v-model="config.returnUrl" type="url" class="input" /></div>
            </div>
          </template>

          <!-- Wxpay Fields -->
          <template v-else-if="form.provider_key === 'wxpay'">
            <div class="grid grid-cols-2 gap-4">
              <div><label class="input-label">App ID</label><input v-model="config.appId" type="text" class="input" /></div>
              <div><label class="input-label">Merchant ID</label><input v-model="config.mchId" type="text" class="input" /></div>
            </div>
            <div class="mt-3"><label class="input-label">Private Key</label><textarea v-model="config.privateKey" rows="3" class="input font-mono text-xs"></textarea></div>
            <div class="mt-3"><label class="input-label">API V3 Key</label><input v-model="config.apiV3Key" type="password" class="input" /></div>
            <div class="mt-3"><label class="input-label">Public Key</label><textarea v-model="config.publicKey" rows="2" class="input font-mono text-xs"></textarea></div>
            <div class="mt-3 grid grid-cols-2 gap-4">
              <div><label class="input-label">Public Key ID</label><input v-model="config.publicKeyId" type="text" class="input" /></div>
              <div><label class="input-label">Cert Serial</label><input v-model="config.certSerial" type="text" class="input" /></div>
            </div>
            <div class="mt-3"><label class="input-label">Notify URL</label><input v-model="config.notifyUrl" type="url" class="input" /></div>
          </template>

          <!-- Stripe Fields -->
          <template v-else-if="form.provider_key === 'stripe'">
            <div><label class="input-label">Secret Key</label><input v-model="config.secretKey" type="password" class="input" /></div>
            <div class="mt-3"><label class="input-label">Publishable Key</label><input v-model="config.publishableKey" type="text" class="input" /></div>
            <div class="mt-3"><label class="input-label">Webhook Secret</label><input v-model="config.webhookSecret" type="password" class="input" /></div>
          </template>

          <div v-else class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.selectProviderKey') }}</div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="showDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="submit" form="provider-form" :disabled="saving" class="btn btn-primary">{{ saving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="showDeleteDialog" :title="t('payment.admin.deleteProvider')" :message="t('payment.admin.deleteProviderConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDelete" @cancel="showDeleteDialog = false" />
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
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const providers = ref<ProviderInstance[]>([])
const showDialog = ref(false)
const showDeleteDialog = ref(false)
const editingProvider = ref<ProviderInstance | null>(null)
const deletingProviderId = ref<number | null>(null)

const form = reactive({
  name: '',
  provider_key: 'easypay',
  supported_types: '',
  enabled: true,
  refund_enabled: false,
})

const config = reactive({
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
  loading.value = true
  try {
    const res = await adminPaymentAPI.getProviders()
    providers.value = res.data || []
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  } finally { loading.value = false }
}

function resetForm() {
  form.name = ''; form.provider_key = 'easypay'; form.supported_types = ''
  form.enabled = true; form.refund_enabled = false
  Object.keys(config).forEach(k => { (config as any)[k] = '' })
}

function openCreateDialog() {
  editingProvider.value = null
  resetForm()
  showDialog.value = true
}

function openEditDialog(provider: ProviderInstance) {
  editingProvider.value = provider
  form.name = provider.name
  form.provider_key = provider.provider_key
  form.supported_types = provider.supported_types
  form.enabled = provider.enabled
  form.refund_enabled = provider.refund_enabled
  Object.keys(config).forEach(k => { (config as any)[k] = '' })
  showDialog.value = true
}

async function handleSave() {
  saving.value = true
  try {
    const data = { ...form, config: { ...config } } as any
    if (editingProvider.value) {
      await adminPaymentAPI.updateProvider(editingProvider.value.id, data)
    } else {
      await adminPaymentAPI.createProvider(data)
    }
    appStore.showSuccess(t('common.saved'))
    showDialog.value = false
    loadProviders()
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  } finally { saving.value = false }
}

function confirmDelete(provider: ProviderInstance) {
  deletingProviderId.value = provider.id
  showDeleteDialog.value = true
}

async function handleDelete() {
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

onMounted(() => { loadProviders() })
</script>
