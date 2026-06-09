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
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
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

      <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.accounts.dataImportInspectToggle') }}
            </div>
            <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.accounts.dataImportInspectHint') }}
            </div>
          </div>
          <button
            type="button"
            role="switch"
            :aria-checked="inspectEnabled"
            class="relative inline-flex h-6 w-11 flex-shrink-0 rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-dark-900"
            :class="inspectEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'"
            :disabled="importing || Boolean(pendingImportPayload)"
            @click="inspectEnabled = !inspectEnabled"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow transition"
              :class="inspectEnabled ? 'translate-x-5' : 'translate-x-0'"
            />
          </button>
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.accounts.dataImportGroupTitle') }}
            </div>
            <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.accounts.dataImportGroupHint') }}
            </div>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="importing" @click="showCreateGroup = !showCreateGroup">
            <Icon name="plus" size="sm" class="mr-1.5" />
            {{ t('admin.accounts.dataImportNewGroup') }}
          </button>
        </div>

        <div
          v-if="showCreateGroup"
          class="mb-3 grid gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800 md:grid-cols-[1fr_10rem_auto]"
        >
          <div>
            <label class="input-label">{{ t('admin.accounts.dataImportNewGroupName') }}</label>
            <input
              v-model.trim="newGroupName"
              type="text"
              class="input"
              :disabled="importing || creatingGroup"
              :placeholder="t('admin.accounts.dataImportNewGroupNamePlaceholder')"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.dataImportNewGroupPlatform') }}</label>
            <select v-model="newGroupPlatform" class="input" :disabled="importing || creatingGroup">
              <option value="anthropic">Anthropic</option>
              <option value="openai">OpenAI</option>
              <option value="gemini">Gemini</option>
              <option value="antigravity">Antigravity</option>
            </select>
          </div>
          <div class="flex items-end">
            <button
              type="button"
              class="btn btn-primary w-full"
              :disabled="importing || creatingGroup || !newGroupName"
              @click="handleCreateGroup"
            >
              {{ creatingGroup ? t('common.loading') : t('common.create') }}
            </button>
          </div>
        </div>

        <div class="grid max-h-40 grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2">
          <label
            v-for="group in availableGroups"
            :key="group.id"
            class="flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm transition-colors"
            :class="selectedGroupId === group.id
              ? 'border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-300'
              : 'border-gray-200 text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:text-dark-200 dark:hover:bg-dark-800'"
          >
            <input
              type="checkbox"
              name="import-data-group"
              :value="group.id"
              :checked="selectedGroupId === group.id"
              :disabled="importing"
              class="h-3.5 w-3.5 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
              @change="selectGroup(group.id)"
            />
            <span class="min-w-0 flex-1 truncate">{{ group.name }}</span>
            <span class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-dark-300">
              {{ group.platform }}
            </span>
          </label>
          <div
            v-if="availableGroups.length === 0"
            class="rounded-lg border border-dashed border-gray-200 px-3 py-3 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400 sm:col-span-2"
          >
            {{ t('common.noGroupsAvailable') }}
          </div>
        </div>
      </div>

      <div
        v-if="progress.started"
        class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="mb-2 flex items-center justify-between gap-3 text-sm">
          <span class="font-medium text-gray-900 dark:text-white">{{ progressLabel }}</span>
          <span class="text-gray-500 dark:text-dark-400">{{ progressPercent }}%</span>
        </div>
        <div class="h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
          <div class="h-full rounded-full bg-primary-500 transition-all" :style="{ width: `${progressPercent}%` }"></div>
        </div>
        <div
          class="mt-3 grid grid-cols-2 gap-2 text-xs text-gray-600 dark:text-dark-300"
          :class="progress.phase === 'import' ? 'sm:grid-cols-4' : 'sm:grid-cols-3'"
        >
          <div>{{ t('admin.accounts.dataImportProgressScanned', { count: progress.inspected }) }}</div>
          <div>{{ t('admin.accounts.dataImportProgressHealthy', { count: progress.healthy }) }}</div>
          <div>{{ t('admin.accounts.dataImportProgressAbnormal', { count: progress.unhealthy }) }}</div>
          <div v-if="progress.phase === 'import'">
            {{ t('admin.accounts.dataImportProgressImported', { count: progress.imported }) }}
          </div>
        </div>
      </div>

      <div
        v-if="inspectLogItems.length"
        class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900"
      >
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div class="text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.accounts.dataImportInspectLog') }}
          </div>
          <div class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.accounts.dataImportInspectLogCount', { count: inspectLogItems.length }) }}
          </div>
        </div>
        <div
          ref="inspectLogPanel"
          class="max-h-72 overflow-y-auto rounded-lg border border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
        >
          <div
            v-for="item in inspectLogItems"
            :key="item.id"
            v-memo="[item.id]"
            class="border-b px-3 py-2 last:border-b-0 dark:border-dark-700"
            :class="item.healthy
              ? 'border-gray-100 bg-white dark:bg-dark-900'
              : 'border-red-100 bg-red-50/80 dark:border-red-900/60 dark:bg-red-950/20'"
          >
            <div class="flex flex-wrap items-center gap-2">
              <span
                class="min-w-0 max-w-full break-all text-sm font-semibold"
                :class="item.healthy ? 'text-gray-900 dark:text-white' : 'text-red-700 dark:text-red-300'"
              >
                {{ item.account }}
              </span>
              <span class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[11px] font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-300">
                {{ item.typeLabel }}
              </span>
              <span class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-dark-400">
                {{ item.platform }}
              </span>
            </div>
            <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
              <span class="min-w-0 break-all">{{ item.source }}</span>
              <span
                class="shrink-0 font-medium"
                :class="item.healthy ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'"
              >
                {{ item.healthy ? t('admin.accounts.dataImportInspectStatusNormal') : t('admin.accounts.dataImportInspectStatusError') }}
              </span>
            </div>
            <div
              v-if="!item.healthy && item.message"
              class="mt-1 whitespace-pre-wrap break-words text-xs font-medium text-red-700 dark:text-red-300"
            >
              {{ item.message }}
            </div>
          </div>
        </div>
      </div>

      <div
        v-if="errorItems.length && inspectLogItems.length === 0"
        class="space-y-2 rounded-lg border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-red-600 dark:text-red-400">
          {{ t('admin.accounts.dataImportErrors') }}
        </div>
        <div
          class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800"
        >
          <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
            {{ item.kind }} {{ item.name || item.proxy_key || '-' }} - {{ item.message }}
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
          {{ primaryButtonLabel }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type {
  AdminDataAccount,
  AdminDataImportError,
  AdminDataImportResult,
  AdminDataInspectItem,
  AdminDataInspectResult,
  AdminDataInspectStreamEvent,
  AdminDataPayload,
  AdminDataProxy,
  AdminGroup,
  GroupPlatform
} from '@/types'

interface Props {
  show: boolean
  groups?: AdminGroup[]
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
  (e: 'group-created', group: AdminGroup): void
}

interface ImportProgress {
  started: boolean
  phase: 'idle' | 'inspect' | 'review' | 'import'
  total: number
  inspected: number
  healthy: number
  unhealthy: number
  imported: number
}

const INSPECT_CHUNK_SIZE = 100
const IMPORT_CHUNK_SIZE = 200
const MAX_DISPLAY_ERRORS = 200

interface InspectLogItem {
  id: number
  account: string
  typeLabel: string
  platform: string
  source: string
  healthy: boolean
  message: string
}

const props = withDefaults(defineProps<Props>(), {
  groups: () => []
})
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const importing = ref(false)
const creatingGroup = ref(false)
const inspectEnabled = ref(true)
const showCreateGroup = ref(false)
const newGroupName = ref('')
const newGroupPlatform = ref<GroupPlatform>('anthropic')
const localGroups = ref<AdminGroup[]>([])
const selectedGroupId = ref<number | null>(null)
const file = ref<File | null>(null)
const result = ref<AdminDataImportResult | null>(null)
const pendingImportPayload = ref<AdminDataPayload | null>(null)
const inspectLogItems = ref<InspectLogItem[]>([])
const progress = ref<ImportProgress>({
  started: false,
  phase: 'idle',
  total: 0,
  inspected: 0,
  healthy: 0,
  unhealthy: 0,
  imported: 0
})

const fileInput = ref<HTMLInputElement | null>(null)
const inspectLogPanel = ref<HTMLElement | null>(null)
const fileName = computed(() => file.value?.name || '')
const availableGroups = computed(() => {
  const groupsById = new Map<number, AdminGroup>()
  for (const group of localGroups.value) {
    groupsById.set(group.id, group)
  }
  for (const group of props.groups) {
    groupsById.set(group.id, group)
  }
  return Array.from(groupsById.values())
})
const errorItems = computed(() => result.value?.errors || [])
const progressLabel = computed(() => {
  if (progress.value.phase === 'inspect') {
    return t('admin.accounts.dataImportInspecting')
  }
  if (progress.value.phase === 'review') {
    return t('admin.accounts.dataImportInspectComplete')
  }
  if (progress.value.phase === 'import') {
    return t('admin.accounts.dataImporting')
  }
  return t('admin.accounts.dataImportPreparing')
})
const progressPercent = computed(() => {
  if (progress.value.total <= 0) return 0
  const done = progress.value.phase === 'import'
    ? progress.value.imported
    : progress.value.inspected
  return Math.min(100, Math.round((done / progress.value.total) * 100))
})
const primaryButtonLabel = computed(() => {
  if (importing.value) {
    return progress.value.phase === 'inspect'
      ? t('admin.accounts.dataImportInspecting')
      : t('admin.accounts.dataImporting')
  }
  if (pendingImportPayload.value) {
    return t('admin.accounts.dataImportConfirmImport', {
      count: pendingImportPayload.value.accounts.length
    })
  }
  return t('admin.accounts.dataImportButton')
})

watch(
  () => props.show,
  (open) => {
    if (open) {
      resetState()
    }
  }
)

const resetState = () => {
  file.value = null
  result.value = null
  pendingImportPayload.value = null
  inspectLogItems.value = []
  selectedGroupId.value = null
  localGroups.value = []
  showCreateGroup.value = false
  newGroupName.value = ''
  newGroupPlatform.value = 'anthropic'
  inspectEnabled.value = true
  resetProgress()
  if (fileInput.value) {
    fileInput.value.value = ''
  }
}

const resetProgress = () => {
  progress.value = {
    started: false,
    phase: 'idle',
    total: 0,
    inspected: 0,
    healthy: 0,
    unhealthy: 0,
    imported: 0
  }
}

const openFilePicker = () => {
  fileInput.value?.click()
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  file.value = target.files?.[0] || null
  result.value = null
  pendingImportPayload.value = null
  inspectLogItems.value = []
  resetProgress()
}

const handleClose = () => {
  if (importing.value) return
  emit('close')
}

const selectGroup = (groupId: number) => {
  selectedGroupId.value = groupId
}

const handleCreateGroup = async () => {
  if (!newGroupName.value) return
  creatingGroup.value = true
  try {
    const group = await adminAPI.groups.create({
      name: newGroupName.value,
      platform: newGroupPlatform.value,
      rate_multiplier: 1,
      subscription_type: 'standard'
    })
    localGroups.value.push(group)
    selectedGroupId.value = group.id
    newGroupName.value = ''
    showCreateGroup.value = false
    emit('group-created', group)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.dataImportCreateGroupFailed'))
  } finally {
    creatingGroup.value = false
  }
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
  if (pendingImportPayload.value) {
    importing.value = true
    try {
      await importPreparedPayload(pendingImportPayload.value)
    } catch (error: any) {
      appStore.showError(error?.message || t('admin.accounts.dataImportFailed'))
    } finally {
      importing.value = false
    }
    return
  }

  if (!file.value) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }

  importing.value = true
  resetProgress()
  pendingImportPayload.value = null
  inspectLogItems.value = []
  try {
    const text = await readFileAsText(file.value)
    const dataPayload = JSON.parse(text) as AdminDataPayload
    normalizeImportPayload(dataPayload)

    if (!inspectEnabled.value) {
      await importPreparedPayload(dataPayload)
      return
    }

    const importPayload = await inspectPayload(dataPayload)
    if (importPayload.accounts.length === 0 && dataPayload.accounts.length > 0) {
      result.value = {
        proxy_created: 0,
        proxy_reused: 0,
        proxy_failed: 0,
        account_created: 0,
        account_failed: progress.value.unhealthy,
        errors: result.value?.errors || []
      }
      progress.value.phase = 'review'
      appStore.showError(t('admin.accounts.dataImportNoHealthyAccounts'))
      return
    }

    pendingImportPayload.value = importPayload
    progress.value.phase = 'review'
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

const importPreparedPayload = async (payload: AdminDataPayload) => {
  const res = await importPayloadInChunks(payload)
  result.value = res
  pendingImportPayload.value = null
  appStore.showSuccess(t('admin.accounts.dataImportImportedCount', { count: res.account_created }))
  emit('imported')
}

const normalizeImportPayload = (payload: AdminDataPayload) => {
  if (!Array.isArray(payload.proxies)) {
    payload.proxies = []
  }
  if (!Array.isArray(payload.accounts)) {
    payload.accounts = []
  }
}

const inspectPayload = async (payload: AdminDataPayload): Promise<AdminDataPayload> => {
  progress.value = {
    started: true,
    phase: 'inspect',
    total: payload.accounts.length,
    inspected: 0,
    healthy: 0,
    unhealthy: 0,
    imported: 0
  }

  const healthyAccounts: AdminDataAccount[] = []
  const inspectErrors: AdminDataImportError[] = []
  const finalInspectErrors: AdminDataImportError[] = []
  const logIndexByAccountIndex = new Map<number, number>()
  let validProxyKeys: string[] = []
  let finalizedInspected = 0
  let finalizedHealthy = 0
  let finalizedUnhealthy = 0

  for (let start = 0; start < payload.accounts.length; start += INSPECT_CHUNK_SIZE) {
    const accounts = payload.accounts.slice(start, start + INSPECT_CHUNK_SIZE)
    const chunkPayload = {
      ...payload,
      proxies: collectReferencedProxies(payload.proxies, accounts),
      accounts
    }
    const inspectResultRef: { value: AdminDataInspectResult | null } = { value: null }
    await adminAPI.accounts.inspectDataStream({
      data: chunkPayload,
      valid_proxy_keys: validProxyKeys
    }, async (event: AdminDataInspectStreamEvent) => {
      if (event.type === 'item') {
        const item = event.item
        const original = accounts[item.index]
        if (!original) return
        inspectLogItems.value.push(createInspectLogItem(item, original))
        logIndexByAccountIndex.set(start + item.index, inspectLogItems.value.length - 1)
        if (item.healthy) {
          progress.value.healthy += 1
        } else {
          addInspectError(inspectErrors, item, original)
          progress.value.unhealthy += 1
        }
        progress.value.inspected += 1
        result.value = {
          proxy_created: 0,
          proxy_reused: 0,
          proxy_failed: 0,
          account_created: 0,
          account_failed: inspectErrors.length,
          errors: inspectErrors
        }
        await flushInspectItemToUi()
      } else if (event.type === 'done') {
        inspectResultRef.value = event.result
      } else if (event.type === 'error') {
        throw new Error(event.message)
      }
    })

    const inspectResult = inspectResultRef.value
    if (!inspectResult) {
      throw new Error(t('admin.accounts.dataImportInspectFailed'))
    }
    if (inspectResult.valid_proxy_keys?.length) {
      validProxyKeys = Array.from(new Set([...validProxyKeys, ...inspectResult.valid_proxy_keys]))
    }
    for (const item of inspectResult.results) {
      const original = accounts[item.index]
      if (!original) continue
      const existingLogIndex = logIndexByAccountIndex.get(start + item.index)
      const finalLogItem = createInspectLogItem(item, original)
      if (existingLogIndex === undefined) {
        inspectLogItems.value.push(finalLogItem)
        logIndexByAccountIndex.set(start + item.index, inspectLogItems.value.length - 1)
      } else {
        inspectLogItems.value[existingLogIndex] = {
          ...finalLogItem,
          id: inspectLogItems.value[existingLogIndex].id
        }
      }
      if (item.healthy) {
        healthyAccounts.push(original)
      } else {
        addInspectError(finalInspectErrors, item, original)
      }
    }
    inspectErrors.splice(0, inspectErrors.length, ...finalInspectErrors)
    finalizedInspected += inspectResult.results.length || inspectResult.total
    finalizedHealthy += inspectResult.healthy
    finalizedUnhealthy += inspectResult.unhealthy
    progress.value.inspected = finalizedInspected
    progress.value.healthy = finalizedHealthy
    progress.value.unhealthy = finalizedUnhealthy
    result.value = {
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 0,
      account_failed: inspectErrors.length,
      errors: inspectErrors
    }
  }

  return {
    ...payload,
    accounts: healthyAccounts
  }
}

const addInspectError = (
  target: AdminDataImportError[],
  item: AdminDataInspectItem,
  account: AdminDataAccount
) => {
  if (target.length >= MAX_DISPLAY_ERRORS) return
  target.push({
    kind: 'account',
    name: item.name || account.name,
    proxy_key: item.proxy_key || account.proxy_key || undefined,
    message: (item.reasons || []).join('; ') || t('admin.accounts.dataImportInspectFailed')
  })
}

const createInspectLogItem = (
  item: AdminDataInspectItem,
  account: AdminDataAccount
): InspectLogItem => {
  const reasons = (item.reasons || []).filter(Boolean)
  return {
    id: inspectLogItems.value.length + 1,
    account: inspectAccountIdentity(item, account),
    typeLabel: inspectTypeLabel(item.type || account.type),
    platform: inspectPlatformLabel(item.platform || account.platform),
    source: fileName.value || '-',
    healthy: item.healthy,
    message: reasons.join('\n') || (item.healthy ? '' : t('admin.accounts.dataImportInspectFailed'))
  }
}

const inspectAccountIdentity = (
  item: AdminDataInspectItem,
  account: AdminDataAccount
): string => {
  return credentialString(account.credentials, [
    'email',
    'account_email',
    'username',
    'user',
    'account',
    'login',
    'name'
  ]) || item.name || account.name || '-'
}

const credentialString = (
  credentials: Record<string, unknown> | undefined,
  keys: string[]
): string => {
  if (!credentials) return ''
  for (const key of keys) {
    const value = credentials[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
    if (typeof value === 'number' && Number.isFinite(value)) {
      return String(value)
    }
  }
  return ''
}

const inspectTypeLabel = (type?: AdminDataAccount['type']): string => {
  switch (type) {
    case 'oauth':
    case 'setup-token':
      return 'Access Token'
    case 'apikey':
      return 'API Key'
    case 'upstream':
      return 'Upstream'
    case 'bedrock':
      return 'Bedrock'
    case 'service_account':
      return 'Service Account'
    default:
      return type || 'Account'
  }
}

const inspectPlatformLabel = (platform?: AdminDataAccount['platform']): string => {
  switch (platform) {
    case 'openai':
      return 'OpenAI'
    case 'anthropic':
      return 'Anthropic'
    case 'gemini':
      return 'Gemini'
    case 'antigravity':
      return 'Antigravity'
    default:
      return platform || '-'
  }
}

const flushInspectItemToUi = async () => {
  await nextTick()
  scrollInspectLogToBottom()
  if (progress.value.inspected < 20 || progress.value.inspected % 25 === 0) {
    await new Promise((resolve) => window.setTimeout(resolve, 0))
  }
}

const scrollInspectLogToBottom = () => {
  const panel = inspectLogPanel.value
  if (!panel) return
  if (typeof panel.scrollTo === 'function') {
    const behavior = progress.value.inspected < 50 ? 'smooth' : 'auto'
    panel.scrollTo({ top: panel.scrollHeight, behavior })
  } else {
    panel.scrollTop = panel.scrollHeight
  }
}

const dataProxyKey = (proxy: AdminDataProxy): string => {
  if (proxy.proxy_key) {
    return proxy.proxy_key
  }
  return [
    String(proxy.protocol || '').trim(),
    String(proxy.host || '').trim(),
    proxy.port,
    String(proxy.username || '').trim(),
    String(proxy.password || '').trim()
  ].join('|')
}

const collectReferencedProxies = (
  proxies: AdminDataProxy[],
  accounts: AdminDataAccount[]
): AdminDataProxy[] => {
  if (proxies.length === 0 || accounts.length === 0) {
    return []
  }

  const referencedKeys = new Set(
    accounts
      .map((account) => account.proxy_key?.trim())
      .filter((key): key is string => Boolean(key))
  )
  if (referencedKeys.size === 0) {
    return []
  }

  return proxies.filter((proxy) => referencedKeys.has(dataProxyKey(proxy)))
}

const importPayloadInChunks = async (payload: AdminDataPayload): Promise<AdminDataImportResult> => {
  progress.value.started = true
  progress.value.phase = 'import'
  progress.value.total = Math.max(payload.accounts.length, 1)
  progress.value.imported = 0

  const finalResult: AdminDataImportResult = {
    proxy_created: 0,
    proxy_reused: 0,
    proxy_failed: 0,
    account_created: 0,
    account_failed: progress.value.unhealthy,
    errors: result.value?.errors ? [...result.value.errors] : []
  }

  if (payload.accounts.length === 0) {
    const res = await importOneChunk(payload)
    mergeImportResult(finalResult, res)
    return finalResult
  }

  for (let start = 0; start < payload.accounts.length; start += IMPORT_CHUNK_SIZE) {
    const accounts = payload.accounts.slice(start, start + IMPORT_CHUNK_SIZE)
    const chunkPayload = {
      ...payload,
      proxies: start === 0 ? payload.proxies : [],
      accounts
    }
    const res = await importOneChunk(chunkPayload)
    mergeImportResult(finalResult, res)
    progress.value.imported += accounts.length
    result.value = { ...finalResult }
    await yieldToUi()
  }

  return finalResult
}

const importOneChunk = (payload: AdminDataPayload) => {
  return adminAPI.accounts.importData({
    data: payload,
    group_ids: selectedGroupId.value ? [selectedGroupId.value] : [],
    skip_default_group_bind: true
  })
}

const mergeImportResult = (target: AdminDataImportResult, source: AdminDataImportResult) => {
  target.proxy_created += source.proxy_created
  target.proxy_reused += source.proxy_reused
  target.proxy_failed += source.proxy_failed
  target.account_created += source.account_created
  target.account_failed += source.account_failed
  if (source.errors?.length) {
    const current = target.errors || []
    target.errors = current.concat(source.errors).slice(0, MAX_DISPLAY_ERRORS)
  }
}

const yieldToUi = async () => {
  await nextTick()
  await new Promise((resolve) => window.setTimeout(resolve, 0))
}
</script>
