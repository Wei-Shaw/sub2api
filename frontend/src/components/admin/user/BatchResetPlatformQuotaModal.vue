<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.bulkQuotaReset.title')"
    width="normal"
    @close="emit('close')"
  >
    <form id="batch-reset-platform-quota-form" class="space-y-5" @submit.prevent="handleSubmit">
      <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.users.bulkQuotaReset.selectedCount', { count: selectedIds.length }) }}
      </p>

      <fieldset class="border-y border-gray-200 py-4 dark:border-dark-700">
        <legend class="input-label px-1">{{ t('admin.users.bulkQuotaReset.platforms') }}</legend>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <label
            v-for="platform in PLATFORMS"
            :key="platform"
            class="flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
          >
            <input
              v-model="selectedPlatforms"
              type="checkbox"
              :value="platform"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            <span class="font-mono text-xs">{{ platform }}</span>
          </label>
        </div>
      </fieldset>

      <fieldset class="border-b border-gray-200 pb-4 dark:border-dark-700">
        <legend class="input-label px-1">{{ t('admin.users.bulkQuotaReset.windows') }}</legend>
        <div class="grid grid-cols-2 gap-3">
          <label
            v-for="window in WINDOWS"
            :key="window.value"
            class="flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
          >
            <input
              v-model="selectedWindows"
              type="checkbox"
              :value="window.value"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            {{ window.label }}
          </label>
        </div>
      </fieldset>

      <p v-if="selectionTooLarge" class="text-sm text-red-600 dark:text-red-400">
        {{ t('admin.users.bulkQuotaReset.selectionLimit', { max: MAX_BATCH_USER_IDS }) }}
      </p>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="batch-reset-platform-quota-form"
          class="btn btn-primary"
          :disabled="!canSubmit"
          data-test="submit-batch-quota-reset"
        >
          {{ submitting ? t('admin.users.bulkQuotaReset.resetting') : t('admin.users.bulkQuotaReset.submit') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { PlatformQuotaPlatform, PlatformQuotaWindow } from '@/api/admin/users'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{
  show: boolean
  selectedIds: number[]
}>()

const emit = defineEmits<{
  close: []
  success: [affected: number]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const MAX_BATCH_USER_IDS = 500
const PLATFORMS: PlatformQuotaPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']
const WINDOWS = computed<Array<{ value: PlatformQuotaWindow; label: string }>>(() => [
  { value: 'five_hour', label: t('admin.users.platformQuota.windowFiveHour') },
  { value: 'daily', label: t('admin.users.platformQuota.windowDaily') },
  { value: 'weekly', label: t('admin.users.platformQuota.windowWeekly') },
  { value: 'monthly', label: t('admin.users.platformQuota.windowMonthly') },
])

const selectedPlatforms = ref<PlatformQuotaPlatform[]>([])
const selectedWindows = ref<PlatformQuotaWindow[]>([])
const submitting = ref(false)
const selectionTooLarge = computed(() => props.selectedIds.length > MAX_BATCH_USER_IDS)
const canSubmit = computed(() =>
  props.selectedIds.length > 0
  && !selectionTooLarge.value
  && selectedPlatforms.value.length > 0
  && selectedWindows.value.length > 0
  && !submitting.value
)

watch(
  () => props.show,
  (show) => {
    if (!show) return
    selectedPlatforms.value = []
    selectedWindows.value = []
    submitting.value = false
  }
)

const handleSubmit = async () => {
  if (!canSubmit.value) return

  const confirmed = window.confirm(
    t('admin.users.bulkQuotaReset.confirm', {
      count: props.selectedIds.length,
      platforms: selectedPlatforms.value.join(', '),
      windows: selectedWindows.value.map((window) =>
        WINDOWS.value.find((item) => item.value === window)?.label ?? window
      ).join(', '),
    })
  )
  if (!confirmed) return

  submitting.value = true
  try {
    const result = await adminAPI.users.batchResetPlatformQuotaWindows({
      user_ids: [...props.selectedIds],
      platforms: [...selectedPlatforms.value],
      windows: [...selectedWindows.value],
    })
    appStore.showSuccess(t('admin.users.bulkQuotaReset.success', { count: result.affected }))
    emit('success', result.affected)
    emit('close')
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.message
      || error.response?.data?.detail
      || t('admin.users.bulkQuotaReset.failed')
    )
  } finally {
    submitting.value = false
  }
}
</script>
