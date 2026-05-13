<template>
  <AppLayout>
    <div data-testid="chat-page-shell" class="w-full">
      <div v-if="!featureEnabled" class="card p-6">
        <div class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20">
          <p class="text-sm text-amber-700 dark:text-amber-300">
            {{ t('chatCompletion.disabled') }}
          </p>
        </div>
      </div>

      <template v-else>
        <section
          data-testid="chat-workbench"
          class="flex h-[calc(100vh-8rem)] min-h-[680px] flex-col overflow-hidden rounded-[28px] bg-white shadow-sm ring-1 ring-gray-100/80 dark:bg-dark-950 dark:ring-white/5"
        >
          <div class="flex min-h-0 flex-1 flex-col lg:flex-row">
            <aside
              data-testid="chat-session-sidebar"
              class="flex max-h-80 shrink-0 flex-col bg-gray-50/80 dark:bg-dark-900/80 lg:max-h-none lg:w-80"
            >
              <div class="space-y-3 px-4 py-4">
                <div class="flex items-center justify-between gap-3">
                  <h1 class="text-base font-semibold text-gray-950 dark:text-white">
                    {{ t('chatCompletion.history') }}
                  </h1>
                  <button
                    type="button"
                    data-testid="chat-new-button"
                    class="inline-flex h-9 items-center gap-2 rounded-2xl bg-gray-950 px-3 text-sm font-medium text-white shadow-sm transition hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-white dark:text-dark-950 dark:hover:bg-gray-200"
                    :disabled="streaming"
                    @click="startNewConversation"
                  >
                    <Icon name="plus" size="sm" />
                    {{ t('chatCompletion.newChat') }}
                  </button>
                </div>
              </div>

              <div class="min-h-0 flex-1 space-y-1 overflow-y-auto px-2 pb-4">
                <p v-if="loadingSessions" class="px-3 py-3 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('chatCompletion.loadingHistory') }}
                </p>
                <p v-else-if="sessions.length === 0" class="px-3 py-3 text-xs leading-5 text-gray-500 dark:text-gray-400">
                  {{ t('chatCompletion.noHistory') }}
                </p>
                <button
                  v-for="session in sessions"
                  :key="session.id"
                  type="button"
                  data-testid="chat-session-item"
                  class="group flex w-full items-start rounded-2xl px-3 py-2.5 text-left transition"
                  :class="activeSessionId === session.id
                    ? 'bg-white text-gray-950 shadow-sm dark:bg-dark-800 dark:text-white'
                    : 'text-gray-600 hover:bg-white/70 hover:text-gray-950 dark:text-gray-300 dark:hover:bg-dark-800/70 dark:hover:text-white'"
                  :disabled="streaming"
                  @click="selectSession(session.id)"
                >
                  <span class="min-w-0">
                    <span class="block truncate text-sm font-medium">{{ session.title }}</span>
                    <span class="mt-0.5 block truncate text-xs opacity-70">{{ session.model }}</span>
                  </span>
                </button>
              </div>

              <div class="px-3 pb-4">
                <button
                  type="button"
                  data-testid="chat-settings-toggle"
                  class="flex h-11 w-full items-center justify-between rounded-2xl px-3 text-sm text-gray-600 transition hover:bg-white/80 hover:text-gray-950 dark:text-gray-300 dark:hover:bg-dark-800 dark:hover:text-white"
                  @click="settingsOpen = !settingsOpen"
                >
                  <span class="inline-flex items-center gap-2">
                    <Icon name="cog" size="sm" />
                    {{ t('chatCompletion.chatSettings') }}
                  </span>
                  <Icon :name="settingsOpen ? 'chevronDown' : 'chevronUp'" size="sm" />
                </button>

                <div
                  v-if="settingsOpen"
                  data-testid="chat-settings-panel"
                  class="mt-2 space-y-3 rounded-[22px] bg-white/80 p-3 shadow-sm ring-1 ring-gray-100/80 dark:bg-dark-800/80 dark:ring-white/5"
                >
                  <div class="space-y-1.5">
                    <span class="input-label text-xs">
                      {{ t('chatCompletion.baseUrl') }}
                    </span>
                    <div
                      data-testid="chat-base-url"
                      class="min-h-10 break-all rounded-2xl bg-gray-50 px-4 py-2.5 font-mono text-xs leading-5 text-gray-600 ring-1 ring-gray-100 dark:bg-dark-900 dark:text-gray-300 dark:ring-white/5"
                    >
                      {{ selectedBaseUrl }}
                    </div>
                  </div>

                  <div class="space-y-1.5">
                    <label class="input-label text-xs" for="chat-api-key">
                      {{ t('chatCompletion.apiKey') }}
                    </label>
                    <select
                      id="chat-api-key"
                      v-model="selectedKeyId"
                      class="input h-10 rounded-2xl text-sm shadow-none"
                      :disabled="streaming || loadingKeys"
                    >
                      <option value="">{{ t('chatCompletion.selectApiKey') }}</option>
                      <option v-for="key in activeApiKeys" :key="key.id" :value="String(key.id)">
                        {{ key.name || maskKey(key.key) }}
                      </option>
                    </select>
                  </div>

                  <div class="space-y-1.5">
                    <label class="input-label text-xs" for="chat-model">
                      {{ t('chatCompletion.model') }}
                    </label>
                    <select
                      id="chat-model"
                      v-model="selectedModel"
                      class="input h-10 rounded-2xl text-sm shadow-none"
                      :disabled="streaming || loadingChannels || chatModels.length === 0"
                    >
                      <option value="">{{ modelPlaceholder }}</option>
                      <option v-for="model in chatModels" :key="model.id" :value="model.id">
                        {{ model.display_name || model.id }}
                      </option>
                    </select>
                  </div>

                  <button
                    type="button"
                    data-testid="chat-clear-button"
                    class="h-9 rounded-2xl px-3 text-xs font-medium text-gray-500 transition hover:bg-gray-100 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-red-300"
                    :disabled="streaming || messages.length === 0"
                    @click="clearConversation"
                  >
                    {{ t('chatCompletion.clear') }}
                  </button>

                  <p v-if="loadingKeys || loadingChannels" class="text-xs text-gray-500 dark:text-gray-400">
                    {{ loadingKeys ? t('chatCompletion.loadingKeys') : t('chatCompletion.loadingModels') }}
                  </p>
                  <p v-else-if="activeApiKeys.length === 0" class="text-xs text-amber-600 dark:text-amber-300">
                    {{ t('chatCompletion.noApiKeys') }}
                  </p>
                  <p v-else-if="selectedApiKey && chatModels.length === 0" class="text-xs text-amber-600 dark:text-amber-300">
                    {{ t('chatCompletion.noModels') }}
                  </p>
                </div>
              </div>
            </aside>

            <main class="flex min-h-0 flex-1 flex-col bg-gray-100/70 dark:bg-dark-950">
              <div
                ref="messageListRef"
                data-testid="chat-message-list"
                class="min-h-0 flex-1 overflow-y-auto px-4 pb-36 pt-8 sm:px-6"
              >
                <div v-if="loadingMessages" class="flex h-full min-h-[360px] items-center justify-center text-sm text-gray-500 dark:text-gray-400">
                  {{ t('chatCompletion.loadingMessages') }}
                </div>

                <div v-else-if="messages.length === 0" class="flex h-full min-h-[420px] items-center justify-center px-3">
                  <div class="w-full max-w-xl text-center">
                    <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-white/80 text-sm font-semibold text-gray-700 shadow-sm ring-1 ring-gray-100 dark:bg-dark-800 dark:text-gray-200 dark:ring-white/5">
                      AI
                    </div>
                    <h2 class="mt-5 text-xl font-semibold text-gray-950 dark:text-white">
                      {{ t('chatCompletion.emptyTitle') }}
                    </h2>
                    <p class="mx-auto mt-2 max-w-md text-sm leading-6 text-gray-500 dark:text-gray-400">
                      {{ t('chatCompletion.empty') }}
                    </p>
                    <div class="mt-7 grid gap-3 sm:grid-cols-3" :aria-label="t('chatCompletion.suggestions')">
                      <button
                        v-for="(prompt, index) in suggestedPrompts"
                        :key="prompt"
                        type="button"
                        :data-testid="`chat-suggestion-${index}`"
                        class="min-h-20 rounded-[22px] bg-white/65 px-4 py-3 text-left text-sm font-medium leading-5 text-gray-700 shadow-sm ring-1 ring-gray-100/70 transition hover:-translate-y-0.5 hover:bg-white hover:text-gray-950 hover:shadow-md disabled:cursor-not-allowed disabled:opacity-60 dark:bg-dark-800/70 dark:text-gray-200 dark:ring-white/5 dark:hover:bg-dark-800 dark:hover:text-white"
                        :disabled="streaming"
                        @click="applySuggestion(prompt)"
                      >
                        {{ prompt }}
                      </button>
                    </div>
                  </div>
                </div>

                <div v-else class="mx-auto w-full max-w-3xl min-w-0 space-y-5">
                  <div
                    v-for="message in messages"
                    :key="message.localId"
                    class="flex min-w-0"
                    :class="message.role === 'user' ? 'justify-end' : 'justify-start'"
                  >
                    <article
                      class="min-w-0 max-w-[min(720px,92%)] rounded-[22px] px-4 py-3 text-sm leading-6"
                      :class="message.role === 'user'
                        ? 'rounded-br-lg bg-gray-950 text-white shadow-sm dark:bg-white dark:text-dark-950'
                        : 'w-full rounded-bl-lg bg-white/75 text-gray-800 shadow-sm ring-1 ring-gray-100/70 dark:bg-dark-900/80 dark:text-gray-100 dark:ring-white/5'"
                    >
                      <div v-if="message.role === 'assistant'">
                        <MarkdownMessage v-if="message.content" :content="message.content" />
                        <div
                          v-else-if="isGeneratingMessage(message)"
                          data-testid="chat-generating-indicator"
                          class="inline-flex items-center gap-2 text-gray-500 dark:text-gray-400"
                        >
                          <span class="flex items-center gap-1" aria-hidden="true">
                            <span class="h-1.5 w-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.2s]"></span>
                            <span class="h-1.5 w-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.1s]"></span>
                            <span class="h-1.5 w-1.5 animate-bounce rounded-full bg-current"></span>
                          </span>
                          <span>{{ t('chatCompletion.generatingReply') }}</span>
                        </div>
                        <div
                          v-else
                          data-testid="chat-empty-reply"
                          class="text-gray-400 dark:text-gray-500"
                        >
                          {{ t('chatCompletion.emptyReply') }}
                        </div>
                      </div>
                      <div v-else class="whitespace-pre-wrap break-words">
                        {{ message.content }}
                      </div>

                      <div v-if="message.content" class="mt-2 flex justify-start">
                        <button
                          type="button"
                          data-testid="chat-copy-message-button"
                          class="inline-flex h-8 w-8 items-center justify-center rounded-xl transition"
                          :class="message.role === 'user'
                            ? 'text-white/60 hover:bg-white/10 hover:text-white dark:text-dark-950/50 dark:hover:bg-dark-950/10 dark:hover:text-dark-950'
                            : 'text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-white/10 dark:hover:text-gray-100'"
                          :aria-label="t('chatCompletion.copy')"
                          :title="t('chatCompletion.copy')"
                          @click="copyMessage(message.content)"
                        >
                          <Icon name="copy" size="sm" />
                        </button>
                      </div>

                      <div
                        v-if="message.role === 'assistant' && shouldShowAssistantMeta(message)"
                        class="mt-2 flex flex-wrap items-center gap-2 border-t pt-2 text-[11px]"
                        :class="'border-red-100 text-red-600 dark:border-red-900/50 dark:text-red-300'"
                      >
                        <span>{{ message.error_message || t('chatCompletion.error') }}</span>
                      </div>
                    </article>
                  </div>
                </div>
              </div>

              <div v-if="errorMessage" class="mx-auto mb-3 w-full max-w-3xl rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 shadow-sm ring-1 ring-red-100 dark:bg-red-950/40 dark:text-red-300 dark:ring-red-900/60">
                {{ errorMessage }}
              </div>

              <form
                data-testid="chat-composer"
                class="pointer-events-none sticky bottom-0 -mt-28 bg-gradient-to-t from-gray-100 via-gray-100/95 to-transparent px-4 pb-4 pt-10 dark:from-dark-950 dark:via-dark-950/95 sm:px-6"
                @submit.prevent="sendMessage"
              >
                <div
                  data-testid="chat-composer-shell"
                  class="pointer-events-auto mx-auto flex max-w-3xl items-end gap-2 rounded-[28px] bg-white/95 p-2 shadow-[0_18px_50px_-24px_rgba(15,23,42,0.55)] ring-1 ring-gray-200/80 backdrop-blur dark:bg-dark-900/95 dark:ring-white/10"
                >
                  <textarea
                    id="chat-message"
                    v-model="draft"
                    data-testid="chat-message-input"
                    class="min-h-[84px] flex-1 resize-none rounded-[22px] border-0 bg-transparent px-4 py-3 text-base leading-7 text-gray-950 placeholder:text-gray-500 focus:outline-none focus:ring-0 disabled:cursor-not-allowed disabled:opacity-60 dark:text-gray-100 dark:placeholder:text-gray-400"
                    :placeholder="composerPlaceholder"
                    :disabled="streaming || !selectedApiKey || !selectedModel"
                    @keydown="handleComposerKeydown"
                  />
                  <div class="flex shrink-0 items-center gap-1 pb-1 pr-1">
                    <button
                      type="button"
                      data-testid="chat-regenerate-button"
                      class="inline-flex h-10 w-10 items-center justify-center rounded-2xl text-gray-500 transition hover:bg-gray-100 hover:text-gray-950 disabled:cursor-not-allowed disabled:opacity-35 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white"
                      :aria-label="t('chatCompletion.regenerate')"
                      :title="t('chatCompletion.regenerate')"
                      :disabled="streaming || !canRegenerate"
                      @click="regenerateLast"
                    >
                      <Icon name="refresh" size="sm" />
                    </button>
                    <button
                      v-if="streaming"
                      type="button"
                      data-testid="chat-stop-button"
                      class="inline-flex h-10 w-10 items-center justify-center rounded-2xl bg-gray-950 text-white shadow-sm transition hover:bg-gray-800 dark:bg-white dark:text-dark-950 dark:hover:bg-gray-200"
                      :aria-label="t('chatCompletion.stop')"
                      :title="t('chatCompletion.stop')"
                      @click="stopStreaming"
                    >
                      <Icon name="x" size="sm" />
                    </button>
                    <button
                      v-else
                      type="submit"
                      data-testid="chat-send-button"
                      class="inline-flex h-10 w-10 items-center justify-center rounded-2xl bg-gray-950 text-white shadow-sm transition hover:bg-gray-800 disabled:cursor-not-allowed disabled:bg-gray-200 disabled:text-gray-400 dark:bg-white dark:text-dark-950 dark:hover:bg-gray-200 dark:disabled:bg-dark-700 dark:disabled:text-gray-500"
                      :aria-label="t('chatCompletion.send')"
                      :title="t('chatCompletion.send')"
                      :disabled="!canSend"
                    >
                      <Icon name="arrowUp" size="sm" />
                    </button>
                  </div>
                </div>
              </form>
            </main>
          </div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import MarkdownMessage from '@/components/chat/MarkdownMessage.vue'
