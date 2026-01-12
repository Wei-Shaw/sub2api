<template>
  <BaseDialog :show="show" :title="operation === 'add' ? t('admin.users.deposit') : t('admin.users.withdraw')" width="narrow" @close="$emit('close')">
    <form v-if="user" id="balance-form" @submit.prevent="handleBalanceSubmit" class="space-y-5">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100"><span class="text-lg font-medium text-primary-700">{{ user.email.charAt(0).toUpperCase() REDACTEDREDACTED</span></div>
        <div class="flex-1"><p class="font-medium text-gray-900">{{ user.email REDACTEDREDACTED</p><p class="text-sm text-gray-500">{{ t('admin.users.currentBalance') REDACTEDREDACTED: ${{ user.balance.toFixed(2) REDACTEDREDACTED</p></div>
      </div>
      <div>
        <label class="input-label">{{ operation === 'add' ? t('admin.users.depositAmount') : t('admin.users.withdrawAmount') REDACTEDREDACTED</label>
        <div class="relative"><div class="absolute left-3 top-1/2 -translate-y-1/2 font-medium text-gray-500">$</div><input v-model.number="form.amount" type="number" step="0.01" min="0.01" required class="input pl-8" /></div>
      </div>
      <div><label class="input-label">{{ t('admin.users.notes') REDACTEDREDACTED</label><textarea v-model="form.notes" rows="3" class="input"></textarea></div>
      <div v-if="form.amount > 0" class="rounded-xl border border-blue-200 bg-blue-50 p-4"><div class="flex items-center justify-between text-sm"><span>{{ t('admin.users.newBalance') REDACTEDREDACTED:</span><span class="font-bold">${{ calculateNewBalance().toFixed(2) REDACTEDREDACTED</span></div></div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" class="btn btn-secondary">{{ t('common.cancel') REDACTEDREDACTED</button>
        <button type="submit" form="balance-form" :disabled="submitting || !form.amount" class="btn" :class="operation === 'add' ? 'bg-emerald-600 text-white' : 'btn-danger'">{{ submitting ? t('common.saving') : t('common.confirm') REDACTEDREDACTED</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { User REDACTED from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean, user: User | null, operation: 'add' | 'subtract' REDACTED>()
const emit = defineEmits(['close', 'success']); const { t REDACTED = useI18n(); const appStore = useAppStore()

const submitting = ref(false); const form = reactive({ amount: 0, notes: '' REDACTED)
watch(() => props.show, (v) => { if(v) { form.amount = 0; form.notes = '' REDACTED REDACTED)

const calculateNewBalance = () => {
  if (!props.user) return 0
  const result = props.operation === 'add' ? props.user.balance + form.amount : props.user.balance - form.amount
  // 避免浮点数精度问题导致的 -0.00 显示
  return result === 0 || Object.is(result, -0) ? 0 : result
REDACTED
const handleBalanceSubmit = async () => {
  if (!props.user) return
  if (!form.amount || form.amount <= 0) {
    appStore.showError(t('admin.users.amountRequired'))
    return
  REDACTED
  // 使用小数点后两位精度比较，避免浮点数精度问题
  const amount = Math.round(form.amount * 100) / 100
  const balance = Math.round(props.user.balance * 100) / 100
  if (props.operation === 'subtract' && amount > balance) {
    appStore.showError(t('admin.users.insufficientBalance'))
    return
  REDACTED
  submitting.value = true
  try {
    await adminAPI.users.updateBalance(props.user.id, form.amount, props.operation, form.notes)
    appStore.showSuccess(t('common.success')); emit('success'); emit('close')
  REDACTED catch (e: any) {
    console.error('Failed to update balance:', e)
    appStore.showError(e.response?.data?.detail || t('common.error'))
  REDACTED finally { submitting.value = false REDACTED
REDACTED
</script>
