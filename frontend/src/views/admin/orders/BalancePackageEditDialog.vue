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

      <div class="grid grid-cols-3 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.amountLedger') }} <span class="text-red-500">*</span></label>
          <input v-model.number="form.amount_ledger" type="number" step="0.01" min="0.01" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.creditLedger') }} <span class="text-red-500">*</span></label>
          <input v-model.number="form.credit_ledger" type="number" step="0.01" min="0.01" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.creditMultiplier') }}</label>
          <input v-model.number="form.credit_multiplier" type="number" step="0.000001" min="0.000001" class="input" />
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-600 dark:bg-dark-800">
        <div class="grid grid-cols-3 gap-3">
          <div><span class="text-gray-500">{{ t('payment.admin.payAmount') }}:</span> <span class="font-medium">${{ safeNumber(form.amount_ledger).toFixed(2) }}</span></div>
          <div><span class="text-gray-500">{{ t('payment.admin.creditAmount') }}:</span> <span class="font-medium">${{ safeNumber(form.credit_ledger).toFixed(2) }}</span></div>
          <div><span class="text-gray-500">{{ t('payment.admin.bonusLedger') }}:</span> <span class="font-medium">${{ computedBonus.toFixed(2) }}</span></div>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.balancePackageFormula') }}</p>
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
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { BalancePackage } from '@/types/payment'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean; pkg: BalancePackage | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const saving = ref(false)

const form = reactive({ code: '', label: '', description: '', amount_ledger: 0, credit_ledger: 0, credit_multiplier: 0, badge: '', popular: false, for_sale: true, sort_order: 0 })
const safeNumber = (v: unknown) => Number.isFinite(Number(v)) ? Number(v) : 0
const computedBonus = computed(() => Math.max(0, safeNumber(form.credit_ledger) - safeNumber(form.amount_ledger)))

watch(() => props.show, (visible) => {
  if (!visible) return
  if (props.pkg) {
    Object.assign(form, {
      code: props.pkg.code,
      label: props.pkg.label,
      description: props.pkg.description || '',
      amount_ledger: props.pkg.amount_ledger,
      credit_ledger: props.pkg.credit_ledger,
      credit_multiplier: props.pkg.credit_multiplier,
      badge: props.pkg.badge || '',
      popular: !!props.pkg.popular,
      for_sale: !!props.pkg.for_sale,
      sort_order: props.pkg.sort_order || 0,
    })
  } else {
    Object.assign(form, { code: '', label: '', description: '', amount_ledger: 0, credit_ledger: 0, credit_multiplier: 0, badge: '', popular: false, for_sale: true, sort_order: 0 })
  }
})

watch(() => [form.amount_ledger, form.credit_ledger], () => {
  const amount = safeNumber(form.amount_ledger)
  const credit = safeNumber(form.credit_ledger)
  if (amount > 0 && credit > 0) {
    form.credit_multiplier = Number((credit / amount).toFixed(6))
  }
})

function buildPayload() {
  return {
    code: form.code.trim(),
    label: form.label.trim(),
    description: form.description.trim(),
    amount_ledger: safeNumber(form.amount_ledger),
    credit_ledger: safeNumber(form.credit_ledger),
    credit_multiplier: safeNumber(form.credit_multiplier),
    badge: form.badge.trim(),
    popular: form.popular,
    for_sale: form.for_sale,
    sort_order: safeNumber(form.sort_order),
  }
}

async function handleSave() {
  if (!form.code.trim() || !form.label.trim()) {
    appStore.showError(t('payment.admin.packageRequired'))
    return
  }
  if (safeNumber(form.amount_ledger) <= 0 || safeNumber(form.credit_ledger) <= 0) {
    appStore.showError(t('payment.admin.packageAmountRequired'))
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
