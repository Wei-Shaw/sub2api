<!--
  PaymentSettingsView — standalone admin page mounted at /admin/payment/settings.

  Mirrors the release/custom-0.1.121 host SettingsView payment tab (lines
  ~4694-5160): a single global save button + 5 inline form rows + provider
  list. The plugin no longer surfaces a custom settings component inside
  the plugin-management dialog (SettingsComponentPath was removed in
  manifest); admins reach payment configuration via the dedicated sidebar
  entry under "支付管理 / Payment".

  Design points kept verbatim from release:
    - flat reactive `form` with `payment_*` field names
    - one global save (no per-section save)
    - `<PaymentProviderList>` rendered only when payment_enabled is true
    - badge-style enabled_payment_types selector
-->
<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Page header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">
            {{ t('admin.settings.payment.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.payment.description') }}
          </p>
        </div>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="saving || loading"
          @click="saveSettings"
        >
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>

      <!-- Loading state -->
      <div v-if="loading" class="card p-6 text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>

      <template v-else>
        <!-- Payment system settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.payment.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.payment.description') }}
            </p>
          </div>
          <div class="space-y-4 p-6">
            <!-- Enable toggle -->
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">
                  {{ t('admin.settings.payment.enabled') }}
                </label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.payment.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.payment_enabled" />
            </div>

            <template v-if="form.payment_enabled">
              <!-- Row 1: Product name prefix / suffix / preview -->
              <div class="grid grid-cols-3 gap-3">
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.productNamePrefix') }}
                  </label>
                  <input
                    v-model="form.payment_product_name_prefix"
                    type="text"
                    class="input"
                    placeholder="Sub2API"
                  />
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.productNameSuffix') }}
                  </label>
                  <input
                    v-model="form.payment_product_name_suffix"
                    type="text"
                    class="input"
                    placeholder="CNY"
                  />
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.preview') }}
                  </label>
                  <div
                    class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300"
                  >
                    {{
                      (form.payment_product_name_prefix || 'Sub2API') +
                      ' 100 ' +
                      (form.payment_product_name_suffix || 'CNY')
                    }}
                  </div>
                </div>
              </div>

              <!-- Row 2: Amount range + multiplier + fee rate + order timeout -->
              <div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.minAmount') }}
                  </label>
                  <input
                    v-model="form.payment_min_amount"
                    type="number"
                    step="0.01"
                    min="0"
                    class="input"
                    :placeholder="t('admin.settings.payment.noLimit')"
                  />
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.maxAmount') }}
                  </label>
                  <input
                    v-model="form.payment_max_amount"
                    type="number"
                    step="0.01"
                    min="0"
                    class="input"
                    :placeholder="t('admin.settings.payment.noLimit')"
                  />
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.dailyLimit') }}
                  </label>
                  <input
                    v-model="form.payment_daily_limit"
                    type="number"
                    step="0.01"
                    min="0"
                    class="input"
                    :placeholder="t('admin.settings.payment.noLimit')"
                  />
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.balanceRechargeMultiplier') }}
                  </label>
                  <input
                    v-model="form.payment_balance_recharge_multiplier"
                    type="number"
                    step="0.01"
                    min="0.01"
                    class="input"
                  />
                  <p class="mt-0.5 text-xs text-gray-400">
                    {{ t('admin.settings.payment.balanceRechargeMultiplierHint') }}
                  </p>
                  <p class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400">
                    {{
                      t('admin.settings.payment.balanceRechargePreview', {
                        usd: balanceMultiplierPreview,
                      })
                    }}
                  </p>
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.rechargeFeeRate') }}
                  </label>
                  <div class="relative">
                    <input
                      v-model="form.payment_recharge_fee_rate"
                      type="number"
                      step="0.01"
                      min="0"
                      max="100"
                      class="input pr-8"
                    />
                    <span
                      class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400"
                      >%</span
                    >
                  </div>
                  <p class="mt-0.5 text-xs text-gray-400">
                    {{ t('admin.settings.payment.rechargeFeeRateHint') }}
                  </p>
                  <p
                    v-if="hasFeeRate"
                    class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400"
                  >
                    {{
                      t('admin.settings.payment.rechargeFeePreview', {
                        fee: feeRatePreview,
                      })
                    }}
                  </p>
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.orderTimeout') }}
                    <span class="text-red-500">*</span>
                  </label>
                  <input
                    v-model.number="form.payment_order_timeout_minutes"
                    type="number"
                    min="1"
                    class="input"
                    required
                  />
                  <p class="mt-0.5 text-xs text-gray-400">
                    {{ t('admin.settings.payment.orderTimeoutHint') }}
                  </p>
                </div>
              </div>

              <!-- Row 3: Pending orders + load balance + cancel rate limit (inline) -->
              <div class="flex flex-wrap items-end gap-4">
                <div class="w-28">
                  <label class="input-label">
                    {{ t('admin.settings.payment.maxPendingOrders') }}
                  </label>
                  <input
                    v-model.number="form.payment_max_pending_orders"
                    type="number"
                    min="1"
                    class="input"
                  />
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.loadBalanceStrategy') }}
                  </label>
                  <Select
                    v-model="form.payment_load_balance_strategy"
                    :options="loadBalanceOptions"
                    class="w-40"
                  />
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.cancelRateLimit') }}
                  </label>
                  <div class="flex items-center gap-2">
                    <button
                      type="button"
                      :class="[
                        'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                        form.payment_cancel_rate_limit_enabled
                          ? 'bg-primary-500'
                          : 'bg-gray-300 dark:bg-dark-600',
                      ]"
                      @click="
                        form.payment_cancel_rate_limit_enabled =
                          !form.payment_cancel_rate_limit_enabled
                      "
                    >
                      <span
                        :class="[
                          'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                          form.payment_cancel_rate_limit_enabled
                            ? 'translate-x-5'
                            : 'translate-x-0',
                        ]"
                      />
                    </button>
                    <Select
                      v-model="form.payment_cancel_rate_limit_window_mode"
                      :options="cancelRateLimitModeOptions"
                      class="w-24"
                      :disabled="!form.payment_cancel_rate_limit_enabled"
                    />
                    <span
                      :class="[
                        'whitespace-nowrap text-sm',
                        form.payment_cancel_rate_limit_enabled
                          ? 'text-gray-700 dark:text-gray-300'
                          : 'text-gray-400 dark:text-gray-600',
                      ]"
                      >{{ t('admin.settings.payment.cancelRateLimitEvery') }}</span
                    >
                    <input
                      v-model.number="form.payment_cancel_rate_limit_window"
                      type="number"
                      min="1"
                      required
                      class="input w-14 text-center"
                      :disabled="!form.payment_cancel_rate_limit_enabled"
                    />
                    <Select
                      v-model="form.payment_cancel_rate_limit_unit"
                      :options="cancelRateLimitUnitOptions"
                      class="w-28"
                      :disabled="!form.payment_cancel_rate_limit_enabled"
                    />
                    <span
                      :class="[
                        'whitespace-nowrap text-sm',
                        form.payment_cancel_rate_limit_enabled
                          ? 'text-gray-700 dark:text-gray-300'
                          : 'text-gray-400 dark:text-gray-600',
                      ]"
                      >{{ t('admin.settings.payment.cancelRateLimitAllowMax') }}</span
                    >
                    <input
                      v-model.number="form.payment_cancel_rate_limit_max"
                      type="number"
                      min="1"
                      required
                      class="input w-14 text-center"
                      :disabled="!form.payment_cancel_rate_limit_enabled"
                    />
                    <span
                      :class="[
                        'whitespace-nowrap text-sm',
                        form.payment_cancel_rate_limit_enabled
                          ? 'text-gray-700 dark:text-gray-300'
                          : 'text-gray-400 dark:text-gray-600',
                      ]"
                      >{{ t('admin.settings.payment.cancelRateLimitTimes') }}</span
                    >
                  </div>
                </div>
              </div>

              <!-- Row 4: Enabled payment types (badges) -->
              <div>
                <label class="input-label">
                  {{ t('admin.settings.payment.enabledPaymentTypes') }}
                </label>
                <div class="mt-1.5 flex flex-wrap gap-2">
                  <button
                    v-for="pt in allPaymentTypes"
                    :key="pt.value"
                    type="button"
                    :class="[
                      'rounded-lg border px-3 py-1.5 text-sm font-medium transition-all',
                      form.payment_enabled_types.includes(pt.value)
                        ? 'border-primary-500 bg-primary-500 text-white shadow-sm'
                        : 'border-gray-300 bg-white text-gray-600 hover:border-gray-400 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500',
                    ]"
                    @click="togglePaymentType(pt.value)"
                  >
                    {{ pt.label }}
                  </button>
                </div>
                <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
                  {{ t('admin.settings.payment.enabledPaymentTypesHint') }}
                </p>
              </div>

              <!-- Row 5: Help image URL + help text -->
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.helpImage') }}
                  </label>
                  <input
                    v-model="form.payment_help_image_url"
                    type="text"
                    class="input"
                    :placeholder="t('admin.settings.payment.helpImagePlaceholder')"
                  />
                  <div
                    v-if="form.payment_help_image_url"
                    class="mt-2 overflow-hidden rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800"
                  >
                    <img
                      :src="form.payment_help_image_url"
                      class="mx-auto max-h-32 object-contain"
                      alt=""
                    />
                  </div>
                </div>
                <div>
                  <label class="input-label">
                    {{ t('admin.settings.payment.helpText') }}
                  </label>
                  <textarea
                    v-model="form.payment_help_text"
                    rows="3"
                    class="input"
                    :placeholder="t('admin.settings.payment.helpTextPlaceholder')"
                  ></textarea>
                </div>
              </div>
            </template>
          </div>
        </div>

        <!-- Provider management list -->
        <PaymentProviderList
          v-if="form.payment_enabled"
          :providers="providers"
          :loading="providersLoading"
          :can-create="hasAnyPaymentTypeEnabled"
          :enabled-payment-types="form.payment_enabled_types"
          :all-payment-types="allPaymentTypes"
          :redirect-label="t('admin.settings.payment.easypayRedirect')"
          @refresh="loadProviders"
          @create="openCreateProvider"
          @edit="openEditProvider"
          @delete="confirmDeleteProvider"
          @toggle-field="handleToggleField"
          @toggle-type="handleToggleType"
          @reorder="handleReorderProviders"
        />

        <!-- Provider create / edit dialog -->
        <PaymentProviderDialog
          ref="providerDialogRef"
          :show="showProviderDialog"
          :saving="providerSaving"
          :editing="editingProvider"
          :all-key-options="providerKeyOptions"
          :enabled-key-options="enabledProviderKeyOptions"
          :all-payment-types="allPaymentTypes"
          :redirect-label="t('admin.settings.payment.easypayRedirect')"
          @close="onDialogClose"
          @save="handleSaveProvider"
        />

        <!-- Delete confirmation -->
        <ConfirmDialog
          :show="showDeleteProviderDialog"
          :title="t('admin.settings.payment.deleteProvider')"
          :message="t('admin.settings.payment.deleteProviderConfirm')"
          :confirm-text="t('common.delete')"
          danger
          @confirm="handleDeleteProvider"
          @cancel="showDeleteProviderDialog = false"
        />
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ConfirmDialog, Select, Toggle } from '@sub2api/plugin-sdk'

