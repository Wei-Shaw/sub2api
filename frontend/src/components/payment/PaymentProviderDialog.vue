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
        <ToggleSwitch :label="t('admin.settings.payment.refundEnabled')" :checked="form.refund_enabled" @toggle="form.refund_enabled = !form.refund_enabled; if (!form.refund_enabled) form.allow_user_refund = false" />
        <ToggleSwitch v-if="form.refund_enabled" :label="t('admin.settings.payment.allowUserRefund')" :checked="form.allow_user_refund" @toggle="form.allow_user_refund = !form.allow_user_refund" />
        <div v-if="supportsPaymentModeSelector" class="flex items-center gap-2">
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

      <div v-if="form.provider_key === 'easypay'" class="space-y-3 rounded-lg border border-gray-100 p-3 dark:border-dark-700">
        <div class="flex items-center justify-between gap-3">
          <div>
            <h5 class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.settings.payment.easypayCustomMethods') }}
            </h5>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.payment.easypayCustomMethodsHint') }}
            </p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" @click="addEasyPayCustomMethod">
            {{ t('admin.settings.payment.addCustomMethod') }}
          </button>
        </div>
        <div v-if="easyPayCustomMethods.length" class="space-y-2">
          <div
            v-for="(method, index) in easyPayCustomMethods"
            :key="index"
            class="grid grid-cols-[1fr_1fr_1fr_auto] items-end gap-2"
          >
            <div>
              <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.customMethodType') }}</label>
              <input v-model="method.type" type="text" class="input mt-0.5" placeholder="credit_card" />
            </div>
            <div>
              <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.customMethodUpstreamType') }}</label>
              <input v-model="method.upstreamType" type="text" class="input mt-0.5" placeholder="credit_card" />
            </div>
            <div>
              <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.customMethodDisplayName') }}</label>
              <input v-model="method.displayName" type="text" class="input mt-0.5" :placeholder="t('admin.settings.payment.customMethodDisplayNamePlaceholder')" />
            </div>
            <button
              type="button"
              class="rounded-lg border border-red-200 px-2.5 py-2 text-xs font-medium text-red-600 transition-colors hover:bg-red-50 dark:border-red-800/60 dark:text-red-300 dark:hover:bg-red-900/20"
              @click="removeEasyPayCustomMethod(index)"
            >
              {{ t('common.delete') }}
            </button>
          </div>
        </div>
      </div>


      <!-- Payment currencies -->
      <div class="rounded-xl border border-blue-100 bg-blue-50/60 p-4 dark:border-blue-900/40 dark:bg-blue-900/10">
        <div class="mb-3 flex flex-col gap-1 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.payment.providerPaymentCurrencies') }}
            </h4>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.payment.providerPaymentCurrenciesHint') }}
            </p>
          </div>
          <span class="rounded-full bg-blue-100 px-2.5 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-200">
            {{ providerCurrencyBadgeLabel }}
          </span>
        </div>
        <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-start">
          <Select
            v-model="paymentCurrencyDraft"
            :options="availablePaymentCurrencySelectOptions"
            :placeholder="t('admin.settings.payment.providerPaymentCurrencyPlaceholder')"
            :creatable-prefix="t('admin.settings.payment.providerPaymentCurrencyCreatablePrefix')"
            searchable
            creatable
            @change="addPaymentCurrencySelection"
          />
          <button
            type="button"
            class="btn btn-secondary whitespace-nowrap"
            :disabled="!normalizedPaymentCurrencyDraft"
            @click="addPaymentCurrencySelection(normalizedPaymentCurrencyDraft)"
          >
            {{ t('admin.settings.payment.providerPaymentCurrencyAdd') }}
          </button>
        </div>
        <div class="mt-3 flex flex-wrap gap-2">
          <button
            v-for="currency in selectedPaymentCurrencyOptions"
            :key="currency.value"
            type="button"
            class="inline-flex items-center gap-1 rounded-lg border border-primary-500 bg-primary-500 px-3 py-1.5 text-sm font-medium text-white shadow-sm transition-all hover:bg-primary-600"
            @click="removePaymentCurrency(currency.value)"
          >
            <span>{{ currency.label }}</span>
            <span aria-hidden="true" class="text-xs opacity-80">×</span>
          </button>
        </div>
        <p v-if="form.provider_key === 'sepay'" class="mt-2 text-xs text-blue-700 dark:text-blue-300">
          {{ t('admin.settings.payment.sepayProviderCurrencyHint') }}
        </p>
        <p v-else class="mt-2 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.payment.providerPaymentCurrenciesExample') }}
        </p>
      </div>

      <!-- Config fields -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
        <div class="mb-3 flex items-center gap-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.payment.providerConfig') }}
          </h4>
          <HelpTooltip v-if="paymentGuide" trigger="click" width-class="w-80">
            <template #trigger>
              <button
                type="button"
                class="inline-flex h-5 w-5 items-center justify-center rounded-full border border-gray-300 text-[11px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-dark-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400"
                :aria-label="t('admin.settings.payment.paymentGuideTrigger')"
                :title="t('admin.settings.payment.paymentGuideTrigger')"
              >
                ?
              </button>
            </template>
            <div class="space-y-3">
              <p class="font-medium text-white">{{ paymentGuide.summary }}</p>
              <div
                v-for="item in paymentGuide.items"
                :key="item.title"
                class="space-y-1.5 border-t border-white/10 pt-2 first:border-t-0 first:pt-0"
              >
                <p class="font-medium text-white">{{ item.title }}</p>
                <p><span class="text-gray-300">{{ t('admin.settings.payment.guideOpenLabel') }}</span>{{ item.open }}</p>
                <p><span class="text-gray-300">{{ t('admin.settings.payment.guideCallLabel') }}</span>{{ item.call }}</p>
                <p><span class="text-gray-300">{{ t('admin.settings.payment.guideFallbackLabel') }}</span>{{ item.fallback }}</p>
              </div>
              <p v-if="paymentGuide.note" class="border-t border-white/10 pt-2 text-[11px] text-gray-300">
                {{ paymentGuide.note }}
              </p>
            </div>
          </HelpTooltip>
        </div>
        <p v-if="paymentGuide" class="mb-3 text-xs text-gray-500 dark:text-gray-400">
          {{ paymentGuide.summary }}
        </p>
        <div class="space-y-3">
          <div v-for="field in resolvedFields" :key="field.key">
            <label class="input-label">
              {{ field.label }}
              <span v-if="field.optional" class="text-xs text-gray-400">({{ t('common.optional') }})</span>
              <span v-else class="text-red-500"> *</span>
            </label>
            <textarea
              v-if="field.sensitive && isMultilineSecretField(field.key)"
              v-model="config[field.key]"
              rows="3"
              class="input font-mono text-xs"
              autocomplete="new-password"
              data-1p-ignore
              data-lpignore="true"
              data-bwignore="true"
              spellcheck="false"
              :placeholder="editing ? t('admin.accounts.leaveEmptyToKeep') : ''"
            />
            <div v-else-if="field.sensitive" class="space-y-2">
              <div class="flex gap-2">
                <div class="relative min-w-0 flex-1">
                  <input
                    :type="visibleFields[field.key] ? 'text' : 'password'"
                    v-model="config[field.key]"
                    class="input pr-10"
                    autocomplete="new-password"
                    data-1p-ignore
                    data-lpignore="true"
                    data-bwignore="true"
                    spellcheck="false"
                    :placeholder="editing ? t('admin.accounts.leaveEmptyToKeep') : (field.defaultValue || '')"
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
                <button
                  v-if="isSepayWebhookApiKeyField(field.key)"
                  type="button"
                  class="btn btn-secondary whitespace-nowrap"
                  @click="generateSepayWebhookApiKey"
                >
                  {{ t('admin.settings.payment.sepayGenerateWebhookApiKey') }}
                </button>
              </div>
              <p v-if="isSepayWebhookApiKeyField(field.key)" class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.payment.sepayWebhookApiKeyHint') }}
              </p>
            </div>
            <div v-else-if="isSepayBankAccountField(field.key)" class="space-y-2">
              <div class="flex gap-2">
                <div class="min-w-0 flex-1">
                  <Select
                    v-model="config[field.key]"
                    :options="sepayBankAccountSelectOptions"
                    :placeholder="t('admin.settings.payment.sepayBankAccountPlaceholder')"
                    :disabled="sepayBankAccountsLoading"
                    searchable
                  />
                </div>
                <button
                  type="button"
                  class="btn btn-secondary whitespace-nowrap"
                  :disabled="sepayBankAccountsLoading || !canLoadSepayBankAccounts"
                  @click="loadSepayBankAccounts"
                >
                  {{ sepayBankAccountsLoading ? t('common.loading') : t('admin.settings.payment.sepayLoadBankAccounts') }}
                </button>
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.payment.sepayBankAccountHint') }}
              </p>
              <p v-if="sepayBankAccountsError" class="text-xs text-red-500">
                {{ sepayBankAccountsError }}
              </p>
            </div>
            <div v-else-if="isManualQRCodeField(field.key)" class="space-y-2">
              <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
                <input
                  type="file"
                  accept="image/*"
                  class="block w-full text-sm text-gray-600 file:mr-3 file:rounded-lg file:border-0 file:bg-primary-50 file:px-3 file:py-2 file:text-sm file:font-medium file:text-primary-700 hover:file:bg-primary-100 dark:text-gray-300 dark:file:bg-primary-900/30 dark:file:text-primary-200"
                  :disabled="manualQrUploading"
                  @change="handleManualQRCodeUpload"
                />
                <button
                  v-if="config.manualQrCodeImg"
                  type="button"
                  class="btn btn-secondary whitespace-nowrap"
                  @click="config.manualQrCodeImg = ''"
                >
                  {{ t('admin.settings.payment.clearManualQRCode') }}
                </button>
              </div>
              <div v-if="config.manualQrCodeImg" class="inline-flex rounded-lg bg-white p-2 shadow-sm dark:bg-dark-800">
                <img :src="config.manualQrCodeImg" :alt="t('admin.settings.payment.manualQRCodePreview')" class="h-32 w-32 object-contain" />
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ manualQrUploading ? t('admin.settings.payment.manualQRCodeUploading') : t('admin.settings.payment.manualQRCodeHint') }}
              </p>
            </div>
            <Select
              v-else-if="field.options?.length"
              v-model="config[field.key]"
              :options="field.options"
              :searchable="field.options.length > 5"
            />
            <input
              v-else
              type="text"
              v-model="config[field.key]"
              class="input"
              :placeholder="field.defaultValue || ''"
            />
            <p v-if="field.hintKey" class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
              {{ t(field.hintKey) }}
            </p>
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

        <!-- 服务商 Webhook 提示 -->
        <div v-if="providerWebhookUrl" class="mt-3 rounded-lg border border-blue-200 bg-blue-50 p-3 dark:border-blue-800/50 dark:bg-blue-900/20">
          <p class="text-xs text-blue-700 dark:text-blue-300">
            {{ t(providerWebhookHint) }}
          </p>
          <code class="mt-1 block break-all rounded bg-blue-100 px-2 py-1 text-xs text-blue-800 dark:bg-blue-900/40 dark:text-blue-200">
            {{ providerWebhookUrl }}
          </code>
          <p v-if="form.provider_key === 'stripe'" class="mt-2 text-xs leading-relaxed text-blue-700 dark:text-blue-300">
            {{ t('admin.settings.payment.stripeWebhookApiVersionHint', { version: STRIPE_SDK_API_VERSION }) }}
          </p>
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
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Select from '@/components/common/Select.vue'
import type { SelectOption } from '@/components/common/Select.vue'
import ToggleSwitch from './ToggleSwitch.vue'
import type { ProviderInstance } from '@/types/payment'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { SepayBankAccountOption } from '@/api/admin/payment'
import type { EasyPayCustomMethod, TypeOption } from './providerConfig'
import {
  PROVIDER_CONFIG_FIELDS,
  PROVIDER_SUPPORTED_TYPES,
  PROVIDER_CALLBACK_PATHS,
  PROVIDER_DEFAULT_PAYMENT_CURRENCIES,
  PAYMENT_CURRENCY_OPTIONS,
  WEBHOOK_PATHS,
  PAYMENT_MODE_QRCODE,
  PAYMENT_MODE_POPUP,
  PAYMENT_MODE_REDIRECT,
  PAYMENT_MODE_MANUAL,
  STRIPE_SDK_API_VERSION,
  defaultProviderPaymentMode,
  getAvailableTypes,
  getProviderPaymentModes,
  isValidProviderPaymentMode,
  extractBaseUrl,
  parseEasyPayCustomMethods,
  serializeEasyPayCustomMethods,
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
    allow_user_refund: boolean
    config: Record<string, string>
    limits: string
  }]
}>()

