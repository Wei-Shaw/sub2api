<template>
  <BaseDialog
    :show="show"
    :title="editing ? t('payment.adminSettings.editProvider') : t('payment.adminSettings.createProvider')"
    width="wide"
    @close="emit('close')"
  >
    <form id="provider-form" @submit.prevent="handleSave" class="space-y-4">
      <!-- Name + Key -->
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">
            {{ t('payment.adminSettings.providerName') }}
            <span class="input-required">*</span>
          </label>
          <input v-model="form.name" type="text" class="input" required />
        </div>
        <div>
          <label class="input-label">
            {{ t('payment.adminSettings.providerKey') }}
            <span class="input-required">*</span>
          </label>
          <Select
            v-model="form.provider_key"
            :options="(!!editing ? allKeyOptions : enabledKeyOptions) as SelectOption[]"
            :disabled="!!editing"
            @change="onKeyChange"
          />
        </div>
      </div>

      <!-- Toggles + Payment mode + Supported types (single row) -->
      <div class="flex flex-wrap items-center gap-x-5 gap-y-2">
        <ToggleSwitch :label="t('common.enabled')" :checked="form.enabled" @toggle="form.enabled = !form.enabled" />
        <ToggleSwitch :label="t('payment.adminSettings.refundEnabled')" :checked="form.refund_enabled" @toggle="form.refund_enabled = !form.refund_enabled; if (!form.refund_enabled) form.allow_user_refund = false" />
        <ToggleSwitch v-if="form.refund_enabled" :label="t('payment.adminSettings.allowUserRefund')" :checked="form.allow_user_refund" @toggle="form.allow_user_refund = !form.allow_user_refund" />
        <div v-if="form.provider_key === 'easypay'" class="flex items-center gap-2">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.adminSettings.paymentMode') }}</span>
          <div class="flex gap-1.5">
            <button
              v-for="mode in paymentModeOptions"
              :key="mode.value"
              type="button"
              @click="form.payment_mode = mode.value"
              :class="[
                'rounded-lg border px-2.5 py-1 text-xs font-medium transition-all',
                form.payment_mode === mode.value
                  ? 'border-primary-500 bg-primary-500 text-white shadow-sm'
                  : 'border-gray-300 bg-white text-gray-600 hover:border-gray-400 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500',
              ]"
            >{{ mode.label }}</button>
          </div>
        </div>
        <div v-if="availableTypes.length > 1" class="flex items-center gap-2">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.adminSettings.supportedTypes') }}</span>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="pt in availableTypes"
              :key="pt.value"
              type="button"
              @click="toggleType(pt.value)"
              :class="[
                'rounded-lg border px-2.5 py-1 text-xs font-medium transition-all',
                isTypeSelected(pt.value)
                  ? 'border-primary-500 bg-primary-500 text-white shadow-sm'
                  : 'border-gray-300 bg-white text-gray-600 hover:border-gray-400 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500',
              ]"
            >{{ pt.label }}</button>
          </div>
        </div>
      </div>

      <!-- Config fields + Callback URLs + Webhook hint -->
      <ProviderConfigSection
        :editing="!!editing"
        :provider-key="form.provider_key"
        :config="config"
        :visible-fields="visibleFields"
        :resolved-fields="resolvedFields"
        :callback-paths="callbackPaths"
        v-model:notify-base-url="notifyBaseUrl"
        v-model:return-base-url="returnBaseUrl"
        :default-base-url="defaultBaseUrl"
        :provider-webhook-url="providerWebhookUrl"
        :provider-webhook-hint="providerWebhookHint"
      />

      <!-- Per-type limits (collapsible) -->
      <ProviderLimitsSection
        v-if="limitableTypes.length"
        :limitable-types="limitableTypes"
        v-model:expanded="limitsExpanded"
        :get-limit-val="getLimitVal"
        :limit-placeholder="limitPlaceholder"
        :set-limit-val="setLimitVal"
      />
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" @click="emit('close')" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="provider-form" :disabled="saving" class="btn btn-primary">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { BaseDialog } from '@sub2api/plugin-sdk'
import { Select } from '@sub2api/plugin-sdk'
import type { SelectOption } from '@sub2api/plugin-sdk'
import ToggleSwitch from './ToggleSwitch.vue'
import ProviderConfigSection from './ProviderConfigSection.vue'
import ProviderLimitsSection from './ProviderLimitsSection.vue'
import type { ProviderInstance } from '../../types/payment'
import type { TypeOption } from './providerConfig'
import { useProviderDialogForm } from './useProviderDialogForm'

const props = defineProps<{
  show: boolean
  saving: boolean
  editing: ProviderInstance | null
  allKeyOptions: TypeOption[]
  enabledKeyOptions: TypeOption[]
  allPaymentTypes: TypeOption[]
  redirectLabel: string
}>()

const emit = defineEmits<{
  close: []
  save: [payload: {
    provider_key: string
    name: string
    supported_types: string[]
    enabled: boolean
    payment_mode: string
    refund_enabled: boolean
    allow_user_refund: boolean
    config: Record<string, string>
    limits: string
  }]
}>()

const { t } = useI18n()

const {
  form,
  config,
  notifyBaseUrl,
  returnBaseUrl,
  limitsExpanded,
  visibleFields,
  defaultBaseUrl,
  providerWebhookUrl,
  providerWebhookHint,
  callbackPaths,
  paymentModeOptions,
  availableTypes,
  resolvedFields,
  limitableTypes,
  isTypeSelected,
  toggleType,
  onKeyChange,
  getLimitVal,
  limitPlaceholder,
  setLimitVal,
  buildPayload,
  reset,
  loadProvider,
} = useProviderDialogForm({
  get editing() { return props.editing },
  get allPaymentTypes() { return props.allPaymentTypes },
  get redirectLabel() { return props.redirectLabel },
})

function handleSave() {
  const payload = buildPayload()
  if (payload) emit('save', payload)
}

defineExpose({ reset, loadProvider })
</script>
