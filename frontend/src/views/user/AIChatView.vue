<template>
  <AppLayout>
    <div class="mx-auto flex h-[calc(100vh-8rem)] w-full max-w-7xl flex-col overflow-hidden">
      <div v-if="!loadingKeys && activeKeys.length === 0" class="flex flex-1 flex-col items-center justify-center rounded-lg border border-dashed border-gray-300 bg-white p-8 text-center dark:border-dark-600 dark:bg-dark-800">
        <Icon name="key" size="xl" class="text-gray-400 dark:text-gray-500" />
        <h2 class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">{{ t('textChat.noKeysTitle') }}</h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('textChat.noKeysDescription') }}</p>
        <router-link to="/keys" class="btn btn-primary mt-5 inline-flex">
          <Icon name="plus" size="md" class="mr-2" />
          {{ t('textChat.createKey') }}
        </router-link>
      </div>

      <div v-else class="grid min-h-0 flex-1 grid-cols-[150px_minmax(0,1fr)] overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800 sm:grid-cols-[190px_minmax(0,1fr)] lg:grid-cols-[240px_minmax(0,1fr)]">
        <aside class="flex min-h-0 flex-col border-r border-gray-200 dark:border-dark-700">
          <div class="border-b border-gray-200 px-3 py-2 dark:border-dark-700">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('textChat.history') }}</h2>
          </div>
          <div class="min-h-0 flex-1 overflow-y-auto p-2">
            <div
              v-for="conversation in sortedConversations"
              :key="conversation.id"
              class="group mb-1 flex items-center gap-1 rounded-md px-2 py-1.5 transition"
              :class="
                conversation.id === currentConversationId
                  ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                  : 'text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-700'
              "
            >
              <button type="button" class="min-w-0 flex-1 text-left" @click="loadConversation(conversation.id)">
                <div class="truncate text-sm font-medium">{{ conversation.title }}</div>
                <div class="mt-1 truncate text-xs text-gray-400">{{ formatConversationTime(conversation.updatedAt) }}</div>
              </button>
              <button
                type="button"
                class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded text-gray-400 opacity-70 transition hover:bg-red-50 hover:text-red-600 group-hover:opacity-100 dark:hover:bg-red-900/20 dark:hover:text-red-300"
                :title="t('textChat.deleteConversation')"
                :aria-label="t('textChat.deleteConversation')"
                @click.stop="deleteConversation(conversation.id)"
              >
                <Icon name="trash" size="xs" />
              </button>
            </div>
            <div v-if="sortedConversations.length === 0" class="flex h-full items-center justify-center px-4 text-center text-sm text-gray-400">
              {{ t('textChat.noHistory') }}
            </div>
          </div>
          <div class="space-y-2 border-t border-gray-200 p-2 dark:border-dark-700">
            <button class="btn btn-primary w-full justify-center px-2 py-2 text-sm" :disabled="sending" @click="startNewConversation">
              <Icon name="plus" size="sm" class="mr-2" />
              {{ t('textChat.newConversation') }}
            </button>
            <button class="btn btn-secondary w-full justify-center px-2 py-2 text-sm" :disabled="sending || messages.length === 0" @click="clearContext">
              <Icon name="trash" size="sm" class="mr-2" />
              {{ t('textChat.clearContext') }}
            </button>
          </div>
        </aside>

        <section class="flex min-h-0 flex-col overflow-hidden">
          <div v-if="modelError" class="border-b border-gray-200 px-3 py-2 text-sm text-red-600 dark:border-dark-700 dark:text-red-400">
            {{ modelError }}
          </div>
          <div ref="messagesEl" class="min-h-0 flex-1 space-y-3 overflow-y-auto p-3">
            <div v-if="messages.length === 0" class="flex h-full flex-col items-center justify-center text-center text-gray-500 dark:text-gray-400">
              <Icon name="chat" size="lg" class="mb-2" />
              <p class="text-sm">{{ t('textChat.empty') }}</p>
              <div class="mt-4 grid w-full max-w-2xl gap-2 sm:grid-cols-2">
                <button
                  v-for="suggestion in suggestions"
                  :key="suggestion"
                  type="button"
                  class="rounded-md border border-gray-200 px-3 py-2 text-left text-xs text-gray-700 transition hover:border-primary-300 hover:bg-primary-50 dark:border-dark-600 dark:text-gray-300 dark:hover:border-primary-600 dark:hover:bg-primary-900/20 sm:text-sm"
                  @click="draft = suggestion"
                >
                  {{ suggestion }}
                </button>
              </div>
            </div>

            <div v-for="(message, index) in messages" :key="index" class="flex" :class="message.role === 'user' ? 'justify-end' : 'justify-start'">
              <div
                class="max-w-[88%] rounded-lg px-3 py-2.5 text-sm leading-6 shadow-sm"
                :class="
                  message.role === 'user'
                    ? 'bg-primary-600 text-white'
                    : 'border border-gray-200 bg-gray-50 text-gray-900 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-100'
                "
              >
                <div class="mb-1 flex items-center justify-between gap-3">
                  <div class="flex min-w-0 items-center gap-2 text-xs font-medium opacity-75">
                    <span>{{ message.role === 'user' ? t('textChat.you') : t('textChat.assistant') }}</span>
                    <span v-if="message.createdAt" class="font-normal opacity-80">{{ formatMessageTime(message.createdAt) }}</span>
                  </div>
                  <button
                    v-if="message.role === 'assistant' && message.content"
                    type="button"
                    class="rounded p-1 opacity-70 transition hover:bg-black/5 hover:opacity-100 dark:hover:bg-white/10"
                    :title="t('textChat.copy')"
                    @click="copyMessage(message.content)"
                  >
                    <Icon name="copy" size="xs" />
                  </button>
                </div>
                <div v-if="message.attachments?.length" class="mb-2 flex flex-wrap gap-2">
                  <div
                    v-for="attachment in message.attachments"
                    :key="attachment.id"
                    class="flex max-w-full items-center gap-2 rounded-md px-2 py-1 text-xs"
                    :class="
                      message.role === 'user'
                        ? 'bg-white/15 text-white'
                        : 'border border-gray-200 bg-white text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200'
                    "
                  >
                    <img
                      v-if="attachment.kind === 'image' && attachment.dataUrl"
                      :src="attachment.dataUrl"
                      :alt="attachment.name"
                      class="h-8 w-8 flex-shrink-0 rounded object-cover"
                    />
                    <Icon v-else name="document" size="sm" class="flex-shrink-0 opacity-80" />
                    <span class="min-w-0">
                      <span class="block truncate font-medium">{{ attachment.name }}</span>
                      <span class="block opacity-75">{{ formatAttachmentSize(attachment.size) }}</span>
                    </span>
                  </div>
                </div>
                <div class="break-words">
                  <template v-if="message.content">
                    <div
                      v-if="message.role === 'assistant' && streamingMessageIndex !== index"
                      class="ai-markdown-body"
                      v-html="renderMarkdown(message.content)"
                    ></div>
                    <p v-else class="whitespace-pre-wrap">{{ message.content }}</p>
                    <span v-if="streamingMessageIndex === index" class="animate-pulse">|</span>
                  </template>
                  <span v-else-if="streamingMessageIndex === index" class="inline-flex items-center gap-1">
                    <span>{{ t('textChat.thinkingShort') }}</span>
                    <span class="inline-flex w-5 items-end gap-0.5">
                      <span class="animate-bounce" style="animation-delay: 0ms">.</span>
                      <span class="animate-bounce" style="animation-delay: 120ms">.</span>
                      <span class="animate-bounce" style="animation-delay: 240ms">.</span>
                    </span>
                  </span>
                </div>
              </div>
            </div>
          </div>

          <div class="border-t border-gray-200 p-2 dark:border-dark-700">
            <p v-if="submitError" class="mb-2 text-sm text-red-600 dark:text-red-400">{{ submitError }}</p>
            <div
              class="rounded-lg border border-gray-200 bg-gray-50 p-2 transition dark:border-dark-700 dark:bg-dark-900"
              :class="
                draggingFiles
                  ? 'border-primary-400 bg-primary-50/50 ring-2 ring-primary-500/20 dark:border-primary-500 dark:bg-primary-900/10'
                  : ''
              "
              @dragenter.prevent="handleComposerDragEnter"
              @dragover.prevent="handleComposerDragOver"
              @dragleave.prevent="handleComposerDragLeave"
              @drop.prevent="handleComposerDrop"
              @paste="handleComposerPaste"
            >
              <input ref="fileInputEl" type="file" class="hidden" multiple @change="handleFileInputChange" />
              <div v-if="pendingAttachments.length > 0" class="mb-2 flex flex-wrap gap-2">
                <div
                  v-for="attachment in pendingAttachments"
                  :key="attachment.id"
                  class="group flex max-w-full items-center gap-2 rounded-md border border-gray-200 bg-white px-2 py-1 text-xs text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
                >
                  <img
                    v-if="attachment.kind === 'image' && attachment.dataUrl"
                    :src="attachment.dataUrl"
                    :alt="attachment.name"
                    class="h-8 w-8 flex-shrink-0 rounded object-cover"
                  />
                  <Icon v-else name="document" size="sm" class="flex-shrink-0 text-gray-400" />
                  <span class="min-w-0">
                    <span class="block truncate font-medium">{{ attachment.name }}</span>
                    <span class="block text-gray-400">{{ formatAttachmentSize(attachment.size) }}</span>
                  </span>
                  <button
                    type="button"
                    class="ml-1 flex h-5 w-5 flex-shrink-0 items-center justify-center rounded text-gray-400 transition hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-300"
                    :title="t('textChat.removeAttachment')"
                    :aria-label="t('textChat.removeAttachment')"
                    @click="removePendingAttachment(attachment.id)"
                  >
                    <Icon name="x" size="xs" />
                  </button>
                </div>
              </div>
              <textarea
                v-model="draft"
                rows="2"
                class="w-full resize-none border-0 bg-transparent px-2 py-1.5 text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none dark:text-white"
                :placeholder="t('textChat.inputPlaceholder')"
                @keydown.enter.exact.prevent="send"
              />
              <div class="flex items-center justify-between gap-2">
                <div class="flex min-w-0 items-center gap-2">
                  <button
                    type="button"
                    class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-500 transition hover:border-primary-300 hover:bg-gray-50 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-primary-600 dark:hover:bg-dark-700"
                    :title="t('textChat.attachFile')"
                    :aria-label="t('textChat.attachFile')"
                    :disabled="sending || readingFiles || pendingAttachments.length >= MAX_ATTACHMENTS"
                    @click="openFilePicker"
                  >
                    <Icon name="upload" size="sm" />
                  </button>
                  <span v-if="readingFiles" class="truncate text-xs text-gray-400">{{ t('textChat.readingFiles') }}</span>
                </div>
                <div class="flex min-w-0 items-center justify-end gap-2">
                  <div ref="modelPickerEl" class="relative min-w-0">
                    <button
                    type="button"
                    data-testid="ai-chat-model-picker"
                    class="flex h-10 w-[178px] max-w-[46vw] items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 text-left text-xs transition hover:border-primary-300 hover:bg-gray-50 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-600 dark:hover:bg-dark-700 sm:w-[210px]"
                    :disabled="loadingModels || modelOptions.length === 0"
                    @click="toggleModelPicker"
                  >
                    <span class="min-w-0 flex-1">
                      <span class="block truncate font-medium text-gray-900 dark:text-white">{{ selectedModelEntry?.modelName || (loadingModels ? t('textChat.loadingModels') : t('textChat.selectModel')) }}</span>
                      <span v-if="selectedModelEntry" class="block truncate text-[11px] text-gray-500 dark:text-gray-400">{{ selectedModelEntry.keyName }}</span>
                    </span>
                    <Icon :name="modelPickerOpen ? 'chevronUp' : 'chevronDown'" size="xs" class="flex-shrink-0 text-gray-400" />
                    </button>

                    <div
                    v-if="modelPickerOpen"
                    data-testid="ai-chat-model-menu"
                    class="absolute bottom-full right-0 z-30 mb-2 w-[min(20rem,calc(100vw-2rem))] overflow-hidden rounded-lg border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"
                  >
                    <div class="border-b border-gray-100 p-2 dark:border-dark-700">
                      <input
                        v-model="modelSearchQuery"
                        data-testid="ai-chat-model-search"
                        type="text"
                        class="w-full rounded-md border border-gray-200 bg-gray-50 px-2.5 py-2 text-xs text-gray-900 placeholder:text-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                        :placeholder="t('textChat.modelSearchPlaceholder')"
                      />
                    </div>
                    <div class="max-h-72 overflow-y-auto p-1">
                      <template v-if="filteredModelOptionGroups.length > 0">
                        <div
                          v-for="(group, groupIndex) in filteredModelOptionGroups"
                          :key="group.keyId"
                          class="py-1"
                          :class="groupIndex < filteredModelOptionGroups.length - 1 ? 'border-b border-gray-100 dark:border-dark-700' : ''"
                        >
                          <div class="px-2 py-1 text-[11px] font-medium text-gray-400 dark:text-gray-500">
                            {{ group.keyName }}
                          </div>
                          <button
                            v-for="option in group.options"
                            :key="option.value"
                            type="button"
                            class="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left transition"
                            :class="
                              option.value === selectedModelOption
                                ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                                : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-dark-700'
                            "
                            @click="selectModelOption(option.value)"
                          >
                            <span class="min-w-0 flex-1">
                              <span class="block truncate text-sm font-medium">{{ option.modelName }}</span>
                            </span>
                            <Icon v-if="option.value === selectedModelOption" name="check" size="sm" class="flex-shrink-0" />
                          </button>
                        </div>
                      </template>
                      <div v-else class="px-3 py-6 text-center text-xs text-gray-400">
                        {{ t('textChat.noMatchedModels') }}
                      </div>
                    </div>
                    </div>
                  </div>
                  <button class="btn btn-primary flex-shrink-0" :disabled="!canSend || sending" @click="send">
                    <Icon name="arrowRight" size="sm" class="mr-2" />
                    {{ sending ? t('textChat.sending') : t('textChat.send') }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import keysAPI from '@/api/keys'
import textChatAPI, { type TextChatContentPart, type TextChatMessage, type TextChatModel } from '@/api/textChat'
import type { ApiKey } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  createdAt: string
  attachments?: ChatAttachment[]
}

