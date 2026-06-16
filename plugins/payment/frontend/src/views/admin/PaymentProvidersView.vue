<template>
  <AppLayout>
    <PaymentProviderList
      :providers="providers"
      :loading="loading"
      :can-create="hasAnyPaymentTypeEnabled"
      :enabled-payment-types="enabledPaymentTypes"
      :all-payment-types="allPaymentTypes"
      :redirect-label="t('payment.adminSettings.easypayRedirect')"
      @refresh="loadAll"
      @create="openCreateProvider"
      @edit="openEditProvider"
      @delete="confirmDeleteProvider"
      @toggle-field="handleToggleField"
      @toggle-type="handleToggleType"
      @reorder="handleReorderProviders"
    />

    <!-- Create / edit dialog -->
    <PaymentProviderDialog
      ref="providerDialogRef"
      :show="showProviderDialog"
      :saving="providerSaving"
      :editing="editingProvider"
      :all-key-options="providerKeyOptions"
      :enabled-key-options="enabledProviderKeyOptions"
      :all-payment-types="allPaymentTypes"
      :redirect-label="t('payment.adminSettings.easypayRedirect')"
      @close="onDialogClose"
      @save="handleSaveProvider"
    />

    <!-- Delete confirmation -->
    <ConfirmDialog
      :show="showDeleteProviderDialog"
      :title="t('payment.adminSettings.deleteProvider')"
      :message="t('payment.adminSettings.deleteProviderConfirm')"
      :confirm-text="t('common.delete')"
      danger
      @confirm="handleDeleteProvider"
      @cancel="showDeleteProviderDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ConfirmDialog } from '@sub2api/plugin-sdk'
import type { ProviderInstance } from '../../types/payment'
import AppLayout from '../../components/common/AppLayout.vue'
import PaymentProviderList from '../../components/payment/PaymentProviderList.vue'
import PaymentProviderDialog from '../../components/payment/PaymentProviderDialog.vue'
import { useProviderCrud } from '../../composables/useProviderCrud'

const { t } = useI18n()

const {
  loading, providers, enabledPaymentTypes,
  showProviderDialog, showDeleteProviderDialog, providerSaving, editingProvider,
  hasAnyPaymentTypeEnabled,
  loadAll, loadProviders,
  handleSaveProvider, handleToggleField, handleToggleType,
  handleReorderProviders, confirmDeleteProvider, handleDeleteProvider,
} = useProviderCrud()

const providerDialogRef = ref<InstanceType<typeof PaymentProviderDialog> | null>(null)

const allPaymentTypes = computed(() => [
  { value: 'easypay', label: t('payment.methods.easypay') },
  { value: 'alipay', label: t('payment.methods.alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay') },
  { value: 'stripe', label: t('payment.methods.stripe') },
  { value: 'airwallex', label: t('payment.methods.airwallex') },
])

const providerKeyOptions = computed(() => [
  { value: 'easypay', label: t('payment.adminSettings.providerEasypay') },
  { value: 'alipay', label: t('payment.adminSettings.providerAlipay') },
  { value: 'wxpay', label: t('payment.adminSettings.providerWxpay') },
  { value: 'stripe', label: t('payment.adminSettings.providerStripe') },
  { value: 'airwallex', label: t('payment.adminSettings.providerAirwallex') },
])

const enabledProviderKeyOptions = computed(() =>
  providerKeyOptions.value.filter(opt => enabledPaymentTypes.value.includes(opt.value)),
)

function openCreateProvider() {
  editingProvider.value = null
  providerDialogRef.value?.reset(enabledProviderKeyOptions.value[0]?.value || 'easypay')
  showProviderDialog.value = true
}

function openEditProvider(provider: ProviderInstance) {
  editingProvider.value = provider
  providerDialogRef.value?.loadProvider(provider)
  showProviderDialog.value = true
}

function onDialogClose() {
  showProviderDialog.value = false
  void loadProviders()
}
</script>
