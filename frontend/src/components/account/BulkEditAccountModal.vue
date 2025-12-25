<template>
  <Modal :show="show" :title="t('admin.accounts.bulkEdit.title')" size="lg" @close="handleClose">
    <form class="space-y-5" @submit.prevent="handleSubmit">
      <!-- Info -->
      <div class="rounded-lg bg-blue-50 dark:bg-blue-900/20 p-4">
        <p class="text-sm text-blue-700 dark:text-blue-400">
          <svg class="w-5 h-5 inline mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
          {{ t('admin.accounts.bulkEdit.selectionInfo', { count: accountIds.length }) }}
        </p>
      </div>

      <!-- Base URL (API Key only) -->
      <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
        <div class="flex items-center justify-between mb-3">
          <label class="input-label mb-0">{{ t('admin.accounts.baseUrl') }}</label>
          <input
            v-model="enableBaseUrl"
            type="checkbox"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <input
          v-model="baseUrl"
          type="text"
          :disabled="!enableBaseUrl"
          class="input"
          :class="!enableBaseUrl && 'opacity-50 cursor-not-allowed'"
          :placeholder="t('admin.accounts.bulkEdit.baseUrlPlaceholder')"
        />
        <p class="input-hint">
          {{ t('admin.accounts.bulkEdit.baseUrlNotice') }}
        </p>
      </div>

      <!-- Model restriction -->
      <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
        <div class="flex items-center justify-between mb-3">
          <label class="input-label mb-0">{{ t('admin.accounts.modelRestriction') }}</label>
          <input
            v-model="enableModelRestriction"
            type="checkbox"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>

        <div :class="!enableModelRestriction && 'opacity-50 pointer-events-none'">
          <!-- Mode Toggle -->
          <div class="flex gap-2 mb-4">
            <button
              type="button"
              :class="[
                'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                modelRestrictionMode === 'whitelist'
                  ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500',
              ]"
              @click="modelRestrictionMode = 'whitelist'"
            >
              <svg
                class="w-4 h-4 inline mr-1.5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
              {{ t('admin.accounts.modelWhitelist') }}
            </button>
            <button
              type="button"
              :class="[
                'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                modelRestrictionMode === 'mapping'
                  ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500',
              ]"
              @click="modelRestrictionMode = 'mapping'"
            >
              <svg
                class="w-4 h-4 inline mr-1.5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
                />
              </svg>
              {{ t('admin.accounts.modelMapping') }}
            </button>
          </div>

          <!-- Whitelist Mode -->
          <div v-if="modelRestrictionMode === 'whitelist'">
            <div class="mb-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 p-3">
              <p class="text-xs text-blue-700 dark:text-blue-400">
                <svg
                  class="w-4 h-4 inline mr-1"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                {{ t('admin.accounts.selectAllowedModels') }}
              </p>
            </div>

            <!-- Model Checkbox List -->
            <div class="grid grid-cols-2 gap-2 mb-3">
              <label
                v-for="model in allModels"
                :key="model.value"
                class="flex cursor-pointer items-center rounded-lg border p-3 transition-all hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
                :class="
                  allowedModels.includes(model.value)
                    ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20'
                    : 'border-gray-200'
                "
              >
                <input
                  v-model="allowedModels"
                  type="checkbox"
                  :value="model.value"
                  class="mr-2 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                <span class="text-sm text-gray-700 dark:text-gray-300">{{ model.label }}</span>
              </label>
            </div>

            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
              <span v-if="allowedModels.length === 0">{{
                t('admin.accounts.supportsAllModels')
              }}</span>
            </p>
          </div>

          <!-- Mapping Mode -->
          <div v-else>
            <div class="mb-3 rounded-lg bg-purple-50 dark:bg-purple-900/20 p-3">
              <p class="text-xs text-purple-700 dark:text-purple-400">
                <svg
                  class="w-4 h-4 inline mr-1"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
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
                <svg
                  class="w-4 h-4 text-gray-400 flex-shrink-0"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M14 5l7 7m0 0l-7 7m7-7H3"
                  />
                </svg>
                <input
                  v-model="mapping.to"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.actualModel')"
                />
                <button
                  type="button"
                  class="p-2 text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-colors"
                  @click="removeModelMapping(index)"
                >
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                    />
                  </svg>
                </button>
              </div>
            </div>

            <button
              type="button"
              class="w-full rounded-lg border-2 border-dashed border-gray-300 dark:border-dark-500 px-4 py-2 text-gray-600 dark:text-gray-400 transition-colors hover:border-gray-400 hover:text-gray-700 dark:hover:border-dark-400 dark:hover:text-gray-300 mb-3"
              @click="addModelMapping"
            >
              <svg
                class="w-4 h-4 inline mr-1"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4v16m8-8H4"
                />
              </svg>
              {{ t('admin.accounts.addMapping') }}
            </button>

            <!-- Quick Add Buttons -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="preset in presetMappings"
                :key="preset.label"
                type="button"
                :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
                @click="addPresetMapping(preset.from, preset.to)"
              >
                + {{ preset.label }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Custom error codes -->
      <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
        <div class="flex items-center justify-between mb-3">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.customErrorCodes') }}</label>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
              {{ t('admin.accounts.customErrorCodesHint') }}
            </p>
          </div>
          <input
            v-model="enableCustomErrorCodes"
            type="checkbox"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>

        <div v-if="enableCustomErrorCodes" class="space-y-3">
          <div class="rounded-lg bg-amber-50 dark:bg-amber-900/20 p-3">
            <p class="text-xs text-amber-700 dark:text-amber-400">
              <svg
                class="w-4 h-4 inline mr-1"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                />
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
              :class="[
                'rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
                selectedErrorCodes.includes(code.value)
                  ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 ring-1 ring-red-500'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500',
              ]"
              @click="toggleErrorCode(code.value)"
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
            <button type="button" class="btn btn-secondary px-3" @click="addCustomErrorCode">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4v16m8-8H4"
                />
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
                class="hover:text-red-900 dark:hover:text-red-300"
                @click="removeErrorCode(code)"
              >
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              </button>
            </span>
            <span v-if="selectedErrorCodes.length === 0" class="text-xs text-gray-400">
              {{ t('admin.accounts.noneSelectedUsesDefault') }}
            </span>
          </div>
        </div>
      </div>

      <!-- Intercept warmup requests (Anthropic only) -->
      <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
        <div class="flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label class="input-label mb-0">{{
              t('admin.accounts.interceptWarmupRequests')
            }}</label>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
              {{ t('admin.accounts.interceptWarmupRequestsDesc') }}
            </p>
          </div>
          <input
            v-model="enableInterceptWarmup"
            type="checkbox"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div v-if="enableInterceptWarmup" class="mt-3">
          <button
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              interceptWarmupRequests ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600',
            ]"
            @click="interceptWarmupRequests = !interceptWarmupRequests"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                interceptWarmupRequests ? 'translate-x-5' : 'translate-x-0',
              ]"
            />
          </button>
        </div>
      </div>

      <!-- Proxy -->
      <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
        <div class="flex items-center justify-between mb-3">
          <label class="input-label mb-0">{{ t('admin.accounts.proxy') }}</label>
          <input
            v-model="enableProxy"
            type="checkbox"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div :class="!enableProxy && 'opacity-50 pointer-events-none'">
          <ProxySelector v-model="proxyId" :proxies="proxies" />
        </div>
      </div>

      <!-- Concurrency & Priority -->
      <div class="grid grid-cols-2 gap-4 border-t border-gray-200 dark:border-dark-600 pt-4">
        <div>
          <div class="flex items-center justify-between mb-3">
            <label class="input-label mb-0">{{ t('admin.accounts.concurrency') }}</label>
            <input
              v-model="enableConcurrency"
              type="checkbox"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
          </div>
          <input
            v-model.number="concurrency"
            type="number"
            min="1"
            :disabled="!enableConcurrency"
            class="input"
            :class="!enableConcurrency && 'opacity-50 cursor-not-allowed'"
          />
        </div>
        <div>
          <div class="flex items-center justify-between mb-3">
            <label class="input-label mb-0">{{ t('admin.accounts.priority') }}</label>
            <input
              v-model="enablePriority"
              type="checkbox"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
          </div>
          <input
            v-model.number="priority"
            type="number"
            min="1"
            :disabled="!enablePriority"
            class="input"
            :class="!enablePriority && 'opacity-50 cursor-not-allowed'"
          />
        </div>
      </div>

      <!-- Status -->
      <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
        <div class="flex items-center justify-between mb-3">
          <label class="input-label mb-0">{{ t('common.status') }}</label>
          <input
            v-model="enableStatus"
            type="checkbox"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div :class="!enableStatus && 'opacity-50 pointer-events-none'">
          <Select v-model="status" :options="statusOptions" />
        </div>
      </div>

      <!-- Groups -->
      <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
        <div class="flex items-center justify-between mb-3">
          <label class="input-label mb-0">{{ t('nav.groups') }}</label>
          <input
            v-model="enableGroups"
            type="checkbox"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div :class="!enableGroups && 'opacity-50 pointer-events-none'">
          <GroupSelector v-model="groupIds" :groups="groups" />
        </div>
      </div>

      <!-- Action buttons -->
      <div class="flex justify-end gap-3 pt-4">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" :disabled="submitting" class="btn btn-primary">
          <svg
            v-if="submitting"
            class="animate-spin -ml-1 mr-2 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
          {{
            submitting ? t('admin.accounts.bulkEdit.updating') : t('admin.accounts.bulkEdit.submit')
          }}
        </button>
      </div>
    </form>
  </Modal>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { Proxy, Group } from '@/types'
