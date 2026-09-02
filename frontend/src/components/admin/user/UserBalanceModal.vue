<template>
  <BaseDialog :show="show" :title="balanceType === 'temporary' ? t('admin.users.temporaryBalance.title') : operation === 'add' ? t('admin.users.deposit') : t('admin.users.withdraw')" width="narrow" @close="$emit('close')">
    <form v-if="user" id="balance-form" @submit.prevent="handleBalanceSubmit" class="space-y-5">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100"><span class="text-lg font-medium text-primary-700">{{ user.email.charAt(0).toUpperCase() }}</span></div>
        <div class="flex-1"><p class="font-medium text-gray-900 dark:text-gray-100">{{ user.email }}</p><p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.users.currentBalance') }}: ${{ formatBalance(user.balance) }}</p></div>
      </div>
      <div v-if="operation === 'add'" class="flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700" role="group">
        <button
          type="button"
          data-test="balance-type-regular"
          class="flex-1 rounded-md px-3 py-2 text-sm transition-colors"
          :class="balanceType === 'regular' ? 'bg-white font-medium text-primary-600 shadow-sm dark:bg-dark-600 dark:text-primary-300' : 'text-gray-500 dark:text-gray-400'"
          @click="balanceType = 'regular'"
        >{{ t('admin.users.temporaryBalance.regularBalance') }}</button>
        <button
          type="button"
          data-test="balance-type-temporary"
          class="flex-1 rounded-md px-3 py-2 text-sm transition-colors"
          :class="balanceType === 'temporary' ? 'bg-white font-medium text-primary-600 shadow-sm dark:bg-dark-600 dark:text-primary-300' : 'text-gray-500 dark:text-gray-400'"
          @click="balanceType = 'temporary'"
        >{{ t('admin.users.temporaryBalance.action') }}</button>
      </div>
      <div>
        <label class="input-label">{{ balanceType === 'temporary' ? t('admin.users.temporaryBalance.amount') : operation === 'add' ? t('admin.users.depositAmount') : t('admin.users.withdrawAmount') }}</label>
        <div class="relative flex gap-2">
          <div class="relative flex-1"><div class="absolute left-3 top-1/2 -translate-y-1/2 font-medium text-gray-500">$</div><input v-model.number="form.amount" data-test="balance-amount" type="number" step="any" min="0" required class="input pl-8" /></div>
          <button v-if="operation === 'subtract'" type="button" @click="fillAllBalance" class="btn btn-secondary whitespace-nowrap">{{ t('admin.users.withdrawAll') }}</button>
        </div>
      </div>
      <div v-if="balanceType === 'temporary'" data-test="temporary-balance-expiry-fields">
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
          <label class="input-label mb-0" for="temporary-balance-expires-at">{{ t('admin.users.temporaryBalance.expiresAt') }}</label>
          <div class="flex flex-wrap gap-1.5">
            <button v-for="preset in expiryPresets" :key="preset.key" :data-test="`temporary-expiry-${preset.key}`" type="button" class="btn btn-secondary px-2 py-1 text-xs" @click="applyExpiryPreset(preset.key)">{{ t(`admin.users.temporaryBalance.${preset.key}`) }}</button>
          </div>
        </div>
        <input id="temporary-balance-expires-at" v-model="form.expiresAt" data-test="temporary-balance-expires-at" type="datetime-local" required class="input" />
      </div>
      <div><label class="input-label">{{ t('admin.users.notes') }}</label><textarea v-model="form.notes" rows="3" class="input"></textarea></div>
      <div v-if="form.amount > 0" class="rounded-xl border border-blue-200 bg-blue-50 p-4 dark:border-blue-800 dark:bg-blue-950">
        <div class="flex items-center justify-between text-sm">
          <span class="text-gray-700 dark:text-gray-300">{{ balanceType === 'temporary' ? t('admin.users.temporaryBalance.preview') : t('admin.users.newBalance') }}:</span>
          <span class="font-bold text-gray-900 dark:text-gray-100">${{ formatBalance(balanceType === 'temporary' ? Number(form.amount) : calculateNewBalance()) }}</span>
        </div>
        <p v-if="balanceType === 'temporary'" class="mt-1 text-xs text-blue-700 dark:text-blue-300">{{ t('admin.users.temporaryBalance.previewHint') }}</p>
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="balance-form" :disabled="submitting || !form.amount" class="btn" :class="operation === 'add' ? 'bg-emerald-600 text-white' : 'btn-danger'">{{ submitting ? t('common.saving') : t('common.confirm') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { AdminUser } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean, user: AdminUser | null, operation: 'add' | 'subtract' }>()
const emit = defineEmits(['close', 'success']); const { t } = useI18n(); const appStore = useAppStore()

const submitting = ref(false)
const balanceType = ref<'regular' | 'temporary'>('regular')
const form = reactive({ amount: 0, notes: '', expiresAt: '' })
type ExpiryPreset = 'today' | 'tomorrow' | 'thisWeek' | 'thisMonth'
const expiryPresets: Array<{ key: ExpiryPreset }> = [
  { key: 'today' }, { key: 'tomorrow' }, { key: 'thisWeek' }, { key: 'thisMonth' }
]
watch(() => props.show, (v) => {
  if (v) {
    form.amount = 0
    form.notes = ''
    form.expiresAt = ''
    balanceType.value = 'regular'
  }
})

// 格式化余额：显示完整精度，去除尾部多余的0
const formatBalance = (value: number) => {
  if (value === 0) return '0.00'
  // 最多保留8位小数，去除尾部的0
  const formatted = value.toFixed(8).replace(/\.?0+$/, '')
  // 确保至少有2位小数
  const parts = formatted.split('.')
  if (parts.length === 1) return formatted + '.00'
  if (parts[1].length === 1) return formatted + '0'
  return formatted
}

// 填入全部余额
const fillAllBalance = () => {
  if (props.user) {
    form.amount = props.user.balance
  }
}

const endOfDay = (date: Date) => {
  const next = new Date(date)
  next.setHours(23, 59, 59, 0)
  return next
}

const toLocalInput = (date: Date) => {
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

const applyExpiryPreset = (preset: ExpiryPreset) => {
  const now = new Date()
  if (preset === 'tomorrow') now.setDate(now.getDate() + 1)
  else if (preset === 'thisWeek') now.setDate(now.getDate() + (7 - now.getDay()))
  else if (preset === 'thisMonth') now.setMonth(now.getMonth() + 1, 0)
  form.expiresAt = toLocalInput(endOfDay(now))
}

const calculateNewBalance = () => {
  if (!props.user) return 0
  const result = props.operation === 'add' ? props.user.balance + form.amount : props.user.balance - form.amount
  // 避免浮点数精度问题导致的 -0.00 显示
  return Math.abs(result) < 1e-10 ? 0 : result
}
const handleBalanceSubmit = async () => {
  if (!props.user) return
  if (!form.amount || form.amount <= 0) {
    appStore.showError(t('admin.users.amountRequired'))
    return
  }
  if (balanceType.value === 'temporary') {
    const expiresAt = new Date(form.expiresAt)
    if (!form.expiresAt || Number.isNaN(expiresAt.getTime()) || expiresAt.getTime() <= Date.now()) {
      appStore.showError(t('admin.users.temporaryBalance.expiryRequired'))
      return
    }
    submitting.value = true
    try {
      await adminAPI.users.setTemporaryBalance(props.user.id, {
        amount: Number(form.amount),
        expires_at: expiresAt.toISOString(),
        notes: form.notes.trim()
      })
      appStore.showSuccess(t('admin.users.temporaryBalance.success'))
      emit('success'); emit('close')
    } catch (e: any) {
      console.error('Failed to grant temporary balance:', e)
      appStore.showError(
        e?.response?.data?.detail
          || e?.message
          || e?.reason
          || e?.detail
          || t('admin.users.temporaryBalance.failed')
      )
    } finally {
      submitting.value = false
    }
    return
  }
  // 退款时验证金额不超过实际余额
  if (props.operation === 'subtract' && form.amount > props.user.balance) {
    appStore.showError(t('admin.users.insufficientBalance'))
    return
  }
  submitting.value = true
  try {
    await adminAPI.users.updateBalance(props.user.id, form.amount, props.operation, form.notes)
    appStore.showSuccess(t('common.success')); emit('success'); emit('close')
  } catch (e: any) {
    console.error('Failed to update balance:', e)
    appStore.showError(e.response?.data?.detail || t('common.error'))
  } finally { submitting.value = false }
}
</script>
