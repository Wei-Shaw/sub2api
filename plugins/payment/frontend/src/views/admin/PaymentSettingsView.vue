<!--
  PaymentSettingsView -- standalone admin page mounted at /admin/payment/settings.

  Mirrors the release/custom-0.1.121 host SettingsView payment tab:
  a single global save button + inline form rows + provider list.
  Logic is extracted into usePaymentSettings and useProviderManagement composables.
-->
<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading state -->
      <div v-if="settings.loading.value" class="card p-6 text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>

      <template v-else>
        <!-- Main settings form -->
        <PaymentSettingsForm
          :form="settings.form"
          :loading="settings.loading.value"
          :saving="settings.saving.value"
          :all-payment-types="settings.allPaymentTypes.value"
          :balance-multiplier-preview="settings.balanceMultiplierPreview.value"
          :has-fee-rate="settings.hasFeeRate.value"
          :fee-rate-preview="settings.feeRatePreview.value"
          :load-balance-options="settings.loadBalanceOptions.value"
          :cancel-rate-limit-unit-options="settings.cancelRateLimitUnitOptions.value"
          :cancel-rate-limit-mode-options="settings.cancelRateLimitModeOptions.value"
          @save="settings.saveSettings"
          @toggle-payment-type="settings.togglePaymentType"
        />

        <!-- Provider management list -->
        <PaymentProviderList
          v-if="settings.form.payment_enabled"
          :providers="pm.providers.value"
          :loading="pm.providersLoading.value"
          :can-create="settings.hasAnyPaymentTypeEnabled.value"
          :enabled-payment-types="settings.form.payment_enabled_types"
          :all-payment-types="settings.allPaymentTypes.value"
          :redirect-label="t('payment.adminSettings.easypayRedirect')"
          @refresh="pm.loadProviders"
          @create="openCreateProvider"
          @edit="openEditProvider"
          @delete="confirmDeleteProvider"
          @toggle-field="pm.handleToggleField"
          @toggle-type="pm.handleToggleType"
          @reorder="pm.handleReorderProviders"
        />

        <!-- Save action #2: end of provider list -->
        <div v-if="settings.form.payment_enabled" class="flex justify-end">
          <button type="button" class="btn btn-primary" :disabled="settings.saving.value || settings.loading.value" @click="settings.saveSettings">
            {{ settings.saving.value ? t('common.saving') : t('common.save') }}
          </button>
        </div>

        <!-- Provider create / edit dialog -->
        <PaymentProviderDialog
          ref="providerDialogRef"
          :show="pm.showProviderDialog.value"
          :saving="pm.providerSaving.value"
          :editing="pm.editingProvider.value"
          :all-key-options="settings.providerKeyOptions.value"
          :enabled-key-options="settings.enabledProviderKeyOptions.value"
          :all-payment-types="settings.allPaymentTypes.value"
          :redirect-label="t('payment.adminSettings.easypayRedirect')"
          @close="onDialogClose"
          @save="pm.handleSaveProvider"
        />

        <!-- Delete confirmation -->
        <ConfirmDialog
          :show="pm.showDeleteProviderDialog.value"
          :title="t('payment.adminSettings.deleteProvider')"
          :message="t('payment.adminSettings.deleteProviderConfirm')"
          :confirm-text="t('common.delete')"
          danger
          @confirm="pm.handleDeleteProvider"
          @cancel="pm.showDeleteProviderDialog.value = false"
        />
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ConfirmDialog } from '@sub2api/plugin-sdk'

import { usePaymentSettings } from '../../composables/usePaymentSettings'
import { useProviderManagement } from '../../composables/useProviderManagement'
import type { ProviderInstance } from '../../types/payment'

import AppLayout from '../../components/common/AppLayout.vue'
import PaymentProviderList from '../../components/payment/PaymentProviderList.vue'
import PaymentProviderDialog from '../../components/payment/PaymentProviderDialog.vue'
import PaymentSettingsForm from '../../components/payment/settings/PaymentSettingsForm.vue'

const { t } = useI18n()

const settings = usePaymentSettings()
const pm = useProviderManagement()

const providerDialogRef = ref<InstanceType<typeof PaymentProviderDialog> | null>(null)

function openCreateProvider() {
  pm.editingProvider.value = null
  providerDialogRef.value?.reset(settings.enabledProviderKeyOptions.value[0]?.value || 'easypay')
  pm.showProviderDialog.value = true
}

function openEditProvider(provider: ProviderInstance) {
  pm.editingProvider.value = provider
  providerDialogRef.value?.loadProvider(provider)
  pm.showProviderDialog.value = true
}

function onDialogClose() {
  pm.showProviderDialog.value = false
  void pm.loadProviders()
}

function confirmDeleteProvider(provider: ProviderInstance) {
  pm.deletingProviderId.value = provider.id
  pm.showDeleteProviderDialog.value = true
}

// -- Lifecycle --
onMounted(async () => {
  settings.loading.value = true
  try {
    await Promise.all([settings.loadConfig(), pm.loadProviders()])
  } finally {
    settings.loading.value = false
  }
})
</script>