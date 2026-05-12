<template>
  <AppLayout>
    <div class="mx-auto flex min-h-[calc(100vh-8rem)] max-w-7xl flex-col gap-4 lg:h-[calc(100vh-8rem)] lg:flex-row">
      <aside class="flex min-h-0 w-full shrink-0 flex-col rounded-2xl border border-gray-100 bg-white p-4 shadow-card dark:border-dark-700 dark:bg-dark-800/70 lg:w-80">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
            <Icon name="chat" size="lg" />
          </div>
          <div class="min-w-0">
            <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">在线聊天</h2>
            <p class="truncate text-xs text-gray-500 dark:text-dark-400">/v1/responses</p>
          </div>
        </div>

        <button class="btn btn-primary btn-sm mt-4 w-full" type="button" :disabled="isSending" @click="startNewConversation">
          <Icon name="plus" size="sm" />
          新对话
        </button>

        <div class="mt-3 grid grid-cols-2 rounded-xl bg-gray-100 p-1 dark:bg-dark-900">
          <button
            type="button"
            class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
            :class="sidebarTab === 'history'
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-100'"
            @click="sidebarTab = 'history'"
          >
            对话
          </button>
          <button
            type="button"
            class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
            :class="sidebarTab === 'settings'
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-100'"
            @click="sidebarTab = 'settings'"
          >
            设置
          </button>
        </div>

        <div class="mt-3 rounded-xl bg-gray-50 px-3 py-2 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-dark-400">
          对话仅保存在当前浏览器。
        </div>

        <div class="mt-4 min-h-0 flex-1 overflow-y-auto pr-1">
          <template v-if="sidebarTab === 'history'">
            <div class="mb-2 flex items-center justify-between gap-2">
              <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400">最近对话</h3>
              <span class="text-xs text-gray-400">{{ conversations.length }}/20</span>
            </div>

            <div v-if="conversations.length === 0" class="rounded-xl border border-dashed border-gray-200 px-3 py-4 text-center text-xs text-gray-400 dark:border-dark-700">
              暂无本地对话
            </div>

            <div v-else class="space-y-1.5">
              <button
                v-for="conversation in conversations"
                :key="conversation.id"
                type="button"
                class="w-full rounded-xl border px-3 py-2 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60"
                :disabled="isSending"
                :class="conversation.id === activeConversationId
                  ? 'border-primary-200 bg-primary-50 dark:border-primary-800/60 dark:bg-primary-900/20'
                  : 'border-gray-100 bg-gray-50 hover:bg-gray-100 dark:border-dark-700 dark:bg-dark-900/50 dark:hover:bg-dark-700'"
                @click="loadConversation(conversation.id)"
              >
                <div class="flex items-center justify-between gap-2">
                  <span class="min-w-0 truncate text-sm font-medium text-gray-800 dark:text-gray-100">{{ conversation.title }}</span>
                  <span class="shrink-0 text-[11px] text-gray-400">{{ formatConversationTime(conversation.updatedAt) }}</span>
                </div>
                <div class="mt-1 flex items-center justify-between gap-2 text-xs text-gray-400">
                  <span class="min-w-0 truncate font-mono">{{ conversation.model || '未选模型' }}</span>
                  <span class="shrink-0">{{ conversation.messages.length }} 条</span>
                </div>
              </button>
            </div>
            <button
              class="btn btn-secondary btn-sm mt-3 w-full"
              type="button"
              :disabled="conversations.length === 0 || isSending"
              @click="clearLocalRecords"
            >
              <Icon name="trash" size="sm" />
              清空本地记录
            </button>
          </template>

          <div v-else class="space-y-4">
            <div>
              <label class="input-label" for="chat-api-key">API Key</label>
              <Select
                id="chat-api-key"
                v-model="selectedApiKeyId"
                :options="apiKeyOptions"
                :disabled="apiKeysLoading || isSending"
                searchable
                :placeholder="apiKeysLoading ? '加载 Key 中' : '选择 API Key'"
                empty-text="暂无可用 Key"
                @change="handleApiKeyChange"
              >
                <template #selected="{ option }">
                  <span v-if="option" class="flex min-w-0 items-center gap-2">
                    <span class="truncate">{{ option.label }}</span>
                    <span class="shrink-0 font-mono text-xs text-gray-400">{{ option.masked }}</span>
                  </span>
                  <span v-else>{{ apiKeysLoading ? '加载 Key 中' : '选择 API Key' }}</span>
                </template>
                <template #option="{ option, selected }">
                  <div class="min-w-0 flex-1">
                    <div class="truncate text-sm font-medium">{{ option.label }}</div>
                    <div class="mt-0.5 flex min-w-0 items-center gap-2 text-xs text-gray-400">
                      <span class="truncate font-mono">{{ option.masked }}</span>
                      <span v-if="option.groupName" class="truncate">{{ option.groupName }}</span>
                    </div>
                  </div>
                  <Icon v-if="selected" name="check" size="sm" class="text-primary-500" :stroke-width="2" />
                </template>
              </Select>
              <p v-if="apiKeysError" class="input-error-text">{{ apiKeysError }}</p>
              <p v-else class="input-hint">只显示状态正常的 Key。</p>
            </div>

            <div>
              <label class="input-label" for="chat-model">模型</label>
              <Select
                id="chat-model"
                v-model="model"
                :options="modelOptions"
                :disabled="modelsLoading || isSending || !apiKey"
                searchable
                creatable
                creatable-prefix="使用模型"
                :placeholder="modelsLoading ? '加载模型中' : '选择模型'"
                empty-text="暂无模型"
                @change="handleModelChange"
              >
                <template #selected="{ option }">
                  <span class="font-mono">{{ option?.value || model || '选择模型' }}</span>
                </template>
                <template #option="{ option, selected }">
                  <div class="min-w-0 flex-1">
                    <div class="truncate font-mono text-sm">{{ option.value }}</div>
                    <div v-if="option.description" class="mt-0.5 truncate text-xs text-gray-400">{{ option.description }}</div>
                  </div>
                  <Icon v-if="selected" name="check" size="sm" class="text-primary-500" :stroke-width="2" />
                </template>
              </Select>
              <p v-if="modelsError" class="input-error-text">{{ modelsError }}</p>
              <p v-else class="input-hint">{{ modelHint }}</p>
            </div>

            <div>
              <label class="input-label" for="chat-system">系统提示词</label>
              <textarea
                id="chat-system"
                v-model="systemPrompt"
                class="input min-h-[96px] resize-none"
                spellcheck="false"
                placeholder="可选"
                :disabled="isSending"
              ></textarea>
            </div>

            <label class="flex cursor-pointer items-center justify-between gap-3 rounded-xl border border-gray-100 bg-gray-50 px-3 py-2.5 dark:border-dark-700 dark:bg-dark-900/50" :class="{ 'opacity-60': isSending }">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-200">记住选择</span>
              <input
                v-model="rememberSelection"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                :disabled="isSending"
              >
            </label>

            <label class="flex cursor-pointer items-center justify-between gap-3 rounded-xl border border-gray-100 bg-gray-50 px-3 py-2.5 dark:border-dark-700 dark:bg-dark-900/50" :class="{ 'opacity-60': isSending }">
              <span class="min-w-0">
                <span class="block text-sm font-medium text-gray-700 dark:text-gray-200">自动压缩上下文</span>
                <span class="mt-0.5 block truncate text-xs text-gray-400">12 轮或约 24k 字符后</span>
              </span>
              <input
                v-model="autoCompactEnabled"
                type="checkbox"
                class="h-4 w-4 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                :disabled="isSending"
              >
            </label>
            <p v-if="compactError" class="input-error-text">{{ compactError }}</p>
          </div>
        </div>
      </aside>

      <section class="flex min-h-[560px] flex-1 flex-col overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-card dark:border-dark-700 dark:bg-dark-800/70 lg:min-h-0">
        <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700">
          <div class="min-w-0">
            <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ currentConversationTitle }}</h3>
            <p class="truncate text-xs text-gray-500 dark:text-dark-400">{{ model || '未选模型' }} · {{ apiBaseLabel }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <span
              v-if="isContextSummaryActive || isCompacting"
              class="inline-flex items-center rounded-full bg-sky-100 px-2.5 py-1 text-xs font-medium text-sky-700 dark:bg-sky-900/30 dark:text-sky-300"
              :title="compactBadgeTitle"
            >
              {{ compactBadgeLabel }}
            </span>
            <span
              class="inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium"
              :class="statusBadgeClass"
              :title="statusText"
            >
              {{ statusLabel }}
            </span>
            <button class="btn btn-secondary btn-icon h-9 w-9 p-0" type="button" title="导出对话" :disabled="messages.length === 0 || isSending" @click="exportConversation">
              <Icon name="download" size="sm" />
            </button>
            <button class="btn btn-secondary btn-icon h-9 w-9 p-0" type="button" title="清空当前" :disabled="messages.length === 0 || isSending" @click="clearMessages">
              <Icon name="trash" size="sm" />
            </button>
            <button class="btn btn-secondary btn-icon h-9 w-9 p-0" type="button" title="停止输出" :disabled="!isSending" @click="stopGeneration">
              <Icon name="x" size="sm" />
            </button>
          </div>
        </div>

        <div ref="messagesPanel" class="min-h-0 flex-1 overflow-y-auto px-4 py-5">
          <div v-if="messages.length === 0" class="flex h-full min-h-[320px] items-center justify-center">
            <div class="text-center">
              <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
                <Icon name="sparkles" size="lg" />
              </div>
              <p class="text-sm font-medium text-gray-800 dark:text-gray-100">输入一句话开始测试模型</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">用量会计入对应 API Key</p>
            </div>
          </div>

          <div v-else class="space-y-4">
            <article
              v-for="message in messages"
              :key="message.id"
              class="flex"
              :class="message.role === 'user' ? 'justify-end' : 'justify-start'"
            >
              <div
                class="max-w-[88%] rounded-2xl px-4 py-3 text-sm leading-6 shadow-sm"
                :class="message.role === 'user'
                  ? 'bg-primary-600 text-white'
                  : message.error
                    ? 'border border-red-200 bg-red-50 text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300'
                    : 'border border-gray-100 bg-gray-50 text-gray-800 dark:border-dark-700 dark:bg-dark-900/70 dark:text-dark-100'"
              >
                <div class="whitespace-pre-wrap break-words">{{ message.content || (message.pending ? '正在思考...' : '') }}</div>
              </div>
            </article>
          </div>
        </div>

        <form class="border-t border-gray-100 p-3 dark:border-dark-700" @submit.prevent="sendMessage">
          <div class="flex items-end gap-3">
            <textarea
              v-model="draft"
              class="input min-h-[52px] flex-1 resize-none py-3"
              rows="2"
              spellcheck="false"
              placeholder="发送消息"
              :disabled="isSending"
              @keydown.enter.exact.prevent="sendMessage"
            ></textarea>
            <button class="btn btn-primary h-[52px] shrink-0 px-4" type="submit" :disabled="!canSend">
              <Icon name="arrowUp" size="sm" />
              <span class="hidden sm:inline">发送</span>
            </button>
          </div>
          <p v-if="errorMessage" class="mt-2 text-xs text-red-500">{{ errorMessage }}</p>
        </form>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { keysAPI } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { ApiKey } from '@/types'
import { maskApiKey } from '@/utils/maskApiKey'

type ChatRole = 'user' | 'assistant'

interface ChatMessage {
  id: string
  role: ChatRole
  content: string
  createdAt: number
  pending?: boolean
  error?: boolean
}

type ResponseInputMessage = {
  role: 'system' | ChatRole
  content: string
}

type ModelOption = {
  id: string
  label: string
  description?: string
}

interface SavedConversation {
  id: string
  title: string
  model: string
  apiKeyId: number | null
  systemPrompt: string
  contextSummary: string
  compactedThroughMessageId: string | null
  compactedAt: number | null
  autoCompactEnabled: boolean
  messages: ChatMessage[]
  createdAt: number
  updatedAt: number
}

const API_KEY_ID_STORAGE_KEY = 'ttapi.chat.apiKeyId'
const REMEMBER_SELECTION_STORAGE_KEY = 'ttapi.chat.rememberSelection'
const MODEL_STORAGE_KEY = 'ttapi.chat.model'
const SYSTEM_PROMPT_STORAGE_KEY = 'ttapi.chat.systemPrompt'
const CONVERSATIONS_STORAGE_KEY = 'ttapi.chat.conversations'
const ACTIVE_CONVERSATION_STORAGE_KEY = 'ttapi.chat.activeConversationId'
const MAX_SAVED_CONVERSATIONS = 20
const AUTOSAVE_DELAY_MS = 400
const COMPACT_ROUND_THRESHOLD = 12
const COMPACT_CHAR_THRESHOLD = 24_000
const RECENT_CONTEXT_MESSAGES = 14
const INCREMENTAL_COMPACT_CHAR_THRESHOLD = 8_000
const SUMMARY_TRANSCRIPT_CHAR_LIMIT = 48_000
const DEFAULT_MODEL = 'gpt-5.5'

const apiKey = ref('')
const selectedApiKeyId = ref<number | null>(null)
const rememberSelection = ref(false)
const model = ref(DEFAULT_MODEL)
const systemPrompt = ref('')
const draft = ref('')
const messages = ref<ChatMessage[]>([])
const conversations = ref<SavedConversation[]>([])
const activeConversationId = ref('')
const sidebarTab = ref<'history' | 'settings'>('settings')
const autoCompactEnabled = ref(true)
const contextSummary = ref('')
const compactedThroughMessageId = ref<string | null>(null)
const compactedAt = ref<number | null>(null)
const isCompacting = ref(false)
const useStaleSummaryForNextRequest = ref(false)
const compactError = ref('')
const isSending = ref(false)
const errorMessage = ref('')
const apiKeys = ref<ApiKey[]>([])
const apiKeysLoading = ref(false)
const apiKeysError = ref('')
const models = ref<ModelOption[]>([])
const modelsLoading = ref(false)
const modelsError = ref('')
const messagesPanel = ref<HTMLElement | null>(null)
const abortController = ref<AbortController | null>(null)
let modelAbortController: AbortController | null = null
let conversationSaveTimer: number | null = null
let hydratingConversation = false
let generationConversationId = ''

const apiBaseLabel = computed(() => `${window.location.origin}/v1/responses`)
const canSend = computed(() => Boolean(
  draft.value.trim() &&
  apiKey.value.trim() &&
  model.value.trim() &&
  !isSending.value &&
  !apiKeysLoading.value &&
  !modelsLoading.value
))
const activeConversation = computed(() => conversations.value.find((item) => item.id === activeConversationId.value) || null)
const currentConversationTitle = computed(() => activeConversation.value?.title || deriveConversationTitle(messages.value))
const selectedApiKey = computed(() => apiKeys.value.find((key) => key.id === selectedApiKeyId.value) || null)
const apiKeyOptions = computed(() => apiKeys.value
  .filter((key) => key.status === 'active')
  .map((key) => ({
    value: key.id,
    label: key.name || maskApiKey(key.key),
    masked: maskApiKey(key.key),
    groupName: key.group?.name || ''
  })))
const modelOptions = computed(() => models.value.map((item) => ({
  value: item.id,
  label: item.label,
  description: item.description || ''
})))
const modelHint = computed(() => {
  if (!apiKey.value.trim()) return '先选择 API Key 后加载模型。'
  if (modelsLoading.value) return '正在从 /v1/models 获取模型。'
  if (models.value.length > 0) return `已加载 ${models.value.length} 个模型，也可搜索后手动使用。`
  return '可搜索后手动使用模型名。'
})
const isContextSummaryActive = computed(() => Boolean(getActiveContextSummary()))
const compactBadgeLabel = computed(() => (isCompacting.value ? '压缩中' : '已压缩上下文'))
const compactBadgeTitle = computed(() => {
  if (isCompacting.value) return '正在把旧消息总结成上下文摘要'
  if (compactedAt.value) return `已压缩到 ${formatConversationTime(compactedAt.value)}，导出仍包含完整历史`
  return '后续请求会携带摘要和最近消息，导出仍包含完整历史'
})
const statusText = computed(() => {
  if (isCompacting.value) return '正在压缩旧上下文'
  if (isSending.value) return '正在等待模型返回'
  if (apiKeysLoading.value) return '正在加载你的 API Key'
  if (!apiKey.value.trim()) return '先选择一个可用的 API Key'
  if (!model.value.trim()) return '模型名称不能为空'
  if (messages.value.length === 0) return '可以开始对话'
  return `当前 ${messages.value.length} 条消息`
})
const statusLabel = computed(() => {
  if (isCompacting.value) return '压缩中'
  if (isSending.value) return '输出中'
  if (apiKeysLoading.value || modelsLoading.value) return '加载中'
  if (!apiKey.value.trim() || !model.value.trim()) return '待配置'
  return '就绪'
})
const statusBadgeClass = computed(() => {
  if (isCompacting.value) return 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
  if (isSending.value) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (!apiKey.value.trim() || !model.value.trim()) return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
})

onMounted(() => {
  rememberSelection.value = localStorage.getItem(REMEMBER_SELECTION_STORAGE_KEY) === 'true'
  model.value = localStorage.getItem(MODEL_STORAGE_KEY) || model.value
  systemPrompt.value = localStorage.getItem(SYSTEM_PROMPT_STORAGE_KEY) || ''
  loadSavedConversations()
  void loadApiKeys()
})

onBeforeUnmount(() => {
  abortController.value?.abort()
  modelAbortController?.abort()
  if (conversationSaveTimer !== null) {
    window.clearTimeout(conversationSaveTimer)
  }
  saveActiveConversationNow()
})

watch([selectedApiKeyId, rememberSelection], () => {
  localStorage.setItem(REMEMBER_SELECTION_STORAGE_KEY, rememberSelection.value ? 'true' : 'false')
  if (rememberSelection.value && selectedApiKeyId.value) {
    localStorage.setItem(API_KEY_ID_STORAGE_KEY, String(selectedApiKeyId.value))
  } else {
    localStorage.removeItem(API_KEY_ID_STORAGE_KEY)
  }
})

watch(model, (value) => {
  if (value.trim()) {
    localStorage.setItem(MODEL_STORAGE_KEY, value.trim())
  }
  scheduleConversationSave()
})

watch(systemPrompt, (value) => {
  localStorage.setItem(SYSTEM_PROMPT_STORAGE_KEY, value)
  scheduleConversationSave()
})

watch(autoCompactEnabled, () => {
  scheduleConversationSave()
})

watch(selectedApiKey, (key) => {
  apiKey.value = key?.key || ''
  if (rememberSelection.value && key) {
    localStorage.setItem(API_KEY_ID_STORAGE_KEY, String(key.id))
  }
  scheduleConversationSave()
  void loadModelsForSelectedKey()
})

async function loadApiKeys() {
  apiKeysLoading.value = true
  apiKeysError.value = ''
  try {
    const response = await keysAPI.list(1, 100, { status: 'active' })
    apiKeys.value = response.items.filter((key) => key.status === 'active')

    if (selectedApiKeyId.value && apiKeys.value.some((key) => key.id === selectedApiKeyId.value)) {
      return
    }

    const savedId = rememberSelection.value ? Number(localStorage.getItem(API_KEY_ID_STORAGE_KEY) || 0) : 0
    const preferred = apiKeys.value.find((key) => key.id === savedId) || apiKeys.value[0] || null
    selectedApiKeyId.value = preferred?.id || null
  } catch (error) {
    apiKeysError.value = readableError(error)
  } finally {
    apiKeysLoading.value = false
  }
}

async function loadModelsForSelectedKey() {
  modelAbortController?.abort()
  models.value = []
  modelsError.value = ''

  if (!apiKey.value.trim()) return

  const controller = new AbortController()
  modelAbortController = controller
  modelsLoading.value = true

  try {
    const response = await fetch('/v1/models', {
      headers: {
        Authorization: `Bearer ${apiKey.value.trim()}`
      },
      signal: controller.signal
    })

    if (!response.ok) {
      throw new Error(`${response.status} ${await readErrorMessage(response)}`)
    }

    const data = await response.json()
    const selectedModel = model.value.trim()
    models.value = withCurrentModelOption(extractModelOptions(data), selectedModel)
    if (!selectedModel && models.value[0]) {
      model.value = models.value[0].id
    }
  } catch (error) {
    const aborted = error instanceof DOMException && error.name === 'AbortError'
    if (!aborted) {
      modelsError.value = readableError(error)
    }
  } finally {
    if (modelAbortController === controller) {
      modelsLoading.value = false
      modelAbortController = null
    }
  }
}

function handleApiKeyChange(value: string | number | boolean | null) {
  selectedApiKeyId.value = typeof value === 'number' ? value : Number(value) || null
}

function handleModelChange(value: string | number | boolean | null) {
  model.value = String(value || '').trim()
}

function loadSavedConversations() {
  const saved = parseJson(localStorage.getItem(CONVERSATIONS_STORAGE_KEY) || '[]')
  const list = Array.isArray(saved) ? saved : []
  conversations.value = list
    .map(normalizeSavedConversation)
    .filter((item): item is SavedConversation => Boolean(item))
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, MAX_SAVED_CONVERSATIONS)

  const savedActiveId = localStorage.getItem(ACTIVE_CONVERSATION_STORAGE_KEY) || ''
  const active = conversations.value.find((item) => item.id === savedActiveId) || conversations.value[0]
  if (active) {
    hydrateConversation(active)
  }
}