const { t } = useI18n()

interface PaymentGuideItem {
  title: string
  open: string
  call: string
  fallback: string
}

interface PaymentGuide {
  summary: string
  items: PaymentGuideItem[]
  note?: string
}

// --- Form state ---
const form = reactive({
  name: '',
  provider_key: 'easypay',
  supported_types: [] as string[],
  enabled: true,
  payment_mode: PAYMENT_MODE_QRCODE,
  refund_enabled: false,
  allow_user_refund: false,
})
const config = reactive<Record<string, string>>({})
const limits = reactive<Record<string, Record<string, number | string[]>>>({})
const notifyBaseUrl = ref('')
const returnBaseUrl = ref('')
const limitsExpanded = ref(false)
const paymentCurrencyDraft = ref('')
const visibleFields = reactive<Record<string, boolean>>({})
const easyPayCustomMethods = reactive<EasyPayCustomMethod[]>([])
const sepayBankAccounts = ref<SepayBankAccountOption[]>([])
const sepayBankAccountsLoading = ref(false)
const sepayBankAccountsError = ref('')
const manualQrUploading = ref(false)

// --- Computed ---
const defaultBaseUrl = typeof window !== 'undefined' ? window.location.origin : ''

const providerWebhookHintMap: Record<string, string> = {
  stripe: 'admin.settings.payment.stripeWebhookHint',
  airwallex: 'admin.settings.payment.airwallexWebhookHint',
}

