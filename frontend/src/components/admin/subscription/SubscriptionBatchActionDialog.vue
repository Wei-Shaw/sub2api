<template>
  <BaseDialog
    :show="show"
    :title="t('admin.subscriptions.batch.title')"
    width="normal"
    :close-on-escape="!submitting"
    :show-close-button="!submitting"
    @close="handleClose"
  >
    <form id="subscription-batch-action-form" class="space-y-5" @submit.prevent="handleSubmit">
      <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.subscriptions.batch.selectedCount', { count: selectedIds.length }) }}
      </p>

      <div>
        <label class="input-label" for="subscription-batch-action">
          {{ t('admin.subscriptions.batch.action') }}
        </label>
        <Select
          id="subscription-batch-action"
          v-model="action"
          :options="actionOptions"
          :searchable="false"
          data-test="batch-action-select"
        />
      </div>

      <div v-if="action === 'adjust'">
        <label class="input-label" for="subscription-batch-days">
          {{ t('admin.subscriptions.form.adjustDays') }}
        </label>
        <input
          id="subscription-batch-days"
          v-model.number="days"
          type="number"
          min="-36500"
          max="36500"
          step="1"
          class="input"
          data-test="batch-days"
        />
        <p class="input-hint">{{ t('admin.subscriptions.adjustHint') }}</p>
        <p v-if="invalidDays" class="mt-1 text-sm text-red-600 dark:text-red-400">
          {{ t('admin.subscriptions.batch.invalidDays') }}
        </p>
      </div>

      <fieldset v-if="action === 'reset_quota'" class="space-y-3">
        <legend class="input-label">{{ t('admin.subscriptions.batch.resetWindows') }}</legend>
        <label
          v-for="window in resetWindowOptions"
          :key="window.key"
          class="flex items-center gap-3 text-sm text-gray-700 dark:text-gray-300"
        >
          <input
            v-model="resetQuota[window.key]"
            type="checkbox"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800"
          />
          <span>{{ window.label }}</span>
        </label>
        <p v-if="noResetWindow" class="text-sm text-red-600 dark:text-red-400">
          {{ t('admin.subscriptions.batch.selectResetWindow') }}
        </p>
      </fieldset>

      <div
        v-if="isDangerousAction"
        class="border-l-4 border-red-500 bg-red-50 px-4 py-3 dark:bg-red-900/20"
        role="alert"
      >
        <p class="text-sm font-medium text-red-800 dark:text-red-300">
          {{ dangerMessage }}
        </p>
        <label class="mt-3 flex items-start gap-3 text-sm text-red-800 dark:text-red-300">
          <input
            v-model="dangerConfirmed"
            type="checkbox"
            class="mt-0.5 h-4 w-4 rounded border-red-300 text-red-600 focus:ring-red-500 dark:border-red-700 dark:bg-dark-800"
            data-test="danger-confirm"
          />
          <span>{{ t('admin.subscriptions.batch.confirmDanger') }}</span>
        </label>
      </div>

      <p v-if="selectionTooLarge" class="text-sm text-red-600 dark:text-red-400">
        {{ t('admin.subscriptions.batch.selectionLimit', { max: MAX_BATCH_SUBSCRIPTIONS }) }}
      </p>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="submitting" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="subscription-batch-action-form"
          :class="isDangerousAction ? 'btn btn-danger' : 'btn btn-primary'"
          :disabled="!canSubmit"
          data-test="submit"
        >
          {{ submitting ? t('admin.subscriptions.batch.applying') : t('admin.subscriptions.batch.apply') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  BatchSubscriptionActionRequest,
  SubscriptionBatchAction,
  SubscriptionBatchActionResult
} from '@/api/admin/subscriptions'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'

const props = defineProps<{
  show: boolean
  selectedIds: number[]
}>()

const emit = defineEmits<{
  close: []
  success: [result: SubscriptionBatchActionResult]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const MAX_BATCH_SUBSCRIPTIONS = 100
const action = ref<SubscriptionBatchAction>('adjust')
const days = ref(30)
const resetQuota = reactive({ daily: true, weekly: true, monthly: true })
const dangerConfirmed = ref(false)
const submitting = ref(false)
let operationSignature = ''
let operationKey = ''

const actionOptions = computed(() => [
  { value: 'adjust', label: t('admin.subscriptions.batch.actions.adjust') },
  { value: 'reset_quota', label: t('admin.subscriptions.batch.actions.resetQuota') },
  { value: 'revoke', label: t('admin.subscriptions.batch.actions.revoke') },
  { value: 'restore', label: t('admin.subscriptions.batch.actions.restore') },
  { value: 'permanent_delete', label: t('admin.subscriptions.batch.actions.permanentDelete') }
])

const resetWindowOptions = computed(() => [
  { key: 'daily' as const, label: t('admin.subscriptions.daily') },
  { key: 'weekly' as const, label: t('admin.subscriptions.weekly') },
  { key: 'monthly' as const, label: t('admin.subscriptions.monthly') }
])

const invalidDays = computed(() =>
  action.value === 'adjust'
  && (!Number.isInteger(days.value) || days.value === 0 || Math.abs(days.value) > 36500)
)
const noResetWindow = computed(() =>
  action.value === 'reset_quota'
  && !resetQuota.daily
  && !resetQuota.weekly
  && !resetQuota.monthly
)
const selectionTooLarge = computed(() => props.selectedIds.length > MAX_BATCH_SUBSCRIPTIONS)
const isDangerousAction = computed(() =>
  action.value === 'revoke' || action.value === 'permanent_delete'
)
const dangerMessage = computed(() =>
  action.value === 'permanent_delete'
    ? t('admin.subscriptions.batch.permanentDeleteWarning')
    : t('admin.subscriptions.batch.revokeWarning')
)
const canSubmit = computed(() =>
  props.selectedIds.length > 0
  && !selectionTooLarge.value
  && !invalidDays.value
  && !noResetWindow.value
  && (!isDangerousAction.value || dangerConfirmed.value)
  && !submitting.value
)

const reset = () => {
  action.value = 'adjust'
  days.value = 30
  resetQuota.daily = true
  resetQuota.weekly = true
  resetQuota.monthly = true
  dangerConfirmed.value = false
  submitting.value = false
  operationSignature = ''
  operationKey = ''
}

watch(
  () => props.show,
  (show) => {
    if (show) reset()
  }
)

watch(action, () => {
  dangerConfirmed.value = false
})

const createOperationKey = () => {
  const requestID = globalThis.crypto?.randomUUID?.()
    ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `subscription-batch-${requestID}`
}

const buildRequest = (): BatchSubscriptionActionRequest => {
  const request: BatchSubscriptionActionRequest = {
    subscription_ids: [...props.selectedIds],
    action: action.value
  }
  if (action.value === 'adjust') request.days = days.value
  if (action.value === 'reset_quota') request.reset_quota = { ...resetQuota }
  return request
}

const getOperationKey = (request: BatchSubscriptionActionRequest) => {
  const signature = JSON.stringify(request)
  if (!operationKey || operationSignature !== signature) {
    operationSignature = signature
    operationKey = createOperationKey()
  }
  return operationKey
}

const handleClose = () => {
  if (submitting.value) return
  emit('close')
}

const handleSubmit = async () => {
  if (!canSubmit.value) return

  const request = buildRequest()
  submitting.value = true
  try {
    const result = await adminAPI.subscriptions.batchAction(request, getOperationKey(request))
    const message = t('admin.subscriptions.batch.result', {
      succeeded: result.succeeded_count,
      skipped: result.skipped_count,
      failed: result.failed_count
    })
    if (result.skipped_count > 0 || result.failed_count > 0) {
      appStore.showWarning(message)
    } else {
      appStore.showSuccess(message)
    }
    emit('success', result)
    emit('close')
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.message
      || error.response?.data?.detail
      || error.message
      || t('admin.subscriptions.batch.failed')
    )
  } finally {
    submitting.value = false
  }
}
</script>