import Icon from '@/components/icons/Icon.vue'
import userChannelsAPI, { type UserAvailableChannel } from '@/api/channels'
import { keysAPI } from '@/api/keys'
import { streamChatCompletion, type ChatMessage, type ChatModel } from '@/api/chat'
import { BILLING_MODE_IMAGE } from '@/constants/channel'
import {
  createChatMessage,
  createChatSession,
  deleteChatSession,
  getChatSessionMessages,
  listChatSessions,
  updateChatMessage,
  type ChatMessageRecord,
  type ChatSessionRecord,
} from '@/api/chatSessions'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import type { ApiKey } from '@/types'

const storageKey = 'sub2api.chat.selected_key_id'
const modelStorageKey = 'sub2api.chat.selected_model'

type LocalChatMessage = ChatMessage & {
  localId: string
  id?: number
  status?: string
  model?: string | null
  duration_ms?: number | null
  actual_cost?: number | null
  error_message?: string | null
}

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const apiKeys = ref<ApiKey[]>([])
const selectedKeyId = ref('')
const selectedModel = ref('')
const channels = ref<UserAvailableChannel[]>([])
const chatModels = ref<ChatModel[]>([])
const sessions = ref<ChatSessionRecord[]>([])
const activeSessionId = ref<number | null>(null)
const draft = ref('')
const messages = ref<LocalChatMessage[]>([])
const messageListRef = ref<HTMLElement | null>(null)
const streaming = ref(false)
const loadingKeys = ref(false)
const loadingChannels = ref(false)
const loadingSessions = ref(false)
const loadingMessages = ref(false)
const settingsOpen = ref(false)
const errorMessage = ref('')
let abortController: AbortController | null = null

