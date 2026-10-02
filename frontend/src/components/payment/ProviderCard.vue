<template>
  <div
    :class="[
      'group relative',
      !enabled && 'bg-surface-sunken opacity-50',
    ]"
    :title="!enabled ? t('admin.settings.payment.typeDisabled') + ' — ' + t('admin.settings.payment.enableTypesFirst') : undefined"
  >
    <div :class="[
      'flex flex-wrap items-center justify-between gap-3 py-2.5',
      !enabled && 'pointer-events-none',
    ]">
      <!-- Left: status dot + name + key + type stamps -->
      <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
        <span
          :class="['h-2 w-2 shrink-0 rounded-full', provider.enabled && enabled ? 'bg-success' : 'bg-border-strong']"
          aria-hidden="true"
        />
        <span class="text-body font-semibold text-fg">{{ provider.name }}</span>
        <span class="text-meta text-fg-muted">{{ keyLabel }}</span>
        <span v-if="provider.payment_mode" class="text-meta text-fg-muted">· {{ modeLabel }}</span>
        <span v-if="enabled && availableTypes.length" class="text-meta text-border-strong">|</span>
        <div v-if="enabled" class="flex items-center gap-1">
          <button
            v-for="pt in availableTypes"
            :key="pt.value"
            type="button"
            @click="emit('toggleType', pt.value)"
            :class="[
              'badge transition-colors',
              isSelected(pt.value)
                ? 'badge-primary'
                : 'text-fg-subtle hover:border-accent hover:text-accent-strong',
            ]"
          >{{ pt.label }}</button>
        </div>
      </div>

      <!-- Right: toggles + actions -->
      <div class="flex items-center gap-4">
        <ToggleSwitch :label="t('common.enabled')" :checked="provider.enabled" @toggle="emit('toggleField', 'enabled')" />
        <div class="flex items-center gap-2 border-l border-border pl-3">
          <button type="button" @click="emit('edit')" class="flex flex-col items-center gap-0.5 rounded-sm p-1.5 text-fg-muted transition-colors hover:bg-accent-weak hover:text-accent-strong">
            <Icon name="edit" size="sm" />
            <span class="text-meta">{{ t('common.edit') }}</span>
          </button>
          <button type="button" @click="emit('delete')" class="flex flex-col items-center gap-0.5 rounded-sm p-1.5 text-fg-muted transition-colors hover:bg-danger-weak hover:text-danger">
            <Icon name="trash" size="sm" />
            <span class="text-meta">{{ t('common.delete') }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ToggleSwitch from './ToggleSwitch.vue'
import type { ProviderInstance } from '@/types/payment'
import type { TypeOption } from './providerConfig'
import { PAYMENT_MODE_QRCODE, PAYMENT_MODE_POPUP, PAYMENT_MODE_REDIRECT } from './providerConfig'

const PROVIDER_KEY_LABELS: Record<string, string> = {
  nowpayments: 'admin.settings.payment.providerNowPayments',
  gpmpay: 'admin.settings.payment.providerGpmPay',
}

const props = defineProps<{
  provider: ProviderInstance
  enabled: boolean
  availableTypes: TypeOption[]
}>()

const emit = defineEmits<{
  toggleField: [field: 'enabled']
  toggleType: [type: string]
  edit: []
  delete: []
}>()

const { t } = useI18n()

const keyLabel = computed(() => t(PROVIDER_KEY_LABELS[props.provider.provider_key] || props.provider.provider_key))

const modeLabel = computed(() => {
  if (props.provider.payment_mode === PAYMENT_MODE_QRCODE) return t('admin.settings.payment.modeQRCode')
  if (props.provider.payment_mode === PAYMENT_MODE_POPUP) return t('admin.settings.payment.modePopup')
  if (props.provider.payment_mode === PAYMENT_MODE_REDIRECT) return t('admin.settings.payment.modeRedirect')
  return ''
})

function isSelected(type: string): boolean {
  return Array.isArray(props.provider.supported_types) && props.provider.supported_types.includes(type)
}
</script>
