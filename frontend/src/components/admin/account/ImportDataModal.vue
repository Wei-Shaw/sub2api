<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.dataImportTitle')"
    width="wide"
    close-on-click-outside
    @close="handleClose"
  >
    <div v-if="!result" class="space-y-5">
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
          {{ t('admin.batches.label', '批次标签') }}
          <span class="text-red-500">*</span>
        </label>
        <select v-if="batches.length > 0" v-model="selectedBatchId" class="input">
          <option :value="null" disabled>{{ t('admin.batches.selectPlaceholder', '请选择批次') }}</option>
          <option v-for="b in batches" :key="b.id" :value="b.id">{{ b.name }}</option>
        </select>
        <p v-else class="text-sm text-amber-600 dark:text-amber-400">
          {{ t('admin.batches.noBatches', '请先在"更多工具 → 批次管理"中创建批次') }}
        </p>
      </div>

      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-4 flex items-start justify-between gap-4">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.accounts.dataImportSharedSettingsTitle') }}
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.dataImportSharedSettingsHint') }}
            </p>
          </div>
          <button
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              schedulable ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            :title="schedulable ? t('admin.accounts.schedulableEnabled') : t('admin.accounts.schedulableDisabled')"
            @click="schedulable = !schedulable"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                schedulable ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
        <div class="mb-4 rounded-lg bg-gray-50 p-3 text-xs text-gray-600 dark:bg-dark-800 dark:text-dark-300">
          {{ schedulable ? t('admin.accounts.dataImportSchedulableOnHint') : t('admin.accounts.dataImportSchedulableOffHint') }}
        </div>

        <div class="space-y-4">
          <div>
            <label class="input-label">{{ t('admin.accounts.proxy') }}</label>
            <ProxySelector v-model="proxyId" :proxies="proxies" />
          </div>

          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
              <input
                v-model.number="concurrency"
                type="number"
                min="1"
                class="input"
                @input="concurrency = Math.max(1, concurrency || 1)"
              />
              <p class="input-hint">{{ t('admin.accounts.dataImportConcurrencyHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.loadFactor') }}</label>
              <input
                v-model.number="loadFactor"
                type="number"
                min="1"
                class="input"
                :placeholder="String(concurrency || 1)"
                @input="loadFactor = (loadFactor && loadFactor >= 1) ? loadFactor : null"
              />
              <p class="input-hint">{{ t('admin.accounts.loadFactorHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.priority') }}</label>
              <input v-model.number="priority" type="number" min="1" class="input" />
              <p class="input-hint">{{ t('admin.accounts.priorityHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') }}</label>
              <input v-model.number="rateMultiplier" type="number" min="0" step="0.001" class="input" />
              <p class="input-hint">{{ t('admin.accounts.billingRateMultiplierHint') }}</p>
            </div>
          </div>

          <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
            <label class="input-label">{{ t('admin.accounts.expiresAt') }}</label>
            <input v-model="expiresAtInput" type="datetime-local" class="input" />
            <p class="input-hint">{{ t('admin.accounts.expiresAtHint') }}</p>
          </div>

          <div class="flex items-center justify-between border-t border-gray-200 pt-4 dark:border-dark-600">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.autoPauseOnExpired') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.autoPauseOnExpiredDesc') }}
              </p>
            </div>
            <button
              type="button"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                autoPauseOnExpired ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
              @click="autoPauseOnExpired = !autoPauseOnExpired"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  autoPauseOnExpired ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
            <GroupSelector v-model="groupIds" :groups="groups" searchable />
          </div>

          <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
            <div class="mb-3 flex items-start justify-between gap-4">
              <div>
                <label class="input-label mb-0">{{ t('admin.accounts.modelRestriction') }}</label>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.dataImportModelRestrictionHint') }}
                </p>
              </div>
              <button
                type="button"
                data-testid="auto-detect-models-toggle"
                :class="[
                  'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                  autoDetectModels ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
                ]"
                @click="autoDetectModels = !autoDetectModels"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    autoDetectModels ? 'translate-x-5' : 'translate-x-0'
                  ]"
                />
              </button>
            </div>
            <div class="mb-3 rounded-lg bg-gray-50 p-3 text-xs text-gray-600 dark:bg-dark-800 dark:text-dark-300">
              {{ autoDetectModels ? t('admin.accounts.dataImportAutoDetectModelsOnHint') : t('admin.accounts.dataImportAutoDetectModelsOffHint') }}
            </div>
            <ModelWhitelistSelector
              v-model="allowedModels"
              :platforms="importPlatforms"
            />
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
              <span v-if="allowedModels.length === 0">{{ t('admin.accounts.supportsAllModels') }}</span>
            </p>
          </div>

          <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
            <div class="flex items-center justify-between">
              <div>
                <label class="input-label mb-0">{{ t('admin.accounts.interceptWarmupRequests') }}</label>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.interceptWarmupRequestsDesc') }}
                </p>
              </div>
              <button
                type="button"
                :class="[
                  'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                  interceptWarmupRequests ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
                ]"
                @click="interceptWarmupRequests = !interceptWarmupRequests"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    interceptWarmupRequests ? 'translate-x-5' : 'translate-x-0'
                  ]"
                />
              </button>
            </div>
          </div>

          <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
            <div class="mb-3 flex items-center justify-between">
              <div>
                <label class="input-label mb-0">{{ t('admin.accounts.tempUnschedulable.title') }}</label>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.tempUnschedulable.hint') }}
                </p>
              </div>
              <button
                type="button"
                :class="[
                  'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                  tempUnschedEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
                ]"
                @click="tempUnschedEnabled = !tempUnschedEnabled"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    tempUnschedEnabled ? 'translate-x-5' : 'translate-x-0'
                  ]"
                />
              </button>
            </div>

            <div v-if="tempUnschedEnabled" class="space-y-3">
              <div class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
                <p class="text-xs text-blue-700 dark:text-blue-400">
                  <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
                  {{ t('admin.accounts.tempUnschedulable.notice') }}
                </p>
              </div>

              <div class="flex flex-wrap gap-2">
                <button
                  v-for="preset in tempUnschedPresets"
                  :key="preset.label"
                  type="button"
                  class="rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
                  @click="addTempUnschedRule(preset.rule)"
                >
                  + {{ preset.label }}
                </button>
              </div>

              <div v-if="tempUnschedRules.length > 0" class="space-y-3">
                <div
                  v-for="(rule, index) in tempUnschedRules"
                  :key="getTempUnschedRuleKey(rule)"
                  class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
                >
                  <div class="mb-2 flex items-center justify-between">
                    <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
                      {{ t('admin.accounts.tempUnschedulable.ruleIndex', { index: index + 1 }) }}
                    </span>
                    <div class="flex items-center gap-2">
                      <button
                        type="button"
                        :disabled="index === 0"
                        class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
                        @click="moveTempUnschedRule(index, -1)"
                      >
                        <Icon name="chevronUp" size="sm" :stroke-width="2" />
                      </button>
                      <button
                        type="button"
                        :disabled="index === tempUnschedRules.length - 1"
                        class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
                        @click="moveTempUnschedRule(index, 1)"
                      >
                        <Icon name="chevronDown" size="sm" :stroke-width="2" />
                      </button>
                      <button
                        type="button"
                        class="rounded p-1 text-red-500 transition-colors hover:text-red-600"
                        @click="removeTempUnschedRule(index)"
                      >
                        <Icon name="x" size="sm" :stroke-width="2" />
                      </button>
                    </div>
                  </div>

                  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <div>
                      <label class="input-label">{{ t('admin.accounts.tempUnschedulable.errorCode') }}</label>
                      <input
                        v-model.number="rule.error_code"
                        type="number"
                        min="100"
                        max="599"
                        class="input"
                        :placeholder="t('admin.accounts.tempUnschedulable.errorCodePlaceholder')"
                      />
                    </div>
                    <div>
                      <label class="input-label">{{ t('admin.accounts.tempUnschedulable.durationMinutes') }}</label>
                      <input
                        v-model.number="rule.duration_minutes"
                        type="number"
                        min="1"
                        class="input"
                        :placeholder="t('admin.accounts.tempUnschedulable.durationPlaceholder')"
                      />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="input-label">{{ t('admin.accounts.tempUnschedulable.keywords') }}</label>
                      <input
                        v-model="rule.keywords"
                        type="text"
                        class="input"
                        :placeholder="t('admin.accounts.tempUnschedulable.keywordsPlaceholder')"
                      />
                      <p class="input-hint">{{ t('admin.accounts.tempUnschedulable.keywordsHint') }}</p>
                    </div>
                    <div class="sm:col-span-2">
                      <label class="input-label">{{ t('admin.accounts.tempUnschedulable.description') }}</label>
                      <input
                        v-model="rule.description"
                        type="text"
                        class="input"
                        :placeholder="t('admin.accounts.tempUnschedulable.descriptionPlaceholder')"
                      />
                    </div>
                  </div>
                </div>
              </div>

              <button
                type="button"
                class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-sm text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
                @click="addTempUnschedRule()"
              >
                <Icon name="plus" size="sm" class="mr-1 inline" />
                {{ t('admin.accounts.tempUnschedulable.addRule') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-else class="space-y-2 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
      <div class="text-sm font-medium text-gray-900 dark:text-white">
        {{ t('admin.accounts.dataImportResult') }}
      </div>
      <div class="text-sm text-gray-700 dark:text-dark-300">
        {{ t('admin.accounts.dataImportResultSummary', result) }}
      </div>
      <div v-if="hasModelSyncResult" class="text-sm text-gray-700 dark:text-dark-300">
        {{ t('admin.accounts.dataImportModelSyncResult', modelSyncResultParams) }}
      </div>
      <div v-if="errorItems.length" class="mt-2">
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
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ result ? t('common.close') : t('common.cancel') }}
        </button>
        <button
          v-if="!result"
          class="btn btn-primary"
          type="button"
          data-testid="confirm-import"
          :disabled="!canProceed || importing"
          @click="handleImport"
        >
          {{ importing ? t('admin.accounts.dataImporting') : t('admin.accounts.dataImportConfirm', '确认导入') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="showMixedChannelWarning"
    :title="t('admin.accounts.mixedChannelWarningTitle')"
    :message="mixedChannelWarningMessage"
    :confirm-text="t('common.confirm')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="handleMixedChannelConfirm"
    @cancel="handleMixedChannelCancel"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { applyInterceptWarmup } from '@/components/account/credentialsBuilder'
import { buildModelMappingObject } from '@/composables/useModelWhitelist'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'
import type {
  AdminDataImportRequest,
  AdminDataImportResult,
  AdminDataImportPayload,
  AdminGroup,
  Proxy as AccountProxy
} from '@/types'

interface Props {
  show: boolean
  batches?: { id: number; name: string }[]
  groups?: AdminGroup[]
  proxies?: AccountProxy[]
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

interface TempUnschedRuleForm {
  error_code: number | null
  keywords: string
  duration_minutes: number | null
  description: string
}

const props = withDefaults(defineProps<Props>(), {
  batches: () => [],
  groups: () => [],
  proxies: () => []
})
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const importing = ref(false)
const file = ref<File | null>(null)
const filePayload = ref<AdminDataImportPayload | null>(null)
const result = ref<AdminDataImportResult | null>(null)
const defaultImportConcurrency = 1
const defaultImportSchedulable = false
const selectedBatchId = ref<number | null>(null)
const proxyId = ref<number | null>(null)
const groupIds = ref<number[]>([])
const concurrency = ref(defaultImportConcurrency)
const loadFactor = ref<number | null>(null)
const priority = ref(1)
const rateMultiplier = ref(1)
const expiresAt = ref<number | null>(null)
const autoPauseOnExpired = ref(true)
const schedulable = ref(defaultImportSchedulable)
const autoDetectModels = ref(false)
const allowedModels = ref<string[]>([])
const interceptWarmupRequests = ref(false)
const tempUnschedEnabled = ref(false)
const tempUnschedRules = ref<TempUnschedRuleForm[]>([])
const showMixedChannelWarning = ref(false)
const mixedChannelWarningMessage = ref('')
const pendingImportPayload = ref<AdminDataImportRequest | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const fileName = computed(() => file.value?.name || '')
const errorItems = computed(() => result.value?.errors || [])
const modelSyncResultParams = computed(() => ({
  model_sync_succeeded: result.value?.model_sync_succeeded || 0,
  model_sync_failed: result.value?.model_sync_failed || 0
}))
const hasModelSyncResult = computed(
  () => modelSyncResultParams.value.model_sync_succeeded > 0 || modelSyncResultParams.value.model_sync_failed > 0
)
const batches = computed(() => props.batches || [])
const groups = computed(() => props.groups || [])
const proxies = computed(() => props.proxies || [])
const getTempUnschedRuleKey = createStableObjectKeyResolver<TempUnschedRuleForm>('import-temp-unsched-rule')

const importPlatforms = computed(() => {
  const platforms = new Set<string>()
  const accounts = Array.isArray(filePayload.value?.accounts) ? filePayload.value.accounts : []
  for (const account of accounts) {
    const platform = String(account.platform || '').trim()
    if (platform) {
      platforms.add(platform)
    }
  }
  return Array.from(platforms)
})

const canProceed = computed(() => Boolean(file.value && selectedBatchId.value && !importing.value))

const expiresAtInput = computed({
  get: () => {
    if (!expiresAt.value) return ''
    const date = new Date(expiresAt.value * 1000)
    const offset = date.getTimezoneOffset()
    const localDate = new Date(date.getTime() - offset * 60 * 1000)
    return localDate.toISOString().slice(0, 16)
  },
  set: (value: string) => {
    expiresAt.value = value ? Math.floor(new Date(value).getTime() / 1000) : null
  }
})

const tempUnschedPresets = computed(() => [
  {
    label: t('admin.accounts.tempUnschedulable.presets.overloadLabel'),
    rule: {
      error_code: 529,
      keywords: 'overloaded, too many',
      duration_minutes: 60,
      description: t('admin.accounts.tempUnschedulable.presets.overloadDesc')
    }
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.rateLimitLabel'),
    rule: {
      error_code: 429,
      keywords: 'rate limit, too many requests',
      duration_minutes: 10,
      description: t('admin.accounts.tempUnschedulable.presets.rateLimitDesc')
    }
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.unavailableLabel'),
    rule: {
      error_code: 503,
      keywords: 'unavailable, maintenance',
      duration_minutes: 30,
      description: t('admin.accounts.tempUnschedulable.presets.unavailableDesc')
    }
  }
])

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
  filePayload.value = null
  result.value = null
  selectedBatchId.value = null
  proxyId.value = null
  groupIds.value = []
  concurrency.value = defaultImportConcurrency
  loadFactor.value = null
  priority.value = 1
  rateMultiplier.value = 1
  expiresAt.value = null
  autoPauseOnExpired.value = true
  schedulable.value = defaultImportSchedulable
  autoDetectModels.value = false
  allowedModels.value = []
  interceptWarmupRequests.value = false
  tempUnschedEnabled.value = false
  tempUnschedRules.value = []
  showMixedChannelWarning.value = false
  mixedChannelWarningMessage.value = ''
  pendingImportPayload.value = null
  if (fileInput.value) {
    fileInput.value.value = ''
  }
}

const openFilePicker = () => {
  fileInput.value?.click()
}

const handleFileChange = async (event: Event) => {
  const target = event.target as HTMLInputElement
  file.value = target.files?.[0] || null
  filePayload.value = null
  if (!file.value) return

  try {
    const text = await readFileAsText(file.value)
    filePayload.value = JSON.parse(text) as AdminDataImportPayload
  } catch {
    filePayload.value = null
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

const splitTempUnschedKeywords = (value: string) => {
  return value
    .split(/[,;]/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
}

const buildTempUnschedRules = (rules: TempUnschedRuleForm[]) => {
  const out: Array<{
    error_code: number
    keywords: string[]
    duration_minutes: number
    description: string
  }> = []

  for (const rule of rules) {
    const errorCode = Number(rule.error_code)
    const duration = Number(rule.duration_minutes)
    const keywords = splitTempUnschedKeywords(rule.keywords)
    if (!Number.isFinite(errorCode) || errorCode < 100 || errorCode > 599) continue
    if (!Number.isFinite(duration) || duration <= 0) continue
    if (keywords.length === 0) continue
    out.push({
      error_code: Math.trunc(errorCode),
      keywords,
      duration_minutes: Math.trunc(duration),
      description: rule.description.trim()
    })
  }

  return out
}

const buildCredentialExtras = (): Record<string, unknown> | undefined => {
  const credentials: Record<string, unknown> = {}
  applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')

  const modelMapping = buildModelMappingObject('whitelist', allowedModels.value, [])
  if (modelMapping) {
    credentials.model_mapping = modelMapping
  }

  if (tempUnschedEnabled.value) {
    const rules = buildTempUnschedRules(tempUnschedRules.value)
    if (rules.length === 0) {
      appStore.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
      return undefined
    }
    credentials.temp_unschedulable_enabled = true
    credentials.temp_unschedulable_rules = rules
  }

  return Object.keys(credentials).length > 0 ? credentials : undefined
}

const addTempUnschedRule = (preset?: TempUnschedRuleForm) => {
  if (preset) {
    tempUnschedRules.value.push({ ...preset })
    return
  }
  tempUnschedRules.value.push({
    error_code: null,
    keywords: '',
    duration_minutes: 30,
    description: ''
  })
}

const removeTempUnschedRule = (index: number) => {
  tempUnschedRules.value.splice(index, 1)
}

const moveTempUnschedRule = (index: number, direction: number) => {
  const target = index + direction
  if (target < 0 || target >= tempUnschedRules.value.length) return
  const rules = tempUnschedRules.value
  const current = rules[index]
  rules[index] = rules[target]
  rules[target] = current
}

const buildImportPayload = async (): Promise<AdminDataImportRequest | null> => {
  if (!file.value || !selectedBatchId.value) return null

  const credentialExtras = buildCredentialExtras()
  if ((interceptWarmupRequests.value || tempUnschedEnabled.value) && credentialExtras === undefined) {
    return null
  }

  const text = await readFileAsText(file.value)
  const dataPayload = JSON.parse(text) as AdminDataImportPayload
  filePayload.value = dataPayload

  return {
    data: dataPayload,
    skip_default_group_bind: groupIds.value.length === 0,
    batch_id: selectedBatchId.value,
    group_ids: [...groupIds.value],
    proxy_id: proxyId.value,
    concurrency: concurrency.value,
    load_factor: loadFactor.value ?? undefined,
    priority: priority.value,
    rate_multiplier: rateMultiplier.value,
    expires_at: expiresAt.value,
    auto_pause_on_expired: autoPauseOnExpired.value,
    schedulable: schedulable.value,
    credential_extras: credentialExtras,
    auto_detect_models: autoDetectModels.value
  }
}

const submitImportPayload = async (payload: AdminDataImportRequest) => {
  importing.value = true
  try {
    const res = await adminAPI.accounts.importData(payload)
    result.value = res

    const msgParams: Record<string, unknown> = {
      account_created: res.account_created,
      account_failed: res.account_failed,
      proxy_created: res.proxy_created,
      proxy_reused: res.proxy_reused,
      proxy_failed: res.proxy_failed,
      model_sync_succeeded: res.model_sync_succeeded || 0,
      model_sync_failed: res.model_sync_failed || 0
    }
    if (res.account_failed > 0 || res.proxy_failed > 0 || (res.model_sync_failed || 0) > 0) {
      appStore.showError(t('admin.accounts.dataImportCompletedWithErrors', msgParams))
    } else {
      appStore.showSuccess(t('admin.accounts.dataImportSuccess', msgParams))
      emit('imported')
    }
  } catch (error: any) {
    if (error?.status === 409 && error?.error === 'mixed_channel_warning') {
      pendingImportPayload.value = payload
      mixedChannelWarningMessage.value = error.message || t('admin.accounts.mixedChannelWarningTitle')
      showMixedChannelWarning.value = true
      return
    }
    if (error instanceof SyntaxError) {
      appStore.showError(t('admin.accounts.dataImportParseFailed'))
    } else {
      appStore.showError(error?.message || t('admin.accounts.dataImportFailed'))
    }
  } finally {
    importing.value = false
  }
}

const handleImport = async () => {
  try {
    const payload = await buildImportPayload()
    if (!payload) return
    await submitImportPayload(payload)
  } catch (error: any) {
    if (error instanceof SyntaxError) {
      appStore.showError(t('admin.accounts.dataImportParseFailed'))
    } else {
      appStore.showError(error?.message || t('admin.accounts.dataImportFailed'))
    }
  }
}

const handleMixedChannelConfirm = async () => {
  showMixedChannelWarning.value = false
  if (!pendingImportPayload.value) return
  await submitImportPayload({
    ...pendingImportPayload.value,
    confirm_mixed_channel_risk: true
  })
}

const handleMixedChannelCancel = () => {
  showMixedChannelWarning.value = false
  pendingImportPayload.value = null
}
</script>
