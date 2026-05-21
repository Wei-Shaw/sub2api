<template>
  <div class="flex flex-wrap items-center gap-3">
    <SearchInput
      :model-value="searchQuery"
      :placeholder="t('admin.accounts.searchAccounts')"
      class="w-full sm:w-64"
      @update:model-value="$emit('update:searchQuery', $event)"
      @search="$emit('change')"
    />
    <Select :model-value="filters.platform" class="w-40" :options="pOpts" @update:model-value="updatePlatform" @change="$emit('change')" />
    <Select :model-value="filters.type" class="w-40" :options="tOpts" @update:model-value="updateType" @change="$emit('change')" />
    <Select :model-value="filters.status" class="w-40" :options="sOpts" @update:model-value="updateStatus" @change="handleStatusSelectChange" />
    <Select :model-value="filters.privacy_mode" class="w-40" :options="privacyOpts" @update:model-value="updatePrivacyMode" @change="$emit('change')" />
    <Select :model-value="filters.group" class="w-40" :options="gOpts" @update:model-value="updateGroup" @change="$emit('change')" />
  </div>
  <BaseDialog
    :show="showQuotaRangeDialog"
    :title="t('admin.accounts.status.openAIQuotaUsedRange')"
    width="narrow"
    @close="showQuotaRangeDialog = false"
  >
    <div class="space-y-4">
      <div class="space-y-2">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.status.openAIQuotaWindow') }}</label>
        <div class="flex gap-4">
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="quotaRangeForm.window" type="radio" value="5h" class="text-primary-600 focus:ring-primary-500" />
            <span>5H</span>
          </label>
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="quotaRangeForm.window" type="radio" value="7d" class="text-primary-600 focus:ring-primary-500" />
            <span>7D</span>
          </label>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <label class="space-y-1 text-sm font-medium text-gray-700 dark:text-gray-300">
          <span>{{ t('admin.accounts.status.openAIQuotaRangeMin') }}</span>
          <input v-model.number="quotaRangeForm.min" type="number" min="0" max="100" step="1" class="input" />
        </label>
        <label class="space-y-1 text-sm font-medium text-gray-700 dark:text-gray-300">
          <span>{{ t('admin.accounts.status.openAIQuotaRangeMax') }}</span>
          <input v-model.number="quotaRangeForm.max" type="number" min="0" max="100" step="1" class="input" />
        </label>
      </div>
      <p v-if="quotaRangeError" class="text-sm text-red-600 dark:text-red-400">{{ quotaRangeError }}</p>
      <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.accounts.status.openAIQuotaUsedRangeHint') }}</p>
    </div>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="showQuotaRangeDialog = false">{{ t('common.cancel') }}</button>
      <button type="button" class="btn btn-primary" @click="applyQuotaRangeFilter">{{ t('common.confirm') }}</button>
    </template>
  </BaseDialog>
  <BaseDialog
    :show="showQuotaFullDialog"
    :title="t('admin.accounts.status.openAIQuotaFull')"
    width="narrow"
    @close="showQuotaFullDialog = false"
  >
    <div class="space-y-4">
      <div class="space-y-2">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.status.openAIQuotaWindow') }}</label>
        <div class="flex gap-4">
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="quotaFullWindow" type="radio" value="5h" class="text-primary-600 focus:ring-primary-500" />
            <span>5H</span>
          </label>
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="quotaFullWindow" type="radio" value="7d" class="text-primary-600 focus:ring-primary-500" />
            <span>7D</span>
          </label>
        </div>
      </div>
      <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.accounts.status.openAIQuotaFullHint') }}</p>
    </div>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="showQuotaFullDialog = false">{{ t('common.cancel') }}</button>
      <button type="button" class="btn btn-primary" @click="applyQuotaFullFilter">{{ t('common.confirm') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'; import { useI18n } from 'vue-i18n'; import Select from '@/components/common/Select.vue'; import SearchInput from '@/components/common/SearchInput.vue'; import BaseDialog from '@/components/common/BaseDialog.vue'
import type { AdminGroup } from '@/types'
import {
  ACCOUNT_STATUS_FILTER_QUOTA_FULL_PICKER,
  ACCOUNT_STATUS_FILTER_QUOTA_USED_RANGE_PICKER,
  encodeOpenAIQuotaFullStatus,
  encodeOpenAIQuotaUsedRangeStatus,
  parseOpenAIQuotaFullStatus,
  parseOpenAIQuotaUsedRangeStatus,
  type OpenAIQuotaStatusWindow
} from './accountStatusFilter'
const props = defineProps<{ searchQuery: string; filters: Record<string, any>; groups?: AdminGroup[] }>()
const emit = defineEmits(['update:searchQuery', 'update:filters', 'change']); const { t } = useI18n()
const updatePlatform = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, platform: value }) }
const updateType = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, type: value }) }
const showQuotaRangeDialog = ref(false)
const showQuotaFullDialog = ref(false)
const quotaRangeError = ref('')
const quotaRangeForm = reactive<{ window: OpenAIQuotaStatusWindow; min: number; max: number }>({ window: '7d', min: 0, max: 100 })
const quotaFullWindow = ref<OpenAIQuotaStatusWindow>('7d')
const emitStatusFilter = (status: string) => {
  emit('update:filters', { ...props.filters, status })
  emit('change')
}
const updateStatus = (value: string | number | boolean | null) => {
  const next = String(value || '')
  if (next === ACCOUNT_STATUS_FILTER_QUOTA_USED_RANGE_PICKER) {
    const current = parseOpenAIQuotaUsedRangeStatus(String(props.filters.status || ''))
    if (current) {
      quotaRangeForm.window = current.window
      quotaRangeForm.min = current.min
      quotaRangeForm.max = current.max
    }
    quotaRangeError.value = ''
    showQuotaRangeDialog.value = true
    return
  }
  if (next === ACCOUNT_STATUS_FILTER_QUOTA_FULL_PICKER) {
    quotaFullWindow.value = parseOpenAIQuotaFullStatus(String(props.filters.status || '')) || '7d'
    showQuotaFullDialog.value = true
    return
  }
  emit('update:filters', { ...props.filters, status: value })
}
const handleStatusSelectChange = (value: string | number | boolean | null) => {
  const next = String(value || '')
  if (next === ACCOUNT_STATUS_FILTER_QUOTA_USED_RANGE_PICKER || next === ACCOUNT_STATUS_FILTER_QUOTA_FULL_PICKER) return
  emit('change')
}
const applyQuotaRangeFilter = () => {
  quotaRangeError.value = ''
  const min = Number(quotaRangeForm.min)
  const max = Number(quotaRangeForm.max)
  if (!Number.isFinite(min) || !Number.isFinite(max) || min < 0 || max < 0 || min > 100 || max > 100 || min > max) {
    quotaRangeError.value = t('admin.accounts.status.openAIQuotaRangeInvalid')
    return
  }
  showQuotaRangeDialog.value = false
  emitStatusFilter(encodeOpenAIQuotaUsedRangeStatus(quotaRangeForm.window, min, max))
}
const applyQuotaFullFilter = () => {
  showQuotaFullDialog.value = false
  emitStatusFilter(encodeOpenAIQuotaFullStatus(quotaFullWindow.value))
}
const updatePrivacyMode = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, privacy_mode: value }) }
const updateGroup = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, group: value }) }
const pOpts = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') }, { value: 'anthropic', label: 'Anthropic' }, { value: 'openai', label: 'OpenAI' }, { value: 'gemini', label: 'Gemini' }, { value: 'antigravity', label: 'Antigravity' }])
const tOpts = computed(() => [{ value: '', label: t('admin.accounts.allTypes') }, { value: 'oauth', label: t('admin.accounts.oauthType') }, { value: 'setup-token', label: t('admin.accounts.setupToken') }, { value: 'apikey', label: t('admin.accounts.apiKey') }, { value: 'bedrock', label: 'AWS Bedrock' }])
const sOpts = computed(() => [
  { value: '', label: t('admin.accounts.allStatus') },
  { value: 'active', label: t('admin.accounts.status.active') },
  { value: 'active_excluding_quota_stopped', label: t('admin.accounts.status.activeExcludingQuotaStopped') },
  { value: 'openai_5h_used_zero', label: t('admin.accounts.status.openAI5HUsedZero') },
  { value: 'openai_7d_used_zero', label: t('admin.accounts.status.openAI7DUsedZero') },
  { value: ACCOUNT_STATUS_FILTER_QUOTA_USED_RANGE_PICKER, label: t('admin.accounts.status.openAIQuotaUsedRange') },
  { value: 'inactive', label: t('admin.accounts.status.inactive') },
  { value: 'error', label: t('admin.accounts.status.error') },
  { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') },
  { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') },
  { value: 'unschedulable', label: t('admin.accounts.status.unschedulable') },
  { value: ACCOUNT_STATUS_FILTER_QUOTA_FULL_PICKER, label: t('admin.accounts.status.openAIQuotaFull') },
  ...encodedStatusOption.value
])
const encodedStatusOption = computed(() => {
  const status = String(props.filters.status || '')
  const range = parseOpenAIQuotaUsedRangeStatus(status)
  if (range) {
    return [{
      value: status,
      label: t('admin.accounts.status.openAIQuotaUsedRangeValue', {
        window: range.window.toUpperCase(),
        min: range.min,
        max: range.max
      })
    }]
  }
  const fullWindow = parseOpenAIQuotaFullStatus(status)
  if (fullWindow) {
    return [{
      value: status,
      label: t('admin.accounts.status.openAIQuotaFullValue', { window: fullWindow.toUpperCase() })
    }]
  }
  return []
})
const privacyOpts = computed(() => [
  { value: '', label: t('admin.accounts.allPrivacyModes') },
  { value: '__unset__', label: t('admin.accounts.privacyUnset') },
  { value: 'training_off', label: 'Privacy' },
  { value: 'training_set_cf_blocked', label: 'CF' },
  { value: 'training_set_failed', label: 'Fail' }
])
const gOpts = computed(() => [
  { value: '', label: t('admin.accounts.allGroups') },
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') },
  ...(props.groups || []).map(g => ({ value: String(g.id), label: g.name }))
])
</script>
