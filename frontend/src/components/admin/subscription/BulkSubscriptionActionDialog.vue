<template>
  <BaseDialog
    :show="show"
    :title="t(`admin.subscriptions.bulk.${currentAction}`)"
    width="normal"
    :close-on-escape="canClose"
    :show-close-button="canClose"
    @close="handleClose"
  >
    <form id="bulk-subscription-action-form" class="space-y-4" novalidate @submit.prevent="submit">
      <div>
        <p class="mb-2 text-sm font-medium text-gray-900 dark:text-gray-100">
          {{ t('admin.subscriptions.bulk.confirmTargets', { count: targets.length }) }}
        </p>
        <ul class="max-h-48 divide-y divide-gray-100 overflow-y-auto rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-600">
          <li v-for="subscription in targets" :key="subscription.id" class="px-3 py-2 text-sm">
            <div class="break-all text-gray-900 dark:text-gray-100">
              {{ subscription.email || `#${subscription.id}` }}
            </div>
            <div class="break-words text-xs text-gray-500 dark:text-gray-400">
              {{ subscription.group || t('admin.subscriptions.bulk.groupFallback', { id: subscription.groupId }) }}
              <span v-if="subscription.email" class="ml-2">#{{ subscription.id }}</span>
            </div>
          </li>
        </ul>
      </div>

      <fieldset :disabled="parametersLocked" class="space-y-3 disabled:opacity-70">
        <div v-if="currentAction === 'extend'">
          <label for="bulk-subscription-days" class="input-label">
            {{ t('admin.subscriptions.form.adjustDays') }}
          </label>
          <input
            id="bulk-subscription-days"
            v-model.number="days"
            type="number"
            min="-36500"
            max="36500"
            step="1"
            class="input"
            :disabled="parametersLocked"
            :placeholder="t('admin.subscriptions.adjustDaysPlaceholder')"
            aria-describedby="bulk-subscription-days-hint"
          />
          <p id="bulk-subscription-days-hint" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.subscriptions.bulk.extendHint') }}
          </p>
        </div>

        <template v-else-if="currentAction === 'reset_quota'">
          <legend class="text-sm font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.subscriptions.bulk.resetWindows') }}
          </legend>
          <div class="flex flex-wrap gap-5">
            <label v-for="window in quotaWindows" :key="window" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input
                v-model="windows[window]"
                :name="window"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                :disabled="parametersLocked"
              />
              {{ t(`admin.subscriptions.${window}`) }}
            </label>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.subscriptions.bulk.resetHint') }}
          </p>
        </template>

        <p v-else class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
          {{ t(`admin.subscriptions.bulk.${currentAction}Hint`) }}
        </p>
      </fieldset>

      <p v-if="validationError && !parametersLocked" role="alert" class="text-sm text-red-600 dark:text-red-400">
        {{ validationError }}
      </p>

      <div v-if="requestError" role="alert" class="space-y-2 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
        <p>{{ requestError }}</p>
        <p v-if="pendingOperation">{{ t('admin.subscriptions.bulk.retryHint') }}</p>
      </div>

      <div v-if="result" aria-live="polite" class="space-y-3">
        <p class="rounded-lg bg-gray-50 p-3 text-sm font-medium text-gray-900 dark:bg-dark-700 dark:text-gray-100">
          {{ t('admin.subscriptions.bulk.result', { success: result.success_count, failed: result.failed_count }) }}
        </p>
        <ul v-if="failedResults.length" class="max-h-48 space-y-2 overflow-y-auto">
          <li v-for="item in failedResults" :key="item.subscription_id" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
            <p class="break-all font-medium">{{ failedTargetLabel(item.subscription_id) }}</p>
            <p class="mt-1 break-words">{{ item.error || t('admin.subscriptions.bulk.itemFailed') }}</p>
          </li>
        </ul>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="!canClose" @click="handleClose">
          {{ result ? t('common.close') : t('common.cancel') }}
        </button>
        <button
          v-if="!result"
          type="submit"
          form="bulk-subscription-action-form"
          :class="['btn', currentAction === 'revoke' ? 'btn-danger' : 'btn-primary']"
          :disabled="submitting || (!pendingOperation && !!validationError)"
        >
          {{ submitting ? t('common.processing') : pendingOperation ? t('admin.subscriptions.bulk.retry') : t('admin.subscriptions.bulk.confirm') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { SubscriptionBulkAction, SubscriptionBulkActionRequest, SubscriptionBulkActionResult } from '@/api/admin/subscriptions'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { UserSubscription } from '@/types'
import { completeBulkSubscriptionOperation, prepareBulkSubscriptionOperation, type BulkSubscriptionOperation } from './bulkSubscriptionOperation'

const props = defineProps<{
  show: boolean
  action: SubscriptionBulkAction
  subscriptions: UserSubscription[]
}>()

const emit = defineEmits<{
  close: []
  completed: [result: SubscriptionBulkActionResult]
}>()

const { t } = useI18n()
const quotaWindows = ['daily', 'weekly', 'monthly'] as const
const days = ref<number | string>(30)
const windows = reactive({ daily: true, weekly: true, monthly: true })
const submitting = ref(false)
const requestError = ref('')
const result = shallowRef<SubscriptionBulkActionResult | null>(null)
const pendingOperation = shallowRef<BulkSubscriptionOperation | null>(null)
const targets = ref<{ id: number; email?: string; group?: string; groupId: number }[]>([])
const submittedAction = ref<SubscriptionBulkAction | null>(null)

const currentAction = computed(() => submittedAction.value ?? props.action)
const parametersLocked = computed(() => submitting.value || !!pendingOperation.value || !!result.value)
const canClose = computed(() => !submitting.value)
const failedResults = computed(() => result.value?.results.filter(item => !item.success) ?? [])
const validationError = computed(() => {
  if (targets.value.length === 0) return t('admin.subscriptions.bulk.selectionRequired')
  if (targets.value.length > 100) return t('admin.subscriptions.bulk.selectionLimit')
  if (currentAction.value === 'extend') {
    const value = Number(days.value)
    if (!Number.isInteger(value) || value === 0 || Math.abs(value) > 36500) {
      return t('admin.subscriptions.bulk.invalidDays')
    }
  }
  if (currentAction.value === 'reset_quota' && !quotaWindows.some(window => windows[window])) {
    return t('admin.subscriptions.bulk.selectWindow')
  }
  return ''
})

watch(() => props.subscriptions, subscriptions => {
  if (parametersLocked.value) return
  targets.value = subscriptions.map(subscription => ({
    id: subscription.id,
    email: subscription.user?.email,
    group: subscription.group?.name,
    groupId: subscription.group_id
  }))
}, { immediate: true })

function handleClose() {
  if (canClose.value) emit('close')
}

function failedTargetLabel(id: number) {
  const target = targets.value.find(item => item.id === id)
  return target?.email ? `${target.email} · #${id}` : `#${id}`
}

async function submit() {
  if (submitting.value || result.value) return
  if (!pendingOperation.value) {
    if (validationError.value) return
    const request: SubscriptionBulkActionRequest = {
      subscription_ids: targets.value.map(subscription => subscription.id),
      action: props.action
    }
    if (request.action === 'extend') request.days = Number(days.value)
    if (request.action === 'reset_quota') Object.assign(request, { ...windows })
    pendingOperation.value = prepareBulkSubscriptionOperation(request)
    submittedAction.value = request.action
  }

  const operation = pendingOperation.value
  submitting.value = true
  requestError.value = ''
  try {
    result.value = await adminAPI.subscriptions.bulkAction(operation.request, operation.key)
    completeBulkSubscriptionOperation(operation)
    pendingOperation.value = null
    emit('completed', result.value)
  } catch (error: unknown) {
    const failure = error as { status?: number; message?: string; response?: { status?: number; data?: { message?: string } } }
    requestError.value = failure?.message || failure?.response?.data?.message || t('admin.subscriptions.bulk.requestFailed')
    const status = failure?.status ?? failure?.response?.status
    // A timeout, server error or in-progress conflict may arrive after writes.
    // Retry the frozen payload and key until a definitive result is available.
    if (!operation.outcomeUncertain && status && status >= 400 && status < 500 && status !== 408 && status !== 409) {
      completeBulkSubscriptionOperation(operation)
      pendingOperation.value = null
      submittedAction.value = null
    } else {
      operation.outcomeUncertain = true
    }
  } finally {
    submitting.value = false
  }
}
</script>