import { useAppStore } from '../../stores/host'
import { adminPaymentAPI } from '../../api/admin/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '../../utils/apiError'
import { money, formatMoney } from '../../utils/decimal'
import type { ProviderInstance } from '../../types/payment'

import AppLayout from '../../components/common/AppLayout.vue'
import PaymentProviderList from '../../components/payment/PaymentProviderList.vue'
import PaymentProviderDialog from '../../components/payment/PaymentProviderDialog.vue'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== Form state ====================

// Money fields stay as decimal strings so the user can type "0.01" without
// JS Number coercion. The Go backend's shopspring/decimal accepts strings
// directly. Empty string == "no value" for the optional limit fields.
const form = reactive({
  payment_enabled: false,
  payment_min_amount: '',
  payment_max_amount: '',
  payment_daily_limit: '',
  payment_order_timeout_minutes: 30,
  payment_max_pending_orders: 3,
  payment_balance_disabled: false,
  payment_balance_recharge_multiplier: '1',
  payment_recharge_fee_rate: '0',
  payment_load_balance_strategy: 'round-robin',
  payment_product_name_prefix: '',
  payment_product_name_suffix: '',
  payment_help_image_url: '',
  payment_help_text: '',
  payment_cancel_rate_limit_enabled: false,
  payment_cancel_rate_limit_max: 0,
  payment_cancel_rate_limit_window: 0,
  payment_cancel_rate_limit_unit: 'minute',
  payment_cancel_rate_limit_window_mode: 'rolling',
  payment_visible_method_alipay_enabled: true,
  payment_visible_method_alipay_source: 'official',
  payment_visible_method_wxpay_enabled: true,
  payment_visible_method_wxpay_source: 'official',
  payment_enabled_types: [] as string[],
})

