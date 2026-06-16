<template>
  <BaseDialog :show="show" :title="t('admin.accounts.health.batchModal.title')" width="wide" @close="handleClose">
    <div class="space-y-4">
      <p class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.health.batchModal.subtitle', { count: accountIds.length }) }}
      </p>

      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <!-- Model -->
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
            {{ t('admin.scheduledTests.model') }}
          </label>
          <Select
            v-model="form.model_id"
            :options="modelOptions"
            :placeholder="t('admin.scheduledTests.model')"
            :searchable="modelOptions.length > 5"
          />
          <p v-if="modelOptions.length === 0" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
            {{ t('admin.accounts.health.batchModal.noModels') }}
          </p>
        </div>

        <!-- Cron -->
        <div>
          <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
            {{ t('admin.scheduledTests.cronExpression') }}
            <HelpTooltip>
              <template #trigger>
                <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                  ?
                </span>
              </template>
              <div class="space-y-1.5">
                <p class="font-medium">{{ t('admin.scheduledTests.cronTooltipTitle') }}</p>
                <p>{{ t('admin.scheduledTests.cronTooltipExampleEvery30Min') }}</p>
                <p>{{ t('admin.scheduledTests.cronTooltipExampleHourly') }}</p>
                <p>{{ t('admin.scheduledTests.cronTooltipExampleDaily') }}</p>
                <p>{{ t('admin.scheduledTests.cronTooltipRange') }}</p>
              </div>
            </HelpTooltip>
          </label>
          <Input v-model="form.cron_expression" :placeholder="'*/30 * * * *'" :hint="t('admin.scheduledTests.cronHelp')" />
        </div>

        <!-- Max results -->
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
            {{ t('admin.scheduledTests.maxResults') }}
          </label>
          <Input v-model="form.max_results" type="number" placeholder="100" />
        </div>

        <!-- Conflict strategy -->
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
            {{ t('admin.accounts.health.batchModal.conflictStrategy') }}
          </label>
          <Select v-model="form.conflict_strategy" :options="conflictOptions" />
        </div>
      </div>

      <div class="flex flex-wrap gap-4">
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.enabled" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('admin.scheduledTests.enabled') }}
        </label>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.auto_recover" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('admin.scheduledTests.autoRecover') }}
        </label>
      </div>

      <!-- Result summary -->
      <div v-if="result" class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-600 dark:bg-dark-700/40">
        <div class="mb-1 font-medium text-gray-700 dark:text-gray-200">
          {{ t('admin.accounts.health.batchModal.resultSummary', { success: result.success, failed: result.failed, skipped: result.skipped }) }}
        </div>
        <ul v-if="failedItems.length > 0" class="mt-1 max-h-32 space-y-0.5 overflow-y-auto text-xs text-red-600 dark:text-red-400">
          <li v-for="item in failedItems" :key="item.account_id">
            #{{ item.account_id }}: {{ item.error }}
          </li>
        </ul>
      </div>
    </div>

    <template #footer>
      <button class="btn btn-secondary" @click="handleClose">{{ t('common.cancel') }}</button>
      <button class="btn btn-primary" :disabled="submitting || !canSubmit" @click="handleSubmit">
        {{ submitting ? t('common.loading') : t('admin.accounts.health.batchModal.submit') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import BaseDialog from '@/components/common/BaseDialog.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Input from '@/components/common/Input.vue'
import type { BatchPlanConflictStrategy, BatchCreateScheduledTestPlansResult } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const props = defineProps<{
  show: boolean
  accountIds: number[]
  modelOptions: SelectOption[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submitted', result: BatchCreateScheduledTestPlansResult): void
}>()

const form = reactive<{
  model_id: string
  cron_expression: string
  max_results: number | string
  enabled: boolean
  auto_recover: boolean
  conflict_strategy: string
}>({
  model_id: '',
  cron_expression: '*/30 * * * *',
  max_results: 100,
  enabled: true,
  auto_recover: false,
  conflict_strategy: 'overwrite'
})

const submitting = ref(false)
const result = ref<BatchCreateScheduledTestPlansResult | null>(null)

const conflictOptions = computed<SelectOption[]>(() => [
  { value: 'overwrite', label: t('admin.accounts.health.batchModal.conflictOverwrite') },
  { value: 'skip', label: t('admin.accounts.health.batchModal.conflictSkip') },
  { value: 'add', label: t('admin.accounts.health.batchModal.conflictAdd') }
])

const failedItems = computed(() => (result.value?.results ?? []).filter(r => r.action === 'failed'))

const canSubmit = computed(() => props.accountIds.length > 0 && !!form.model_id && !!form.cron_expression.trim())

// Reset form/result each time the modal opens
watch(
  () => props.show,
  (open) => {
    if (open) {
      result.value = null
      if (!form.model_id && props.modelOptions.length > 0) {
        form.model_id = String(props.modelOptions[0].value)
      }
    }
  }
)

watch(
  () => props.modelOptions,
  (options) => {
    if (props.show && !form.model_id && options.length > 0) {
      form.model_id = String(options[0].value)
    }
  },
  { deep: true }
)

const handleClose = () => {
  if (submitting.value) return
  emit('close')
}

const handleSubmit = async () => {
  if (!canSubmit.value) return
  submitting.value = true
  result.value = null
  try {
    const maxResults = Number(form.max_results)
    const res = await adminAPI.scheduledTests.batchCreate({
      account_ids: props.accountIds,
      model_id: form.model_id,
      cron_expression: form.cron_expression.trim(),
      enabled: form.enabled,
      max_results: Number.isFinite(maxResults) && maxResults > 0 ? maxResults : undefined,
      auto_recover: form.auto_recover,
      conflict_strategy: form.conflict_strategy as BatchPlanConflictStrategy
    })
    result.value = res
    if (res.failed === 0) {
      appStore.showSuccess(
        t('admin.accounts.health.batchModal.success', { success: res.success, skipped: res.skipped })
      )
      emit('submitted', res)
      emit('close')
    } else {
      appStore.showError(
        t('admin.accounts.health.batchModal.partial', { success: res.success, failed: res.failed })
      )
      emit('submitted', res)
    }
  } catch (e: unknown) {
    const err = e as { message?: string }
    appStore.showError(err?.message || t('common.error'))
  } finally {
    submitting.value = false
  }
}
</script>
