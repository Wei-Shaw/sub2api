<template>
  <BaseDialog
    :show="show"
    :title="editing ? t('admin.settings.payment.editProvider') : t('admin.settings.payment.createProvider')"
    width="wide"
    @close="emit('close')"
  >
    <form id="provider-form" @submit.prevent="handleSave" class="space-y-4">
      <!-- Name + Key -->
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">
            {{ t('admin.settings.payment.providerName') }}
            <span class="text-red-500">*</span>
          </label>
          <input v-model="form.name" type="text" class="input" required />
        </div>
        <div>
          <label class="input-label">
            {{ t('admin.settings.payment.providerKey') }}
            <span class="text-red-500">*</span>
          </label>
          <Select
            v-model="form.provider_key"
            :options="(!!editing ? allKeyOptions : enabledKeyOptions) as any"
            :disabled="!!editing"
            @change="onKeyChange"
          />
        </div>
      </div>

      <!-- Supported types -->
      <div>
        <label class="input-label">
          {{ t('admin.settings.payment.supportedTypes') }}
          <span class="text-red-500">*</span>
        </label>
        <div class="mt-2 flex flex-wrap gap-2">
          <button
            v-for="pt in availableTypes"
            :key="pt.value"
            type="button"
            @click="toggleType(pt.value)"
            :class="[
              'rounded-lg border px-4 py-2 text-sm font-medium transition-all',
              isTypeSelected(pt.value)
                ? 'border-primary-500 bg-primary-500 text-white shadow-sm'
                : 'border-gray-300 bg-white text-gray-600 hover:border-gray-400 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500',
            ]"
          >{{ pt.label }}</button>
        </div>
      </div>

      <!-- Enabled + Refund checkboxes -->
      <div class="flex items-center gap-4">
        <label class="flex items-center gap-2">
          <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('common.enabled') }}</span>
        </label>
        <label class="flex items-center gap-2">
          <input v-model="form.refund_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.settings.payment.refundEnabled') }}</span>
        </label>
      </div>

      <!-- Config fields -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.payment.providerConfig') }}
        </h4>
        <div class="space-y-3">
          <div v-for="field in resolvedFields" :key="field.key">
            <label class="input-label">
              {{ field.label }}
              <span v-if="field.optional" class="text-xs text-gray-400">({{ t('common.optional') }})</span>
              <span v-else class="text-red-500"> *</span>
            </label>
            <textarea
              v-if="field.sensitive && field.key.toLowerCase().includes('key') && field.key !== 'pkey'"
              v-model="config[field.key]"
              rows="3"
              class="input font-mono text-xs"
            />
            <input
              v-else
              :type="field.sensitive ? 'password' : 'text'"
              v-model="config[field.key]"
              class="input"
              :placeholder="field.defaultValue || ''"
            />
          </div>
        </div>

        <!-- Stripe webhook hint -->
        <div v-if="stripeWebhookUrl" class="mt-3 rounded-lg border border-blue-200 bg-blue-50 p-3 dark:border-blue-800/50 dark:bg-blue-900/20">
          <p class="text-xs text-blue-700 dark:text-blue-300">
            {{ t('admin.settings.payment.stripeWebhookHint') }}
          </p>
          <code class="mt-1 block break-all rounded bg-blue-100 px-2 py-1 text-xs text-blue-800 dark:bg-blue-900/40 dark:text-blue-200">
            {{ stripeWebhookUrl }}
          </code>
        </div>
      </div>

      <!-- Per-type limits -->
      <div v-if="limitableTypes.length" class="border-t border-gray-200 pt-4 dark:border-dark-700">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.payment.limitsTitle') }}
        </h4>
        <div class="space-y-3">
          <div
            v-for="lt in limitableTypes"
            :key="lt.value"
            class="rounded-lg border border-gray-100 p-3 dark:border-dark-700"
          >
            <p class="mb-2 text-xs font-medium text-gray-700 dark:text-gray-300">{{ lt.label }}</p>
            <div class="grid grid-cols-3 gap-3">
              <div>
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.limitSingleMin') }}</label>
                <input
                  type="number"
                  :value="getLimitVal(lt.value, 'singleMin')"
                  @input="setLimitVal(lt.value, 'singleMin', ($event.target as HTMLInputElement).value)"
                  class="input mt-0.5" min="0" step="0.01" placeholder="0"
                />
              </div>
              <div>
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.limitSingleMax') }}</label>
                <input
                  type="number"
                  :value="getLimitVal(lt.value, 'singleMax')"
                  @input="setLimitVal(lt.value, 'singleMax', ($event.target as HTMLInputElement).value)"
                  class="input mt-0.5" min="0" step="0.01" placeholder="0"
                />
              </div>
              <div>
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.limitDaily') }}</label>
                <input
                  type="number"
                  :value="getLimitVal(lt.value, 'dailyLimit')"
                  @input="setLimitVal(lt.value, 'dailyLimit', ($event.target as HTMLInputElement).value)"
                  class="input mt-0.5" min="0" step="0.01" placeholder="0"
                />
              </div>
            </div>
          </div>
        </div>
        <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">{{ t('admin.settings.payment.limitsHint') }}</p>
      </div>
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
import { reactive, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import type { ProviderInstance } from '@/types/payment'
import type { TypeOption } from './providerConfig'
import {
  PROVIDER_CONFIG_FIELDS,
  PROVIDER_SUPPORTED_TYPES,
  WEBHOOK_PATHS,
  parseTypes,
  getAvailableTypes,
} from './providerConfig'

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
    providerKey: string
    name: string
    supportedTypes: string
    enabled: boolean
    refundEnabled: boolean
    config: Record<string, string>
    limits: string
  }]
}>()