interface ChatAttachment {
  id: string
  name: string
  type: string
  size: number
  kind: 'text' | 'image' | 'file'
  text?: string
  dataUrl?: string
  truncated?: boolean
}

interface ConversationArchive {
  id: string
  title: string
  messages: ChatMessage[]
  apiKeyId: number
  apiKeyName: string
  model: string
  modelName: string
  createdAt: string
  updatedAt: string
}

interface TextChatModelOption {
  value: string
  keyId: number
  keyName: string
  apiKey: string
  modelId: string
  modelName: string
  label: string
}

interface TextChatModelOptionGroup {
  keyId: number
  keyName: string
  options: TextChatModelOption[]
}

const CHAT_HISTORY_STORAGE_KEY = 'sub2api.aiChat.history'
const MAX_ARCHIVED_CONVERSATIONS = 50
const MAX_ATTACHMENTS = 6
const MAX_ATTACHMENT_SIZE_BYTES = 10 * 1024 * 1024
const MAX_TEXT_ATTACHMENT_CHARS = 180000
const TYPEWRITER_DELAY_MS = 6

marked.setOptions({
  breaks: true,
  gfm: true
})

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const keys = ref<ApiKey[]>([])
const models = ref<TextChatModel[]>([])
const modelOptions = ref<TextChatModelOption[]>([])
const selectedKeyId = ref(0)
const selectedModel = ref('')
const selectedModelOption = ref('')
const modelPickerOpen = ref(false)
const modelSearchQuery = ref('')
const draft = ref('')
const messages = ref<ChatMessage[]>([])
const pendingAttachments = ref<ChatAttachment[]>([])
const conversations = ref<ConversationArchive[]>([])
const currentConversationId = ref(createConversationId())
const historyStorageKey = ref(getCurrentHistoryStorageKey())
const streamingMessageIndex = ref<number | null>(null)
const loadingKeys = ref(false)
const loadingModels = ref(false)
const readingFiles = ref(false)
const draggingFiles = ref(false)
const sending = ref(false)
const modelError = ref('')
const submitError = ref('')
const messagesEl = ref<HTMLElement | null>(null)
const modelPickerEl = ref<HTMLElement | null>(null)
const fileInputEl = ref<HTMLInputElement | null>(null)