const loading = ref(true)
const saving = ref(false)

// ==================== Provider state ====================

const providers = ref<ProviderInstance[]>([])
const providersLoading = ref(false)

const showProviderDialog = ref(false)
const showDeleteProviderDialog = ref(false)
const providerSaving = ref(false)
const editingProvider = ref<ProviderInstance | null>(null)
const deletingProviderId = ref<number | null>(null)
const providerDialogRef = ref<InstanceType<typeof PaymentProviderDialog> | null>(null)

// ==================== Computed (option lists & helpers) ====================

const allPaymentTypes = computed(() => [
  { value: 'easypay', label: t('admin.settings.payment.providerEasypay') },
  { value: 'alipay', label: t('admin.settings.payment.providerAlipay') },
  { value: 'wxpay', label: t('admin.settings.payment.providerWxpay') },
  { value: 'stripe', label: t('admin.settings.payment.providerStripe') },
])

const hasAnyPaymentTypeEnabled = computed(() => form.payment_enabled_types.length > 0)

const balanceMultiplierPreview = computed(() => {
  const m = money(form.payment_balance_recharge_multiplier || '1')
  return formatMoney(m.gt(0) ? m : 1)
})

const hasFeeRate = computed(() => money(form.payment_recharge_fee_rate).gt(0))