function normalizeSavedConversation(value: any): SavedConversation | null {
  if (!value || typeof value !== 'object') return null
  const id = String(value.id || '').trim()
  if (!id) return null

  const rawMessages = Array.isArray(value.messages) ? value.messages : []
  const safeMessages: ChatMessage[] = rawMessages
    .map((message: any): ChatMessage | null => {
      const role = message?.role === 'user' || message?.role === 'assistant' ? message.role : null
      const content = typeof message?.content === 'string' ? message.content : ''
      if (!role || (!content && !message?.error)) return null
      return {
        id: String(message.id || createMessageId()),
        role,
        content,
        createdAt: Number(message.createdAt || Date.now()),
        error: Boolean(message.error),
        pending: false
      }
    })
    .filter((message: ChatMessage | null): message is ChatMessage => Boolean(message))

  const compactedMessageId = typeof value.compactedThroughMessageId === 'string' && value.compactedThroughMessageId
    ? value.compactedThroughMessageId
    : null
  const hasCompactedMessage = Boolean(compactedMessageId && safeMessages.some((message) => message.id === compactedMessageId))

  return {
    id,
    title: String(value.title || '').trim() || deriveConversationTitle(safeMessages),
    model: String(value.model || '').trim(),
    apiKeyId: typeof value.apiKeyId === 'number' ? value.apiKeyId : Number(value.apiKeyId) || null,
    systemPrompt: typeof value.systemPrompt === 'string' ? value.systemPrompt : '',
    contextSummary: hasCompactedMessage && typeof value.contextSummary === 'string' ? value.contextSummary : '',
    compactedThroughMessageId: hasCompactedMessage ? compactedMessageId : null,
    compactedAt: hasCompactedMessage ? Number(value.compactedAt || 0) || null : null,
    autoCompactEnabled: value.autoCompactEnabled !== false,
    messages: safeMessages,
    createdAt: Number(value.createdAt || Date.now()),
    updatedAt: Number(value.updatedAt || Date.now())
  }
}

