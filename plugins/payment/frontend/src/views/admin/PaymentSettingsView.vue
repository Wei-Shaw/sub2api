<!--
  PaymentSettingsView — plugin-owned admin settings page.

  When the host PluginSettingsForm sees Manifest.SettingsComponentPath !==
  empty, it mounts THIS component instead of the generic JSON-schema
  renderer. The component owns its own load + per-section save flow
  against the host's plugin-settings REST endpoints
  (GET/PUT /api/v1/admin/plugin-settings/payment[/:key]).

  Sections mirror the business domains declared in
  plugins/payment/internal/settings/settings_schema.json:
    1. 基础  — enable / 商品名前后缀 / 帮助文案
    2. 限额  — 充值范围 / 订单超时 / 待支付订单上限
    3. 可见方式 — alipay / wxpay 启停 + 来源 + 启用渠道列表
    4. 费率  — 充值倍率 / 手续费率 / 余额支付 / 负载均衡
    5. 取消限流 — 取消频率限制 (启停 + 时间窗口 + 模式)
-->
<template>
  <div class="payment-settings space-y-4">
    <div v-if="state === 'loading'" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </div>
    <div
      v-else-if="state === 'error'"
      class="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-200"
    >
      {{ loadError || t('common.error') }}
    </div>
    <template v-else>
      <!-- Section: 基础 -->
      <section class="ps-card">
        <header class="ps-section-header">
          <h3 class="ps-section-title">{{ t('payment.adminSettings.sectionBasic') }}</h3>
          <button
            type="button"
            class="ps-btn ps-btn-primary"
            :disabled="saving === 'basic' || !sectionDirty.basic"
            @click="saveSection('basic')"
          >
            {{ saving === 'basic' ? t('common.saving') : t('common.save') }}
          </button>
        </header>
        <div class="ps-section-body">
          <div class="ps-toggle-row">
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.enabled') }}</label>
              <p class="ps-hint">{{ t('admin.settings.payment.enabledHint') }}</p>
            </div>
            <Toggle
              :model-value="Boolean(values.enabled)"
              @update:model-value="setValue('enabled', $event, 'basic')"
            />
          </div>
          <div class="ps-grid ps-grid-3">
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.productNamePrefix') }}</label>
              <input
                type="text"
                class="ps-input"
                :value="String(values.product_name_prefix ?? '')"
                @input="setValue('product_name_prefix', ($event.target as HTMLInputElement).value, 'basic')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.productNameSuffix') }}</label>
              <input
                type="text"
                class="ps-input"
                :value="String(values.product_name_suffix ?? '')"
                @input="setValue('product_name_suffix', ($event.target as HTMLInputElement).value, 'basic')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.helpImageUrl') }}</label>
              <input
                type="text"
                class="ps-input"
                :placeholder="t('admin.settings.payment.helpImagePlaceholder')"
                :value="String(values.help_image_url ?? '')"
                @input="setValue('help_image_url', ($event.target as HTMLInputElement).value, 'basic')"
              />
            </div>
          </div>
          <div>
            <label class="ps-label">{{ t('admin.settings.payment.helpText') }}</label>
            <textarea
              rows="3"
              class="ps-input"
              :placeholder="t('admin.settings.payment.helpTextPlaceholder')"
              :value="String(values.help_text ?? '')"
              @input="setValue('help_text', ($event.target as HTMLTextAreaElement).value, 'basic')"
            />
          </div>
        </div>
      </section>

      <!-- Section: 限额 -->
      <section class="ps-card">
        <header class="ps-section-header">
          <h3 class="ps-section-title">{{ t('payment.adminSettings.sectionLimits') }}</h3>
          <button
            type="button"
            class="ps-btn ps-btn-primary"
            :disabled="saving === 'limits' || !sectionDirty.limits"
            @click="saveSection('limits')"
          >
            {{ saving === 'limits' ? t('common.saving') : t('common.save') }}
          </button>
        </header>
        <div class="ps-section-body">
          <div class="ps-grid ps-grid-3">
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.minAmount') }}</label>
              <input
                type="number"
                min="0"
                step="0.01"
                class="ps-input"
                :value="numAsString(values.min_recharge_amount)"
                @input="setValue('min_recharge_amount', toNumber(($event.target as HTMLInputElement).value), 'limits')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.maxAmount') }}</label>
              <input
                type="number"
                min="0"
                step="0.01"
                class="ps-input"
                :value="numAsString(values.max_recharge_amount)"
                @input="setValue('max_recharge_amount', toNumber(($event.target as HTMLInputElement).value), 'limits')"
              />
              <p class="ps-hint">{{ t('admin.settings.payment.noLimit') }}</p>
            </div>
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.dailyLimit') }}</label>
              <input
                type="number"
                min="0"
                step="0.01"
                class="ps-input"
                :value="numAsString(values.daily_recharge_limit)"
                @input="setValue('daily_recharge_limit', toNumber(($event.target as HTMLInputElement).value), 'limits')"
              />
              <p class="ps-hint">{{ t('admin.settings.payment.noLimit') }}</p>
            </div>
          </div>
          <div class="ps-grid ps-grid-2">
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.orderTimeout') }}</label>
              <input
                type="number"
                min="1"
                step="1"
                class="ps-input"
                :value="numAsString(values.order_timeout_minutes)"
                @input="setValue('order_timeout_minutes', toNumber(($event.target as HTMLInputElement).value), 'limits')"
              />
              <p class="ps-hint">{{ t('admin.settings.payment.orderTimeoutHint') }}</p>
            </div>
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.maxPendingOrders') }}</label>
              <input
                type="number"
                min="1"
                step="1"
                class="ps-input"
                :value="numAsString(values.max_pending_orders)"
                @input="setValue('max_pending_orders', toNumber(($event.target as HTMLInputElement).value), 'limits')"
              />
            </div>
          </div>
        </div>
      </section>

      <!-- Section: 可见方式 -->
      <section class="ps-card">
        <header class="ps-section-header">
          <h3 class="ps-section-title">{{ t('payment.adminSettings.sectionVisible') }}</h3>
          <button
            type="button"
            class="ps-btn ps-btn-primary"
            :disabled="saving === 'visible' || !sectionDirty.visible"
            @click="saveSection('visible')"
          >
            {{ saving === 'visible' ? t('common.saving') : t('common.save') }}
          </button>
        </header>
        <div class="ps-section-body">
          <div class="ps-grid ps-grid-2">
            <div class="ps-toggle-row">
              <label class="ps-label">{{ t('admin.settings.payment.providerAlipay') }}</label>
              <Toggle
                :model-value="Boolean(values.visible_method_alipay_enabled)"
                @update:model-value="setValue('visible_method_alipay_enabled', $event, 'visible')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('payment.adminSettings.sourceOfficial') }} / {{ t('payment.adminSettings.sourceEasypay') }}</label>
              <Select
                class="ps-select"
                :model-value="String(values.visible_method_alipay_source ?? 'official')"
                :options="visibleSourceOptions"
                @update:model-value="setValue('visible_method_alipay_source', String($event), 'visible')"
              />
            </div>
          </div>
          <div class="ps-grid ps-grid-2">
            <div class="ps-toggle-row">
              <label class="ps-label">{{ t('admin.settings.payment.providerWxpay') }}</label>
              <Toggle
                :model-value="Boolean(values.visible_method_wxpay_enabled)"
                @update:model-value="setValue('visible_method_wxpay_enabled', $event, 'visible')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('payment.adminSettings.sourceOfficial') }} / {{ t('payment.adminSettings.sourceEasypay') }}</label>
              <Select
                class="ps-select"
                :model-value="String(values.visible_method_wxpay_source ?? 'official')"
                :options="visibleSourceOptions"
                @update:model-value="setValue('visible_method_wxpay_source', String($event), 'visible')"
              />
            </div>
          </div>
          <div>
            <label class="ps-label">{{ t('admin.settings.payment.enabledPaymentTypes') }}</label>
            <div class="ps-checkbox-list">
              <label v-for="opt in paymentTypeOptions" :key="opt.value" class="ps-checkbox">
                <input
                  type="checkbox"
                  :checked="enabledTypes.includes(opt.value)"
                  @change="toggleEnabledType(opt.value, ($event.target as HTMLInputElement).checked)"
                />
                <span>{{ opt.label }}</span>
              </label>
            </div>
            <p class="ps-hint">{{ t('admin.settings.payment.enabledPaymentTypesHint') }}</p>
          </div>
        </div>
      </section>

      <!-- Section: 费率 -->
      <section class="ps-card">
        <header class="ps-section-header">
          <h3 class="ps-section-title">{{ t('payment.adminSettings.sectionFees') }}</h3>
          <button
            type="button"
            class="ps-btn ps-btn-primary"
            :disabled="saving === 'fees' || !sectionDirty.fees"
            @click="saveSection('fees')"
          >
            {{ saving === 'fees' ? t('common.saving') : t('common.save') }}
          </button>
        </header>
        <div class="ps-section-body">
          <div class="ps-grid ps-grid-2">
            <div>
              <label class="ps-label">{{ t('payment.adminSettings.balanceMultiplier') }}</label>
              <input
                type="number"
                min="0"
                step="0.01"
                class="ps-input"
                :value="numAsString(values.balance_recharge_multiplier)"
                @input="setValue('balance_recharge_multiplier', toNumber(($event.target as HTMLInputElement).value), 'fees')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('payment.adminSettings.rechargeFeeRate') }}</label>
              <input
                type="number"
                min="0"
                max="1"
                step="0.001"
                class="ps-input"
                :value="numAsString(values.recharge_fee_rate)"
                @input="setValue('recharge_fee_rate', toNumber(($event.target as HTMLInputElement).value), 'fees')"
              />
              <p class="ps-hint">{{ t('payment.adminSettings.rechargeFeeRateHint') }}</p>
            </div>
          </div>
          <div class="ps-grid ps-grid-2">
            <div class="ps-toggle-row">
              <label class="ps-label">{{ t('admin.settings.payment.balancePaymentDisabled') }}</label>
              <Toggle
                :model-value="Boolean(values.balance_payment_disabled)"
                @update:model-value="setValue('balance_payment_disabled', $event, 'fees')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.loadBalanceStrategy') }}</label>
              <Select
                class="ps-select"
                :model-value="String(values.load_balance_strategy ?? 'round-robin')"
                :options="loadBalanceOptions"
                @update:model-value="setValue('load_balance_strategy', String($event), 'fees')"
              />
            </div>
          </div>
        </div>
      </section>

      <!-- Section: 取消限流 -->
      <section class="ps-card">
        <header class="ps-section-header">
          <h3 class="ps-section-title">{{ t('admin.settings.payment.cancelRateLimit') }}</h3>
          <button
            type="button"
            class="ps-btn ps-btn-primary"
            :disabled="saving === 'cancel' || !sectionDirty.cancel"
            @click="saveSection('cancel')"
          >
            {{ saving === 'cancel' ? t('common.saving') : t('common.save') }}
          </button>
        </header>
        <div class="ps-section-body">
          <div class="ps-toggle-row">
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.cancelRateLimit') }}</label>
              <p class="ps-hint">{{ t('admin.settings.payment.cancelRateLimitHint') }}</p>
            </div>
            <Toggle
              :model-value="Boolean(values.cancel_rate_limit_enabled)"
              @update:model-value="setValue('cancel_rate_limit_enabled', $event, 'cancel')"
            />
          </div>
          <div class="ps-grid ps-grid-2 sm:ps-grid-4">
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.cancelRateLimitMax') }}</label>
              <input
                type="number"
                min="1"
                step="1"
                class="ps-input"
                :disabled="!values.cancel_rate_limit_enabled"
                :value="numAsString(values.cancel_rate_limit_max)"
                @input="setValue('cancel_rate_limit_max', toNumber(($event.target as HTMLInputElement).value), 'cancel')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.cancelRateLimitWindow') }}</label>
              <input
                type="number"
                min="1"
                step="1"
                class="ps-input"
                :disabled="!values.cancel_rate_limit_enabled"
                :value="numAsString(values.cancel_rate_limit_window)"
                @input="setValue('cancel_rate_limit_window', toNumber(($event.target as HTMLInputElement).value), 'cancel')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.cancelRateLimitUnit') }}</label>
              <Select
                class="ps-select"
                :disabled="!values.cancel_rate_limit_enabled"
                :model-value="String(values.cancel_rate_limit_unit ?? 'hour')"
                :options="cancelUnitOptions"
                @update:model-value="setValue('cancel_rate_limit_unit', String($event), 'cancel')"
              />
            </div>
            <div>
              <label class="ps-label">{{ t('admin.settings.payment.cancelRateLimitWindowMode') }}</label>
              <Select
                class="ps-select"
                :disabled="!values.cancel_rate_limit_enabled"
                :model-value="String(values.cancel_rate_limit_window_mode ?? 'rolling')"
                :options="cancelModeOptions"
                @update:model-value="setValue('cancel_rate_limit_window_mode', String($event), 'cancel')"
              />
            </div>
          </div>
        </div>
      </section>

      <!-- Section: 收款渠道 -->
      <section class="ps-card">
        <header class="ps-section-header">
          <h3 class="ps-section-title">{{ t('payment.adminSettings.sectionProviders') }}</h3>
        </header>
        <PaymentProviderList
          :providers="providers"
          :loading="providersLoading"
          :can-create="hasAnyPaymentTypeEnabled"
          :enabled-payment-types="providerEnabledTypes"
          :all-payment-types="allPaymentTypes"
          :redirect-label="t('admin.settings.payment.easypayRedirect')"
          @refresh="loadProvidersAndTypes"
          @create="openCreateProvider"
          @edit="openEditProvider"
          @delete="confirmDeleteProvider"
          @toggle-field="handleToggleField"
          @toggle-type="handleToggleType"
          @reorder="handleReorderProviders"
        />
      </section>

      <!-- Provider create/edit dialog -->
      <PaymentProviderDialog
        ref="providerDialogRef"
        :show="showProviderDialog"
        :saving="providerSaving"
        :editing="editingProvider"
        :all-key-options="providerKeyOptions"
        :enabled-key-options="enabledProviderKeyOptions"
        :all-payment-types="allPaymentTypes"
        :redirect-label="t('admin.settings.payment.easypayRedirect')"
        @close="onProviderDialogClose"
        @save="handleSaveProvider"
      />

      <!-- Provider delete confirmation -->
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
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SelectOption } from '@sub2api/plugin-sdk'
import { Toggle, Select } from '@sub2api/plugin-sdk'