const providerWebhookUrl = computed(() => {
  const path = WEBHOOK_PATHS[form.provider_key]
  return providerWebhookHintMap[form.provider_key] && path ? defaultBaseUrl + path : ''
})

const providerWebhookHint = computed(() =>
  providerWebhookHintMap[form.provider_key] || 'admin.settings.payment.stripeWebhookHint',
)

const callbackPaths = computed(() => {
  if (supportsManualPaymentMode.value && form.payment_mode === PAYMENT_MODE_MANUAL) return null
  return PROVIDER_CALLBACK_PATHS[form.provider_key] || null
})

const supportsManualPaymentMode = computed(() => ['alipay', 'wxpay'].includes(form.provider_key))
const supportsPaymentModeSelector = computed(() => getProviderPaymentModes(form.provider_key).length > 0)

const paymentModeOptions = computed(() => {
  const labels: Record<string, string> = {
    [PAYMENT_MODE_QRCODE]: t('admin.settings.payment.modeQRCode'),
    [PAYMENT_MODE_POPUP]: t('admin.settings.payment.modePopup'),
    [PAYMENT_MODE_REDIRECT]: t('admin.settings.payment.modeRedirect'),
    [PAYMENT_MODE_MANUAL]: t('admin.settings.payment.modeManual')
  }
  return getProviderPaymentModes(form.provider_key).map(value => ({
    value,
    label: labels[value] || value
  }))
})

