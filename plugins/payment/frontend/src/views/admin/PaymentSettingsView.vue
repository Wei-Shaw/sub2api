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
        <div class="ps-rows">
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.enabled') }}</label>
              <p class="ps-row-hint">{{ t('admin.settings.payment.enabledHint') }}</p>
            </div>
            <div class="ps-row-control">
              <Toggle
                :model-value="Boolean(values.enabled)"
                @update:model-value="setValue('enabled', $event, 'basic')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.productNamePrefix') }}</label>
            </div>
            <div class="ps-row-control">
              <input
                type="text"
                class="ps-input"
                :value="String(values.product_name_prefix ?? '')"
                @input="setValue('product_name_prefix', ($event.target as HTMLInputElement).value, 'basic')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.productNameSuffix') }}</label>
            </div>
            <div class="ps-row-control">
              <input
                type="text"
                class="ps-input"
                :value="String(values.product_name_suffix ?? '')"
                @input="setValue('product_name_suffix', ($event.target as HTMLInputElement).value, 'basic')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.helpImageUrl') }}</label>
            </div>
            <div class="ps-row-control">
              <input
                type="text"
                class="ps-input"
                :placeholder="t('admin.settings.payment.helpImagePlaceholder')"
                :value="String(values.help_image_url ?? '')"
                @input="setValue('help_image_url', ($event.target as HTMLInputElement).value, 'basic')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.helpText') }}</label>
            </div>
            <div class="ps-row-control">
              <textarea
                rows="3"
                class="ps-input"
                :placeholder="t('admin.settings.payment.helpTextPlaceholder')"
                :value="String(values.help_text ?? '')"
                @input="setValue('help_text', ($event.target as HTMLTextAreaElement).value, 'basic')"
              />
            </div>
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
        <div class="ps-rows">
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.minAmount') }}</label>
            </div>
            <div class="ps-row-control">
              <input
                type="number"
                min="0"
                step="0.01"
                class="ps-input"
                :value="numAsString(values.min_recharge_amount)"
                @input="setValue('min_recharge_amount', toNumber(($event.target as HTMLInputElement).value), 'limits')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.maxAmount') }}</label>
              <p class="ps-row-hint">{{ t('admin.settings.payment.noLimit') }}</p>
            </div>
            <div class="ps-row-control">
              <input
                type="number"
                min="0"
                step="0.01"
                class="ps-input"
                :value="numAsString(values.max_recharge_amount)"
                @input="setValue('max_recharge_amount', toNumber(($event.target as HTMLInputElement).value), 'limits')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.dailyLimit') }}</label>
              <p class="ps-row-hint">{{ t('admin.settings.payment.noLimit') }}</p>
            </div>
            <div class="ps-row-control">
              <input
                type="number"
                min="0"
                step="0.01"
                class="ps-input"
                :value="numAsString(values.daily_recharge_limit)"
                @input="setValue('daily_recharge_limit', toNumber(($event.target as HTMLInputElement).value), 'limits')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.orderTimeout') }}</label>
              <p class="ps-row-hint">{{ t('admin.settings.payment.orderTimeoutHint') }}</p>
            </div>
            <div class="ps-row-control">
              <input
                type="number"
                min="1"
                step="1"
                class="ps-input"
                :value="numAsString(values.order_timeout_minutes)"
                @input="setValue('order_timeout_minutes', toNumber(($event.target as HTMLInputElement).value), 'limits')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.maxPendingOrders') }}</label>
            </div>
            <div class="ps-row-control">
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
        <div class="ps-rows">
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.providerAlipay') }}</label>
            </div>
            <div class="ps-row-control ps-inline">
              <Toggle
                :model-value="Boolean(values.visible_method_alipay_enabled)"
                @update:model-value="setValue('visible_method_alipay_enabled', $event, 'visible')"
              />
              <Select
                class="ps-select"
                :model-value="String(values.visible_method_alipay_source ?? 'official')"
                :options="visibleSourceOptions"
                @update:model-value="setValue('visible_method_alipay_source', String($event), 'visible')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.providerWxpay') }}</label>
            </div>
            <div class="ps-row-control ps-inline">
              <Toggle
                :model-value="Boolean(values.visible_method_wxpay_enabled)"
                @update:model-value="setValue('visible_method_wxpay_enabled', $event, 'visible')"
              />
              <Select
                class="ps-select"
                :model-value="String(values.visible_method_wxpay_source ?? 'official')"
                :options="visibleSourceOptions"
                @update:model-value="setValue('visible_method_wxpay_source', String($event), 'visible')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.enabledPaymentTypes') }}</label>
              <p class="ps-row-hint">{{ t('admin.settings.payment.enabledPaymentTypesHint') }}</p>
            </div>
            <div class="ps-row-control">
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
            </div>
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
        <div class="ps-rows">
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('payment.adminSettings.balanceMultiplier') }}</label>
            </div>
            <div class="ps-row-control">
              <input
                type="number"
                min="0"
                step="0.01"
                class="ps-input"
                :value="numAsString(values.balance_recharge_multiplier)"
                @input="setValue('balance_recharge_multiplier', toNumber(($event.target as HTMLInputElement).value), 'fees')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('payment.adminSettings.rechargeFeeRate') }}</label>
              <p class="ps-row-hint">{{ t('payment.adminSettings.rechargeFeeRateHint') }}</p>
            </div>
            <div class="ps-row-control">
              <input
                type="number"
                min="0"
                max="1"
                step="0.001"
                class="ps-input"
                :value="numAsString(values.recharge_fee_rate)"
                @input="setValue('recharge_fee_rate', toNumber(($event.target as HTMLInputElement).value), 'fees')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.balancePaymentDisabled') }}</label>
            </div>
            <div class="ps-row-control">
              <Toggle
                :model-value="Boolean(values.balance_payment_disabled)"
                @update:model-value="setValue('balance_payment_disabled', $event, 'fees')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.loadBalanceStrategy') }}</label>
            </div>
            <div class="ps-row-control">
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
        <div class="ps-rows">
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.cancelRateLimit') }}</label>
              <p class="ps-row-hint">{{ t('admin.settings.payment.cancelRateLimitHint') }}</p>
            </div>
            <div class="ps-row-control">
              <Toggle
                :model-value="Boolean(values.cancel_rate_limit_enabled)"
                @update:model-value="setValue('cancel_rate_limit_enabled', $event, 'cancel')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.cancelRateLimitMax') }}</label>
            </div>
            <div class="ps-row-control">
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
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.cancelRateLimitWindow') }}</label>
            </div>
            <div class="ps-row-control">
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
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.cancelRateLimitUnit') }}</label>
            </div>
            <div class="ps-row-control">
              <Select
                class="ps-select"
                :disabled="!values.cancel_rate_limit_enabled"
                :model-value="String(values.cancel_rate_limit_unit ?? 'hour')"
                :options="cancelUnitOptions"
                @update:model-value="setValue('cancel_rate_limit_unit', String($event), 'cancel')"
              />
            </div>
          </div>
          <div class="ps-row">
            <div class="ps-row-label">
              <label>{{ t('admin.settings.payment.cancelRateLimitWindowMode') }}</label>
            </div>
            <div class="ps-row-control">
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

onMounted(() => {
  void loadAll()
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
.ps-rows {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.ps-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 0.5rem;
  align-items: start;
}
@media (min-width: 640px) {
  .ps-row {
    grid-template-columns: 12rem minmax(0, 1fr);
    gap: 1rem;
  }
}
.ps-row-label label {
  display: block;
  font-size: 0.875rem;
  font-weight: 500;
  color: inherit;
}
.ps-row-hint {
  margin: 0.25rem 0 0;
  font-size: 0.75rem;
  color: rgb(107 114 128);
}
.dark .ps-row-hint {
  color: rgb(156 163 175);
}
.ps-row-control {
  min-width: 0;
}
.ps-inline {
  display: flex;
  align-items: center;
  gap: 0.75rem;
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
  min-width: 12rem;
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