import { getClient } from '../../api/client'
import { useAppStore } from '../../stores/host'
import { extractApiErrorMessage } from '../../utils/apiError'
import { ConfirmDialog } from '@sub2api/plugin-sdk'

import PaymentProviderList from '../../components/payment/PaymentProviderList.vue'
import PaymentProviderDialog from '../../components/payment/PaymentProviderDialog.vue'
import { adminPaymentAPI } from '../../api/admin/payment'
import { extractI18nErrorMessage } from '../../utils/apiError'
import type { ProviderInstance } from '../../types/payment'

const { t } = useI18n()
const appStore = useAppStore()
const client = getClient()

type SectionKey = 'basic' | 'limits' | 'visible' | 'fees' | 'cancel'

const SECTION_KEYS: Record<SectionKey, string[]> = {
  basic: ['enabled', 'product_name_prefix', 'product_name_suffix', 'help_image_url', 'help_text'],
  limits: [
    'min_recharge_amount',
    'max_recharge_amount',
    'daily_recharge_limit',
    'order_timeout_minutes',
    'max_pending_orders',
  ],
  visible: [
    'visible_method_alipay_enabled',
    'visible_method_alipay_source',
    'visible_method_wxpay_enabled',
    'visible_method_wxpay_source',
    'enabled_payment_types',
  ],
  fees: [
    'balance_recharge_multiplier',
    'recharge_fee_rate',
    'balance_payment_disabled',
    'load_balance_strategy',
  ],
  cancel: [
    'cancel_rate_limit_enabled',
    'cancel_rate_limit_max',
    'cancel_rate_limit_window',
    'cancel_rate_limit_unit',
    'cancel_rate_limit_window_mode',
  ],
}

