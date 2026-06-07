<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.dataImportTitle')"
    width="wide"
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
          class="flex items-center justify-between gap-3 rounded-lg border border-dashed px-4 py-3 transition-colors"
          :class="dragActive
            ? 'border-primary-400 bg-primary-50/70 dark:border-primary-500 dark:bg-primary-900/20'
            : 'border-gray-300 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'"
          @dragenter.prevent="handleDragEnter"
          @dragover.prevent
          @dragleave.prevent="handleDragLeave"
          @drop.prevent="handleDrop"
        >
          <div class="min-w-0">
            <div class="truncate text-sm text-gray-700 dark:text-dark-200" :title="fileListTitle">
              {{ selectedFilesLabel || t('admin.accounts.dataImportSelectFile') }}
            </div>
            <div class="text-xs text-gray-500 dark:text-dark-400">
              JSON (.json)
              <span v-if="files.length > 1"> · {{ fileListTitle }}</span>
            </div>
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
          multiple
          @change="handleFileChange"
        />
      </div>

      <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <div class="mb-2 flex items-center justify-between gap-3">
          <label class="input-label mb-0">{{ t('admin.accounts.dataImportGroup') }}</label>
          <button
            v-if="selectedGroupId !== null"
            type="button"
            class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
            @click="selectedGroupId = null"
          >
            {{ t('admin.accounts.dataImportClearGroup') }}
          </button>
        </div>
        <div class="grid max-h-40 grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2">
          <button
            v-for="group in activeGroups"
            :key="group.id"
            type="button"
            :class="[
              'flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2 text-left transition-colors',
              selectedGroupId === group.id
                ? 'border-primary-400 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20'
                : 'border-gray-200 bg-white hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-dark-500'
            ]"
            @click="selectedGroupId = group.id"
          >
            <GroupBadge
              :name="group.name"
              :platform="group.platform"
              :subscription-type="group.subscription_type"
              :rate-multiplier="group.rate_multiplier"
              class="min-w-0 flex-1"
            />
            <span v-if="selectedGroupId === group.id" class="shrink-0 text-xs text-primary-600 dark:text-primary-400">✓</span>
          </button>
          <div
            v-if="activeGroups.length === 0"
            class="rounded-lg border border-dashed border-gray-200 px-3 py-4 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400 sm:col-span-2"
          >
            {{ t('common.noGroupsAvailable') }}
          </div>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">
          {{ t('admin.accounts.dataImportGroupHint') }}
        </p>
      </div>

      <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <label class="input-label">{{ t('admin.accounts.dataImportProxyAssignment') }}</label>
        <div class="grid gap-2 sm:grid-cols-2">
          <label
            v-for="option in proxyAssignmentOptions"
            :key="option.value"
            :class="[
              'cursor-pointer rounded-lg border p-3 transition-colors',
              proxyAssignmentMode === option.value
                ? 'border-primary-400 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20'
                : 'border-gray-200 bg-white hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-dark-500'
            ]"
            @click="proxyAssignmentMode = option.value"
          >
            <div class="flex items-start gap-2">
              <input
                v-model="proxyAssignmentMode"
                type="radio"
                name="data-import-proxy-assignment"
                class="mt-0.5 h-4 w-4 border-gray-300 text-primary-600 focus:ring-primary-500"
                :value="option.value"
              />
              <div class="min-w-0">
                <div class="text-sm font-medium text-gray-900 dark:text-white">{{ option.label }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ option.description }}</div>
              </div>
            </div>
          </label>
        </div>
        <div v-if="proxyAssignmentMode === 'default_live'" class="mt-3">
          <label class="input-label">{{ t('admin.accounts.dataImportDefaultProxy') }}</label>
          <ProxySelector v-model="defaultProxyId" :proxies="activeProxies" />
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">
          {{ t('admin.accounts.dataImportProxyAssignmentHint') }}
        </p>
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
import GroupBadge from '@/components/common/GroupBadge.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type {
  AdminDataImportProxyAssignment,
  AdminDataImportResult,
  AdminDataPayload,
  AdminGroup,
  Proxy
} from '@/types'

type AdminImportData = AdminDataPayload
type ProxyAssignmentMode = AdminDataImportProxyAssignment['mode']