import Modal from '@/components/common/Modal.vue'
import Select from '@/components/common/Select.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'

interface Props {
  show: boolean
  accountIds: number[]
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

// State - field enable flags
const enableBaseUrl = ref(false)
const enableModelRestriction = ref(false)
const enableCustomErrorCodes = ref(false)
const enableInterceptWarmup = ref(false)
const enableProxy = ref(false)
const enableConcurrency = ref(false)
const enablePriority = ref(false)
const enableStatus = ref(false)
const enableGroups = ref(false)

// State - field values
const submitting = ref(false)
const baseUrl = ref('')
const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const allowedModels = ref<string[]>([])
const modelMappings = ref<ModelMapping[]>([])
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const interceptWarmupRequests = ref(false)
const proxyId = ref<number | null>(null)
const concurrency = ref(1)
const priority = ref(1)
const status = ref<'active' | 'inactive'>('active')
const groupIds = ref<number[]>([])

// All models list (combined Anthropic + OpenAI)
const allModels = [
  { value: 'claude-opus-4-5-20251101', label: 'Claude Opus 4.5' },
  { value: 'claude-sonnet-4-20250514', label: 'Claude Sonnet 4' },
  { value: 'claude-sonnet-4-5-20250929', label: 'Claude Sonnet 4.5' },
  { value: 'claude-3-5-haiku-20241022', label: 'Claude 3.5 Haiku' },
  { value: 'claude-haiku-4-5-20251001', label: 'Claude Haiku 4.5' },
  { value: 'claude-3-opus-20240229', label: 'Claude 3 Opus' },
  { value: 'claude-3-5-sonnet-20241022', label: 'Claude 3.5 Sonnet' },
  { value: 'claude-3-haiku-20240307', label: 'Claude 3 Haiku' },
  { value: 'gpt-5.2-2025-12-11', label: 'GPT-5.2' },
  { value: 'gpt-5.2-codex', label: 'GPT-5.2 Codex' },
  { value: 'gpt-5.1-codex-max', label: 'GPT-5.1 Codex Max' },
  { value: 'gpt-5.1-codex', label: 'GPT-5.1 Codex' },
  { value: 'gpt-5.1-2025-11-13', label: 'GPT-5.1' },
  { value: 'gpt-5.1-codex-mini', label: 'GPT-5.1 Codex Mini' },
  { value: 'gpt-5-2025-08-07', label: 'GPT-5' },
]

// Preset mappings (combined Anthropic + OpenAI)
const presetMappings = [
  {
    label: 'Sonnet 4',
    from: 'claude-sonnet-4-20250514',
    to: 'claude-sonnet-4-20250514',
    color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400',
  },
  {
    label: 'Sonnet 4.5',
    from: 'claude-sonnet-4-5-20250929',
    to: 'claude-sonnet-4-5-20250929',
    color:
      'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400',
  },
  {
    label: 'Opus 4.5',
    from: 'claude-opus-4-5-20251101',
    to: 'claude-opus-4-5-20251101',
    color:
      'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400',
  },
  {
    label: 'Opus->Sonnet',
    from: 'claude-opus-4-5-20251101',
    to: 'claude-sonnet-4-5-20250929',
    color:
      'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400',
  },
  {
    label: 'GPT-5.2',
    from: 'gpt-5.2-2025-12-11',
    to: 'gpt-5.2-2025-12-11',
    color:
      'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400',
  },
  {
    label: 'GPT-5.2 Codex',
    from: 'gpt-5.2-codex',
    to: 'gpt-5.2-codex',
    color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400',
  },
  {
    label: 'Max->Codex',
    from: 'gpt-5.1-codex-max',
    to: 'gpt-5.1-codex',
    color: 'bg-pink-100 text-pink-700 hover:bg-pink-200 dark:bg-pink-900/30 dark:text-pink-400',
  },
]

// Common HTTP error codes
const commonErrorCodes = [
  { value: 401, label: 'Unauthorized' },
  { value: 403, label: 'Forbidden' },
  { value: 429, label: 'Rate Limit' },
  { value: 500, label: 'Server Error' },
  { value: 502, label: 'Bad Gateway' },
  { value: 503, label: 'Unavailable' },
  { value: 529, label: 'Overloaded' },
]

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') },
])