const PLUGIN_NAME = 'payment'

const state = ref<'loading' | 'ready' | 'error'>('loading')
const loadError = ref<string>('')
const saving = ref<SectionKey | null>(null)

const values = reactive<Record<string, unknown>>({})
const initialValues = reactive<Record<string, unknown>>({})
const sectionDirty = reactive<Record<SectionKey, boolean>>({
  basic: false,
  limits: false,
  visible: false,
  fees: false,
  cancel: false,
})

const visibleSourceOptions = computed<SelectOption[]>(() => [
  { value: 'official', label: t('payment.adminSettings.sourceOfficial') },
  { value: 'easypay', label: t('payment.adminSettings.sourceEasypay') },
])

const loadBalanceOptions = computed<SelectOption[]>(() => [
  { value: 'round-robin', label: t('admin.settings.payment.strategyRoundRobin') },
  { value: 'least-amount', label: t('admin.settings.payment.strategyLeastAmount') },
])

const cancelUnitOptions = computed<SelectOption[]>(() => [
  { value: 'minute', label: t('admin.settings.payment.cancelRateLimitUnitMinute') },
  { value: 'hour', label: t('admin.settings.payment.cancelRateLimitUnitHour') },
  { value: 'day', label: t('admin.settings.payment.cancelRateLimitUnitDay') },
])