function hydrateConversation(conversation: SavedConversation) {
  hydratingConversation = true
  activeConversationId.value = conversation.id
  selectedApiKeyId.value = conversation.apiKeyId
  model.value = conversation.model || model.value
  systemPrompt.value = conversation.systemPrompt
  contextSummary.value = conversation.contextSummary || ''
  compactedThroughMessageId.value = conversation.compactedThroughMessageId || null
  compactedAt.value = conversation.compactedAt || null
  autoCompactEnabled.value = conversation.autoCompactEnabled !== false
  compactError.value = ''
  messages.value = cloneMessages(conversation.messages)
  errorMessage.value = ''
  localStorage.setItem(ACTIVE_CONVERSATION_STORAGE_KEY, conversation.id)
  void nextTick(() => {
    hydratingConversation = false
    void scrollToBottom('auto')
  })
}

function loadConversation(id: string) {
  if (isSending.value) return
  const conversation = conversations.value.find((item) => item.id === id)
  if (!conversation) return
  saveActiveConversationNow()
  hydrateConversation(conversation)
}

function startNewConversation() {
  if (isSending.value) return
  saveActiveConversationNow()
  activeConversationId.value = createMessageId()
  localStorage.setItem(ACTIVE_CONVERSATION_STORAGE_KEY, activeConversationId.value)
  messages.value = []
  draft.value = ''
  errorMessage.value = ''
  resetCompactState()
  void scrollToBottom('auto')
}