const availableTypes = computed(() => {
  const base = getAvailableTypes(form.provider_key, props.allPaymentTypes, props.redirectLabel)
  if (form.provider_key === 'easypay') {
    for (const method of normalizedEasyPayCustomMethods()) {
      if (!base.some(opt => opt.value === method.type)) {
        base.push({
          value: method.type,
          label: method.displayName || method.type,
        })
      }
    }
  }
  // Resolve i18n labels for types not in allPaymentTypes (e.g. card, link inside stripe)
  return base.map(opt =>
    opt.label === opt.value
      ? { ...opt, label: t(`payment.methods.${opt.value}`, opt.value) }
      : opt,
  )
})

const resolvedFields = computed(() => {
  const fields = PROVIDER_CONFIG_FIELDS[form.provider_key] || []
  const visibleProviderFields = supportsManualPaymentMode.value && form.payment_mode === PAYMENT_MODE_MANUAL
    ? fields.filter(f => f.key === 'manualQrCodeImg')
    : fields
  return visibleProviderFields.map(f => ({
    ...f,
    label: f.label || t(`admin.settings.payment.field_${f.key}`),
  }))
})

const paymentGuide = computed<PaymentGuide | null>(() => {
  if (supportsManualPaymentMode.value && form.payment_mode === PAYMENT_MODE_MANUAL) return null

  if (form.provider_key === 'alipay') {
    return {
      summary: t('admin.settings.payment.alipayGuideSummary'),
      items: [
        {
          title: t('admin.settings.payment.alipayGuideFaceToFaceTitle'),
          open: t('admin.settings.payment.alipayGuideFaceToFaceOpen'),
          call: t('admin.settings.payment.alipayGuideFaceToFaceCall'),
          fallback: t('admin.settings.payment.alipayGuideFaceToFaceFallback'),
        },
        {
          title: t('admin.settings.payment.alipayGuidePagePayTitle'),
          open: t('admin.settings.payment.alipayGuidePagePayOpen'),
          call: t('admin.settings.payment.alipayGuidePagePayCall'),
          fallback: t('admin.settings.payment.alipayGuidePagePayFallback'),
        },
        {
          title: t('admin.settings.payment.alipayGuideWapTitle'),
          open: t('admin.settings.payment.alipayGuideWapOpen'),
          call: t('admin.settings.payment.alipayGuideWapCall'),
          fallback: t('admin.settings.payment.alipayGuideWapFallback'),
        },
      ],
    }
  }

  if (form.provider_key === 'wxpay') {
    return {
      summary: t('admin.settings.payment.wxpayGuideSummary'),
      note: t('admin.settings.payment.wxpayGuideNote'),
      items: [
        {
          title: t('admin.settings.payment.wxpayGuideNativeTitle'),
          open: t('admin.settings.payment.wxpayGuideNativeOpen'),
          call: t('admin.settings.payment.wxpayGuideNativeCall'),
          fallback: t('admin.settings.payment.wxpayGuideNativeFallback'),
        },
        {
          title: t('admin.settings.payment.wxpayGuideJsapiTitle'),
          open: t('admin.settings.payment.wxpayGuideJsapiOpen'),
          call: t('admin.settings.payment.wxpayGuideJsapiCall'),
          fallback: t('admin.settings.payment.wxpayGuideJsapiFallback'),
        },
        {
          title: t('admin.settings.payment.wxpayGuideH5Title'),
          open: t('admin.settings.payment.wxpayGuideH5Open'),
          call: t('admin.settings.payment.wxpayGuideH5Call'),
          fallback: t('admin.settings.payment.wxpayGuideH5Fallback'),
        },
      ],
    }
  }

  if (form.provider_key === 'airwallex') {
    return {
      summary: t('admin.settings.payment.airwallexGuideSummary'),
      note: t('admin.settings.payment.airwallexGuideNote'),
      items: [],
    }
  }

  if (form.provider_key === 'sepay') {
    return {
      summary: t('admin.settings.payment.sepayGuideSummary'),
      note: t('admin.settings.payment.sepayGuideNote'),
      items: [
        {
          title: t('admin.settings.payment.sepayGuideApiTokenTitle'),
          open: t('admin.settings.payment.sepayGuideApiTokenOpen'),
          call: t('admin.settings.payment.sepayGuideApiTokenCall'),
          fallback: t('admin.settings.payment.sepayGuideApiTokenFallback'),
        },
        {
          title: t('admin.settings.payment.sepayGuideWebhookTitle'),
          open: t('admin.settings.payment.sepayGuideWebhookOpen'),
          call: t('admin.settings.payment.sepayGuideWebhookCall'),
          fallback: t('admin.settings.payment.sepayGuideWebhookFallback'),
        },
      ],
    }
  }

  return null
})

