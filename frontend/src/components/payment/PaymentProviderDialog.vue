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
            :options="(!!editing ? allKeyOptions : enabledKeyOptions) as SelectOption[]"
            :disabled="!!editing"
            @change="onKeyChange"
          />
        </div>
      </div>

      <!-- Toggles + Payment mode + Supported types (single row) -->
      <div class="flex flex-wrap items-center gap-x-5 gap-y-2">
        <ToggleSwitch :label="t('common.enabled')" :checked="form.enabled" @toggle="form.enabled = !form.enabled" />
        <ToggleSwitch :label="t('admin.settings.payment.refundEnabled')" :checked="form.refund_enabled" @toggle="form.refund_enabled = !form.refund_enabled" />
        <div v-if="form.provider_key === 'easypay'" class="flex items-center gap-2">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.paymentMode') }}</span>
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
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.supportedTypes') }}</span>
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
            <div v-else-if="field.sensitive" class="relative">
              <input
                :type="visibleFields[field.key] ? 'text' : 'password'"
                v-model="config[field.key]"
                class="input pr-10"
                :placeholder="field.defaultValue || ''"
              />
              <button
                type="button"
                @click="visibleFields[field.key] = !visibleFields[field.key]"
                class="absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              >
                <svg v-if="visibleFields[field.key]" class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" /></svg>
                <svg v-else class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" /></svg>
              </button>
            </div>
            <input
              v-else
              type="text"
              v-model="config[field.key]"
              class="input"
              :placeholder="field.defaultValue || ''"
            />
          </div>
        </div>

        <!-- Callback URLs (each = editable URL + fixed path) -->
        <div v-if="callbackPaths" class="mt-4 space-y-3">
          <div v-if="callbackPaths.notifyUrl">
            <label class="input-label">{{ t('admin.settings.payment.field_notifyUrl') }} <span class="text-red-500">*</span></label>
            <div class="flex">
              <input v-model="notifyBaseUrl" type="text" class="input min-w-0 flex-1 !rounded-r-none !border-r-0" :placeholder="defaultBaseUrl" />
              <span class="inline-flex items-center whitespace-nowrap rounded-r-lg border border-gray-300 bg-gray-50 px-3 text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-400">{{ callbackPaths.notifyUrl }}</span>
            </div>
          </div>
          <div v-if="callbackPaths.returnUrl">
            <label class="input-label">{{ t('admin.settings.payment.field_returnUrl') }} <span class="text-red-500">*</span></label>
            <div class="flex">
              <input v-model="returnBaseUrl" type="text" class="input min-w-0 flex-1 !rounded-r-none !border-r-0" :placeholder="defaultBaseUrl" />
              <span class="inline-flex items-center whitespace-nowrap rounded-r-lg border border-gray-300 bg-gray-50 px-3 text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-400">{{ callbackPaths.returnUrl }}</span>
            </div>
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

      <!-- Per-type limits (collapsible) -->
      <div v-if="limitableTypes.length" class="border-t border-gray-200 pt-4 dark:border-dark-700">
        <button type="button" @click="limitsExpanded = !limitsExpanded" class="flex w-full items-center justify-between">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.payment.limitsTitle') }}
          </h4>
          <svg :class="['h-4 w-4 text-gray-400 transition-transform', limitsExpanded && 'rotate-180']" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" /></svg>
        </button>
        <div v-show="limitsExpanded" class="mt-3 space-y-3">
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
                  class="input mt-0.5" min="1" step="0.01" :placeholder="limitPlaceholder(lt.value)"
                />
              </div>
              <div>
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.limitSingleMax') }}</label>
                <input
                  type="number"
                  :value="getLimitVal(lt.value, 'singleMax')"
                  @input="setLimitVal(lt.value, 'singleMax', ($event.target as HTMLInputElement).value)"
                  class="input mt-0.5" min="1" step="0.01" :placeholder="limitPlaceholder(lt.value)"
                />
              </div>
              <div>
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.limitDaily') }}</label>
                <input
                  type="number"
                  :value="getLimitVal(lt.value, 'dailyLimit')"
                  @input="setLimitVal(lt.value, 'dailyLimit', ($event.target as HTMLInputElement).value)"
                  class="input mt-0.5" min="1" step="0.01" :placeholder="limitPlaceholder(lt.value)"
                />
              </div>
            </div>
          </div>
          <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('admin.settings.payment.limitsHint') }}</p>
        </div>
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
import { reactive, computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import type { SelectOption } from '@/components/common/Select.vue'
import ToggleSwitch from './ToggleSwitch.vue'
import type { ProviderInstance } from '@/types/payment'
import type { TypeOption } from './providerConfig'
import {
  PROVIDER_CONFIG_FIELDS,
  PROVIDER_SUPPORTED_TYPES,
  PROVIDER_CALLBACK_PATHS,
  WEBHOOK_PATHS,
  PAYMENT_MODE_QRCODE,
  PAYMENT_MODE_POPUP,
  getAvailableTypes,
  extractBaseUrl,
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
    provider_key: string
    name: string
    supported_types: string[]
    enabled: boolean
    payment_mode: string
    refund_enabled: boolean
    config: Record<string, string>
    limits: string
  }]
}>()