const feeRatePreview = computed(() => formatMoney(form.payment_recharge_fee_rate || '0'))

const providerKeyOptions = computed(() => [
  { value: 'easypay', label: t('admin.settings.payment.providerEasypay') },
  { value: 'alipay', label: t('admin.settings.payment.providerAlipay') },
  { value: 'wxpay', label: t('admin.settings.payment.providerWxpay') },
  { value: 'stripe', label: t('admin.settings.payment.providerStripe') },
])

const enabledProviderKeyOptions = computed(() =>
  providerKeyOptions.value.filter(opt => form.payment_enabled_types.includes(opt.value)),
)

const loadBalanceOptions = computed(() => [
  { value: 'round-robin', label: t('admin.settings.payment.strategyRoundRobin') },
  { value: 'least-amount', label: t('admin.settings.payment.strategyLeastAmount') },
])

const cancelRateLimitUnitOptions = computed(() => [
  { value: 'minute', label: t('admin.settings.payment.cancelRateLimitUnitMinute') },
  { value: 'hour', label: t('admin.settings.payment.cancelRateLimitUnitHour') },
  { value: 'day', label: t('admin.settings.payment.cancelRateLimitUnitDay') },
])

const cancelRateLimitModeOptions = computed(() => [
  { value: 'rolling', label: t('admin.settings.payment.cancelRateLimitWindowModeRolling') },
  { value: 'fixed', label: t('admin.settings.payment.cancelRateLimitWindowModeFixed') },
])