function normalizePaymentCurrencyCode(value: unknown): string {
  const code = String(value || '').trim().toUpperCase()
  return /^[A-Z]{3}$/.test(code) ? code : ''
}

function formatPaymentCurrencyLabel(currency: string): string {
  const code = normalizePaymentCurrencyCode(currency)
  const symbols: Record<string, string> = {
    USD: '$',
    CNY: '¥',
    VND: '₫',
    KRW: '₩',
    EUR: '€',
    JPY: '¥',
    GBP: '£',
  }
  return `${symbols[code] ? `${symbols[code]} ` : ''}${code || currency}`
}

const paymentCurrencyOptions = computed<SelectOption[]>(() => PAYMENT_CURRENCY_OPTIONS.map(option => ({
  value: String(option.value),
  label: formatPaymentCurrencyLabel(String(option.value)),
})))

const selectedPaymentCurrencies = computed(() => parseCurrencyList(config.allowed_payment_currencies))

const selectedPaymentCurrencyOptions = computed(() => selectedPaymentCurrencies.value.map(currency => ({
  value: currency,
  label: formatPaymentCurrencyLabel(currency),
})))

const normalizedPaymentCurrencyDraft = computed(() => normalizePaymentCurrencyCode(paymentCurrencyDraft.value))

const availablePaymentCurrencySelectOptions = computed<SelectOption[]>(() => {
  const selected = new Set(selectedPaymentCurrencies.value)
  const options = paymentCurrencyOptions.value.filter(option => !selected.has(String(option.value)))
  const draft = normalizedPaymentCurrencyDraft.value
  if (draft && !selected.has(draft) && !options.some(option => option.value === draft)) {
    options.unshift({
      value: draft,
      label: formatPaymentCurrencyLabel(draft),
    })
  }
  return options
})

const providerCurrencyBadgeLabel = computed(() => {
  const currencies = selectedPaymentCurrencies.value
  return currencies.length > 0
    ? currencies.join(', ')
    : t('admin.settings.payment.providerCurrencyConfigured')
})

const sepayBankAccountSelectOptions = computed<SelectOption[]>(() => {
  const options = sepayBankAccounts.value.map(account => ({
    value: account.id,
    label: account.label || formatSepayBankAccountLabel(account),
  }))
  const current = (config.bankAccountId || '').trim()
  if (current && !options.some(option => option.value === current)) {
    options.unshift({ value: current, label: t('admin.settings.payment.sepayStoredBankAccount', { id: current }) })
  }
  return options
})