function scheduleConversationSave() {
  if (hydratingConversation) return
  if (conversationSaveTimer !== null) {
    window.clearTimeout(conversationSaveTimer)
  }
  conversationSaveTimer = window.setTimeout(() => {
    conversationSaveTimer = null
    saveActiveConversationNow()
  }, AUTOSAVE_DELAY_MS)
}

function saveActiveConversationNow() {
  if (hydratingConversation) return
  if (conversationSaveTimer !== null) {
    window.clearTimeout(conversationSaveTimer)
    conversationSaveTimer = null
  }

  const safeMessages = cloneMessages(messages.value)
  const existing = activeConversation.value
  if (safeMessages.length === 0 && !existing) return

  const now = Date.now()
  const id = ensureActiveConversationId(existing?.id)

  const savedConversation: SavedConversation = {
    id,
    title: deriveConversationTitle(safeMessages),
    model: model.value.trim(),
    apiKeyId: selectedApiKeyId.value,
    systemPrompt: systemPrompt.value,
    contextSummary: contextSummary.value,
    compactedThroughMessageId: compactedThroughMessageId.value,
    compactedAt: compactedAt.value,
    autoCompactEnabled: autoCompactEnabled.value,
    messages: safeMessages,
    createdAt: existing?.createdAt || now,
    updatedAt: now
  }

  const next = [
    savedConversation,
    ...conversations.value.filter((item) => item.id !== id)
  ]
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, MAX_SAVED_CONVERSATIONS)

  conversations.value = next
  persistConversations()
  localStorage.setItem(ACTIVE_CONVERSATION_STORAGE_KEY, id)
}