const featureEnabled = computed(() => appStore.cachedPublicSettings?.chat_completion_enabled === true)

const activeApiKeys = computed(() => apiKeys.value.filter((key) => {
  if (key.status !== 'active') return false
  if (!key.group_id) return false
  if (!key.expires_at) return true
  return new Date(key.expires_at).getTime() > Date.now()
}))

const selectedApiKey = computed(() => activeApiKeys.value.find((key) => String(key.id) === selectedKeyId.value) || null)
const selectedChatModel = computed(() => chatModels.value.find((item) => item.id === selectedModel.value) || null)

const modelPlaceholder = computed(() => {
  if (!selectedApiKey.value) return t('chatCompletion.selectKeyFirst')
  if (loadingChannels.value) return t('chatCompletion.loadingModels')
  if (chatModels.value.length === 0) return t('chatCompletion.noModels')
  return t('chatCompletion.selectModel')
})

const composerPlaceholder = computed(() => {
  if (!selectedApiKey.value) return t('chatCompletion.selectKeyFirst')
  if (!selectedModel.value) return t('chatCompletion.selectModelFirst')
  return t('chatCompletion.message')
})

const selectedBaseUrl = computed(() => {
  return appStore.cachedPublicSettings?.api_base_url?.trim()
    || appStore.apiBaseUrl?.trim()
    || `${window.location.origin}/v1`
})

