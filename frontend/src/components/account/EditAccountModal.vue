<template>
  <Modal
    :show="show"
    :title="t('admin.accounts.editAccount')"
    size="lg"
    @close="handleClose"
  >
    <form v-if="account" @submit.prevent="handleSubmit" class="space-y-5">
      <div>
        <label class="input-label">{{ t('common.name') }}</label>
        <input
          v-model="form.name"
          type="text"
          required
          class="input"
        />
      </div>

      <!-- API Key fields (only for apikey type) -->
      <div v-if="account.type === 'apikey'" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
          <input
            v-model="editBaseUrl"
            type="text"
            class="input"
            :placeholder="account.platform === 'openai' ? 'https://api.openai.com' : 'https://api.anthropic.com'"
          />
          <p class="input-hint">{{ t('admin.accounts.baseUrlHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
          <input
            v-model="editApiKey"
            type="password"
            class="input font-mono"
            :placeholder="account.platform === 'openai' ? 'sk-proj-...' : 'sk-ant-...'"
          />
          <p class="input-hint">{{ t('admin.accounts.leaveEmptyToKeep') }}</p>
        </div>

        <!-- Model Restriction Section -->
        <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
          <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

          <!-- Mode Toggle -->
          <div class="flex gap-2 mb-4">
            <button
              type="button"
              @click="modelRestrictionMode = 'whitelist'"
              :class="[
                'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                modelRestrictionMode === 'whitelist'
                  ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
            >
              <svg class="w-4 h-4 inline mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              {{ t('admin.accounts.modelWhitelist') }}
            </button>
            <button
              type="button"
              @click="modelRestrictionMode = 'mapping'"
              :class="[
                'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                modelRestrictionMode === 'mapping'
                  ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
            >
              <svg class="w-4 h-4 inline mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
              </svg>
              {{ t('admin.accounts.modelMapping') }}
            </button>
          </div>

          <!-- Whitelist Mode -->
          <div v-if="modelRestrictionMode === 'whitelist'">
            <div class="mb-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 p-3">
              <p class="text-xs text-blue-700 dark:text-blue-400">
                <svg class="w-4 h-4 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {{ t('admin.accounts.selectAllowedModels') }}
              </p>
            </div>

            <!-- Model Checkbox List -->
            <div class="grid grid-cols-2 gap-2 mb-3">
              <label
                v-for="model in commonModels"
                :key="model.value"
                class="flex cursor-pointer items-center rounded-lg border p-3 transition-all hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
                :class="allowedModels.includes(model.value) ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20' : 'border-gray-200'"
              >
                <input
                  type="checkbox"
                  :value="model.value"
                  v-model="allowedModels"
                  class="mr-2 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                <span class="text-sm text-gray-700 dark:text-gray-300">{{ model.label }}</span>
              </label>
            </div>

            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
              <span v-if="allowedModels.length === 0">{{ t('admin.accounts.supportsAllModels') }}</span>
            </p>
          </div>

          <!-- Mapping Mode -->
          <div v-else>
            <div class="mb-3 rounded-lg bg-purple-50 dark:bg-purple-900/20 p-3">
              <p class="text-xs text-purple-700 dark:text-purple-400">
                <svg class="w-4 h-4 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {{ t('admin.accounts.mapRequestModels') }}
              </p>
            </div>

            <!-- Model Mapping List -->
            <div v-if="modelMappings.length > 0" class="space-y-2 mb-3">
              <div
                v-for="(mapping, index) in modelMappings"
                :key="index"
                class="flex items-center gap-2"
              >
                <input
                  v-model="mapping.from"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.requestModel')"
                />
                <svg class="w-4 h-4 text-gray-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 5l7 7m0 0l-7 7m7-7H3" />
                </svg>
                <input
                  v-model="mapping.to"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.actualModel')"
                />
                <button
                  type="button"
                  @click="removeModelMapping(index)"
                  class="p-2 text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-colors"
                >
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                </button>
              </div>
            </div>

            <button
              type="button"
              @click="addModelMapping"
              class="w-full rounded-lg border-2 border-dashed border-gray-300 dark:border-dark-500 px-4 py-2 text-gray-600 dark:text-gray-400 transition-colors hover:border-gray-400 hover:text-gray-700 dark:hover:border-dark-400 dark:hover:text-gray-300 mb-3"
            >
              <svg class="w-4 h-4 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
              </svg>
              {{ t('admin.accounts.addMapping') }}
            </button>

            <!-- Quick Add Buttons -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="preset in presetMappings"
                :key="preset.label"
                type="button"
                @click="addPresetMapping(preset.from, preset.to)"
                :class="[
                  'rounded-lg px-3 py-1 text-xs transition-colors',
                  preset.color
                ]"
              >
                + {{ preset.label }}
              </button>
            </div>
          </div>
        </div>

        <!-- Custom Error Codes Section -->
        <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
          <div class="flex items-center justify-between mb-3">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.customErrorCodes') }}</label>
              <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.accounts.customErrorCodesHint') }}</p>
            </div>
            <button
              type="button"
              @click="customErrorCodesEnabled = !customErrorCodesEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                customErrorCodesEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  customErrorCodesEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="customErrorCodesEnabled" class="space-y-3">
            <div class="rounded-lg bg-amber-50 dark:bg-amber-900/20 p-3">
              <p class="text-xs text-amber-700 dark:text-amber-400">
                <svg class="w-4 h-4 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
                {{ t('admin.accounts.customErrorCodesWarning') }}
              </p>
            </div>

            <!-- Error Code Buttons -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="code in commonErrorCodes"
                :key="code.value"
                type="button"
                @click="toggleErrorCode(code.value)"
                :class="[
                  'rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
                  selectedErrorCodes.includes(code.value)
                    ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 ring-1 ring-red-500'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
              >
                {{ code.value }} {{ code.label }}
              </button>
            </div>

            <!-- Manual input -->
            <div class="flex items-center gap-2">
              <input
                v-model="customErrorCodeInput"
                type="number"
                min="100"
                max="599"
                class="input flex-1"
                :placeholder="t('admin.accounts.enterErrorCode')"
                @keyup.enter="addCustomErrorCode"
              />
              <button
                type="button"
                @click="addCustomErrorCode"
                class="btn btn-secondary px-3"
              >
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
                </svg>
              </button>
            </div>

            <!-- Selected codes summary -->
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="code in selectedErrorCodes.sort((a, b) => a - b)"
                :key="code"
                class="inline-flex items-center gap-1 rounded-full bg-red-100 dark:bg-red-900/30 px-2.5 py-0.5 text-sm font-medium text-red-700 dark:text-red-400"
              >
                {{ code }}
                <button
                  type="button"
                  @click="removeErrorCode(code)"
                  class="hover:text-red-900 dark:hover:text-red-300"
                >
                  <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </span>
              <span v-if="selectedErrorCodes.length === 0" class="text-xs text-gray-400">
                {{ t('admin.accounts.noneSelectedUsesDefault') }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- Intercept Warmup Requests (Anthropic only) -->
      <div v-if="account?.platform === 'anthropic'" class="border-t border-gray-200 dark:border-dark-600 pt-4">
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.interceptWarmupRequests') }}</label>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.accounts.interceptWarmupRequestsDesc') }}</p>
          </div>
          <button
            type="button"
            @click="interceptWarmupRequests = !interceptWarmupRequests"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              interceptWarmupRequests ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
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

      <div>
        <label class="input-label">{{ t('admin.accounts.proxy') }}</label>
        <ProxySelector
          v-model="form.proxy_id"
          :proxies="proxies"
        />
      </div>

      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
          <input
            v-model.number="form.concurrency"
            type="number"
            min="1"
            class="input"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.priority') }}</label>
          <input
            v-model.number="form.priority"
            type="number"
            min="1"
            class="input"
          />
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('common.status') }}</label>
        <Select
          v-model="form.status"
          :options="statusOptions"
        />
      </div>

      <!-- Group Selection -->
      <GroupSelector
        v-model="form.group_ids"
        :groups="groups"
        :platform="account?.platform"
      />

      <div class="flex justify-end gap-3 pt-4">
        <button
          @click="handleClose"
          type="button"
          class="btn btn-secondary"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          :disabled="submitting"
          class="btn btn-primary"
        >
          <svg
            v-if="submitting"
            class="animate-spin -ml-1 mr-2 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          {{ submitting ? t('admin.accounts.updating') : t('common.update') }}
        </button>
      </div>
    </form>
  </Modal>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { Account, Proxy, Group } from '@/types'