const { t } = useI18n()

// --- Form state ---
const form = reactive({
  name: '',
  provider_key: 'easypay',
  supported_types: [] as string[],
  enabled: true,
  payment_mode: PAYMENT_MODE_QRCODE,
  refund_enabled: false,
})
const config = reactive<Record<string, string>>({})
const limits = reactive<Record<string, Record<string, number>>>({})
const notifyBaseUrl = ref('')
const returnBaseUrl = ref('')
const limitsExpanded = ref(false)
const visibleFields = reactive<Record<string, boolean>>({})

// --- Computed ---
const defaultBaseUrl = typeof window !== 'undefined' ? window.location.origin : ''

const stripeWebhookUrl = computed(() =>
  form.provider_key === 'stripe' ? defaultBaseUrl + WEBHOOK_PATHS.stripe : '',
)

const callbackPaths = computed(() => PROVIDER_CALLBACK_PATHS[form.provider_key] || null)

const paymentModeOptions = computed(() => {
  return [
    { value: PAYMENT_MODE_QRCODE, label: t('admin.settings.payment.modeQRCode') },
    { value: PAYMENT_MODE_POPUP, label: t('admin.settings.payment.modePopup') },
  ]
})

const availableTypes = computed(() => {
  const base = getAvailableTypes(form.provider_key, props.allPaymentTypes, props.redirectLabel)
  // Resolve i18n labels for types not in allPaymentTypes (e.g. card, link inside stripe)
  return base.map(opt =>
    opt.label === opt.value
      ? { ...opt, label: t(`payment.methods.${opt.value}`, opt.value) }
      : opt,
  )
})

const resolvedFields = computed(() => {
  const fields = PROVIDER_CONFIG_FIELDS[form.provider_key] || []
  return fields.map(f => ({
    ...f,
    label: f.label || t(`admin.settings.payment.field_${f.key}`),
  }))
})

const limitableTypes = computed(() => {
  // Stripe: single "stripe" entry (one set of shared limits)
  if (form.provider_key === 'stripe') {
    return [{ value: 'stripe', label: 'Stripe' }]
  }
  const selected = form.supported_types.filter(t => t !== 'easypay')
  return selected.map(v => {
    const found = props.allPaymentTypes.find(pt => pt.value === v)
    return found || { value: v, label: v }
  })
})

// --- Methods ---
function isTypeSelected(type: string): boolean {
  return form.supported_types.includes(type)
}

function toggleType(type: string) {
  if (form.supported_types.includes(type)) {
    form.supported_types = form.supported_types.filter(t => t !== type)
  } else {
    form.supported_types = [...form.supported_types, type]
  }
}

function onKeyChange() {
  form.supported_types = [...(PROVIDER_SUPPORTED_TYPES[form.provider_key] || [])]
  clearConfig()
  applyDefaults()
}

