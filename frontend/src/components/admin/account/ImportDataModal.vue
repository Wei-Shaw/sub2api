<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.dataImportTitle')"
    width="normal"
    close-on-click-outside
    @close="handleClose"
  >
    <form id="import-data-form" class="space-y-4" @submit.prevent="handleImport">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.dataImportHint') }}
      </div>
      <div
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-600 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-400"
      >
        {{ t('admin.accounts.dataImportWarning') }}
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.dataImportFile') }}</label>
        <div
          class="flex items-center justify-between gap-3 rounded-lg border border-dashed border-gray-300 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800"
        >
          <div class="min-w-0">
            <div class="truncate text-sm text-gray-700 dark:text-dark-200">
              {{ fileName || t('admin.accounts.dataImportSelectFile') }}
            </div>
            <div class="text-xs text-gray-500 dark:text-dark-400">JSON (.json)</div>
          </div>
          <button type="button" class="btn btn-secondary shrink-0" @click="openFilePicker">
            {{ t('common.chooseFile') }}
          </button>
        </div>
        <input
          ref="fileInput"
          type="file"
          class="hidden"
          accept="application/json,.json"
          @change="handleFileChange"
        />
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.accounts.dataImportTargetGroups') }}
          <span class="font-normal text-gray-400">
            {{ t('common.selectedCount', { count: selectedGroupIds.length }) }}
          </span>
        </label>
        <div
          class="grid max-h-40 grid-cols-1 gap-1 overflow-y-auto rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800 sm:grid-cols-2"
        >
          <label
            v-for="group in importTargetGroups"
            :key="group.id"
            class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm transition-colors hover:bg-white dark:hover:bg-dark-700"
          >
            <input
              v-model="selectedGroupIds"
              type="checkbox"
              :value="group.id"
              class="h-3.5 w-3.5 shrink-0 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
            />
            <span class="min-w-0 flex-1 truncate text-gray-700 dark:text-dark-200">
              {{ group.name }}
            </span>
            <span class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-dark-300">
              {{ group.platform }}
            </span>
            <span
              v-if="group.status === 'inactive'"
              class="shrink-0 rounded bg-amber-100 px-1.5 py-0.5 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
            >
              {{ t('admin.accounts.dataImportGroupInactive') }}
            </span>
          </label>
          <div
            v-if="importTargetGroups.length === 0"
            class="py-2 text-center text-sm text-gray-500 dark:text-gray-400 sm:col-span-2"
          >
            {{ t('common.noGroupsAvailable') }}
          </div>
        </div>
      </div>

      <div
        v-if="result"
        class="space-y-2 rounded-xl border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.dataImportResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.dataImportResultSummary', result) }}
        </div>

        <div v-if="errorItems.length" class="mt-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.dataImportErrors') }}
          </div>
          <div
            class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800"
          >
            <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
              {{ item.kind }} {{ item.name || item.proxy_key || '-' }} — {{ item.message }}
            </div>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          class="btn btn-primary"
          type="submit"
          form="import-data-form"
          :disabled="importing"
        >
          {{ importing ? t('admin.accounts.dataImporting') : t('admin.accounts.dataImportButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { AdminDataImportResult, AdminGroup } from '@/types'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const importing = ref(false)
const file = ref<File | null>(null)
const result = ref<AdminDataImportResult | null>(null)
const importTargetGroups = ref<AdminGroup[]>([])
const selectedGroupIds = ref<number[]>([])

const fileInput = ref<HTMLInputElement | null>(null)
const fileName = computed(() => file.value?.name || '')

const errorItems = computed(() => result.value?.errors || [])
const selectedImportTargetGroups = computed(() =>
  importTargetGroups.value.filter((group) => selectedGroupIds.value.includes(group.id))
)

watch(
  () => props.show,
  (open) => {
    if (open) {
      file.value = null
      result.value = null
      selectedGroupIds.value = []
      void loadImportTargetGroups()
      if (fileInput.value) {
        fileInput.value.value = ''
      }
    }
  },
  { immediate: true }
)

const openFilePicker = () => {
  fileInput.value?.click()
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  file.value = target.files?.[0] || null
}

async function loadImportTargetGroups() {
  try {
    const pageSize = 1000
    let page = 1
    let totalPages = 1
    const groups: AdminGroup[] = []

    do {
      const response = await adminAPI.groups.list(page, pageSize, undefined)
      groups.push(...response.items)
      totalPages = response.pages || page
      page += 1
    } while (page <= totalPages)

    importTargetGroups.value = groups
  } catch {
    importTargetGroups.value = []
  }
}

const handleClose = () => {
  if (importing.value) return
  emit('close')
}

const readFileAsText = async (sourceFile: File): Promise<string> => {
  if (typeof sourceFile.text === 'function') {
    return sourceFile.text()
  }

  if (typeof sourceFile.arrayBuffer === 'function') {
    const buffer = await sourceFile.arrayBuffer()
    return new TextDecoder().decode(buffer)
  }

  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.readAsText(sourceFile)
  })
}

const handleImport = async () => {
  if (!file.value) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }

  importing.value = true
  try {
    const text = await readFileAsText(file.value)
    const dataPayload = JSON.parse(text)
    const selectedPlatforms = new Set(selectedImportTargetGroups.value.map((group) => group.platform))

    if (selectedPlatforms.size > 1) {
      appStore.showError(t('admin.accounts.dataImportTargetGroupMixedPlatforms'))
      return
    }

    const expectedPlatform = Array.from(selectedPlatforms)[0]
    if (expectedPlatform) {
      const importedAccounts = Array.isArray(dataPayload?.accounts) ? dataPayload.accounts : []
      const mismatchedAccounts = importedAccounts.filter(
        (account: { platform?: string }) => account.platform !== expectedPlatform
      )

      if (mismatchedAccounts.length > 0) {
        const examples = mismatchedAccounts
          .slice(0, 5)
          .map((account: { name?: string; platform?: string }) => `${account.name || '-'} (${account.platform || '-'})`)
          .join(', ')

        appStore.showError(
          t('admin.accounts.dataImportAccountPlatformMismatch', {
            expected_platform: expectedPlatform,
            mismatch_count: mismatchedAccounts.length,
            examples
          })
        )
        return
      }
    }

    const importPayload = {
      data: dataPayload,
      skip_default_group_bind: true,
      ...(selectedGroupIds.value.length > 0 ? { group_ids: selectedGroupIds.value } : {})
    }

    const res = await adminAPI.accounts.importData(importPayload)

    result.value = res

    const msgParams: Record<string, unknown> = {
      account_created: res.account_created,
      account_failed: res.account_failed,
      proxy_created: res.proxy_created,
      proxy_reused: res.proxy_reused,
      proxy_failed: res.proxy_failed,
    }
    if (res.account_failed > 0 || res.proxy_failed > 0) {
      appStore.showError(t('admin.accounts.dataImportCompletedWithErrors', msgParams))
    } else {
      appStore.showSuccess(t('admin.accounts.dataImportSuccess', msgParams))
      emit('imported')
    }
  } catch (error: any) {
    if (error instanceof SyntaxError) {
      appStore.showError(t('admin.accounts.dataImportParseFailed'))
    } else {
      appStore.showError(error?.message || t('admin.accounts.dataImportFailed'))
    }
  } finally {
    importing.value = false
  }
}
</script>