const cancelModeOptions = computed<SelectOption[]>(() => [
  { value: 'rolling', label: t('admin.settings.payment.cancelRateLimitWindowModeRolling') },
  { value: 'fixed', label: t('admin.settings.payment.cancelRateLimitWindowModeFixed') },
])

const paymentTypeOptions = computed<Array<{ value: string; label: string }>>(() => [
  { value: 'easypay', label: t('admin.settings.payment.providerEasypay') },
  { value: 'alipay', label: t('admin.settings.payment.providerAlipay') },
  { value: 'wxpay', label: t('admin.settings.payment.providerWxpay') },
  { value: 'stripe', label: t('admin.settings.payment.providerStripe') },
])

const enabledTypes = computed<string[]>(() => {
  const v = values.enabled_payment_types
  if (Array.isArray(v)) {
    return v.map((x) => String(x))
  }
  if (typeof v === 'string' && v.length > 0) {
    return v.split(',').map((s) => s.trim()).filter(Boolean)
  }
  return []
})

function toggleEnabledType(value: string, checked: boolean): void {
  const current = enabledTypes.value.slice()
  const idx = current.indexOf(value)
  if (checked && idx < 0) {
    current.push(value)
  } else if (!checked && idx >= 0) {
    current.splice(idx, 1)
  }
  setValue('enabled_payment_types', current, 'visible')
}