function ensureActiveConversationId(fallbackId = ''): string {
  if (!activeConversationId.value) {
    activeConversationId.value = fallbackId || createMessageId()
    localStorage.setItem(ACTIVE_CONVERSATION_STORAGE_KEY, activeConversationId.value)
  }
  return activeConversationId.value
}

function persistConversations() {
  localStorage.setItem(CONVERSATIONS_STORAGE_KEY, JSON.stringify(conversations.value))
}

function clearLocalRecords() {
  if (isSending.value) return
  if (!window.confirm('确定清空当前浏览器里的全部对话记录吗？')) return
  conversations.value = []
  activeConversationId.value = createMessageId()
  messages.value = []
  draft.value = ''
  errorMessage.value = ''
  resetCompactState()
  localStorage.removeItem(CONVERSATIONS_STORAGE_KEY)
  localStorage.removeItem(ACTIVE_CONVERSATION_STORAGE_KEY)
}

function exportConversation() {
  const conversation = buildConversationSnapshot()
  const content = JSON.stringify(conversation, null, 2)
  const blob = new Blob([content], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  const safeTitle = conversation.title.replace(/[^\w\u4e00-\u9fa5.-]+/g, '-').slice(0, 48) || 'chat'
  link.href = url
  link.download = `${safeTitle}-${formatDateForFile(new Date(conversation.updatedAt))}.json`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

function buildConversationSnapshot(): SavedConversation {
  const existing = activeConversation.value
  const now = Date.now()
  const safeMessages = cloneMessages(messages.value)
  return {
    id: activeConversationId.value || existing?.id || createMessageId(),
    title: deriveConversationTitle(safeMessages),
    model: model.value.trim(),
    apiKeyId: selectedApiKeyId.value,
    systemPrompt: systemPrompt.value,
    contextSummary: contextSummary.value,
    compactedThroughMessageId: compactedThroughMessageId.value,
    compactedAt: compactedAt.value,
    autoCompactEnabled: autoCompactEnabled.value,
    messages: safeMessages,
    createdAt: existing?.createdAt || now,
    updatedAt: now
  }
}

async function sendMessage() {
  if (!canSend.value) return

  const conversationId = ensureActiveConversationId()
  const content = draft.value.trim()
  draft.value = ''
  errorMessage.value = ''
  generationConversationId = conversationId

  const userMessage: ChatMessage = {
    id: createMessageId(),
    role: 'user',
    content,
    createdAt: Date.now()
  }
  const assistantMessage: ChatMessage = {
    id: createMessageId(),
    role: 'assistant',
    content: '',
    createdAt: Date.now(),
    pending: true
  }

  messages.value.push(userMessage, assistantMessage)
  scheduleConversationSave()
  isSending.value = true
  abortController.value = new AbortController()
  await scrollToBottom('auto')

  try {
    await maybeCompactContext()
    await streamResponse(assistantMessage)
    if (!assistantMessage.content.trim()) {
      assistantMessage.content = '模型没有返回文本。'
    }
  } catch (error) {
    const aborted = error instanceof DOMException && error.name === 'AbortError'
    assistantMessage.error = !aborted
    assistantMessage.content = aborted ? (assistantMessage.content || '已停止。') : readableError(error)
    errorMessage.value = aborted ? '' : assistantMessage.content
  } finally {
    assistantMessage.pending = false
    isSending.value = false
    abortController.value = null
    if (generationConversationId === activeConversationId.value) {
      saveActiveConversationNow()
    }
    useStaleSummaryForNextRequest.value = false
    generationConversationId = ''
    await scrollToBottom('smooth')
  }
}

async function streamResponse(assistantMessage: ChatMessage) {
  const response = await fetch('/v1/responses', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiKey.value.trim()}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      model: model.value.trim(),
      input: buildResponseInput(),
      stream: true
    }),
    signal: abortController.value?.signal
  })

  if (!response.ok) {
    throw new Error(`${response.status} ${await readErrorMessage(response)}`)
  }

  if (!response.body) {
    const data = await response.json()
    assistantMessage.content = extractResponseText(data)
    return
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { value, done } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split(/\r?\n/)
    buffer = lines.pop() || ''

    for (const rawLine of lines) {
      const line = rawLine.trim()
      if (!line || line.startsWith(':') || line.startsWith('event:')) continue
      if (!line.startsWith('data:')) continue

      applyStreamPayload(line.slice(5).trim(), assistantMessage)
    }
  }

  if (buffer.trim().startsWith('data:')) {
    applyStreamPayload(buffer.trim().slice(5).trim(), assistantMessage)
  }
}