const canLoadSepayBankAccounts = computed(() => {
  if (form.provider_key !== 'sepay') return false
  return !!(config.apiToken || '').trim() || !!props.editing?.id
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

function parseCurrencyList(raw: string | undefined): string[] {
  return (raw || '')
    .split(',')
    .map(value => normalizePaymentCurrencyCode(value))
    .filter((value, index, list) => !!value && list.indexOf(value) === index)
}

function syncPaymentCurrencies(currencies: string[]) {
  const normalized = parseCurrencyList(currencies.join(','))
  if (normalized.length === 0) {
    delete config.allowed_payment_currencies
    for (const fields of Object.values(limits)) delete fields.allowedPaymentCurrencies
    return
  }
  config.allowed_payment_currencies = normalized.join(',')
  for (const pt of form.supported_types) {
    if (!limits[pt]) limits[pt] = {}
    limits[pt].allowedPaymentCurrencies = [...normalized]
  }
}

function parseCurrencyValue(value: unknown): string[] {
  if (Array.isArray(value)) return parseCurrencyList(value.join(','))
  if (typeof value === 'string') return parseCurrencyList(value)
  return []
}

function hydratePaymentCurrenciesFromLimits(): boolean {
  for (const pt of form.supported_types) {
    const fields = limits[pt] as Record<string, unknown> | undefined
    if (!fields) continue
    for (const key of ['allowedPaymentCurrencies', 'allowed_payment_currencies', 'paymentCurrencies', 'payment_currencies', 'currencies']) {
      const currencies = parseCurrencyValue(fields[key])
      if (currencies.length > 0) {
        syncPaymentCurrencies(currencies)
        return true
      }
    }
  }
  return false
}

function applyDefaultPaymentCurrencies() {
  if (parseCurrencyList(config.allowed_payment_currencies).length > 0) return
  syncPaymentCurrencies(PROVIDER_DEFAULT_PAYMENT_CURRENCIES[form.provider_key] || ['USD'])
}

function addPaymentCurrencySelection(value: string | number | boolean | null) {
  const currency = normalizePaymentCurrencyCode(value)
  if (!currency) return
  const next = selectedPaymentCurrencies.value.includes(currency)
    ? selectedPaymentCurrencies.value
    : [...selectedPaymentCurrencies.value, currency]
  syncPaymentCurrencies(next)
  paymentCurrencyDraft.value = ''
}

function removePaymentCurrency(currency: string) {
  const normalized = normalizePaymentCurrencyCode(currency)
  const next = selectedPaymentCurrencies.value.filter(value => value !== normalized)
  syncPaymentCurrencies(next)
}

function toggleType(type: string) {
  if (form.supported_types.includes(type)) {
    form.supported_types = form.supported_types.filter(t => t !== type)
  } else {
    form.supported_types = [...form.supported_types, type]
    const currencies = selectedPaymentCurrencies.value
    if (currencies.length > 0) {
      if (!limits[type]) limits[type] = {}
      limits[type].allowedPaymentCurrencies = [...currencies]
    }
  }
}

function normalizedEasyPayCustomMethods(): EasyPayCustomMethod[] {
  return easyPayCustomMethods
    .map(method => ({
      type: normalizeEasyPayCustomMethodCode(method.type),
      upstreamType: normalizeEasyPayCustomMethodCode(method.upstreamType),
      displayName: method.displayName.trim(),
    }))
    .filter(method => method.type || method.upstreamType || method.displayName)
}

function normalizeEasyPayCustomMethodCode(value: string): string {
  return value.trim().toLowerCase()
}

function addEasyPayCustomMethod() {
  easyPayCustomMethods.push({ type: '', upstreamType: '', displayName: '' })
}

function removeEasyPayCustomMethod(index: number) {
  easyPayCustomMethods.splice(index, 1)
}

function isSepayBankAccountField(key: string): boolean {
  return form.provider_key === 'sepay' && key === 'bankAccountId'
}

function isSepayWebhookApiKeyField(key: string): boolean {
  return form.provider_key === 'sepay' && key === 'webhookApiKey'
}

function isManualQRCodeField(key: string): boolean {
  return supportsManualPaymentMode.value && key === 'manualQrCodeImg'
}

function isConfigFieldRequired(field: { key: string; optional?: boolean; sensitive?: boolean }): boolean {
  if (supportsManualPaymentMode.value && form.payment_mode === PAYMENT_MODE_MANUAL) {
    return field.key === 'manualQrCodeImg'
  }
  if (field.optional) return false
  return true
}

function isMultilineSecretField(key: string): boolean {
  return ['privateKey', 'publicKey', 'alipayPublicKey'].includes(key)
}

function formatSepayBankAccountLabel(account: SepayBankAccountOption): string {
  const parts = [account.bank_short_name, account.account_number, account.account_holder_name]
    .map(part => (part || '').trim())
    .filter(Boolean)
  return parts.join(' · ') || account.id
}

async function handleManualQRCodeUpload(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!file.type.startsWith('image/')) {
    emitValidationError(t('admin.settings.payment.manualQRCodeImageRequired'))
    return
  }
  if (file.size > 1024 * 1024) {
    emitValidationError(t('admin.settings.payment.manualQRCodeTooLarge'))
    return
  }
  manualQrUploading.value = true
  try {
    const res = await adminPaymentAPI.uploadManualQRCode(file)
    config.manualQrCodeImg = res.data.url
  } catch (err: unknown) {
    emitValidationError(getErrorMessage(err))
  } finally {
    manualQrUploading.value = false
  }
}

function resetSepayBankAccounts() {
  sepayBankAccounts.value = []
  sepayBankAccountsError.value = ''
  sepayBankAccountsLoading.value = false
}

function generateSepayWebhookApiKey() {
  const cryptoObj = globalThis.crypto
  if (!cryptoObj?.getRandomValues) {
    emitValidationError(t('admin.settings.payment.sepayWebhookTokenGenerationFailed'))
    return
  }
  const bytes = new Uint8Array(32)
  cryptoObj.getRandomValues(bytes)
  config.webhookApiKey = `vcwh_${Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')}`
  visibleFields.webhookApiKey = true
}

function getErrorMessage(err: unknown): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const message = (err as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }
  if (err instanceof Error && err.message.trim()) return err.message
  return t('admin.settings.payment.sepayLoadBankAccountsFailed')
}

