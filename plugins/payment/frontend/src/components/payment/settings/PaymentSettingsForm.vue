<!--
  PaymentSettingsForm: the main settings card with toggles, inputs, and payment type badges.
  Receives the reactive form object + computed options from parent via props.
-->
<template>
  <div class="card">
    <div class="space-y-4 p-6">
      <!-- Enable toggle -->
      <div class="flex items-center justify-between">
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{ t('payment.adminSettings.enabled') }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.adminSettings.enabledHint') }}</p>
        </div>
        <Toggle v-model="form.payment_enabled" />
      </div>

      <template v-if="form.payment_enabled">
        <!-- Row 1: Product name prefix / suffix / preview -->
        <div class="grid grid-cols-3 gap-3">
          <div>
            <label class="input-label">{{ t('payment.adminSettings.productNamePrefix') }}</label>
            <input v-model="form.payment_product_name_prefix" type="text" class="input" placeholder="Sub2API" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.adminSettings.productNameSuffix') }}</label>
            <input v-model="form.payment_product_name_suffix" type="text" class="input" placeholder="CNY" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.adminSettings.preview') }}</label>
            <div class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300">
              {{ (form.payment_product_name_prefix || 'Sub2API') + ' 100 ' + (form.payment_product_name_suffix || 'CNY') }}
            </div>
          </div>
        </div>

        <!-- Row 2: Amount range + multiplier + fee rate + order timeout -->
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
          <div>
            <label class="input-label">{{ t('payment.adminSettings.minAmount') }}</label>
            <input v-model="form.payment_min_amount" type="number" step="0.01" min="0" class="input" :placeholder="t('payment.adminSettings.noLimit')" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.adminSettings.maxAmount') }}</label>
            <input v-model="form.payment_max_amount" type="number" step="0.01" min="0" class="input" :placeholder="t('payment.adminSettings.noLimit')" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.adminSettings.dailyLimit') }}</label>
            <input v-model="form.payment_daily_limit" type="number" step="0.01" min="0" class="input" :placeholder="t('payment.adminSettings.noLimit')" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.adminSettings.balanceRechargeMultiplier') }}</label>
            <input v-model="form.payment_balance_recharge_multiplier" type="number" step="0.01" min="0.01" class="input" />
            <p class="mt-0.5 text-xs text-gray-400">{{ t('payment.adminSettings.balanceRechargeMultiplierHint') }}</p>
            <p class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400">
              {{ t('payment.adminSettings.balanceRechargePreview', { usd: balanceMultiplierPreview }) }}
            </p>
          </div>
          <div>
            <label class="input-label">{{ t('payment.adminSettings.rechargeFeeRate') }}</label>
            <div class="relative">
              <input v-model="form.payment_recharge_fee_rate" type="number" step="0.01" min="0" max="100" class="input pr-8" />
              <span class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400">%</span>
            </div>
            <p class="mt-0.5 text-xs text-gray-400">{{ t('payment.adminSettings.rechargeFeeRateHint') }}</p>
            <p v-if="hasFeeRate" class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400">
              {{ t('payment.adminSettings.rechargeFeePreview', { fee: feeRatePreview }) }}
            </p>
          </div>
          <div>
            <label class="input-label">{{ t('payment.adminSettings.orderTimeout') }} <span class="input-required">*</span></label>
            <input v-model.number="form.payment_order_timeout_minutes" type="number" min="1" class="input" required />
            <p class="mt-0.5 text-xs text-gray-400">{{ t('payment.adminSettings.orderTimeoutHint') }}</p>
          </div>
        </div>

        <!-- Row 3: Pending orders + load balance + cancel rate limit -->
        <CancelRateLimitRow
          :form="form"
          :load-balance-options="loadBalanceOptions"
          :cancel-rate-limit-mode-options="cancelRateLimitModeOptions"
          :cancel-rate-limit-unit-options="cancelRateLimitUnitOptions"
        />

        <!-- Row 4: Enabled payment types (badges) -->
        <div>
          <label class="input-label">{{ t('payment.adminSettings.enabledPaymentTypes') }}</label>
          <div class="mt-1.5 flex flex-wrap gap-2">
            <button v-for="pt in allPaymentTypes" :key="pt.value" type="button"
              :class="['rounded-lg border px-3 py-1.5 text-sm font-medium transition-all',
                form.payment_enabled_types.includes(pt.value)
                  ? 'border-primary-500 bg-primary-500 text-white shadow-sm'
                  : 'border-gray-300 bg-white text-gray-600 hover:border-gray-400 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500']"
              @click="$emit('toggle-payment-type', pt.value)">
              {{ pt.label }}
            </button>
          </div>
          <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">{{ t('payment.adminSettings.enabledPaymentTypesHint') }}</p>
        </div>

        <!-- Row 5: Help image upload + help text -->
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="input-label">{{ t('payment.adminSettings.helpImage') }}</label>
            <ImageUploadInput v-model="form.payment_help_image_url" :hint="t('payment.adminSettings.helpImageHint')" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.adminSettings.helpText') }}</label>
            <textarea v-model="form.payment_help_text" rows="3" class="input" :placeholder="t('payment.adminSettings.helpTextPlaceholder')"></textarea>
          </div>
        </div>
      </template>

      <div class="flex justify-end pt-2">
        <button type="button" class="btn btn-primary" :disabled="saving || loading" @click="$emit('save')">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Toggle } from '@sub2api/plugin-sdk'
import ImageUploadInput from '../../common/ImageUploadInput.vue'
import CancelRateLimitRow from './CancelRateLimitRow.vue'

const { t } = useI18n()

defineProps<{
  form: Record<string, unknown>
  loading: boolean
  saving: boolean
  allPaymentTypes: { value: string; label: string }[]
  balanceMultiplierPreview: string
  hasFeeRate: boolean
  feeRatePreview: string
  loadBalanceOptions: { value: string; label: string }[]
  cancelRateLimitUnitOptions: { value: string; label: string }[]
  cancelRateLimitModeOptions: { value: string; label: string }[]
}>()

defineEmits<{
  save: []
  'toggle-payment-type': [type: string]
}>()
</script>