const suggestedPrompts = computed(() => [
  t('chatCompletion.suggestion1'),
  t('chatCompletion.suggestion2'),
  t('chatCompletion.suggestion3'),
])

const canSend = computed(() =>
  featureEnabled.value &&
  !streaming.value &&
  Boolean(selectedApiKey.value?.key) &&
  Boolean(selectedModel.value) &&
  Boolean(draft.value.trim()),
)

const canRegenerate = computed(() => Boolean(activeSessionId.value && lastUserMessageIndex() >= 0))

watch(selectedKeyId, (value) => {
  if (value) localStorage.setItem(storageKey, value)
  refreshModelsForSelectedKey()
})

watch(selectedModel, (value) => {
  if (value) localStorage.setItem(modelStorageKey, value)
})

onMounted(() => {
  if (featureEnabled.value) {
    void initializeChat()
  }
})

watch(featureEnabled, (enabled) => {
  if (enabled && apiKeys.value.length === 0) {
    void initializeChat()
  }
})

onBeforeUnmount(() => {
  stopStreaming()
})

async function initializeChat() {
  await Promise.all([loadApiKeys(), loadSessions()])
}

async function loadApiKeys() {
  loadingKeys.value = true
  try {
    if (channels.value.length === 0) {
      await loadChannels()
    }
    const result = await keysAPI.list(1, 100, { status: 'active' })
    apiKeys.value = result.items || []
    const saved = localStorage.getItem(storageKey)
    selectedKeyId.value = activeApiKeys.value.some((key) => String(key.id) === saved)
      ? saved || ''
      : String(activeApiKeys.value[0]?.id || '')
  } catch (error) {
    handleError(error)
  } finally {
    loadingKeys.value = false
  }
}