// Model mapping helpers
const addModelMapping = () => {
  modelMappings.value.push({ from: '', to: '' })
}

const removeModelMapping = (index: number) => {
  modelMappings.value.splice(index, 1)
}

const addPresetMapping = (from: string, to: string) => {
  const exists = modelMappings.value.some((m) => m.from === from)
  if (exists) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
    return
  }
  modelMappings.value.push({ from, to })
}

// Error code helpers
const toggleErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index === -1) {
    selectedErrorCodes.value.push(code)
  } else {
    selectedErrorCodes.value.splice(index, 1)
  }
}

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

const removeErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index !== -1) {
    selectedErrorCodes.value.splice(index, 1)
  }
}

const buildModelMappingObject = (): Record<string, string> | null => {
  const mapping: Record<string, string> = {}

  if (modelRestrictionMode.value === 'whitelist') {
    for (const model of allowedModels.value) {
      mapping[model] = model
    }
  } else {
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

const buildUpdatePayload = (): Record<string, unknown> | null => {
  const updates: Record<string, unknown> = {}
  const credentials: Record<string, unknown> = {}
  let credentialsChanged = false

  if (enableProxy.value) {
    updates.proxy_id = proxyId.value
  }

  if (enableConcurrency.value) {
    updates.concurrency = concurrency.value
  }

  if (enablePriority.value) {
    updates.priority = priority.value
  }

  if (enableStatus.value) {
    updates.status = status.value
  }

  if (enableGroups.value) {
    updates.group_ids = groupIds.value
  }

  if (enableBaseUrl.value) {
    const baseUrlValue = baseUrl.value.trim()
    if (baseUrlValue) {
      credentials.base_url = baseUrlValue
      credentialsChanged = true
    }
  }

  if (enableModelRestriction.value) {
    const modelMapping = buildModelMappingObject()
    if (modelMapping) {
      credentials.model_mapping = modelMapping
      credentialsChanged = true
    }
  }

  if (enableCustomErrorCodes.value) {
    credentials.custom_error_codes_enabled = true
    credentials.custom_error_codes = [...selectedErrorCodes.value]
    credentialsChanged = true
  }

  if (enableInterceptWarmup.value) {
    credentials.intercept_warmup_requests = interceptWarmupRequests.value
    credentialsChanged = true
  }

  if (credentialsChanged) {
    updates.credentials = credentials
  }

  return Object.keys(updates).length > 0 ? updates : null
}

const handleClose = () => {
  emit('close')
}

const handleSubmit = async () => {
  if (props.accountIds.length === 0) {
    appStore.showError(t('admin.accounts.bulkEdit.noSelection'))
    return
  }

  const hasAnyFieldEnabled =
    enableBaseUrl.value ||
    enableModelRestriction.value ||
    enableCustomErrorCodes.value ||
    enableInterceptWarmup.value ||
    enableProxy.value ||
    enableConcurrency.value ||
    enablePriority.value ||
    enableStatus.value ||
    enableGroups.value

  if (!hasAnyFieldEnabled) {
    appStore.showError(t('admin.accounts.bulkEdit.noFieldsSelected'))
    return
  }

  const updates = buildUpdatePayload()
  if (!updates) {
    appStore.showError(t('admin.accounts.bulkEdit.noFieldsSelected'))
    return
  }

  submitting.value = true

  try {
    const res = await adminAPI.accounts.bulkUpdate(props.accountIds, updates)
    const success = res.success || 0
    const failed = res.failed || 0

    if (success > 0 && failed === 0) {
      appStore.showSuccess(t('admin.accounts.bulkEdit.success', { count: success }))
    } else if (success > 0) {
      appStore.showError(t('admin.accounts.bulkEdit.partialSuccess', { success, failed }))
    } else {
      appStore.showError(t('admin.accounts.bulkEdit.failed'))
    }

    if (success > 0) {
      emit('updated')
      handleClose()
    }
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.accounts.bulkEdit.failed'))
    console.error('Error bulk updating accounts:', error)
  } finally {
    submitting.value = false
  }
}

// Reset form when modal closes
watch(
  () => props.show,
  (newShow) => {
    if (!newShow) {
      // Reset all enable flags
      enableBaseUrl.value = false
      enableModelRestriction.value = false
      enableCustomErrorCodes.value = false
      enableInterceptWarmup.value = false
      enableProxy.value = false
      enableConcurrency.value = false
      enablePriority.value = false
      enableStatus.value = false
      enableGroups.value = false

      // Reset all values
      baseUrl.value = ''
      modelRestrictionMode.value = 'whitelist'
      allowedModels.value = []
      modelMappings.value = []
      selectedErrorCodes.value = []
      customErrorCodeInput.value = null
      interceptWarmupRequests.value = false
      proxyId.value = null
      concurrency.value = 1
      priority.value = 1
      status.value = 'active'
      groupIds.value = []
    }
  }
)
</script>