async function loadSepayBankAccounts() {
  sepayBankAccountsError.value = ''
  const apiToken = (config.apiToken || '').trim()
  const providerId = props.editing?.id
  if (!apiToken && !providerId) {
    sepayBankAccountsError.value = t('admin.settings.payment.sepayApiTokenRequired')
    return
  }
  sepayBankAccountsLoading.value = true
  try {
    const request = apiToken ? { apiToken } : { providerId }
    const res = await adminPaymentAPI.listSepayBankAccounts(request)
    const accounts = res.data || []
    sepayBankAccounts.value = accounts
    const current = (config.bankAccountId || '').trim()
    if (accounts.length === 1 && !current) {
      config.bankAccountId = accounts[0].id
    }
    if (accounts.length === 0) {
      sepayBankAccountsError.value = t('admin.settings.payment.sepayNoBankAccounts')
    }
  } catch (err: unknown) {
    sepayBankAccounts.value = []
    sepayBankAccountsError.value = getErrorMessage(err)
  } finally {
    sepayBankAccountsLoading.value = false
  }
}

function onKeyChange() {
  form.supported_types = [...(PROVIDER_SUPPORTED_TYPES[form.provider_key] || [])]
  form.payment_mode = defaultProviderPaymentMode(form.provider_key)
  clearConfig()
  applyDefaults()
}

function clearConfig() {
  Object.keys(config).forEach(k => delete config[k])
  Object.keys(limits).forEach(k => delete limits[k])
  Object.keys(visibleFields).forEach(k => delete visibleFields[k])
  notifyBaseUrl.value = ''
  returnBaseUrl.value = ''
  paymentCurrencyDraft.value = ''
  limitsExpanded.value = false
  easyPayCustomMethods.splice(0, easyPayCustomMethods.length)
  resetSepayBankAccounts()
}

function applyDefaults() {
  for (const f of PROVIDER_CONFIG_FIELDS[form.provider_key] || []) {
    if (f.defaultValue && !config[f.key]) config[f.key] = f.defaultValue
  }
  applyDefaultPaymentCurrencies()
}

function getLimitVal(paymentType: string, field: string): string {
  const val = limits[paymentType]?.[field]
  return typeof val === 'number' && val > 0 ? String(val) : ''
}

/** Returns true if any limit field for this payment type has a value */
function hasAnyLimit(paymentType: string): boolean {
  const l = limits[paymentType]
  if (!l) return false
  return (numericLimitValue(l.singleMin) > 0) || (numericLimitValue(l.singleMax) > 0) || (numericLimitValue(l.dailyLimit) > 0)
}