function buildResponseInput(): ResponseInputMessage[] {
  const input: ResponseInputMessage[] = []
  const selectedModel = model.value.trim()
  if (selectedModel) {
    input.push({
      role: 'system',
      content: buildModelIdentityInstruction(selectedModel)
    })
  }

  const prompt = systemPrompt.value.trim()
  if (prompt) {
    input.push({ role: 'system', content: prompt })
  }

  const summary = getActiveContextSummary()
  if (summary) {
    input.push({
      role: 'system',
      content: `Conversation summary so far:\n${summary}`
    })
  }

  for (const message of getRequestContextMessages()) {
    input.push({
      role: message.role,
      content: message.content
    })
  }

  return input
}

async function maybeCompactContext() {
  compactError.value = ''

  if (!autoCompactEnabled.value || isCompacting.value || !apiKey.value.trim() || !model.value.trim()) return

  const eligibleMessages = getEligibleContextMessages()
  if (!shouldCompactContext(eligibleMessages)) return

  const oldMessages = getCompactionCandidateMessages(eligibleMessages)
  const lastOldMessage = oldMessages[oldMessages.length - 1]
  if (!lastOldMessage || lastOldMessage.id === compactedThroughMessageId.value) return

  const startIndex = getIncrementalCompactStartIndex(eligibleMessages)
  const lastOldIndex = eligibleMessages.findIndex((message) => message.id === lastOldMessage.id)
  if (lastOldIndex < startIndex) return

  const incrementalMessages = eligibleMessages.slice(startIndex, lastOldIndex + 1)
  if (!incrementalMessages.length) return

  if (isContextSummaryActive.value && !shouldCompactPendingContext(eligibleMessages)) return

  isCompacting.value = true
  try {
    const summary = await requestConversationSummary(incrementalMessages)
    if (!summary) throw new Error('模型没有返回摘要')

    contextSummary.value = summary
    compactedThroughMessageId.value = lastOldMessage.id
    compactedAt.value = Date.now()
    saveActiveConversationNow()
  } catch (error) {
    const aborted = error instanceof DOMException && error.name === 'AbortError'
    if (!aborted) {
      compactError.value = `上下文压缩失败：${readableError(error)}`
      useStaleSummaryForNextRequest.value = Boolean(getActiveContextSummary())
    }
  } finally {
    isCompacting.value = false
  }
}

