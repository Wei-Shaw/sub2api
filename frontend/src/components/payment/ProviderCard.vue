<template>
  <div
    :class="[
      'group relative rounded border transition-colors duration-fast ease-out',
      enabled ? 'border-line bg-surface' : 'border-line bg-surface-sunken opacity-50',
    ]"
    :title="!enabled ? t('admin.settings.payment.typeDisabled') + ' — ' + t('admin.settings.payment.enableTypesFirst') : undefined"
  >
    <div :class="[
      'flex items-center justify-between px-4 py-2.5',
      !enabled && 'pointer-events-none',
    ]">
      <!-- Left: icon + name + status + key badge + type badges -->
      <div class="flex items-center gap-3">
        <div class="rounded border border-line-subtle bg-surface-sunken p-1.5">
          <Icon name="server" size="sm" class="text-ink-tertiary" />
        </div>
        <span class="text-sm font-medium text-ink">{{ provider.name }}</span>
        <!--
          A written status, not a tinted icon square: colour alone on the icon
          told the same story as the "Enabled" toggle already does on the
          right, and it did so in a way a grayscale screenshot could not read.
        -->
        <StatusDot
          :tone="provider.enabled ? 'success' : 'neutral'"
          :label="provider.enabled ? t('common.enabled') : t('common.disabled')"
          :muted="!provider.enabled"
        />
        <span class="text-xs text-ink-tertiary">{{ keyLabel }}</span>
        <span v-if="provider.payment_mode" class="text-xs text-ink-tertiary">· {{ modeLabel }}</span>
        <span v-if="enabled && availableTypes.length" class="text-xs text-ink-disabled">|</span>
        <!--
          These chips are a SELECTION, not a status — which payment types this
          provider currently serves — so the selected one carries the accent,
          the one tone this system reserves for interaction and selection.
        -->
        <div v-if="enabled" class="flex items-center gap-1">
          <button
            v-for="pt in availableTypes"
            :key="pt.value"
            type="button"
            @click="emit('toggleType', pt.value)"
            :class="[
              'rounded border px-2 py-0.5 text-xs font-medium transition-colors duration-fast ease-out',
              isSelected(pt.value)
                ? 'border-accent bg-accent-tint text-accent'
                : 'border-line bg-surface text-ink-tertiary hover:border-line-strong',
            ]"
          >{{ pt.label }}</button>
        </div>
      </div>

      <!-- Right: toggles + actions -->
      <div class="flex items-center gap-4">
        <ToggleSwitch :label="t('common.enabled')" :checked="provider.enabled" @toggle="emit('toggleField', 'enabled')" />
        <div class="flex items-center gap-2 border-l border-line pl-3">
          <button
            type="button"
            @click="emit('edit')"
            class="flex flex-col items-center gap-0.5 rounded p-1.5 text-ink-tertiary transition-colors duration-fast ease-out hover:bg-surface-hover hover:text-accent"
          >
            <Icon name="edit" size="sm" />
            <span class="text-xs">{{ t('common.edit') }}</span>
          </button>
          <button
            type="button"
            @click="emit('delete')"
            class="flex flex-col items-center gap-0.5 rounded p-1.5 text-ink-tertiary transition-colors duration-fast ease-out hover:bg-danger-tint hover:text-danger"
          >
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
import Icon from '@/components/icons/Icon.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import ToggleSwitch from './ToggleSwitch.vue'
import type { ProviderInstance } from '@/types/payment'
import type { TypeOption } from './providerConfig'
import { PAYMENT_MODE_QRCODE, PAYMENT_MODE_REDIRECT } from './providerConfig'

const PROVIDER_KEY_LABELS: Record<string, string> = {
  easypay: 'admin.settings.payment.providerEasypay',
  alipay: 'admin.settings.payment.providerAlipay',
  wxpay: 'admin.settings.payment.providerWxpay',
  stripe: 'admin.settings.payment.providerStripe',
  airwallex: 'admin.settings.payment.providerAirwallex',
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
  if (props.provider.payment_mode === PAYMENT_MODE_REDIRECT) return t('admin.settings.payment.modeRedirect')
  return ''
})

function isSelected(type: string): boolean {
  return Array.isArray(props.provider.supported_types) && props.provider.supported_types.includes(type)
}
</script>