const { t } = useI18n()

// --- Form state ---
const form = reactive({
  name: '',
  provider_key: 'easypay',
  supported_types: '',
  enabled: true,
  refund_enabled: false,
})
const config = reactive<Record<string, string>>({})
const limits = reactive<Record<string, Record<string, number>>>({})

// --- Computed ---
const baseURL = typeof window !== 'undefined' ? window.location.origin : ''

const stripeWebhookUrl = computed(() =>
  form.provider_key === 'stripe' ? baseURL + WEBHOOK_PATHS.stripe : '',
)

const availableTypes = computed(() =>
  getAvailableTypes(form.provider_key, props.allPaymentTypes, props.redirectLabel),
)

const resolvedFields = computed(() => {
  const fields = PROVIDER_CONFIG_FIELDS[form.provider_key] || []
  return fields.map(f => ({
    ...f,
    label: f.label || t(`admin.settings.payment.field_${f.key}`),
  }))
})

const limitableTypes = computed(() => {
  const selected = parseTypes(form.supported_types).filter(t => t !== 'easypay')
  return selected.map(v => {
    const found = props.allPaymentTypes.find(pt => pt.value === v)
    return found || { value: v, label: v }
  })
})

// --- Methods ---
function isTypeSelected(type: string): boolean {
  return parseTypes(form.supported_types).includes(type)
}

function toggleType(type: string) {
  const current = parseTypes(form.supported_types)
  form.supported_types = current.includes(type)
    ? current.filter(t => t !== type).join(',')
    : [...current, type].join(',')
}

function onKeyChange() {
  form.supported_types = (PROVIDER_SUPPORTED_TYPES[form.provider_key] || []).join(',')
  clearConfig()
  applyDefaults()
}

function clearConfig() {
  Object.keys(config).forEach(k => delete config[k])
  Object.keys(limits).forEach(k => delete limits[k])
}

function applyDefaults() {
  for (const f of PROVIDER_CONFIG_FIELDS[form.provider_key] || []) {
    if (f.defaultValue && !config[f.key]) config[f.key] = f.defaultValue
  }
}

function getLimitVal(paymentType: string, field: string): string {
  const val = limits[paymentType]?.[field]
  return val && val > 0 ? String(val) : ''
}

function setLimitVal(paymentType: string, field: string, val: string) {
  if (!limits[paymentType]) limits[paymentType] = {}
  limits[paymentType][field] = Number(val) || 0
}

function serializeLimits(): string {
  const result: Record<string, Record<string, number>> = {}
  for (const [pt, fields] of Object.entries(limits)) {
    const clean: Record<string, number> = {}
    for (const [k, v] of Object.entries(fields)) {
      if (v > 0) clean[k] = v
    }
    if (Object.keys(clean).length > 0) result[pt] = clean
  }
  return Object.keys(result).length > 0 ? JSON.stringify(result) : ''
}

function handleSave() {
  // Validate required fields
  if (!form.name.trim()) {
    emitValidationError(t('admin.settings.payment.validationNameRequired'))
    return
  }
  if (!form.supported_types.trim()) {
    emitValidationError(t('admin.settings.payment.validationTypesRequired'))
    return
  }
  // Validate required config on create
  if (!props.editing) {
    for (const f of PROVIDER_CONFIG_FIELDS[form.provider_key] || []) {
      if (!f.optional && !(config[f.key] || '').trim()) {
        const label = f.label || t(`admin.settings.payment.field_${f.key}`)
        emitValidationError(t('admin.settings.payment.validationFieldRequired', { field: label }))
        return
      }
    }
  }

  const filteredConfig: Record<string, string> = {}
  for (const [k, v] of Object.entries(config)) {
    if (v && v.trim()) filteredConfig[k] = v
  }

  emit('save', {
    providerKey: form.provider_key,
    name: form.name,
    supportedTypes: form.supported_types,
    enabled: form.enabled,
    refundEnabled: form.refund_enabled,
    config: filteredConfig,
    limits: serializeLimits(),
  })
}

function emitValidationError(msg: string) {
  // Use a custom event or inject appStore — for now use window alert fallback
  // The parent handles this via the save event validation
  import('@/stores').then(m => m.useAppStore().showError(msg))
}

// --- Public API for parent to call ---
function reset(defaultKey: string) {
  form.name = ''
  form.provider_key = defaultKey
  form.supported_types = (PROVIDER_SUPPORTED_TYPES[defaultKey] || []).join(',')
  form.enabled = true
  form.refund_enabled = false
  clearConfig()
  applyDefaults()
}

function loadProvider(provider: ProviderInstance) {
  form.name = provider.name
  form.provider_key = provider.provider_key
  form.supported_types = provider.supported_types
  form.enabled = provider.enabled
  form.refund_enabled = provider.refund_enabled
  clearConfig()
  // Parse existing limits
  if (provider.limits) {
    try {
      const parsed = JSON.parse(provider.limits)
      for (const [pt, fields] of Object.entries(parsed as Record<string, Record<string, number>>)) {
        limits[pt] = { ...fields }
      }
    } catch { /* ignore */ }
  }
}

defineExpose({ reset, loadProvider })
</script>