function togglePaymentType(type: string): void {
  const idx = form.payment_enabled_types.indexOf(type)
  if (idx >= 0) form.payment_enabled_types.splice(idx, 1)
  else form.payment_enabled_types.push(type)
}

// ==================== Provider conflict detection ====================
//
// Mirrors PaymentProvidersView.vue and the host's legacy SettingsView
// behaviour: a provider can only "claim" a visible alipay/wxpay slot once.

type ProviderEnablementCandidate = Pick<
  ProviderInstance,
  'id' | 'provider_key' | 'supported_types' | 'enabled' | 'name'
>

function normalizeVisibleMethod(type: string): string {
  if (type === 'alipay_direct') return 'alipay'
  if (type === 'wxpay_direct') return 'wxpay'
  return type
}

function getProviderVisibleMethods(
  provider: ProviderEnablementCandidate,
): Array<'alipay' | 'wxpay'> {
  if (!provider.enabled) return []
  const supportedTypes = Array.isArray(provider.supported_types) ? provider.supported_types : []
  const methods = new Set<'alipay' | 'wxpay'>()
  const addMethod = (type: string) => {
    const m = normalizeVisibleMethod(type)
    if (m === 'alipay' || m === 'wxpay') methods.add(m)
  }
  if (provider.provider_key === 'alipay') {
    if (supportedTypes.length === 0) methods.add('alipay')
    else supportedTypes.forEach(typ => { if (normalizeVisibleMethod(typ) === 'alipay') methods.add('alipay') })
  } else if (provider.provider_key === 'wxpay') {
    if (supportedTypes.length === 0) methods.add('wxpay')
    else supportedTypes.forEach(typ => { if (normalizeVisibleMethod(typ) === 'wxpay') methods.add('wxpay') })
  } else if (provider.provider_key === 'easypay') {
    supportedTypes.forEach(addMethod)
  }
  return Array.from(methods)
}

function findProviderEnablementConflict(
  candidate: ProviderEnablementCandidate,
): { method: 'alipay' | 'wxpay'; conflicting: ProviderInstance } | null {
  const claimed = getProviderVisibleMethods(candidate)
  if (claimed.length === 0) return null
  for (const other of providers.value) {
    if (other.id === candidate.id || !other.enabled) continue
    const otherMethods = getProviderVisibleMethods(other)
    const matched = claimed.find(m => otherMethods.includes(m))
    if (matched) return { method: matched, conflicting: other }
  }
  return null
}

function showProviderEnablementConflict(conflict: {
  method: 'alipay' | 'wxpay'
  conflicting: ProviderInstance
}) {
  const fallback = `${conflict.conflicting.name} already handles ${conflict.method}`
  const key = 'admin.settings.payment.enableConflict'
  const translated = t(key, {
    method: t(`payment.methods.${conflict.method}`),
    provider: conflict.conflicting.name,
  })
  appStore.showError(translated === key ? fallback : translated)
}

// ==================== Data loading ====================

