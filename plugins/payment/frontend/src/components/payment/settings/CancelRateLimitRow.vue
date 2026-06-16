<!-- CancelRateLimitRow: pending orders, load balance strategy, cancel rate limit inline form. -->
<template>
  <div class="flex flex-wrap items-end gap-4">
    <div class="w-28">
      <label class="input-label">{{ t('payment.adminSettings.maxPendingOrders') }}</label>
      <input v-model.number="form.payment_max_pending_orders" type="number" min="1" class="input" />
    </div>
    <div>
      <label class="input-label">{{ t('payment.adminSettings.loadBalanceStrategy') }}</label>
      <Select v-model="form.payment_load_balance_strategy" :options="loadBalanceOptions" class="w-40" />
    </div>
    <div>
      <label class="input-label">{{ t('payment.adminSettings.cancelRateLimit') }}</label>
      <div class="flex items-center gap-2">
        <button type="button"
          :class="['relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            form.payment_cancel_rate_limit_enabled ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600']"
          @click="form.payment_cancel_rate_limit_enabled = !form.payment_cancel_rate_limit_enabled">
          <span :class="['pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            form.payment_cancel_rate_limit_enabled ? 'translate-x-5' : 'translate-x-0']" />
        </button>
        <Select v-model="form.payment_cancel_rate_limit_window_mode" :options="cancelRateLimitModeOptions" class="w-24" :disabled="!form.payment_cancel_rate_limit_enabled" />
        <span :class="['whitespace-nowrap text-sm', enabledTextClass]">{{ t('payment.adminSettings.cancelRateLimitEvery') }}</span>
        <input v-model.number="form.payment_cancel_rate_limit_window" type="number" min="1" required class="input w-14 text-center" :disabled="!form.payment_cancel_rate_limit_enabled" />
        <Select v-model="form.payment_cancel_rate_limit_unit" :options="cancelRateLimitUnitOptions" class="w-28" :disabled="!form.payment_cancel_rate_limit_enabled" />
        <span :class="['whitespace-nowrap text-sm', enabledTextClass]">{{ t('payment.adminSettings.cancelRateLimitAllowMax') }}</span>
        <input v-model.number="form.payment_cancel_rate_limit_max" type="number" min="1" required class="input w-14 text-center" :disabled="!form.payment_cancel_rate_limit_enabled" />
        <span :class="['whitespace-nowrap text-sm', enabledTextClass]">{{ t('payment.adminSettings.cancelRateLimitTimes') }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Select } from '@sub2api/plugin-sdk'

const { t } = useI18n()

const props = defineProps<{
  form: Record<string, unknown>
  loadBalanceOptions: { value: string; label: string }[]
  cancelRateLimitModeOptions: { value: string; label: string }[]
  cancelRateLimitUnitOptions: { value: string; label: string }[]
}>()

const enabledTextClass = computed(() =>
  props.form.payment_cancel_rate_limit_enabled
    ? 'text-gray-700 dark:text-gray-300'
    : 'text-gray-400 dark:text-gray-600',
)
</script>