async function loadChannels() {
  loadingChannels.value = true
  try {
    channels.value = await userChannelsAPI.getAvailable()
  } catch (error) {
    handleError(error)
  } finally {
    loadingChannels.value = false
  }
}

async function loadSessions() {
  loadingSessions.value = true
  try {
    sessions.value = await listChatSessions()
  } catch (error) {
    handleError(error)
  } finally {
    loadingSessions.value = false
  }
}

function refreshModelsForSelectedKey() {
  const previousModel = selectedModel.value || localStorage.getItem(modelStorageKey) || ''
  chatModels.value = []
  selectedModel.value = ''
  errorMessage.value = ''
  const groupID = selectedApiKey.value?.group_id
  if (!groupID) return

  const modelsByName = new Map<string, ChatModel>()
  for (const channel of channels.value) {
    for (const section of channel.platforms) {
      if (!section.groups.some((group) => group.id === groupID)) continue
      for (const model of section.supported_models) {
        if (!model.name || modelsByName.has(model.name)) continue
        if (isImageModel(model)) continue
        modelsByName.set(model.name, {
          id: model.name,
          display_name: model.name,
          type: model.platform,
          base_url: section.base_url || defaultBaseUrlForPlatform(model.platform),
        })
      }
    }
  }

  const models = [...modelsByName.values()].sort((a, b) => a.id.localeCompare(b.id))
  chatModels.value = models
  selectedModel.value = models.some((model) => model.id === previousModel)
    ? previousModel
    : models[0]?.id || ''
}