async function loadConfig() {
  try {
    const res = await adminPaymentAPI.getConfig()
    const cfg = res.data
    if (!cfg) return
    form.payment_enabled = !!cfg.enabled
    // Backend now returns string decimals; keep "0" as empty so the
    // placeholder "no limit" text shows up for those optional fields.
    form.payment_min_amount = cfg.min_amount && money(cfg.min_amount).gt(0) ? String(cfg.min_amount) : ''
    form.payment_max_amount = cfg.max_amount && money(cfg.max_amount).gt(0) ? String(cfg.max_amount) : ''
    form.payment_daily_limit = cfg.daily_limit && money(cfg.daily_limit).gt(0) ? String(cfg.daily_limit) : ''
    form.payment_order_timeout_minutes = cfg.order_timeout_minutes ?? 30
    form.payment_max_pending_orders = cfg.max_pending_orders ?? 3
    form.payment_balance_disabled = !!cfg.balance_disabled
    form.payment_balance_recharge_multiplier = cfg.balance_recharge_multiplier ? String(cfg.balance_recharge_multiplier) : '1'
    form.payment_recharge_fee_rate = cfg.recharge_fee_rate ? String(cfg.recharge_fee_rate) : '0'
    form.payment_load_balance_strategy = cfg.load_balance_strategy || 'round-robin'
    form.payment_product_name_prefix = cfg.product_name_prefix || ''
    form.payment_product_name_suffix = cfg.product_name_suffix || ''
    form.payment_help_image_url = cfg.help_image_url || ''
    form.payment_help_text = cfg.help_text || ''
    form.payment_cancel_rate_limit_enabled = !!cfg.cancel_rate_limit_enabled
    form.payment_cancel_rate_limit_max = cfg.cancel_rate_limit_max ?? 0
    form.payment_cancel_rate_limit_window = cfg.cancel_rate_limit_window ?? 0
    form.payment_cancel_rate_limit_unit = cfg.cancel_rate_limit_unit || 'minute'
    form.payment_cancel_rate_limit_window_mode = cfg.cancel_rate_limit_window_mode || 'rolling'
    form.payment_visible_method_alipay_enabled = cfg.visible_method_alipay_enabled ?? true
    form.payment_visible_method_alipay_source = cfg.visible_method_alipay_source || 'official'
    form.payment_visible_method_wxpay_enabled = cfg.visible_method_wxpay_enabled ?? true
    form.payment_visible_method_wxpay_source = cfg.visible_method_wxpay_source || 'official'
    form.payment_enabled_types = Array.isArray(cfg.enabled_payment_types)
      ? [...cfg.enabled_payment_types]
      : []
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

async function loadProviders() {
  providersLoading.value = true
  try {
    const res = await adminPaymentAPI.getProviders()
    providers.value = res.data || []
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    providersLoading.value = false
  }
}

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadConfig(), loadProviders()])
  } finally {
    loading.value = false
  }
}

// ==================== Save ====================

async function saveSettings() {
  saving.value = true
  try {
    await adminPaymentAPI.updateConfig({
      enabled: form.payment_enabled,
      // Empty string -> "0" so backend treats the field as "no limit".
      min_amount: form.payment_min_amount === '' ? '0' : formatMoney(form.payment_min_amount),
      max_amount: form.payment_max_amount === '' ? '0' : formatMoney(form.payment_max_amount),
      daily_limit: form.payment_daily_limit === '' ? '0' : formatMoney(form.payment_daily_limit),
      order_timeout_minutes: form.payment_order_timeout_minutes,
      max_pending_orders: form.payment_max_pending_orders,
      balance_disabled: form.payment_balance_disabled,
      balance_recharge_multiplier: formatMoney(form.payment_balance_recharge_multiplier || '1'),
      recharge_fee_rate: formatMoney(form.payment_recharge_fee_rate || '0'),
      load_balance_strategy: form.payment_load_balance_strategy,
      product_name_prefix: form.payment_product_name_prefix,
      product_name_suffix: form.payment_product_name_suffix,
      help_image_url: form.payment_help_image_url,
      help_text: form.payment_help_text,
      cancel_rate_limit_enabled: form.payment_cancel_rate_limit_enabled,
      cancel_rate_limit_max: form.payment_cancel_rate_limit_max,
      cancel_rate_limit_window: form.payment_cancel_rate_limit_window,
      cancel_rate_limit_unit: form.payment_cancel_rate_limit_unit,
      cancel_rate_limit_window_mode: form.payment_cancel_rate_limit_window_mode,
      visible_method_alipay_enabled: form.payment_visible_method_alipay_enabled,
      visible_method_alipay_source: form.payment_visible_method_alipay_source,
      visible_method_wxpay_enabled: form.payment_visible_method_wxpay_enabled,
      visible_method_wxpay_source: form.payment_visible_method_wxpay_source,
      enabled_payment_types: [...form.payment_enabled_types],
    })
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    saving.value = false
  }
}