const suggestions = computed(() => [
  t('textChat.suggestionRewrite'),
  t('textChat.suggestionSummary'),
  t('textChat.suggestionCaption'),
  t('textChat.suggestionExplain')
])
const activeKeys = computed(() => keys.value.filter((key) => key.status === 'active'))
const selectedModelEntry = computed(() => modelOptions.value.find((option) => option.value === selectedModelOption.value))
const selectedKey = computed(() => activeKeys.value.find((key) => key.id === (selectedModelEntry.value?.keyId || selectedKeyId.value)))
const selectedKeyName = computed(() => selectedModelEntry.value?.keyName || selectedKey.value?.name || t('textChat.apiKeyFallback'))
const selectedModelName = computed(() => selectedModelEntry.value?.modelName || selectedModel.value)
const filteredModelOptions = computed(() => {
  const query = modelSearchQuery.value.trim().toLowerCase()
  if (!query) return modelOptions.value
  return modelOptions.value.filter((option) => {
    return [option.keyName, option.modelName, option.modelId, option.label].some((value) => value.toLowerCase().includes(query))
  })
})
const filteredModelOptionGroups = computed<TextChatModelOptionGroup[]>(() => {
  const groups = new Map<number, TextChatModelOptionGroup>()
  filteredModelOptions.value.forEach((option) => {
    const group = groups.get(option.keyId)
    if (group) {
      group.options.push(option)
      return
    }
    groups.set(option.keyId, {
      keyId: option.keyId,
      keyName: option.keyName,
      options: [option]
    })
  })
  return Array.from(groups.values())
})
const sortedConversations = computed(() =>
  [...conversations.value].sort((left, right) => new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime())
)
const canSend = computed(() => Boolean(selectedModelEntry.value && (draft.value.trim() || pendingAttachments.value.length > 0) && !readingFiles.value))