async function requestConversationSummary(newMessages: ChatMessage[]): Promise<string> {
  const response = await fetch('/v1/responses', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiKey.value.trim()}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      model: model.value.trim(),
      input: buildCompactInput(newMessages),
      max_output_tokens: 1200,
      stream: false
    }),
    signal: abortController.value?.signal
  })

  if (!response.ok) {
    throw new Error(`${response.status} ${await readErrorMessage(response)}`)
  }

  const data = await response.json()
  return extractResponseText(data).trim()
}

function buildCompactInput(newMessages: ChatMessage[]): ResponseInputMessage[] {
  const existingSummary = contextSummary.value.trim() || '暂无'
  const transcript = trimTranscript(formatMessagesForSummary(newMessages))
  return [
    {
      role: 'system',
      content: [
        '你是对话上下文压缩器，只输出一段中文 conversation summary。',
        '保留用户目标、关键约束、已经确认的设置、重要事实、代码/接口决策、未完成待办。',
        '不要回答用户的新问题，不要加入寒暄。'
      ].join('\n')
    },
    {
      role: 'user',
      content: [
        `现有摘要：\n${existingSummary}`,
        '',
        '把下面的新消息合并进摘要：',
        transcript
      ].join('\n')
    }
  ]
}

function getRequestContextMessages(): ChatMessage[] {
  const eligibleMessages = getEligibleContextMessages()
  if (!isContextSummaryActive.value) return eligibleMessages

  const compactedIndex = getCompactedMessageIndex(eligibleMessages)

  if (compactedIndex < 0) return eligibleMessages
  return eligibleMessages.slice(compactedIndex + 1)
}

function getActiveContextSummary(): string {
  const summary = contextSummary.value.trim()
  if ((!autoCompactEnabled.value && !useStaleSummaryForNextRequest.value) || !summary || !compactedThroughMessageId.value) return ''
  return getCompactedMessageIndex(getEligibleContextMessages()) >= 0 ? summary : ''
}

function getCompactedMessageIndex(items: ChatMessage[]): number {
  return compactedThroughMessageId.value
    ? items.findIndex((message) => message.id === compactedThroughMessageId.value)
    : -1
}

function getEligibleContextMessages(): ChatMessage[] {
  return messages.value.filter((message) => !message.pending && !message.error && message.content.trim())
}

function getCompactionCandidateMessages(items: ChatMessage[]): ChatMessage[] {
  if (items.length <= 2) return []

  const defaultSplitIndex = getStableCompactionEndIndex(items, items.length - RECENT_CONTEXT_MESSAGES)
  if (defaultSplitIndex > 0) {
    return items.slice(0, defaultSplitIndex)
  }

  if (countMessageChars(items) < COMPACT_CHAR_THRESHOLD) return []

  const charSplitIndex = getStableCompactionEndIndex(items, items.length - 1)
  return charSplitIndex > 0 ? items.slice(0, charSplitIndex) : []
}

function getStableCompactionEndIndex(items: ChatMessage[], preferredEndIndex: number): number {
  const maxEndIndex = Math.min(Math.max(preferredEndIndex, 0), items.length - 1)
  for (let endIndex = maxEndIndex; endIndex > 0; endIndex -= 1) {
    if (items[endIndex - 1]?.role === 'assistant') return endIndex
  }
  return 0
}

function shouldCompactContext(items: ChatMessage[]): boolean {
  return countUserRounds(items) >= COMPACT_ROUND_THRESHOLD || countMessageChars(items) >= COMPACT_CHAR_THRESHOLD
}

function shouldCompactPendingContext(items: ChatMessage[]): boolean {
  const pendingMessages = items.slice(getIncrementalCompactStartIndex(items))
  return pendingMessages.length > RECENT_CONTEXT_MESSAGES || countMessageChars(pendingMessages) >= INCREMENTAL_COMPACT_CHAR_THRESHOLD
}

function getIncrementalCompactStartIndex(items: ChatMessage[]): number {
  if (!isContextSummaryActive.value || !compactedThroughMessageId.value) return 0
  const compactedIndex = getCompactedMessageIndex(items)
  return compactedIndex >= 0 ? compactedIndex + 1 : 0
}

function formatMessagesForSummary(items: ChatMessage[]): string {
  return items
    .map((message) => {
      const roleLabel = message.role === 'user' ? '用户' : '助手'
      return `${roleLabel}：${message.content.trim()}`
    })
    .join('\n\n')
}

function trimTranscript(value: string): string {
  if (value.length <= SUMMARY_TRANSCRIPT_CHAR_LIMIT) return value
  return value.slice(-SUMMARY_TRANSCRIPT_CHAR_LIMIT)
}

