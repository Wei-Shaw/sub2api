<template>
  <BaseDialog
    :show="show"
    :title="t('keys.useKeyModal.title')"
    width="normal"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- No Group Assigned Warning -->
      <div v-if="!platform" class="flex items-start gap-3 p-4 rounded-lg bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800">
        <svg class="w-5 h-5 text-yellow-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
        </svg>
        <div>
          <p class="text-sm font-medium text-yellow-800 dark:text-yellow-200">
            {{ t('keys.useKeyModal.noGroupTitle') }}
          </p>
          <p class="text-sm text-yellow-700 dark:text-yellow-300 mt-1">
            {{ t('keys.useKeyModal.noGroupDescription') }}
          </p>
        </div>
      </div>

      <template v-else>
        <!-- Description -->
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ t('keys.useKeyModal.simpleDesc') }}
        </p>

        <!-- Prompt Block -->
        <div class="relative">
          <div class="bg-gray-50 dark:bg-dark-800 rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
            <div class="flex items-center justify-between px-4 py-2 border-b border-gray-200 dark:border-gray-700">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('keys.useKeyModal.promptLabel') }}</span>
              <button
                @click="copyText(promptText, 'prompt')"
                class="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md transition-colors"
                :class="copiedField === 'prompt'
                  ? 'bg-green-500/20 text-green-600 dark:text-green-400'
                  : 'bg-white dark:bg-dark-700 hover:bg-gray-100 dark:hover:bg-dark-600 text-gray-600 dark:text-gray-300 border border-gray-200 dark:border-gray-600'"
              >
                <svg v-if="copiedField === 'prompt'" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                <svg v-else class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
                </svg>
                {{ copiedField === 'prompt' ? '✓' : t('keys.useKeyModal.copy') }}
              </button>
            </div>
            <pre class="p-4 text-sm font-mono text-gray-800 dark:text-gray-200 whitespace-pre-wrap leading-relaxed">{{ promptText }}</pre>
          </div>
        </div>

        <!-- API Key Block -->
        <div class="relative">
          <div class="bg-gray-50 dark:bg-dark-800 rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
            <div class="flex items-center justify-between px-4 py-2 border-b border-gray-200 dark:border-gray-700">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">API Key</span>
              <button
                @click="copyText(props.apiKey, 'key')"
                class="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md transition-colors"
                :class="copiedField === 'key'
                  ? 'bg-green-500/20 text-green-600 dark:text-green-400'
                  : 'bg-white dark:bg-dark-700 hover:bg-gray-100 dark:hover:bg-dark-600 text-gray-600 dark:text-gray-300 border border-gray-200 dark:border-gray-600'"
              >
                <svg v-if="copiedField === 'key'" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                <svg v-else class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
                </svg>
                {{ copiedField === 'key' ? '✓' : t('keys.useKeyModal.copy') }}
              </button>
            </div>
            <pre class="p-4 text-sm font-mono text-gray-800 dark:text-gray-200 select-all">{{ props.apiKey }}</pre>
          </div>
        </div>

        <!-- Supported Models -->
        <div class="rounded-lg border border-gray-200 dark:border-gray-700 p-3">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2">{{ t('keys.useKeyModal.supportedModels') }}</p>
          <div class="flex flex-wrap gap-1.5">
            <span
              v-for="model in supportedModels"
              :key="model"
              class="inline-flex items-center px-2 py-0.5 text-xs font-medium rounded-md bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300"
            >{{ model }}</span>
          </div>
        </div>

        <!-- Usage Tip -->
        <div class="flex items-start gap-3 p-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 border border-blue-100 dark:border-blue-800">
          <svg class="w-5 h-5 text-blue-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="m11.25 11.25.041-.02a.75.75 0 0 1 1.063.852l-.708 2.836a.75.75 0 0 0 1.063.853l.041-.021M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9-3.75h.008v.008H12V8.25Z" />
          </svg>
          <p class="text-sm text-blue-700 dark:text-blue-300">
            {{ t('keys.useKeyModal.usageTip') }}
          </p>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button
          @click="emit('close')"
          class="btn btn-secondary"
        >
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useClipboard } from '@/composables/useClipboard'
import type { GroupPlatform } from '@/types'

interface Props {
  show: boolean
  apiKey: string
  baseUrl: string
  platform: GroupPlatform | null
  allowMessagesDispatch?: boolean
}

interface Emits {
  (e: 'close'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const { copyToClipboard: clipboardCopy } = useClipboard()

const copiedField = ref<string | null>(null)

// Reset on open
watch(() => props.show, (val) => {
  if (val) copiedField.value = null
})

const baseRoot = computed(() => {
  const url = props.baseUrl || window.location.origin
  return url.replace(/\/v1\/?$/, '').replace(/\/+$/, '')
})

// Prompt text based on platform
const promptText = computed(() => {
  const base = baseRoot.value
  switch (props.platform) {
    case 'openai':
      return `请帮我配置 Codex CLI 使用自定义 OpenAI 兼容 API。

## API 信息
- Base URL: ${base}/v1
- API Key: (见下方)
- 默认模型: gpt-5.5
- 可用模型: ${supportedModels.value.join(', ')}

## 配置步骤
### 1. 先备份再修改
备份 ~/.codex/config.toml 和 ~/.codex/auth.json

### 2. config.toml 配置
在 ~/.codex/config.toml 顶部添加:
model = "gpt-5.5"
model_provider = "OpenAI"
disable_response_storage = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${base}/v1"
wire_api = "responses"
requires_openai_auth = true

保留文件中已有的 projects/plugins 等配置不动

### 3. auth.json 配置
将 ~/.codex/auth.json 替换为:
{
  "OPENAI_API_KEY": "(见下方)"
}

### 4. 测试验证
运行 codex exec "say hello" 确认 API 连通`
    case 'gemini':
      return `请帮我配置 Gemini CLI 的 API。

Base URL: ${base}/v1beta
API Key: (见下方)

支持的模型：${supportedModels.value.join(', ')}`
    case 'antigravity':
      return `请帮我配置 AI 编程工具的 API（Antigravity 代理模式）。

Anthropic Base URL: ${base}/antigravity/v1
Gemini Base URL: ${base}/antigravity/v1beta
API Key: (见下方)

支持的模型：${supportedModels.value.join(', ')}`
    default:
      return `请帮我配置 AI 编程工具的 API。

Base URL: ${base}/v1
API Key: (见下方)

支持的模型：${supportedModels.value.join(', ')}`
  }
})

// Supported models list
const supportedModels = computed(() => {
  switch (props.platform) {
    case 'openai':
      return ['gpt-5.5', 'gpt-5.4', 'gpt-5.4-mini', 'gpt-5.3-codex', 'gpt-5.2', 'gpt-image-2']
    case 'gemini':
      return ['gemini-2.0-flash', 'gemini-2.5-flash', 'gemini-2.5-pro', 'gemini-3-flash-preview', 'gemini-3-pro-preview']
    case 'antigravity':
      return ['claude-opus-4-6-thinking', 'claude-sonnet-4-6', 'gemini-2.5-flash', 'gemini-2.5-pro', 'gemini-3-flash', 'gemini-3.1-pro-low', 'gemini-3.1-pro-high']
    default:
      return ['gpt-5.5', 'gpt-5.4', 'gpt-5.4-mini', 'gpt-5.3-codex', 'gpt-5.2', 'gpt-image-2']
  }
})

const copyText = async (text: string, field: string) => {
  const success = await clipboardCopy(text, t('keys.copied'))
  if (success) {
    copiedField.value = field
    setTimeout(() => {
      copiedField.value = null
    }, 2000)
  }
}
</script>