watch(selectedModelOption, () => {
  applySelectedModelOption()
})

watch(
  () => authStore.user?.id,
  () => {
    switchConversationArchive()
  }
)

async function loadKeys() {
  loadingKeys.value = true
  try {
    const response = await keysAPI.list(1, 100, { status: 'active' })
    keys.value = response.items
    await loadAllModels()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('textChat.loadKeysFailed')))
  } finally {
    loadingKeys.value = false
  }
}

async function loadAllModels(preferredSelection?: { keyId: number; modelId: string }) {
  loadingModels.value = true
  modelError.value = ''
  try {
    const previousSelection = selectedModelOption.value
    const results = await Promise.allSettled(
      activeKeys.value.map(async (key) => ({
        key,
        models: await textChatAPI.listModels(key.key)
      }))
    )
    const options = results.flatMap((result) => {
      if (result.status !== 'fulfilled') return []
      return result.value.models.map((model) => createModelOption(result.value.key, model))
    })

    modelOptions.value = options
    const preferredValue = preferredSelection ? createModelOptionValue(preferredSelection.keyId, preferredSelection.modelId) : ''
    const nextSelection =
      findModelOption(previousSelection)?.value ||
      findModelOption(preferredValue)?.value ||
      preferredModelOption(options)?.value ||
      ''

    selectedModelOption.value = nextSelection
    applySelectedModelOption()

    if (options.length === 0) {
      modelError.value = t('textChat.noModels')
    }
  } catch (err: unknown) {
    modelError.value = extractApiErrorMessage(err, t('textChat.loadModelsFailed'))
  } finally {
    loadingModels.value = false
  }
}

function preferredModelOption(options: TextChatModelOption[]): TextChatModelOption | undefined {
  return (
    options.find((option) => option.modelId === 'gpt-4.1') ||
    options.find((option) => option.modelId === 'claude-sonnet-4-5') ||
    options.find((option) => option.modelId.toLowerCase().includes('sonnet')) ||
    options[0]
  )
}