// ==================== Provider CRUD ====================

function openCreateProvider() {
  editingProvider.value = null
  providerDialogRef.value?.reset(enabledProviderKeyOptions.value[0]?.value || 'easypay')
  showProviderDialog.value = true
}

function openEditProvider(provider: ProviderInstance) {
  editingProvider.value = provider
  providerDialogRef.value?.loadProvider(provider)
  showProviderDialog.value = true
}

function onDialogClose() {
  showProviderDialog.value = false
  void loadProviders()
}

async function handleSaveProvider(payload: Partial<ProviderInstance>) {
  providerSaving.value = true
  try {
    const candidate: ProviderEnablementCandidate = {
      id: editingProvider.value?.id ?? 0,
      provider_key: payload.provider_key ?? editingProvider.value?.provider_key ?? '',
      supported_types: payload.supported_types ?? editingProvider.value?.supported_types ?? [],
      enabled: payload.enabled ?? editingProvider.value?.enabled ?? false,
      name: payload.name ?? editingProvider.value?.name ?? '',
    }
    const conflict = findProviderEnablementConflict(candidate)
    if (conflict) {
      showProviderEnablementConflict(conflict)
      return
    }

    if (editingProvider.value) {
      await adminPaymentAPI.updateProvider(editingProvider.value.id, payload)
    } else {
      await adminPaymentAPI.createProvider(payload)
    }
    showProviderDialog.value = false
    await loadProviders()
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    providerSaving.value = false
  }
}

async function handleToggleField(
  provider: ProviderInstance,
  field: 'enabled' | 'refund_enabled' | 'allow_user_refund',
) {
  let nextValue: boolean
  if (field === 'enabled') nextValue = !provider.enabled
  else if (field === 'refund_enabled') nextValue = !provider.refund_enabled
  else nextValue = !provider.allow_user_refund

  if (field === 'enabled' && nextValue) {
    const conflict = findProviderEnablementConflict({
      id: provider.id,
      provider_key: provider.provider_key,
      supported_types: provider.supported_types,
      enabled: true,
      name: provider.name,
    })
    if (conflict) {
      showProviderEnablementConflict(conflict)
      return
    }
  }

  const payload: Record<string, boolean> = { [field]: nextValue }
  if (field === 'refund_enabled' && !nextValue) {
    payload.allow_user_refund = false
  }
  try {
    await adminPaymentAPI.updateProvider(provider.id, payload)
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function handleToggleType(provider: ProviderInstance, type: string) {
  const updated = provider.supported_types.includes(type)
    ? provider.supported_types.filter(typ => typ !== type)
    : [...provider.supported_types, type]
  const conflict = findProviderEnablementConflict({
    id: provider.id,
    provider_key: provider.provider_key,
    supported_types: updated,
    enabled: provider.enabled,
    name: provider.name,
  })
  if (conflict) {
    showProviderEnablementConflict(conflict)
    return
  }
  try {
    await adminPaymentAPI.updateProvider(provider.id, {
      supported_types: updated,
    } as Partial<ProviderInstance>)
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function handleReorderProviders(updates: { id: number; sort_order: number }[]) {
  try {
    await Promise.all(
      updates.map(u =>
        adminPaymentAPI.updateProvider(u.id, { sort_order: u.sort_order } as Partial<ProviderInstance>),
      ),
    )
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
    void loadProviders()
  }
}

function confirmDeleteProvider(provider: ProviderInstance) {
  deletingProviderId.value = provider.id
  showDeleteProviderDialog.value = true
}

async function handleDeleteProvider() {
  if (!deletingProviderId.value) return
  try {
    await adminPaymentAPI.deleteProvider(deletingProviderId.value)
    appStore.showSuccess(t('common.deleted'))
    showDeleteProviderDialog.value = false
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

// ==================== Lifecycle ====================

onMounted(() => {
  void loadAll()
})
</script>
