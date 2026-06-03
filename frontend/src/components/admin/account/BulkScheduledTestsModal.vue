<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.bulkScheduledTests.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="rounded-xl border border-primary-200 bg-primary-50/50 p-4 dark:border-primary-800 dark:bg-primary-900/20">
        <div class="mb-3 text-sm text-gray-600 dark:text-gray-300">
          {{ t('admin.accounts.bulkScheduledTests.selectionInfo', { count: accounts.length, platform: platformLabel }) }}
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
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
          </div>
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
                  <p>{{ t('admin.scheduledTests.cronTooltipMeaning') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleEvery30Min') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleHourly') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleDaily') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleWeekly') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipRange') }}</p>
                </div>
              </HelpTooltip>
            </label>
            <Input
              v-model="form.cron_expression"
              :placeholder="'*/30 * * * *'"
              :hint="t('admin.scheduledTests.cronHelp')"
            />
          </div>
          <div>
            <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.maxResults') }}
              <HelpTooltip>
                <template #trigger>
                  <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                    ?
                  </span>
                </template>
                <div class="space-y-1.5">
                  <p class="font-medium">{{ t('admin.scheduledTests.maxResultsTooltipTitle') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipMeaning') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipBody') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipExample') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipRange') }}</p>
                </div>
              </HelpTooltip>
            </label>
            <Input
              v-model="form.max_results"
              type="number"
              placeholder="100"
            />
          </div>
          <div class="flex items-end">
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <Toggle v-model="form.enabled" />
              {{ t('admin.scheduledTests.enabled') }}
            </label>
          </div>
          <div class="flex items-end">
            <div>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <Toggle v-model="form.auto_recover" />
                {{ t('admin.scheduledTests.autoRecover') }}
              </label>
              <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                {{ t('admin.scheduledTests.autoRecoverHelp') }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
        <div class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
          {{ t('admin.accounts.bulkScheduledTests.accounts') }}
        </div>
        <div class="max-h-48 overflow-y-auto">
          <div
            v-for="account in accounts"
            :key="account.id"
            class="flex items-center justify-between gap-3 border-b border-gray-100 py-2 text-sm last:border-b-0 dark:border-dark-700"
          >
            <span class="truncate text-gray-800 dark:text-gray-100">{{ account.name }}</span>
            <span class="text-xs text-gray-400">#{{ account.id }}</span>
          </div>
        </div>
      </div>

      <div class="flex justify-end gap-2">
        <button
          @click="emit('close')"
          class="rounded-lg bg-gray-100 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          @click="handleSubmit"
          :disabled="submitting || !form.model_id || !form.cron_expression || accounts.length === 0"
          class="flex items-center gap-1.5 rounded-lg bg-primary-500 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Icon v-if="submitting" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          {{ t('common.save') }}
        </button>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Input from '@/components/common/Input.vue'
import Toggle from '@/components/common/Toggle.vue'
import { Icon } from '@/components/icons'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { Account } from '@/types'

const props = defineProps<{
  show: boolean
  accounts: Account[]
  modelOptions: SelectOption[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const submitting = ref(false)
const form = reactive({
  model_id: '' as string,
  cron_expression: '' as string,
  max_results: '100' as string,
  enabled: true,
  auto_recover: false
})

const platformLabel = computed(() => props.accounts[0]?.platform ?? '-')

const resetForm = () => {
  form.model_id = ''
  form.cron_expression = ''
  form.max_results = '100'
  form.enabled = true
  form.auto_recover = false
}

watch(
  () => props.show,
  (visible) => {
    if (!visible) {
      resetForm()
    }
  }
)

const handleSubmit = async () => {
  if (submitting.value || props.accounts.length === 0 || !form.model_id || !form.cron_expression) return

  submitting.value = true
  const maxResults = Number(form.max_results) || 100
  let success = 0
  const failedAccounts: string[] = []

  try {
    for (const account of props.accounts) {
      try {
        await adminAPI.scheduledTests.create({
          account_id: account.id,
          model_id: form.model_id,
          cron_expression: form.cron_expression,
          enabled: form.enabled,
          max_results: maxResults,
          auto_recover: form.auto_recover
        })
        success += 1
      } catch (error: any) {
        failedAccounts.push(account.name || `#${account.id}`)
        console.error('Failed to create scheduled test plan for account:', account.id, error)
      }
    }

    if (success > 0 && failedAccounts.length === 0) {
      appStore.showSuccess(t('admin.accounts.bulkScheduledTests.success', { count: success }))
      emit('saved')
      emit('close')
      return
    }

    if (success > 0) {
      appStore.showError(
        t('admin.accounts.bulkScheduledTests.partialSuccess', {
          success,
          failed: failedAccounts.length
        })
      )
      emit('saved')
      emit('close')
      return
    }

    appStore.showError(t('admin.accounts.bulkScheduledTests.failed'))
  } finally {
    submitting.value = false
  }
}
</script>