type SupportedChatModel = UserAvailableChannel['platforms'][number]['supported_models'][number]

function isImageModel(model: SupportedChatModel): boolean {
  if (model.pricing?.billing_mode === BILLING_MODE_IMAGE) return true
  const name = model.name.toLowerCase()
  return [
    'gpt-image',
    'dall-e',
    'dalle',
    'imagen',
    'flux',
    'midjourney',
    'stable-diffusion',
    'sdxl',
  ].some((marker) => name.includes(marker))
}

function defaultBaseUrlForPlatform(platform?: string): string {
  switch (platform) {
    case 'anthropic':
      return 'https://api.anthropic.com'
    case 'gemini':
      return 'https://generativelanguage.googleapis.com'
    case 'antigravity':
      return 'https://cloudcode-pa.googleapis.com'
    case 'openai':
      return 'https://api.openai.com'
    default:
      return t('chatCompletion.baseUrlUnavailable')
  }
}

async function selectSession(sessionId: number) {
  if (streaming.value) return
  const session = sessions.value.find((item) => item.id === sessionId)
  if (!session) return

  activeSessionId.value = sessionId
  selectedKeyId.value = String(session.api_key_id)
  await Promise.resolve()
  selectedModel.value = chatModels.value.some((model) => model.id === session.model)
    ? session.model
    : selectedModel.value

  loadingMessages.value = true
  errorMessage.value = ''
  try {
    const records = await getChatSessionMessages(sessionId)
    messages.value = records.map(toLocalMessage)
    scrollMessagesToBottom('auto')
  } catch (error) {
    handleError(error)
  } finally {
    loadingMessages.value = false
  }
}