import Modal from '@/components/common/Modal.vue'
import Select from '@/components/common/Select.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'

interface Props {
  show: boolean
  account: Account | null
  proxies: Proxy[]
  groups: Group[]
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
  updated: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

// Model mapping type
interface ModelMapping {
  from: string
  to: string
}

// State
const submitting = ref(false)
const editBaseUrl = ref('https://api.anthropic.com')
const editApiKey = ref('')
const modelMappings = ref<ModelMapping[]>([])
const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const allowedModels = ref<string[]>([])
const customErrorCodesEnabled = ref(false)
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const interceptWarmupRequests = ref(false)

// Common models for whitelist - Anthropic
const anthropicModels = [
  { value: 'claude-opus-4-5-20251101', label: 'Claude Opus 4.5' },
  { value: 'claude-sonnet-4-20250514', label: 'Claude Sonnet 4' },
  { value: 'claude-sonnet-4-5-20250929', label: 'Claude Sonnet 4.5' },
  { value: 'claude-3-5-haiku-20241022', label: 'Claude 3.5 Haiku' },
  { value: 'claude-haiku-4-5-20251001', label: 'Claude Haiku 4.5' },
  { value: 'claude-3-opus-20240229', label: 'Claude 3 Opus' },
  { value: 'claude-3-5-sonnet-20241022', label: 'Claude 3.5 Sonnet' },
  { value: 'claude-3-haiku-20240307', label: 'Claude 3 Haiku' }
]

// Common models for whitelist - OpenAI
const openaiModels = [
  { value: 'gpt-5.2-2025-12-11', label: 'GPT-5.2' },
  { value: 'gpt-5.2-codex', label: 'GPT-5.2 Codex' },
  { value: 'gpt-5.1-codex-max', label: 'GPT-5.1 Codex Max' },
  { value: 'gpt-5.1-codex', label: 'GPT-5.1 Codex' },
  { value: 'gpt-5.1-2025-11-13', label: 'GPT-5.1' },
  { value: 'gpt-5.1-codex-mini', label: 'GPT-5.1 Codex Mini' },
  { value: 'gpt-5-2025-08-07', label: 'GPT-5' }
]

// Computed: current models based on platform
const commonModels = computed(() => {
  return props.account?.platform === 'openai' ? openaiModels : anthropicModels
})

// Preset mappings for quick add - Anthropic
const anthropicPresetMappings = [
  { label: 'Sonnet 4', from: 'claude-sonnet-4-20250514', to: 'claude-sonnet-4-20250514', color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400' },
  { label: 'Sonnet 4.5', from: 'claude-sonnet-4-5-20250929', to: 'claude-sonnet-4-5-20250929', color: 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400' },
  { label: 'Opus 4.5', from: 'claude-opus-4-5-20251101', to: 'claude-opus-4-5-20251101', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
  { label: 'Haiku 3.5', from: 'claude-3-5-haiku-20241022', to: 'claude-3-5-haiku-20241022', color: 'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400' },
  { label: 'Haiku 4.5', from: 'claude-haiku-4-5-20251001', to: 'claude-haiku-4-5-20251001', color: 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400' },
  { label: 'Opus->Sonnet', from: 'claude-opus-4-5-20251101', to: 'claude-sonnet-4-5-20250929', color: 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400' }
]

// Preset mappings for quick add - OpenAI
const openaiPresetMappings = [
  { label: 'GPT-5.2', from: 'gpt-5.2-2025-12-11', to: 'gpt-5.2-2025-12-11', color: 'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400' },
  { label: 'GPT-5.2 Codex', from: 'gpt-5.2-codex', to: 'gpt-5.2-codex', color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400' },
  { label: 'GPT-5.1 Codex', from: 'gpt-5.1-codex', to: 'gpt-5.1-codex', color: 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400' },
  { label: 'Codex Max', from: 'gpt-5.1-codex-max', to: 'gpt-5.1-codex-max', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
  { label: 'Codex Mini', from: 'gpt-5.1-codex-mini', to: 'gpt-5.1-codex-mini', color: 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400' },
  { label: 'Max->Codex', from: 'gpt-5.1-codex-max', to: 'gpt-5.1-codex', color: 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400' }
]

// Computed: current preset mappings based on platform
const presetMappings = computed(() => {
  return props.account?.platform === 'openai' ? openaiPresetMappings : anthropicPresetMappings
})

// Computed: default base URL based on platform
const defaultBaseUrl = computed(() => {
  return props.account?.platform === 'openai' ? 'https://api.openai.com' : 'https://api.anthropic.com'
})

// Common HTTP error codes for quick selection
const commonErrorCodes = [
  { value: 401, label: 'Unauthorized' },
  { value: 403, label: 'Forbidden' },
  { value: 429, label: 'Rate Limit' },
  { value: 500, label: 'Server Error' },
  { value: 502, label: 'Bad Gateway' },
  { value: 503, label: 'Unavailable' },
  { value: 529, label: 'Overloaded' }
]

const form = reactive({
  name: '',
  proxy_id: null as number | null,
  concurrency: 1,
  priority: 1,
  status: 'active' as 'active' | 'inactive',
  group_ids: [] as number[]
})

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])

// Watchers
watch(() => props.account, (newAccount) => {
  if (newAccount) {
    form.name = newAccount.name
    form.proxy_id = newAccount.proxy_id
    form.concurrency = newAccount.concurrency
    form.priority = newAccount.priority
    form.status = newAccount.status as 'active' | 'inactive'
    form.group_ids = newAccount.group_ids || []

    // Load intercept warmup requests setting (applies to all account types)
    const credentials = newAccount.credentials as Record<string, unknown> | undefined
    interceptWarmupRequests.value = credentials?.intercept_warmup_requests === true

    // Initialize API Key fields for apikey type
    if (newAccount.type === 'apikey' && newAccount.credentials) {
      const credentials = newAccount.credentials as Record<string, unknown>
      const platformDefaultUrl = newAccount.platform === 'openai' ? 'https://api.openai.com' : 'https://api.anthropic.com'
      editBaseUrl.value = credentials.base_url as string || platformDefaultUrl

      // Load model mappings and detect mode
      const existingMappings = credentials.model_mapping as Record<string, string> | undefined
      if (existingMappings && typeof existingMappings === 'object') {
        const entries = Object.entries(existingMappings)

        // Detect if this is whitelist mode (all from === to) or mapping mode
        const isWhitelistMode = entries.length > 0 && entries.every(([from, to]) => from === to)

        if (isWhitelistMode) {
          // Whitelist mode: populate allowedModels
          modelRestrictionMode.value = 'whitelist'
          allowedModels.value = entries.map(([from]) => from)
          modelMappings.value = []
        } else {
          // Mapping mode: populate modelMappings
          modelRestrictionMode.value = 'mapping'
          modelMappings.value = entries.map(([from, to]) => ({ from, to }))
          allowedModels.value = []
        }
      } else {
        // No mappings: default to whitelist mode with empty selection (allow all)
        modelRestrictionMode.value = 'whitelist'
        modelMappings.value = []
        allowedModels.value = []
      }

      // Load custom error codes
      customErrorCodesEnabled.value = credentials.custom_error_codes_enabled === true
      const existingErrorCodes = credentials.custom_error_codes as number[] | undefined
      if (existingErrorCodes && Array.isArray(existingErrorCodes)) {
        selectedErrorCodes.value = [...existingErrorCodes]
      } else {
        selectedErrorCodes.value = []
      }
    } else {
      const platformDefaultUrl = newAccount.platform === 'openai' ? 'https://api.openai.com' : 'https://api.anthropic.com'
      editBaseUrl.value = platformDefaultUrl
      modelRestrictionMode.value = 'whitelist'
      modelMappings.value = []
      allowedModels.value = []
      customErrorCodesEnabled.value = false
      selectedErrorCodes.value = []
    }
    editApiKey.value = ''
  }
}, { immediate: true })

// Model mapping helpers
const addModelMapping = () => {
  modelMappings.value.push({ from: '', to: '' })
}

const removeModelMapping = (index: number) => {
  modelMappings.value.splice(index, 1)
}

const addPresetMapping = (from: string, to: string) => {
  const exists = modelMappings.value.some(m => m.from === from)
  if (exists) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
    return
  }
  modelMappings.value.push({ from, to })
}

// Error code toggle helper
const toggleErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index === -1) {
    selectedErrorCodes.value.push(code)
  } else {
    selectedErrorCodes.value.splice(index, 1)
  }
}

// Add custom error code from input
const addCustomErrorCode = () => {
  const code = customErrorCodeInput.value
  if (code === null || code < 100 || code > 599) {
    appStore.showError(t('admin.accounts.invalidErrorCode'))
    return
  }
  if (selectedErrorCodes.value.includes(code)) {
    appStore.showInfo(t('admin.accounts.errorCodeExists'))
    return
  }
  selectedErrorCodes.value.push(code)
  customErrorCodeInput.value = null
}

// Remove error code
const removeErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index !== -1) {
    selectedErrorCodes.value.splice(index, 1)
  }
}

const buildModelMappingObject = (): Record<string, string> | null => {
  const mapping: Record<string, string> = {}

  if (modelRestrictionMode.value === 'whitelist') {
    // Whitelist mode: model maps to itself
    for (const model of allowedModels.value) {
      mapping[model] = model
    }
  } else {
    // Mapping mode: use the mapping entries
    for (const m of modelMappings.value) {
      const from = m.from.trim()
      const to = m.to.trim()
      if (from && to) {
        mapping[from] = to
      }
    }
  }

  return Object.keys(mapping).length > 0 ? mapping : null
}

// Methods
const handleClose = () => {
  emit('close')
}

const handleSubmit = async () => {
  if (!props.account) return

  submitting.value = true
  try {
    const updatePayload: Record<string, unknown> = { ...form }

    // For apikey type, handle credentials update
    if (props.account.type === 'apikey') {
      const currentCredentials = props.account.credentials as Record<string, unknown> || {}
      const newBaseUrl = editBaseUrl.value.trim() || defaultBaseUrl.value
      const modelMapping = buildModelMappingObject()

      // Always update credentials for apikey type to handle model mapping changes
      const newCredentials: Record<string, unknown> = {
        base_url: newBaseUrl
      }

      // Handle API key
      if (editApiKey.value.trim()) {
        // User provided a new API key
        newCredentials.api_key = editApiKey.value.trim()
      } else if (currentCredentials.api_key) {
        // Preserve existing api_key
        newCredentials.api_key = currentCredentials.api_key
      } else {
        appStore.showError(t('admin.accounts.apiKeyIsRequired'))
        submitting.value = false
        return
      }

      // Add model mapping if configured
      if (modelMapping) {
        newCredentials.model_mapping = modelMapping
      }

      // Add custom error codes if enabled
      if (customErrorCodesEnabled.value) {
        newCredentials.custom_error_codes_enabled = true
        newCredentials.custom_error_codes = [...selectedErrorCodes.value]
      }

      // Add intercept warmup requests setting
      if (interceptWarmupRequests.value) {
        newCredentials.intercept_warmup_requests = true
      }

      updatePayload.credentials = newCredentials
    } else {
      // For oauth/setup-token types, only update intercept_warmup_requests if changed
      const currentCredentials = props.account.credentials as Record<string, unknown> || {}
      const newCredentials: Record<string, unknown> = { ...currentCredentials }

      if (interceptWarmupRequests.value) {
        newCredentials.intercept_warmup_requests = true
      } else {
        delete newCredentials.intercept_warmup_requests
      }

      updatePayload.credentials = newCredentials
    }

    await adminAPI.accounts.update(props.account.id, updatePayload)
    appStore.showSuccess(t('admin.accounts.accountUpdated'))
    emit('updated')
    handleClose()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.accounts.failedToUpdate'))
  } finally {
    submitting.value = false
  }
}
</script>