function countUserRounds(items: ChatMessage[]): number {
  return items.filter((message) => message.role === 'user').length
}

function countMessageChars(items: ChatMessage[]): number {
  return items.reduce((total, message) => total + message.content.length, 0)
}

function applyStreamPayload(payload: string, assistantMessage: ChatMessage) {
  if (!payload || payload === '[DONE]') return

  const data = parseJson(payload)
  if (!data) return

  if (data.type === 'error' || data.error) {
    throw new Error(data.error?.message || data.message || '模型请求失败')
  }

  const delta = extractDeltaText(data)
  if (delta) {
    assistantMessage.content += delta
    scheduleConversationSave()
    void scrollToBottom('auto')
    return
  }

  if (data.type === 'response.completed') {
    const completedText = extractResponseText(data.response || data)
    if (completedText) {
      assistantMessage.content = completedText
      scheduleConversationSave()
      void scrollToBottom('auto')
    }
  }
}

function extractDeltaText(data: any): string {
  if (typeof data.delta === 'string') return data.delta
  if (typeof data.text === 'string' && data.type?.includes?.('delta')) return data.text
  if (typeof data.choices?.[0]?.delta?.content === 'string') return data.choices[0].delta.content
  if (typeof data.choices?.[0]?.text === 'string') return data.choices[0].text
  return ''
}

function extractResponseText(data: any): string {
  if (!data) return ''
  if (typeof data.output_text === 'string') return data.output_text
  if (typeof data.message?.content === 'string') return data.message.content
  if (typeof data.choices?.[0]?.message?.content === 'string') return data.choices[0].message.content

  const output = Array.isArray(data.output) ? data.output : []
  const chunks: string[] = []
  for (const item of output) {
    const content = Array.isArray(item?.content) ? item.content : []
    for (const part of content) {
      if (typeof part?.text === 'string') chunks.push(part.text)
      if (typeof part?.value === 'string') chunks.push(part.value)
    }
  }
  return chunks.join('')
}

function buildModelIdentityInstruction(selectedModel: string): string {
  return [
    `当前 TTAPI 在线聊天页本次请求的模型参数是 "${selectedModel}"。`,
    '如果用户询问你是什么模型、模型版本或正在运行的模型，请以这个模型参数作答。',
    '不要根据训练记忆、默认自我介绍或历史版本猜测成其它模型。'
  ].join('\n')
}

function extractModelOptions(data: any): ModelOption[] {
  const rawModels = Array.isArray(data?.data) ? data.data : []
  const seen = new Set<string>()
  const items: ModelOption[] = []

  for (const item of rawModels) {
    const id = String(item?.id || item?.name || '').trim()
    if (!id || seen.has(id)) continue
    seen.add(id)
    items.push({
      id,
      label: String(item?.display_name || item?.displayName || id),
      description: item?.owned_by ? String(item.owned_by) : undefined
    })
  }

  return items.sort((a, b) => a.id.localeCompare(b.id))
}

function withCurrentModelOption(items: ModelOption[], currentModel: string): ModelOption[] {
  if (!currentModel || items.some((item) => item.id === currentModel)) return items
  return [
    {
      id: currentModel,
      label: currentModel,
      description: '当前选择'
    },
    ...items
  ]
}

async function readErrorMessage(response: Response): Promise<string> {
  const text = await response.text()
  if (!text) return response.statusText || '请求失败'

  const data = parseJson(text)
  if (data?.error?.message) return data.error.message
  if (data?.message) return data.message
  return text.slice(0, 500)
}

function parseJson(value: string): any | null {
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

function readableError(error: unknown): string {
  if (error instanceof Error && error.message) return error.message
  if (typeof error === 'object' && error !== null && 'message' in error) {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }
  return '请求失败，请稍后再试。'
}

async function scrollToBottom(behavior: 'auto' | 'smooth') {
  await nextTick()
  const panel = messagesPanel.value
  if (!panel) return
  panel.scrollTo({ top: panel.scrollHeight, behavior })
}

function stopGeneration() {
  abortController.value?.abort()
}

function clearMessages() {
  if (isSending.value) return
  messages.value = []
  errorMessage.value = ''
  resetCompactState()
  saveActiveConversationNow()
}

function resetCompactState() {
  autoCompactEnabled.value = true
  contextSummary.value = ''
  compactedThroughMessageId.value = null
  compactedAt.value = null
  compactError.value = ''
  isCompacting.value = false
  useStaleSummaryForNextRequest.value = false
}

function createMessageId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function cloneMessages(items: ChatMessage[]): ChatMessage[] {
  return items
    .map((message) => ({
      id: message.id,
      role: message.role,
      content: message.content,
      createdAt: message.createdAt,
      error: Boolean(message.error),
      pending: false
    }))
    .filter((message) => Boolean(message.content.trim() || message.error))
}

function deriveConversationTitle(items: ChatMessage[]): string {
  const firstUserMessage = items.find((message) => message.role === 'user' && message.content.trim())
  const raw = firstUserMessage?.content.trim() || '新对话'
  return raw.replace(/\s+/g, ' ').slice(0, 32)
}

function formatConversationTime(timestamp: number): string {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  const now = new Date()
  const sameDay = date.toDateString() === now.toDateString()
  if (sameDay) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }
  return date.toLocaleDateString([], { month: '2-digit', day: '2-digit' })
}

function formatDateForFile(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}-${pad(date.getHours())}${pad(date.getMinutes())}`
}
</script>
