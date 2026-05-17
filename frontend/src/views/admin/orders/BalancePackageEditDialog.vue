<template>
  <BaseDialog :show="show" :title="pkg ? t('payment.admin.editBalancePackage') : t('payment.admin.createBalancePackage')" width="wide" @close="emit('close')">
    <form id="balance-package-form" class="space-y-4" @submit.prevent="handleSave">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.packageCode') }} <span class="text-red-500">*</span></label>
          <input v-model="form.code" type="text" class="input" placeholder="standard" required />
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.packageLabel') }} <span class="text-red-500">*</span></label>
          <input v-model="form.label" type="text" class="input" placeholder="Standard" required />
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('payment.admin.packageDescription') }}</label>
        <textarea v-model="form.description" rows="2" class="input" />
      </div>

      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.amountLedger') }} ($) <span class="text-red-500">*</span></label>
          <input v-model.number="form.amount_ledger" type="number" step="0.01" min="0.01" class="input" required />
        </div>
        <div>
          <label class="input-label">Tokens <span class="text-red-500">*</span></label>
          <input v-model.number="form.actual_credits" type="number" step="1" min="1" class="input" required />
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-600 dark:bg-dark-800">
        <div class="grid grid-cols-2 gap-3">
          <div><span class="text-gray-500">{{ t('payment.admin.payAmount') }}:</span> <span class="font-medium">${{ safeNumber(form.amount_ledger).toFixed(2) }}</span></div>
          <div><span class="text-gray-500">Tokens:</span> <span class="font-medium">{{ formatTokens(safeNumber(form.actual_credits)) }}</span></div>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.badge') }}</label>
          <input v-model="form.badge" type="text" class="input" placeholder="Tiết kiệm 95.4%" />
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.sortOrder') }}</label>
          <input v-model.number="form.sort_order" type="number" min="0" class="input" />
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('payment.admin.balanceGroup') }} <span class="text-red-500">*</span></label>
        <select v-model.number="form.balance_group_id" class="input" required :disabled="groupsLoading">
          <option :value="0">{{ groupsLoading ? t('common.loading') : t('payment.admin.selectBalanceGroup') }}</option>
          <option v-for="group in balanceGroups" :key="group.id" :value="group.id">
            #{{ group.id }} · {{ group.name }} · {{ group.platform }} · x{{ safeNumber(group.rate_multiplier).toFixed(6) }}
          </option>
        </select>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.balanceGroupHelp') }}</p>
      </div>

      <!-- Currency Price Overrides -->
      <div>
        <label class="input-label">Currency Price Overrides</label>
        <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">Set custom payment amounts per currency. Leave empty to use auto FX conversion.</p>
        <div v-for="(entry, idx) in currencyOverrideEntries" :key="idx" class="mb-2 flex items-center gap-2">
          <input v-model="entry.currency" type="text" maxlength="3" placeholder="USD" class="input w-20 uppercase" />
          <input v-model.number="entry.amount" type="number" step="0.01" min="0" placeholder="Amount" class="input flex-1" />
          <button type="button" @click="removeCurrencyOverride(idx)" class="text-red-500 hover:text-red-700">&times;</button>
        </div>
        <button type="button" @click="addCurrencyOverride" class="text-xs text-primary-500 hover:text-primary-700">+ Add currency override</button>
      </div>

      <div class="flex flex-wrap items-center gap-6">
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.popular" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('payment.admin.popular') }}
        </label>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.for_sale" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('payment.admin.forSale') }}
        </label>
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="submit" form="balance-package-form" :disabled="saving" class="btn btn-primary">{{ saving ? t('common.saving') : t('common.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { BalancePackage } from '@/types/payment'
import type { AdminGroup } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean; pkg: BalancePackage | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const saving = ref(false)
const groupsLoading = ref(false)
const groups = ref<AdminGroup[]>([])
const balanceGroups = computed(() => groups.value.filter((group) => group.status === 'active' && group.subscription_type === 'standard'))

const form = reactive({ code: '', label: '', description: '', amount_ledger: 0, actual_credits: 0, balance_group_id: 0, badge: '', popular: false, for_sale: true, sort_order: 0 })
const currencyOverrideEntries = ref<{ currency: string; amount: number | null }[]>([])

function addCurrencyOverride() {
  currencyOverrideEntries.value.push({ currency: '', amount: null })
}
function removeCurrencyOverride(idx: number) {
  currencyOverrideEntries.value.splice(idx, 1)
}
function buildCurrencyOverridesMap(): Record<string, number> {
  const map: Record<string, number> = {}
  for (const entry of currencyOverrideEntries.value) {
    const code = entry.currency.trim().toUpperCase()
    if (code.length === 3 && entry.amount && entry.amount > 0) {
      map[code] = entry.amount
    }
  }
  return map
}
function loadCurrencyOverrideEntries(overrides?: Record<string, number>) {
  if (!overrides || Object.keys(overrides).length === 0) {
    currencyOverrideEntries.value = []
    return
  }
  currencyOverrideEntries.value = Object.entries(overrides).map(([currency, amount]) => ({ currency, amount }))
}
const safeNumber = (v: unknown) => Number.isFinite(Number(v)) ? Number(v) : 0

function formatTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(1)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(0)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(0)}K`
  return String(value)
}

watch(() => props.show, (visible) => {
  if (!visible) return
  void loadBalanceGroups()
  if (props.pkg) {
    Object.assign(form, {
      code: props.pkg.code,
      label: props.pkg.label,
      description: props.pkg.description || '',
      amount_ledger: props.pkg.amount_ledger,
      actual_credits: props.pkg.actual_credits || 0,
      balance_group_id: props.pkg.balance_group_id || props.pkg.group_id || 0,
      badge: props.pkg.badge || '',
      popular: !!props.pkg.popular,
      for_sale: !!props.pkg.for_sale,
      sort_order: props.pkg.sort_order || 0,
    })
    loadCurrencyOverrideEntries(props.pkg.currency_overrides)
  } else {
    Object.assign(form, { code: '', label: '', description: '', amount_ledger: 0, actual_credits: 0, balance_group_id: 0, badge: '', popular: false, for_sale: true, sort_order: 0 })
    currencyOverrideEntries.value = []
  }
})

async function loadBalanceGroups() {
  if (groupsLoading.value || groups.value.length > 0) return
  groupsLoading.value = true
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    groupsLoading.value = false
  }
}

function buildPayload() {
  return {
    code: form.code.trim(),
    label: form.label.trim(),
    description: form.description.trim(),
    amount_ledger: safeNumber(form.amount_ledger),
    actual_credits: safeNumber(form.actual_credits),
    balance_group_id: safeNumber(form.balance_group_id),
    badge: form.badge.trim(),
    popular: form.popular,
    for_sale: form.for_sale,
    sort_order: safeNumber(form.sort_order),
    currency_overrides: buildCurrencyOverridesMap(),
  }
}

async function handleSave() {
  if (!form.code.trim() || !form.label.trim()) {
    appStore.showError(t('payment.admin.packageRequired'))
    return
  }
  if (safeNumber(form.amount_ledger) <= 0) {
    appStore.showError(t('payment.admin.packageAmountRequired'))
    return
  }
  if (safeNumber(form.actual_credits) <= 0) {
    appStore.showError('Tokens must be > 0')
    return
  }
  if (safeNumber(form.balance_group_id) <= 0) {
    appStore.showError(t('payment.admin.balanceGroupRequired'))
    return
  }
  saving.value = true
  try {
    const data = buildPayload()
    if (props.pkg) await adminPaymentAPI.updateBalancePackage(props.pkg.id, data)
    else await adminPaymentAPI.createBalancePackage(data)
    appStore.showSuccess(t('common.saved'))
    emit('close')
    emit('saved')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    saving.value = false
  }
}
</script>