async function sendMessage() {
  if (!canSend.value || !selectedApiKey.value) return

  const prompt = draft.value.trim()
  draft.value = ''
  errorMessage.value = ''

  try {
    const session = await ensureActiveSession(prompt)
    const userRecord = await createChatMessage(session.id, {
      role: 'user',
      content: prompt,
      status: 'completed',
      model: selectedModel.value,
    })
    const userMessage = toLocalMessage(userRecord)
    messages.value.push(userMessage)
    scrollMessagesToBottom()
    await streamAssistantResponse(session.id, messages.value)
  } catch (error) {
    if ((error as { name?: string }).name !== 'AbortError') {
      handleError(error)
    }
  }
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey) return
  if (event.isComposing || event.keyCode === 229) return
  event.preventDefault()
  void sendMessage()
}

async function ensureActiveSession(prompt: string): Promise<ChatSessionRecord> {
  const existing = sessions.value.find((session) => session.id === activeSessionId.value)
  if (existing) return existing
  if (!selectedApiKey.value) throw new Error(t('chatCompletion.selectKeyFirst'))

  const session = await createChatSession({
    api_key_id: Number(selectedApiKey.value.id),
    title: createTitle(prompt),
    model: selectedModel.value,
  })
  activeSessionId.value = session.id
  sessions.value = [session, ...sessions.value.filter((item) => item.id !== session.id)]
  return session
}

async function streamAssistantResponse(sessionId: number, contextMessages: LocalChatMessage[]) {
  if (!selectedApiKey.value) return

  const startedAt = Date.now()
  const completionMessages = contextMessages.map(toCompletionMessage)
  const assistantRecord = await createChatMessage(sessionId, {
    role: 'assistant',
    content: '',
    status: 'streaming',
    model: selectedModel.value,
  })
  const assistantMessage = toLocalMessage(assistantRecord)
  messages.value.push(assistantMessage)
  scrollMessagesToBottom()
  streaming.value = true
  abortController = new AbortController()
  const assistantLocalID = assistantMessage.localId

  try {
    await streamChatCompletion({
      apiKey: selectedApiKey.value.key,
      model: selectedModel.value,
      platform: selectedChatModel.value?.type,
      messages: completionMessages,
      promptCacheKey: buildPromptCacheKey(sessionId),
      signal: abortController.signal,
      onDelta(delta) {
        appendAssistantDelta(assistantLocalID, delta)
        assistantMessage.content += delta
        scrollMessagesToBottom()
      },
    })
    const updated = await updateChatMessage(sessionId, assistantRecord.id, {
      content: assistantMessage.content,
      status: 'completed',
      model: selectedModel.value,
      duration_ms: Date.now() - startedAt,
    })
    Object.assign(assistantMessage, toLocalMessage(updated))
    scrollMessagesToBottom()
    await refreshSessionList(sessionId)
  } catch (error) {
    if ((error as { name?: string }).name === 'AbortError') {
      const updated = await updateChatMessage(sessionId, assistantRecord.id, {
        content: assistantMessage.content,
        status: 'stopped',
        duration_ms: Date.now() - startedAt,
      })
      Object.assign(assistantMessage, toLocalMessage(updated))
      scrollMessagesToBottom()
      return
    }

    const message = (error as { message?: string })?.message || t('chatCompletion.error')
    assistantMessage.status = 'failed'
    assistantMessage.error_message = message
    await updateChatMessage(sessionId, assistantRecord.id, {
      content: assistantMessage.content,
      status: 'failed',
      duration_ms: Date.now() - startedAt,
      error_message: message,
    })
    scrollMessagesToBottom()
    throw error
  } finally {
    streaming.value = false
    abortController = null
  }
}