function numAsString(v: unknown): string {
  if (v == null || v === '') return ''
  const n = Number(v)
  return Number.isFinite(n) ? String(n) : ''
}

function toNumber(raw: string): number {
  if (raw === '' || raw == null) return 0
  const n = Number(raw)
  return Number.isFinite(n) ? n : 0
}

function setValue(key: string, value: unknown, section: SectionKey): void {
  values[key] = value
  sectionDirty[section] = !sectionEquals(section)
}

function sectionEquals(section: SectionKey): boolean {
  for (const k of SECTION_KEYS[section]) {
    if (!shallowEqual(values[k], initialValues[k])) {
      return false
    }
  }
  return true
}

function shallowEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) return false
    for (let i = 0; i < a.length; i++) {
      if (a[i] !== b[i]) return false
    }
    return true
  }
  return false
}

async function loadAll(): Promise<void> {
  state.value = 'loading'
  loadError.value = ''
  try {
    const resp = await client.get(`/admin/plugin-settings/${encodeURIComponent(PLUGIN_NAME)}`)
    const data = resp.data as { values?: Record<string, unknown>; defaults?: Record<string, unknown> }
    const merged: Record<string, unknown> = { ...(data.defaults ?? {}), ...(data.values ?? {}) }
    Object.keys(values).forEach((k) => delete values[k])
    Object.keys(initialValues).forEach((k) => delete initialValues[k])
    Object.assign(values, merged)
    Object.assign(initialValues, JSON.parse(JSON.stringify(merged)))
    Object.keys(sectionDirty).forEach((k) => {
      sectionDirty[k as SectionKey] = false
    })
    state.value = 'ready'
  } catch (err: unknown) {
    state.value = 'error'
    loadError.value = extractApiErrorMessage(err, t('common.error'))
  }
}