function numericLimitValue(value: number | string[] | undefined): number {
  return typeof value === 'number' ? value : 0
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
  syncPaymentCurrencies(selectedPaymentCurrencies.value)
  const result: Record<string, Record<string, number | string[]>> = {}
  for (const [pt, fields] of Object.entries(limits)) {
    const clean: Record<string, number | string[]> = {}
    for (const [k, v] of Object.entries(fields)) {
      if (Array.isArray(v)) {
        const currencies = parseCurrencyList(v.join(','))
        if (currencies.length > 0) clean[k] = currencies
        continue
      }
      if (typeof v === 'number' && v > 0) clean[k] = v
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
  if (!isValidProviderPaymentMode(form.provider_key, form.payment_mode)) {
    form.payment_mode = defaultProviderPaymentMode(form.provider_key)
  }
  if (form.provider_key === 'easypay') {
    const validationError = validateEasyPayCustomMethods()
    if (validationError) {
      emitValidationError(validationError)
      return
    }
    syncEasyPayCustomMethods()
  }
  // Validate required config fields — all non-optional fields must be filled.
  // In edit mode, sensitive fields may be left blank to preserve the stored
  // value (backend merges blanks by preserving the existing secret).
  for (const f of PROVIDER_CONFIG_FIELDS[form.provider_key] || []) {
    if (!isConfigFieldRequired(f)) continue
    if (props.editing && f.sensitive) continue
    const val = (config[f.key] || '').trim()
    if (!val) {
      const label = f.label || t(`admin.settings.payment.field_${f.key}`)
      emitValidationError(t('admin.settings.payment.validationFieldRequired', { field: label }))
      return
    }
  }

  const clearableConfigKeys = new Set(
    (PROVIDER_CONFIG_FIELDS[form.provider_key] || [])
      .filter(field => field.clearable)
      .map(field => field.key),
  )
  const filteredConfig: Record<string, string> = {}
  for (const [k, v] of Object.entries(config)) {
    if (!v || !v.trim()) {
      if (clearableConfigKeys.has(k)) {
        filteredConfig[k] = ''
      }
      continue
    }
    filteredConfig[k] = v
  }
  if (form.provider_key === 'easypay') {
    filteredConfig.customMethods = serializeEasyPayCustomMethods(normalizedEasyPayCustomMethods())
  }
  if (selectedPaymentCurrencies.value.length === 0) {
    emitValidationError(t('admin.settings.payment.providerPaymentCurrenciesRequired'))
    return
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
    payment_mode: supportsPaymentModeSelector.value ? form.payment_mode : '',
    refund_enabled: form.refund_enabled,
    allow_user_refund: form.refund_enabled ? form.allow_user_refund : false,
    config: filteredConfig,
    limits: serializeLimits(),
  })
}

function syncEasyPayCustomMethods(): string[] {
  if (form.provider_key !== 'easypay') return []
  const baseTypes = new Set(PROVIDER_SUPPORTED_TYPES.easypay || [])
  const customTypes: string[] = []
  const seen = new Set<string>()
  for (const method of normalizedEasyPayCustomMethods()) {
    if (!method.type || !method.upstreamType) continue
    if (seen.has(method.type)) continue
    seen.add(method.type)
    customTypes.push(method.type)
  }
  form.supported_types = form.supported_types
    .map(type => normalizeEasyPayCustomMethodCode(type))
    .filter(type => baseTypes.has(type) || customTypes.includes(type))
  for (const customType of customTypes) {
    if (!form.supported_types.includes(customType)) {
      form.supported_types.push(customType)
    }
  }
  return customTypes
}

function validateEasyPayCustomMethods(): string | null {
  const seen = new Set<string>()
  for (const method of normalizedEasyPayCustomMethods()) {
    const hasAnyValue = Boolean(method.type || method.upstreamType || method.displayName)
    if (!hasAnyValue) continue
    if (!method.type || !method.upstreamType) {
      return t('admin.settings.payment.validationEasyPayCustomMethodRequired')
    }
    if (!/^[a-z0-9_-]+$/.test(method.type)) {
      return t('admin.settings.payment.validationEasyPayCustomMethodTypeInvalid')
    }
    if (!/^[a-z0-9_-]+$/.test(method.upstreamType)) {
      return t('admin.settings.payment.validationEasyPayCustomMethodUpstreamTypeInvalid')
    }
    if ((PROVIDER_SUPPORTED_TYPES.easypay || []).includes(method.type)) {
      return t('admin.settings.payment.validationEasyPayCustomMethodReserved')
    }
    if (method.type.startsWith('alipay') || method.type.startsWith('wxpay')) {
      return t('admin.settings.payment.validationEasyPayCustomMethodPrefixReserved')
    }
    if (seen.has(method.type)) {
      return t('admin.settings.payment.validationEasyPayCustomMethodDuplicate')
    }
    seen.add(method.type)
  }
  return null
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
  form.payment_mode = defaultProviderPaymentMode(defaultKey)
  form.refund_enabled = false
  form.allow_user_refund = false
  clearConfig()
  applyDefaults()
}

function loadProvider(provider: ProviderInstance) {
  form.name = provider.name
  form.provider_key = provider.provider_key
  form.supported_types = Array.isArray(provider.supported_types)
    ? [...provider.supported_types]
    : []
  form.enabled = provider.enabled
  form.payment_mode = isValidProviderPaymentMode(provider.provider_key, provider.payment_mode || '')
    ? (provider.payment_mode || '')
    : defaultProviderPaymentMode(provider.provider_key)
  form.refund_enabled = provider.refund_enabled
  form.allow_user_refund = provider.allow_user_refund
  clearConfig()
  // Pre-fill config from API response. Backend omits sensitive fields entirely,
  // so those inputs stay blank — submitting blank preserves the stored secret.
  if (provider.config) {
    for (const [k, v] of Object.entries(provider.config)) {
      // Skip notifyUrl/returnUrl — they are derived from callbackBaseUrl
      if (k === 'notifyUrl' || k === 'returnUrl') continue
      if (k === 'customMethods' && provider.provider_key === 'easypay') {
        easyPayCustomMethods.push(...parseEasyPayCustomMethods(v))
        continue
      }
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
  // Parse existing limits before applying defaults so old providers whose
  // capabilities live only in limits still hydrate the currency selector.
  if (provider.limits) {
    try {
      const parsed = JSON.parse(provider.limits)
      for (const [pt, fields] of Object.entries(parsed as Record<string, Record<string, number | string[]>>)) {
        limits[pt] = { ...fields }
      }
      limitsExpanded.value = Object.keys(limits).length > 0
    } catch { /* ignore */ }
  }
  hydratePaymentCurrenciesFromLimits()
  applyDefaults()
}

defineExpose({ reset, loadProvider })
</script>