function stopStreaming() {
  if (abortController) {
    abortController.abort()
    abortController = null
  }
}

async function regenerateLast() {
  if (streaming.value || !activeSessionId.value) return
  const userIndex = lastUserMessageIndex()
  if (userIndex < 0) return
  const context = messages.value.slice(0, userIndex + 1)
  messages.value = context
  errorMessage.value = ''
  try {
    await streamAssistantResponse(activeSessionId.value, context)
  } catch (error) {
    if ((error as { name?: string }).name !== 'AbortError') {
      handleError(error)
    }
  }
}

async function startNewConversation() {
  if (streaming.value) return
  activeSessionId.value = null
  messages.value = []
  errorMessage.value = ''
}

async function clearConversation() {
  if (!activeSessionId.value) {
    messages.value = []
    errorMessage.value = ''
    return
  }
  try {
    await deleteChatSession(activeSessionId.value)
    sessions.value = sessions.value.filter((session) => session.id !== activeSessionId.value)
    activeSessionId.value = null
    messages.value = []
    errorMessage.value = ''
  } catch (error) {
    handleError(error)
  }
}

function applySuggestion(prompt: string) {
  if (streaming.value) return
  draft.value = prompt
}

function copyMessage(content: string) {
  void copyToClipboard(content, t('chatCompletion.copied'))
}

function scrollMessagesToBottom(behavior: ScrollBehavior = 'smooth') {
  void nextTick(() => {
    const el = messageListRef.value
    if (!el) return
    if (typeof el.scrollTo === 'function') {
      el.scrollTo({
        top: el.scrollHeight,
        behavior,
      })
      return
    }
    el.scrollTop = el.scrollHeight
  })
}

function appendAssistantDelta(localId: string, delta: string) {
  const index = messages.value.findIndex((message) => message.localId === localId)
  if (index === -1) return
  const current = messages.value[index]
  messages.value[index] = {
    ...current,
    content: `${current.content}${delta}`,
  }
}

function handleError(error: unknown) {
  const message = (error as { message?: string })?.message || t('chatCompletion.error')
  errorMessage.value = message
  appStore.showError(message)
}

async function refreshSessionList(sessionId: number) {
  try {
    const updated = await listChatSessions()
    sessions.value = updated
    activeSessionId.value = sessionId
  } catch {
    const active = sessions.value.find((session) => session.id === sessionId)
    if (active) {
      active.updated_at = new Date().toISOString()
      sessions.value = [active, ...sessions.value.filter((session) => session.id !== sessionId)]
    }
  }
}

function toLocalMessage(record: ChatMessageRecord): LocalChatMessage {
  return {
    localId: String(record.id),
    id: record.id,
    role: record.role,
    content: record.content || '',
    status: record.status,
    model: record.model,
    duration_ms: record.duration_ms,
    actual_cost: record.actual_cost,
    error_message: record.error_message,
  }
}

function toCompletionMessage(message: LocalChatMessage): ChatMessage {
  return {
    role: message.role,
    content: message.content,
  }
}

function lastUserMessageIndex(): number {
  for (let i = messages.value.length - 1; i >= 0; i -= 1) {
    if (messages.value[i].role === 'user') return i
  }
  return -1
}

function createTitle(prompt: string): string {
  const title = prompt.replace(/\s+/g, ' ').trim()
  return title.length > 40 ? `${title.slice(0, 40)}...` : title || t('chatCompletion.newChat')
}

function buildPromptCacheKey(sessionId: number): string {
  return `chat-session-${sessionId}`
}

function isGeneratingMessage(message: LocalChatMessage): boolean {
  return streaming.value && message.status === 'streaming' && !message.content
}

function shouldShowAssistantMeta(message: LocalChatMessage): boolean {
  return Boolean(message.status === 'failed' || message.error_message)
}

function maskKey(key: string): string {
  if (key.length <= 12) return key
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}
</script>