async function saveSection(section: SectionKey): Promise<void> {
  if (!sectionDirty[section]) return
  saving.value = section
  const keys = SECTION_KEYS[section]
  try {
    for (const key of keys) {
      if (shallowEqual(values[key], initialValues[key])) continue
      await client.put(
        `/admin/plugin-settings/${encodeURIComponent(PLUGIN_NAME)}/${encodeURIComponent(key)}`,
        { value: values[key] },
      )
      initialValues[key] = JSON.parse(JSON.stringify(values[key]))
    }
    sectionDirty[section] = false
    appStore.showSuccess(t('payment.adminSettings.saveSuccess'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    saving.value = null
  }
}


// ==================== Providers (section "shoukuan-qudao") ====================
//
// Migrated from PaymentProvidersView.vue: providers list + create/edit dialog
// + delete confirmation now live inside this settings page as a 6th section.
// enabledTypes (the computed above) already mirrors enabled_payment_types
// from settings, so we reuse it as the source-of-truth instead of refetching
// /admin/payment/config.

const providers = ref<ProviderInstance[]>([])
const providersLoading = ref(false)
const showProviderDialog = ref(false)
const showDeleteProviderDialog = ref(false)
const providerSaving = ref(false)
const editingProvider = ref<ProviderInstance | null>(null)
const deletingProviderId = ref<number | null>(null)
const providerDialogRef = ref<InstanceType<typeof PaymentProviderDialog> | null>(null)

// Reuse the settings computed enabledTypes instead of refetching config.
const providerEnabledTypes = computed<string[]>(() => enabledTypes.value)

const allPaymentTypes = computed(() => [
  { value: 'easypay', label: t('payment.methods.easypay') },
  { value: 'alipay', label: t('payment.methods.alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay') },
  { value: 'stripe', label: t('payment.methods.stripe') },
])

const hasAnyPaymentTypeEnabled = computed(() => providerEnabledTypes.value.length > 0)

const providerKeyOptions = computed(() => [
  { value: 'easypay', label: t('admin.settings.payment.providerEasypay') },
  { value: 'alipay', label: t('admin.settings.payment.providerAlipay') },
  { value: 'wxpay', label: t('admin.settings.payment.providerWxpay') },
  { value: 'stripe', label: t('admin.settings.payment.providerStripe') },
])

const enabledProviderKeyOptions = computed(() =>
  providerKeyOptions.value.filter(opt => providerEnabledTypes.value.includes(opt.value)),
)

// ---- Provider conflict detection (mirror of PaymentProvidersView) ----

type ProviderEnablementCandidate = Pick<
  ProviderInstance,
  'id' | 'provider_key' | 'supported_types' | 'enabled' | 'name'
>

function normalizeVisibleMethod(type: string): string {
  if (type === 'alipay_direct') return 'alipay'
  if (type === 'wxpay_direct') return 'wxpay'
  return type
}

function getProviderVisibleMethods(provider: ProviderEnablementCandidate): Array<'alipay' | 'wxpay'> {
  if (!provider.enabled) return []
  const supportedTypes = Array.isArray(provider.supported_types) ? provider.supported_types : []
  const methods = new Set<'alipay' | 'wxpay'>()
  const addMethod = (type: string) => {
    const m = normalizeVisibleMethod(type)
    if (m === 'alipay' || m === 'wxpay') methods.add(m)
  }

  if (provider.provider_key === 'alipay') {
    if (supportedTypes.length === 0) methods.add('alipay')
    else supportedTypes.forEach(ty => { if (normalizeVisibleMethod(ty) === 'alipay') methods.add('alipay') })
  } else if (provider.provider_key === 'wxpay') {
    if (supportedTypes.length === 0) methods.add('wxpay')
    else supportedTypes.forEach(ty => { if (normalizeVisibleMethod(ty) === 'wxpay') methods.add('wxpay') })
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
  const fallback = conflict.conflicting.name + ' already handles ' + conflict.method
  const key = 'admin.settings.payment.enableConflict'
  const translated = t(key, {
    method: t('payment.methods.' + conflict.method),
    provider: conflict.conflicting.name,
  })
  appStore.showError(translated === key ? fallback : translated)
}

// ---- Data loading ----

async function loadProviders(): Promise<void> {
  try {
    const res = await adminPaymentAPI.getProviders()
    providers.value = res.data || []
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function loadProvidersAndTypes(): Promise<void> {
  providersLoading.value = true
  try {
    await loadProviders()
  } finally {
    providersLoading.value = false
  }
}

// ---- Dialog open / close ----

function openCreateProvider(): void {
  editingProvider.value = null
  providerDialogRef.value?.reset(enabledProviderKeyOptions.value[0]?.value || 'easypay')
  showProviderDialog.value = true
}

function openEditProvider(provider: ProviderInstance): void {
  editingProvider.value = provider
  providerDialogRef.value?.loadProvider(provider)
  showProviderDialog.value = true
}

function onProviderDialogClose(): void {
  showProviderDialog.value = false
  void loadProviders()
}

// ---- CRUD handlers ----

async function handleSaveProvider(payload: Partial<ProviderInstance>): Promise<void> {
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
): Promise<void> {
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

async function handleToggleType(provider: ProviderInstance, type: string): Promise<void> {
  const updated = provider.supported_types.includes(type)
    ? provider.supported_types.filter(x => x !== type)
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

async function handleReorderProviders(updates: { id: number; sort_order: number }[]): Promise<void> {
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

function confirmDeleteProvider(provider: ProviderInstance): void {
  deletingProviderId.value = provider.id
  showDeleteProviderDialog.value = true
}

async function handleDeleteProvider(): Promise<void> {
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
onMounted(() => {
  void loadAll()
  void loadProvidersAndTypes()
})
</script>

<style scoped>
.payment-settings {
  font-family: inherit;
  color: rgb(31 41 55);
}
.dark .payment-settings,
:host(.dark) .payment-settings {
  color: rgb(229 231 235);
}
.ps-card {
  border: 1px solid rgb(229 231 235);
  border-radius: 0.5rem;
  background: white;
  padding: 1rem;
}
.dark .ps-card {
  background: rgb(31 41 55);
  border-color: rgb(55 65 81);
}
.ps-section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.75rem;
}
.ps-section-title {
  font-size: 0.875rem;
  font-weight: 600;
  margin: 0;
}
.ps-section-body {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.ps-grid {
  display: grid;
  gap: 0.75rem;
  grid-template-columns: minmax(0, 1fr);
}
@media (min-width: 640px) {
  .ps-grid-2 {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .ps-grid-3 {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .ps-grid-4,
  .sm\:ps-grid-4 {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}
.ps-label {
  display: block;
  margin-bottom: 0.25rem;
  font-size: 0.75rem;
  font-weight: 500;
  color: rgb(55 65 81);
}
.dark .ps-label {
  color: rgb(209 213 219);
}
.ps-hint {
  margin: 0.25rem 0 0;
  font-size: 0.75rem;
  color: rgb(156 163 175);
}
.dark .ps-hint {
  color: rgb(156 163 175);
}
.ps-toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  min-width: 0;
}
.ps-toggle-row > div {
  min-width: 0;
}
.ps-input {
  display: block;
  width: 100%;
  border: 1px solid rgb(209 213 219);
  border-radius: 0.375rem;
  padding: 0.4rem 0.6rem;
  font-size: 0.875rem;
  color: rgb(17 24 39);
  background: white;
}
.ps-input:focus {
  outline: 2px solid rgb(59 130 246);
  outline-offset: 1px;
}
.ps-input:disabled {
  background: rgb(243 244 246);
  cursor: not-allowed;
}
.dark .ps-input {
  background: rgb(17 24 39);
  border-color: rgb(75 85 99);
  color: rgb(243 244 246);
}
.dark .ps-input:disabled {
  background: rgb(31 41 55);
}
.ps-select {
  width: 100%;
  min-width: 0;
}
.ps-checkbox-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
}
.ps-checkbox {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.875rem;
  cursor: pointer;
}
.ps-checkbox input[type='checkbox'] {
  width: 1rem;
  height: 1rem;
}
.ps-btn {
  display: inline-flex;
  align-items: center;
  border-radius: 0.375rem;
  padding: 0.4rem 0.9rem;
  font-size: 0.75rem;
  font-weight: 500;
  border: 0;
  cursor: pointer;
  transition: background-color 150ms ease;
}
.ps-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.ps-btn-primary {
  background: rgb(37 99 235);
  color: white;
}
.ps-btn-primary:hover:not(:disabled) {
  background: rgb(29 78 216);
}
.space-y-4 > * + * {
  margin-top: 1rem;
}
</style>