function createModelOption(key: ApiKey, model: TextChatModel): TextChatModelOption {
  return {
    value: createModelOptionValue(key.id, model.id),
    keyId: key.id,
    keyName: key.name || t('textChat.apiKeyFallback'),
    apiKey: key.key,
    modelId: model.id,
    modelName: model.name,
    label: `${key.name || t('textChat.apiKeyFallback')}: ${model.name}`
  }
}

function createModelOptionValue(keyId: number, modelId: string) {
  return `${keyId}:${encodeURIComponent(modelId)}`
}

function findModelOption(value: string) {
  if (!value) return undefined
  return modelOptions.value.find((option) => option.value === value)
}

function applySelectedModelOption() {
  const option = selectedModelEntry.value
  if (!option) {
    selectedKeyId.value = 0
    selectedModel.value = ''
    models.value = []
    return
  }

  selectedKeyId.value = option.keyId
  selectedModel.value = option.modelId
  models.value = modelOptions.value
    .filter((item) => item.keyId === option.keyId)
    .map((item) => ({
      id: item.modelId,
      name: item.modelName
    }))
  submitError.value = ''
}

function toggleModelPicker() {
  if (loadingModels.value || modelOptions.value.length === 0) return
  modelPickerOpen.value = !modelPickerOpen.value
  if (modelPickerOpen.value) {
    modelSearchQuery.value = ''
  }
}

function selectModelOption(value: string) {
  selectedModelOption.value = value
  modelPickerOpen.value = false
  modelSearchQuery.value = ''
}

function handleDocumentPointerDown(event: MouseEvent) {
  if (!modelPickerOpen.value) return
  const target = event.target
  if (target instanceof Node && modelPickerEl.value?.contains(target)) return
  modelPickerOpen.value = false
}

function handleDocumentKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    modelPickerOpen.value = false
  }
}

function renderMarkdown(content: string) {
  const html = marked.parse(content) as string
  return DOMPurify.sanitize(html, {
    ADD_ATTR: ['target', 'rel']
  })
}

function openFilePicker() {
  fileInputEl.value?.click()
}

async function handleFileInputChange(event: Event) {
  const input = event.target as HTMLInputElement
  await addFiles(input.files)
  input.value = ''
}

function handleComposerDragEnter(event: DragEvent) {
  if (hasTransferFiles(event.dataTransfer)) {
    draggingFiles.value = true
  }
}

function handleComposerDragOver(event: DragEvent) {
  if (!hasTransferFiles(event.dataTransfer)) return
  draggingFiles.value = true
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'copy'
  }
}

function handleComposerDragLeave(event: DragEvent) {
  const currentTarget = event.currentTarget
  const relatedTarget = event.relatedTarget
  if (currentTarget instanceof Node && relatedTarget instanceof Node && currentTarget.contains(relatedTarget)) {
    return
  }
  draggingFiles.value = false
}

async function handleComposerDrop(event: DragEvent) {
  draggingFiles.value = false
  await addFiles(event.dataTransfer?.files)
}

async function handleComposerPaste(event: ClipboardEvent) {
  const files = extractClipboardFiles(event.clipboardData)
  if (files.length === 0) return
  event.preventDefault()
  await addFiles(files)
}

async function addFiles(fileList?: FileList | File[] | null) {
  if (!fileList || fileList.length === 0 || readingFiles.value) return
  const files = Array.isArray(fileList) ? fileList : Array.from(fileList)
  readingFiles.value = true
  try {
    for (const file of files) {
      if (pendingAttachments.value.length >= MAX_ATTACHMENTS) {
        appStore.showError(t('textChat.tooManyAttachments', { count: MAX_ATTACHMENTS }))
        break
      }

      if (file.size > MAX_ATTACHMENT_SIZE_BYTES) {
        appStore.showError(t('textChat.fileTooLarge', { name: file.name, size: formatAttachmentSize(MAX_ATTACHMENT_SIZE_BYTES) }))
        continue
      }

      try {
        pendingAttachments.value.push(await createAttachmentFromFile(file))
      } catch {
        appStore.showError(t('textChat.fileReadFailed', { name: file.name }))
      }
    }
  } finally {
    readingFiles.value = false
  }
}

function hasTransferFiles(dataTransfer?: DataTransfer | null) {
  if (!dataTransfer) return false
  if (dataTransfer.files.length > 0) return true
  return Array.from(dataTransfer.items || []).some((item) => item.kind === 'file')
}

function extractClipboardFiles(dataTransfer?: DataTransfer | null) {
  if (!dataTransfer) return []
  const files = Array.from(dataTransfer.files || [])
  if (files.length > 0) return files
  return Array.from(dataTransfer.items || [])
    .filter((item) => item.kind === 'file')
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file))
}

async function createAttachmentFromFile(file: File): Promise<ChatAttachment> {
  const baseAttachment = {
    id: createAttachmentId(),
    name: file.name || t('textChat.unnamedFile'),
    type: file.type || inferMimeType(file.name),
    size: file.size
  }

  if (file.type.startsWith('image/')) {
    return {
      ...baseAttachment,
      kind: 'image',
      dataUrl: await readFileAsDataUrl(file)
    }
  }

  if (isTextLikeFile(file)) {
    const rawText = await file.text()
    const truncated = rawText.length > MAX_TEXT_ATTACHMENT_CHARS
    return {
      ...baseAttachment,
      kind: 'text',
      text: truncated ? rawText.slice(0, MAX_TEXT_ATTACHMENT_CHARS) : rawText,
      truncated
    }
  }

  return {
    ...baseAttachment,
    kind: 'file',
    dataUrl: await readFileAsDataUrl(file)
  }
}

