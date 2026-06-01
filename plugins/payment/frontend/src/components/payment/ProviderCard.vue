<template>
  <div
    :class="[
      'group relative rounded-lg border transition-all',
      enabled ? 'border-gray-200 dark:border-dark-600' : 'border-gray-200 bg-gray-50 opacity-50 dark:border-dark-700 dark:bg-dark-800/50',
    ]"
    :title="!enabled ? t('payment.adminSettings.typeDisabled') + ' — ' + t('payment.adminSettings.enableTypesFirst') : undefined"
  >
    <div :class="[
      'flex items-center justify-between px-4 py-2.5',
      !enabled && 'pointer-events-none',
    ]">
      <!-- Left: icon + name + key badge + type badges -->
      <div class="flex items-center gap-3">
        <div :class="[
          'rounded-md p-1.5',
          provider.enabled && enabled ? 'bg-green-100 dark:bg-green-900/30' : 'bg-gray-100 dark:bg-dark-700',
        ]">
          <Icon
            name="server"
            size="sm"
            :class="provider.enabled && enabled ? 'text-green-600 dark:text-green-400' : 'text-gray-400'"
          />
        </div>
        <span class="text-sm font-medium text-gray-900 dark:text-white">{{ provider.name }}</span>
        <span class="text-xs text-gray-400 dark:text-gray-500">{{ keyLabel }}</span>
        <span v-if="provider.payment_mode" class="text-xs text-gray-400 dark:text-gray-500">· {{ modeLabel }}</span>
        <span v-if="enabled && availableTypes.length" class="text-xs text-gray-300 dark:text-gray-600">|</span>
        <div v-if="enabled" class="flex items-center gap-1">
          <button
            v-for="pt in availableTypes"
            :key="pt.value"
            type="button"
            @click="emit('toggleType', pt.value)"
            :class="[
              'rounded px-2 py-0.5 text-xs font-medium transition-all',
              isSelected(pt.value)
                ? 'bg-primary-500 text-white'
                : 'bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-gray-500',
            ]"
          >{{ pt.label }}</button>
        </div>
      </div>

      <!-- Right: toggles + actions -->
      <div class="flex items-center gap-4">
        <ToggleSwitch :label="t('common.enabled')" :checked="provider.enabled" @toggle="emit('toggleField', 'enabled')" />
        <ToggleSwitch :label="t('payment.adminSettings.refundEnabled')" :checked="provider.refund_enabled" @toggle="emit('toggleField', 'refund_enabled')" />
        <ToggleSwitch v-if="provider.refund_enabled" :label="t('payment.adminSettings.allowUserRefund')" :checked="provider.allow_user_refund" @toggle="emit('toggleField', 'allow_user_refund')" />
        <div class="flex items-center gap-2 border-l border-gray-200 pl-3 dark:border-dark-600">
          <button type="button" @click="emit('edit')" class="hover-tint-info flex flex-col items-center gap-0.5 rounded-lg p-1.5">
            <Icon name="edit" size="sm" />
            <span class="text-xs">{{ t('common.edit') }}</span>
          </button>
          <button type="button" @click="emit('delete')" class="hover-tint-danger flex flex-col items-center gap-0.5 rounded-lg p-1.5">
            <Icon name="trash" size="sm" />
            <span class="text-xs">{{ t('common.delete') }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import ToggleSwitch from './ToggleSwitch.vue'
import type { ProviderInstance } from '../../types/payment'
import type { TypeOption } from './providerConfig'
import { PAYMENT_MODE_QRCODE, PAYMENT_MODE_POPUP } from './providerConfig'

const PROVIDER_KEY_LABELS: Record<string, string> = {
  easypay: 'payment.adminSettings.providerEasypay',
  alipay: 'payment.adminSettings.providerAlipay',
  wxpay: 'payment.adminSettings.providerWxpay',
  stripe: 'payment.adminSettings.providerStripe',
  airwallex: 'payment.adminSettings.providerAirwallex',
}

const props = defineProps<{
  provider: ProviderInstance
  enabled: boolean
  availableTypes: TypeOption[]
}>()

const emit = defineEmits<{
  toggleField: [field: 'enabled' | 'refund_enabled' | 'allow_user_refund']
  toggleType: [type: string]
  edit: []
  delete: []
}>()

const { t } = useI18n()

const keyLabel = computed(() => t(PROVIDER_KEY_LABELS[props.provider.provider_key] || props.provider.provider_key))

const modeLabel = computed(() => {
  if (props.provider.payment_mode === PAYMENT_MODE_QRCODE) return t('payment.adminSettings.modeQRCode')
  if (props.provider.payment_mode === PAYMENT_MODE_POPUP) return t('payment.adminSettings.modePopup')
  return ''
})

function isSelected(type: string): boolean {
  return props.provider.supported_types.includes(type)
}
</script>