function clearConfig() {
  Object.keys(config).forEach(k => delete config[k])
  Object.keys(limits).forEach(k => delete limits[k])
  Object.keys(visibleFields).forEach(k => delete visibleFields[k])
  notifyBaseUrl.value = ''
  returnBaseUrl.value = ''
  limitsExpanded.value = false
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

/** Returns true if any limit field for this payment type has a value */
function hasAnyLimit(paymentType: string): boolean {
  const l = limits[paymentType]
  if (!l) return false
  return (l.singleMin > 0) || (l.singleMax > 0) || (l.dailyLimit > 0)
}

/** Dynamic placeholder: "不限制" if sibling has value, "使用全局配置" if all empty */
function limitPlaceholder(paymentType: string): string {
  return hasAnyLimit(paymentType)
    ? t('admin.settings.payment.limitsNoLimit')
    : t('admin.settings.payment.limitsUseGlobal')
}

function setLimitVal(paymentType: string, field: string, val: string) {
  if (!limits[paymentType]) limits[paymentType] = {}
  const num = Number(val)
  // Empty → clear the field (use global); reject ≤0
  if (val === '' || isNaN(num)) {
    delete limits[paymentType][field]
    return
  }
  if (num <= 0) return
  limits[paymentType][field] = num
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
  // Validate required config fields — all non-optional fields must be filled
  for (const f of PROVIDER_CONFIG_FIELDS[form.provider_key] || []) {
    if (f.optional) continue
    const val = (config[f.key] || '').trim()
    if (!val) {
      const label = f.label || t(`admin.settings.payment.field_${f.key}`)
      emitValidationError(t('admin.settings.payment.validationFieldRequired', { field: label }))
      return
    }
  }

  const filteredConfig: Record<string, string> = {}
  for (const [k, v] of Object.entries(config)) {
    if (!v || !v.trim()) continue
    // Skip masked values — backend keeps existing credentials
    if (v === '••••••••') continue
    filteredConfig[k] = v
  }

  // Inject computed callback URLs (each URL = independent base + fixed path)
  // If base URL is empty, auto-fill with current domain
  const paths = PROVIDER_CALLBACK_PATHS[form.provider_key]
  if (paths) {
    const notifyBase = notifyBaseUrl.value.trim() || defaultBaseUrl
    const returnBase = returnBaseUrl.value.trim() || defaultBaseUrl
    notifyBaseUrl.value = notifyBase
    returnBaseUrl.value = returnBase
    if (paths.notifyUrl) filteredConfig['notifyUrl'] = notifyBase + paths.notifyUrl
    if (paths.returnUrl) filteredConfig['returnUrl'] = returnBase + paths.returnUrl
  }

  emit('save', {
    provider_key: form.provider_key,
    name: form.name,
    supported_types: form.supported_types,
    enabled: form.enabled,
    payment_mode: form.provider_key === 'easypay' ? form.payment_mode : '',
    refund_enabled: form.refund_enabled,
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
  form.supported_types = [...(PROVIDER_SUPPORTED_TYPES[defaultKey] || [])]
  form.enabled = true
  form.payment_mode = defaultKey === 'easypay' ? PAYMENT_MODE_QRCODE : ''
  form.refund_enabled = false
  clearConfig()
  applyDefaults()
}

function loadProvider(provider: ProviderInstance) {
  form.name = provider.name
  form.provider_key = provider.provider_key
  form.supported_types = provider.supported_types
  form.enabled = provider.enabled
  form.payment_mode = provider.payment_mode || (provider.provider_key === 'easypay' ? PAYMENT_MODE_QRCODE : '')
  form.refund_enabled = provider.refund_enabled
  clearConfig()
  // Pre-fill config from API response (non-sensitive in cleartext, sensitive masked as ••••••••)
  if (provider.config) {
    for (const [k, v] of Object.entries(provider.config)) {
      // Skip notifyUrl/returnUrl — they are derived from callbackBaseUrl
      if (k === 'notifyUrl' || k === 'returnUrl') continue
      config[k] = v
    }
    // Extract base URLs from existing callback URLs
    const paths = PROVIDER_CALLBACK_PATHS[provider.provider_key]
    if (paths?.notifyUrl && provider.config['notifyUrl']) {
      notifyBaseUrl.value = extractBaseUrl(provider.config['notifyUrl'], paths.notifyUrl)
    }
    if (paths?.returnUrl && provider.config['returnUrl']) {
      returnBaseUrl.value = extractBaseUrl(provider.config['returnUrl'], paths.returnUrl)
    }
  }
  applyDefaults()
  // Parse existing limits
  if (provider.limits) {
    try {
      const parsed = JSON.parse(provider.limits)
      for (const [pt, fields] of Object.entries(parsed as Record<string, Record<string, number>>)) {
        limits[pt] = { ...fields }
      }
      limitsExpanded.value = Object.keys(limits).length > 0
    } catch { /* ignore */ }
  }
}

defineExpose({ reset, loadProvider })
</script>