function removePendingAttachment(id: string) {
  pendingAttachments.value = pendingAttachments.value.filter((attachment) => attachment.id !== id)
}

async function send() {
  const chatOption = selectedModelEntry.value
  if (!chatOption || !canSend.value || sending.value) return
  const userText = draft.value.trim()
  const userAttachments = pendingAttachments.value.map((attachment) => ({ ...attachment }))
  draft.value = ''
  pendingAttachments.value = []
  submitError.value = ''
  messages.value.push(createChatMessage('user', userText, userAttachments))
  const assistantMessage = createChatMessage('assistant', '')
  messages.value.push(assistantMessage)
  const assistantMessageIndex = messages.value.length - 1
  const activeAssistantMessage = messages.value[assistantMessageIndex]
  streamingMessageIndex.value = assistantMessageIndex
  archiveCurrentConversation()
  await scrollToBottom()

  sending.value = true
  try {
    const typeQueue: string[] = []
    let typingTask: Promise<void> | null = null
    const startTyping = () => {
      if (typingTask) return
      typingTask = drainTypingQueue(typeQueue, activeAssistantMessage).finally(() => {
        typingTask = null
        if (typeQueue.length > 0) {
          startTyping()
        }
      })
    }
    const enqueueTyping = (text: string) => {
      typeQueue.push(...Array.from(text))
      startTyping()
    }
    const waitForTypingCompletion = async () => {
      while (typingTask || typeQueue.length > 0) {
        if (!typingTask) {
          startTyping()
        }
        const currentTask = typingTask
        if (currentTask) {
          await currentTask
        }
      }
    }

    const requestMessages: TextChatMessage[] = messages.value
      .filter((message) => message.content.trim() || hasAttachablePayload(message.attachments))
      .map((message) => ({
        role: message.role,
        content: buildRequestContent(message)
      }))

    const streamedReply = await textChatAPI.sendMessageStream(
      chatOption.apiKey,
      {
        model: chatOption.modelId,
        messages: requestMessages,
        temperature: 0.7
      },
      (delta) => {
        enqueueTyping(delta)
      }
    )

    await waitForTypingCompletion()

    if (!streamedReply.trim()) {
      activeAssistantMessage.content = t('textChat.emptyReply')
    }
    archiveCurrentConversation()
  } catch (err: unknown) {
    submitError.value = extractApiErrorMessage(err, t('textChat.sendFailed'))
    messages.value = messages.value.filter((_, index) => index !== assistantMessageIndex)
    archiveCurrentConversation()
  } finally {
    streamingMessageIndex.value = null
    sending.value = false
    await scrollToBottom()
  }
}

async function startNewConversation() {
  if (sending.value) return
  archiveCurrentConversation()
  currentConversationId.value = createConversationId()
  messages.value = []
  streamingMessageIndex.value = null
  draft.value = ''
  pendingAttachments.value = []
  submitError.value = ''
  await scrollToBottom()
}

async function clearContext() {
  if (sending.value || messages.value.length === 0) return
  archiveCurrentConversation()
  currentConversationId.value = createConversationId()
  messages.value = []
  streamingMessageIndex.value = null
  draft.value = ''
  pendingAttachments.value = []
  submitError.value = ''
  await scrollToBottom()
}

async function loadConversation(id: string) {
  const conversation = conversations.value.find((item) => item.id === id)
  if (!conversation || sending.value) return

  currentConversationId.value = conversation.id
  messages.value = conversation.messages.map((message) => ({
    ...message,
    createdAt: message.createdAt || conversation.createdAt
  }))
  streamingMessageIndex.value = null
  draft.value = ''
  pendingAttachments.value = []
  submitError.value = ''

  if (conversation.apiKeyId && conversation.model) {
    const conversationSelection = createModelOptionValue(conversation.apiKeyId, conversation.model)
    if (findModelOption(conversationSelection)) {
      selectedModelOption.value = conversationSelection
    } else if (activeKeys.value.some((key) => key.id === conversation.apiKeyId)) {
      await loadAllModels({ keyId: conversation.apiKeyId, modelId: conversation.model })
    }
  }

  await scrollToBottom()
}

function deleteConversation(id: string) {
  conversations.value = conversations.value.filter((conversation) => conversation.id !== id)
  persistConversations()

  if (id === currentConversationId.value) {
    currentConversationId.value = createConversationId()
    messages.value = []
    streamingMessageIndex.value = null
    draft.value = ''
    pendingAttachments.value = []
    submitError.value = ''
  }
}