const IMPORT_FORMAT_FILE_PREFIX = 'sub2api-account'
const STRUCTURED_EXPORT_ONLY_ERROR = 'Structured export file must be imported alone'
const IMPORT_FORMAT_TYPE = 'sub2api-account-data'
const SUPPORTED_DATA_TYPES = ['sub2api-data', 'sub2api-bundle']
const SUPPORTED_DATA_VERSION = 1

interface Props {
  show: boolean
  proxies: Proxy[]
  groups: AdminGroup[]
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

interface ImportAggregateResult extends AdminDataImportResult {
  errors: NonNullable<AdminDataImportResult['errors']>
}

interface ParsedImportFile {
  data: AdminImportData
}

class ImportFileError extends Error {
  constructor(
    readonly messageKey: string,
    readonly params?: Record<string, unknown>
  ) {
    super(messageKey)
  }
}

const props = withDefaults(defineProps<Props>(), {
  proxies: () => [],
  groups: () => []
})
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const importing = ref(false)
const files = ref<File[]>([])
const dragDepth = ref(0)
const dragActive = computed(() => dragDepth.value > 0)
const hasCreatedData = ref(false)
const result = ref<AdminDataImportResult | null>(null)
const selectedGroupId = ref<number | null>(null)
const proxyAssignmentMode = ref<ProxyAssignmentMode>('keep_file')
const defaultProxyId = ref<number | null>(null)

const fileInput = ref<HTMLInputElement | null>(null)
const selectedFilesLabel = computed(() => {
  if (files.value.length === 0) return ''
  if (files.value.length === 1) return files.value[0]?.name || ''
  return t('admin.accounts.selectedCount', { count: files.value.length })
})
const fileListTitle = computed(() => files.value.map((item) => item.name).join(', '))
const errorItems = computed(() => result.value?.errors || [])
const activeGroups = computed(() => props.groups.filter((group) => group.status === 'active'))
const activeProxies = computed(() => props.proxies.filter((proxy) => proxy.status === 'active'))
const proxyAssignmentOptions = computed(() => [
  {
    value: 'keep_file' as const,
    label: t('admin.accounts.dataImportProxyKeepFile'),
    description: t('admin.accounts.dataImportProxyKeepFileDesc')
  },
  {
    value: 'none' as const,
    label: t('admin.accounts.dataImportProxyNone'),
    description: t('admin.accounts.dataImportProxyNoneDesc')
  },
  {
    value: 'random_live' as const,
    label: t('admin.accounts.dataImportProxyRandomLive'),
    description: t('admin.accounts.dataImportProxyRandomLiveDesc')
  },
  {
    value: 'default_live' as const,
    label: t('admin.accounts.dataImportProxyDefaultLive'),
    description: t('admin.accounts.dataImportProxyDefaultLiveDesc')
  }
])

watch(
  () => props.show,
  (open) => {
    if (open) {
      clearSelection()
      resetImportOptions()
      dragDepth.value = 0
      hasCreatedData.value = false
    }
  }
)

const openFilePicker = () => {
  fileInput.value?.click()
}

const resetInputValue = (input: HTMLInputElement | null) => {
  if (input) {
    input.value = ''
  }
}

const clearSelection = () => {
  files.value = []
  result.value = null
  resetInputValue(fileInput.value)
}

const resetImportOptions = () => {
  selectedGroupId.value = null
  proxyAssignmentMode.value = 'keep_file'
  defaultProxyId.value = null
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  setSelectedFiles(target.files)
  target.value = ''
}

const handleClose = () => {
  if (importing.value) return
  if (hasCreatedData.value) {
    hasCreatedData.value = false
    emit('imported')
  }
  emit('close')
}

const isJsonFile = (sourceFile: File) => {
  const name = sourceFile.name.toLowerCase()
  return name.endsWith('.json') || sourceFile.type === 'application/json'
}

const isStructuredExportFilename = (fileName: string) => {
  const normalized = fileName.toLowerCase()
  return normalized.startsWith(IMPORT_FORMAT_FILE_PREFIX) || normalized.startsWith(IMPORT_FORMAT_TYPE)
}

const normalizeSelectedFiles = (sourceFiles: FileList | File[] | null | undefined): File[] => {
  const selected = Array.from(sourceFiles || [])
  const hasStructuredDataFile = selected.some((file) => isStructuredExportFilename(file.name))
  if (hasStructuredDataFile && selected.length > 1) {
    throw new Error(STRUCTURED_EXPORT_ONLY_ERROR)
  }
  return selected
}

const getImportErrorMessage = (error: unknown, translate: (key: string, params?: Record<string, unknown>) => string): string => {
  if (error instanceof ImportFileError) {
    return translate(error.messageKey, error.params)
  }
  if (error instanceof SyntaxError) {
    return translate('admin.accounts.dataImportParseFailed')
  }
  return error instanceof Error ? error.message : translate('admin.accounts.dataImportFailed')
}

watch(activeGroups, (groups) => {
  if (selectedGroupId.value !== null && !groups.some((group) => group.id === selectedGroupId.value)) {
    selectedGroupId.value = null
  }
})

watch(activeProxies, (proxies) => {
  if (defaultProxyId.value !== null && !proxies.some((proxy) => proxy.id === defaultProxyId.value)) {
    defaultProxyId.value = null
  }
})

const setSelectedFiles = (sourceFiles: FileList | File[] | null | undefined) => {
  if (importing.value) return

  try {
    const incoming = normalizeSelectedFiles(sourceFiles)
    const picked = incoming.filter(isJsonFile)
    if (!picked.length) {
      appStore.showError(t('admin.accounts.dataImportSelectFile'))
      return
    }
    if (picked.length < incoming.length) {
      appStore.showWarning(
        t('admin.accounts.dataImportIgnoredFiles', { count: incoming.length - picked.length })
      )
    }
    files.value = picked
    result.value = null
  } catch (error) {
    files.value = []
    result.value = null
    appStore.showError(getImportErrorMessage(error, t))
  }
}

const handleDragEnter = () => {
  if (importing.value) return
  dragDepth.value += 1
}

const handleDragLeave = () => {
  dragDepth.value = Math.max(0, dragDepth.value - 1)
}

const handleDrop = (event: DragEvent) => {
  dragDepth.value = 0
  if (importing.value) return
  setSelectedFiles(event.dataTransfer?.files)
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

const isPlainObject = (value: unknown): value is Record<string, unknown> => {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

const isStructuredDataPayload = (payload: unknown): payload is AdminDataPayload => {
  if (!isPlainObject(payload)) return false
  if (
    payload.type !== undefined
    && payload.type !== ''
    && !SUPPORTED_DATA_TYPES.includes(payload.type as string)
  ) {
    return false
  }
  if (
    payload.version !== undefined
    && payload.version !== 0
    && payload.version !== SUPPORTED_DATA_VERSION
  ) {
    return false
  }
  return Array.isArray(payload.proxies) && Array.isArray(payload.accounts)
}

const isCPAAccountObject = (value: unknown): value is Record<string, unknown> => {
  return isPlainObject(value) && (
    typeof value.refresh_token === 'string'
    || typeof value.access_token === 'string'
    || typeof value.id_token === 'string'
    || typeof value.account_id === 'string'
  )
}

const normalizeCPAAccountsPayload = (accounts: Record<string, unknown>[]): AdminDataPayload => ({
  exported_at: new Date(0).toISOString(),
  proxies: [],
  accounts: accounts.map((account) => ({
    name: typeof account.name === 'string'
      ? account.name
      : (typeof account.account_id === 'string' ? account.account_id : 'Imported Account'),
    notes: typeof account.notes === 'string' ? account.notes : null,
    platform: typeof account.platform === 'string' ? account.platform as never : 'openai',
    type: typeof account.type === 'string' ? account.type as never : 'apikey',
    credentials: account,
    extra: {},
    proxy_key: typeof account.proxy_key === 'string' ? account.proxy_key : null,
    concurrency: typeof account.concurrency === 'number' ? account.concurrency : 1,
    priority: typeof account.priority === 'number' ? account.priority : 0,
    rate_multiplier: typeof account.rate_multiplier === 'number' ? account.rate_multiplier : null,
    expires_at: typeof account.expires_at === 'number' ? account.expires_at : null,
    auto_pause_on_expired: typeof account.auto_pause_on_expired === 'boolean' ? account.auto_pause_on_expired : false,
  })),
})

const parseImportPayload = (raw: unknown, sourceFile: File): AdminImportData => {
  if (Array.isArray(raw)) {
    if (raw.every(isCPAAccountObject)) {
      return normalizeCPAAccountsPayload(raw)
    }
    throw new ImportFileError('admin.accounts.dataImportInvalidFile', { name: sourceFile.name })
  }

  if (isStructuredDataPayload(raw)) {
    return raw
  }

  if (isCPAAccountObject(raw)) {
    return normalizeCPAAccountsPayload([raw])
  }

  throw new ImportFileError('admin.accounts.dataImportInvalidFile', { name: sourceFile.name })
}

const parseImportFilesFromText = async (selectedFiles: File[]): Promise<AdminImportData[]> => {
  const parsedEntries = await Promise.all(selectedFiles.map(async (sourceFile) => {
    let raw: unknown
    try {
      raw = JSON.parse(await readFileAsText(sourceFile))
    } catch {
      throw new ImportFileError('admin.accounts.dataImportParseFailedFile', { name: sourceFile.name })
    }
    return {
      data: parseImportPayload(raw, sourceFile)
    } satisfies ParsedImportFile
  }))

  return parsedEntries.map((entry) => entry.data)
}

const mergeImportResults = (results: AdminDataImportResult[]): ImportAggregateResult => {
  return results.reduce<ImportAggregateResult>((acc, item) => {
    acc.proxy_created += item.proxy_created
    acc.proxy_reused += item.proxy_reused
    acc.proxy_failed += item.proxy_failed
    acc.account_created += item.account_created
    acc.account_failed += item.account_failed
    if (item.errors?.length) {
      acc.errors.push(...item.errors)
    }
    return acc
  }, {
    proxy_created: 0,
    proxy_reused: 0,
    proxy_failed: 0,
    account_created: 0,
    account_failed: 0,
    errors: []
  })
}

const buildProxyAssignmentPayload = (): AdminDataImportProxyAssignment => ({
  mode: proxyAssignmentMode.value,
  default_proxy_id: proxyAssignmentMode.value === 'default_live' ? defaultProxyId.value : null
})

const importDataPayloads = async (payloads: AdminImportData[]): Promise<ImportAggregateResult> => {
  const importOptions = {
    skip_default_group_bind: true,
    group_id: selectedGroupId.value,
    proxy_assignment: buildProxyAssignmentPayload()
  }
  const responses = await Promise.all(payloads.map((data) => adminAPI.accounts.importData({
    data,
    ...importOptions
  })))

  return mergeImportResults(responses)
}

const readAndImportSelectedFiles = async (selectedFiles: File[]): Promise<ImportAggregateResult> => {
  const payloads = await parseImportFilesFromText(selectedFiles)
  return importDataPayloads(payloads)
}

const handleImport = async () => {
  if (files.value.length === 0) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }
  if (proxyAssignmentMode.value === 'random_live' && activeProxies.value.length === 0) {
    appStore.showError(t('admin.accounts.dataImportNoActiveProxy'))
    return
  }
  if (proxyAssignmentMode.value === 'default_live' && defaultProxyId.value === null) {
    appStore.showError(t('admin.accounts.dataImportSelectDefaultProxy'))
    return
  }

  importing.value = true
  try {
    const aggregatedResult = await readAndImportSelectedFiles(files.value)
    result.value = aggregatedResult

    const msgParams: Record<string, unknown> = {
      account_created: aggregatedResult.account_created,
      account_failed: aggregatedResult.account_failed,
      proxy_created: aggregatedResult.proxy_created,
      proxy_reused: aggregatedResult.proxy_reused,
      proxy_failed: aggregatedResult.proxy_failed,
    }

    if (aggregatedResult.account_failed > 0 || aggregatedResult.proxy_failed > 0) {
      if (aggregatedResult.account_created > 0 || aggregatedResult.proxy_created > 0) {
        hasCreatedData.value = true
      }
      appStore.showError(t('admin.accounts.dataImportCompletedWithErrors', msgParams))
    } else {
      appStore.showSuccess(t('admin.accounts.dataImportSuccess', msgParams))
      emit('imported')
    }
  } catch (error) {
    appStore.showError(getImportErrorMessage(error, t))
  } finally {
    importing.value = false
  }
}
</script>