function archiveCurrentConversation() {
  if (messages.value.length === 0) return

  const now = new Date().toISOString()
  const existing = conversations.value.find((conversation) => conversation.id === currentConversationId.value)
  const firstUserMessage = messages.value.find((message) => message.role === 'user')?.content.trim()
  const archive: ConversationArchive = {
    id: currentConversationId.value,
    title: firstUserMessage ? firstUserMessage.slice(0, 32) : t('textChat.untitledConversation'),
    messages: messages.value.map(serializeMessageForArchive),
    apiKeyId: selectedKeyId.value,
    apiKeyName: selectedKeyName.value,
    model: selectedModel.value,
    modelName: selectedModelName.value,
    createdAt: existing?.createdAt || now,
    updatedAt: now
  }

  conversations.value = [
    archive,
    ...conversations.value.filter((conversation) => conversation.id !== currentConversationId.value)
  ].slice(0, MAX_ARCHIVED_CONVERSATIONS)
  persistConversations()
}

function loadConversationArchive() {
  try {
    const raw = localStorage.getItem(historyStorageKey.value)
    if (!raw) return
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return
    conversations.value = parsed
      .filter((item): item is ConversationArchive => {
        return Boolean(item?.id && Array.isArray(item?.messages) && item?.createdAt && item?.updatedAt)
      })
      .map((item) => ({
        ...item,
        messages: item.messages.map((message) => ({
          ...message,
          createdAt: message.createdAt || item.createdAt
        }))
      }))
      .slice(0, MAX_ARCHIVED_CONVERSATIONS)
  } catch {
    conversations.value = []
  }
}

function persistConversations() {
  try {
    localStorage.setItem(historyStorageKey.value, JSON.stringify(conversations.value))
  } catch {
    // Ignore storage failures so chat sending is never blocked by local archive persistence.
  }
}

function switchConversationArchive() {
  archiveCurrentConversation()
  historyStorageKey.value = getCurrentHistoryStorageKey()
  conversations.value = []
  currentConversationId.value = createConversationId()
  messages.value = []
  streamingMessageIndex.value = null
  draft.value = ''
  pendingAttachments.value = []
  submitError.value = ''
  loadConversationArchive()
}

function getCurrentHistoryStorageKey() {
  const userId = authStore.user?.id
  if (userId) {
    return `${CHAT_HISTORY_STORAGE_KEY}:user:${userId}`
  }
  return `${CHAT_HISTORY_STORAGE_KEY}:anonymous`
}

function createConversationId() {
  return `chat-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function createAttachmentId() {
  return `attachment-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function createChatMessage(role: ChatMessage['role'], content: string, attachments: ChatAttachment[] = []): ChatMessage {
  return {
    role,
    content,
    createdAt: new Date().toISOString(),
    attachments: attachments.length > 0 ? attachments : undefined
  }
}

function buildRequestContent(message: ChatMessage): TextChatMessage['content'] {
  const attachments = message.attachments?.filter(hasAttachmentPayload) ?? []
  if (attachments.length === 0) {
    return message.content
  }

  const parts: TextChatContentPart[] = []
  if (message.content.trim()) {
    parts.push({ type: 'text', text: message.content.trim() })
  }

  attachments.forEach((attachment) => {
    if (attachment.kind === 'text' && attachment.text) {
      parts.push({
        type: 'text',
        text: [
          `\n\n[${t('textChat.attachmentContextLabel')}: ${attachment.name}]`,
          attachment.truncated ? t('textChat.attachmentTruncated') : '',
          attachment.text
        ]
          .filter(Boolean)
          .join('\n')
      })
      return
    }

    if (attachment.kind === 'image' && attachment.dataUrl) {
      parts.push({ type: 'text', text: `${t('textChat.imageAttachmentInstruction')}: ${attachment.name}` })
      parts.push({
        type: 'image_url',
        image_url: {
          url: attachment.dataUrl,
          detail: 'auto'
        }
      })
      return
    }

    if (attachment.dataUrl) {
      parts.push({ type: 'text', text: `${t('textChat.fileAttachmentInstruction')}: ${attachment.name}` })
      parts.push({
        type: 'file',
        file: {
          filename: attachment.name,
          file_data: attachment.dataUrl
        }
      })
    }
  })

  return parts.length > 0 ? parts : message.content
}

function hasAttachablePayload(attachments?: ChatAttachment[]) {
  return Boolean(attachments?.some(hasAttachmentPayload))
}

function hasAttachmentPayload(attachment: ChatAttachment) {
  return Boolean(attachment.text || attachment.dataUrl)
}

function serializeMessageForArchive(message: ChatMessage): ChatMessage {
  return {
    ...message,
    attachments: message.attachments?.map((attachment) => ({
      id: attachment.id,
      name: attachment.name,
      type: attachment.type,
      size: attachment.size,
      kind: attachment.kind,
      truncated: attachment.truncated
    }))
  }
}

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

function isTextLikeFile(file: File) {
  if (file.type.startsWith('text/')) return true
  const extension = file.name.split('.').pop()?.toLowerCase() || ''
  return [
    'txt',
    'md',
    'markdown',
    'json',
    'jsonl',
    'csv',
    'tsv',
    'yaml',
    'yml',
    'xml',
    'html',
    'css',
    'js',
    'jsx',
    'ts',
    'tsx',
    'vue',
    'go',
    'py',
    'java',
    'c',
    'cpp',
    'h',
    'hpp',
    'rs',
    'php',
    'rb',
    'sql',
    'sh',
    'toml',
    'ini',
    'log'
  ].includes(extension)
}

function inferMimeType(filename: string) {
  const extension = filename.split('.').pop()?.toLowerCase()
  if (extension === 'pdf') return 'application/pdf'
  if (extension === 'docx') return 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
  if (extension === 'xlsx') return 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  if (extension === 'pptx') return 'application/vnd.openxmlformats-officedocument.presentationml.presentation'
  return 'application/octet-stream'
}

function formatAttachmentSize(size: number) {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

async function drainTypingQueue(queue: string[], message: ChatMessage) {
  while (queue.length > 0) {
    message.content += queue.shift() ?? ''
    await scrollToBottom()
    await sleep(TYPEWRITER_DELAY_MS)
  }
}

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

function formatConversationTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''

  const now = new Date()
  const sameDay = date.toDateString() === now.toDateString()
  const formatter = new Intl.DateTimeFormat(undefined, {
    month: sameDay ? undefined : '2-digit',
    day: sameDay ? undefined : '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
  return formatter.format(date)
}

function formatMessageTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit'
  }).format(date)
}

async function copyMessage(content: string) {
  try {
    await navigator.clipboard.writeText(content)
    appStore.showSuccess(t('common.copied'))
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}

async function scrollToBottom() {
  await nextTick()
  if (messagesEl.value) {
    messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  }
}

onMounted(async () => {
  document.addEventListener('mousedown', handleDocumentPointerDown)
  document.addEventListener('keydown', handleDocumentKeydown)
  loadConversationArchive()
  await loadKeys()
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleDocumentPointerDown)
  document.removeEventListener('keydown', handleDocumentKeydown)
})
</script>

<style scoped>
.ai-markdown-body {
  line-height: 1.65;
}

.ai-markdown-body :deep(*) {
  overflow-wrap: anywhere;
}

.ai-markdown-body :deep(p) {
  margin: 0.35rem 0;
}

.ai-markdown-body :deep(p:first-child),
.ai-markdown-body :deep(ul:first-child),
.ai-markdown-body :deep(ol:first-child),
.ai-markdown-body :deep(pre:first-child),
.ai-markdown-body :deep(blockquote:first-child),
.ai-markdown-body :deep(table:first-child) {
  margin-top: 0;
}

.ai-markdown-body :deep(p:last-child),
.ai-markdown-body :deep(ul:last-child),
.ai-markdown-body :deep(ol:last-child),
.ai-markdown-body :deep(pre:last-child),
.ai-markdown-body :deep(blockquote:last-child),
.ai-markdown-body :deep(table:last-child) {
  margin-bottom: 0;
}

.ai-markdown-body :deep(h1),
.ai-markdown-body :deep(h2),
.ai-markdown-body :deep(h3) {
  margin: 0.75rem 0 0.35rem;
  font-weight: 700;
  line-height: 1.35;
}

.ai-markdown-body :deep(h1) {
  font-size: 1.2rem;
}

.ai-markdown-body :deep(h2) {
  font-size: 1.08rem;
}

.ai-markdown-body :deep(h3) {
  font-size: 1rem;
}

.ai-markdown-body :deep(ul),
.ai-markdown-body :deep(ol) {
  margin: 0.45rem 0;
  padding-left: 1.25rem;
}

.ai-markdown-body :deep(ul) {
  list-style: disc;
}

.ai-markdown-body :deep(ol) {
  list-style: decimal;
}

.ai-markdown-body :deep(li) {
  margin: 0.2rem 0;
}

.ai-markdown-body :deep(blockquote) {
  margin: 0.65rem 0;
  border-left: 3px solid rgb(209 213 219);
  padding: 0.15rem 0 0.15rem 0.75rem;
  color: rgb(75 85 99);
}

.dark .ai-markdown-body :deep(blockquote) {
  border-color: rgb(75 85 99);
  color: rgb(209 213 219);
}

.ai-markdown-body :deep(code) {
  border-radius: 0.35rem;
  background: rgb(229 231 235);
  padding: 0.1rem 0.3rem;
  font-size: 0.86em;
}

.dark .ai-markdown-body :deep(code) {
  background: rgb(31 41 55);
}

.ai-markdown-body :deep(pre) {
  margin: 0.65rem 0;
  max-width: 100%;
  overflow-x: auto;
  border-radius: 0.5rem;
  background: rgb(17 24 39);
  padding: 0.75rem;
  color: rgb(243 244 246);
}

.ai-markdown-body :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
}

.ai-markdown-body :deep(a) {
  color: rgb(37 99 235);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.dark .ai-markdown-body :deep(a) {
  color: rgb(147 197 253);
}

.ai-markdown-body :deep(table) {
  margin: 0.65rem 0;
  display: block;
  max-width: 100%;
  overflow-x: auto;
  border-collapse: collapse;
}

.ai-markdown-body :deep(th),
.ai-markdown-body :deep(td) {
  border: 1px solid rgb(209 213 219);
  padding: 0.35rem 0.55rem;
  text-align: left;
  white-space: nowrap;
}

.dark .ai-markdown-body :deep(th),
.dark .ai-markdown-body :deep(td) {
  border-color: rgb(75 85 99);
}

.ai-markdown-body :deep(th) {
  background: rgb(243 244 246);
  font-weight: 600;
}

.dark .ai-markdown-body :deep(th) {
  background: rgb(31 41 55);
}
</style>
