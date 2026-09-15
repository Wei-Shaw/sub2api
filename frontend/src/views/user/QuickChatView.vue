<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-5xl flex-col gap-5">
      <section class="flex flex-col gap-1">
        <div class="flex items-center gap-2 text-primary-600 dark:text-primary-400">
          <Icon name="chatBubble" size="md" />
          <span class="text-sm font-medium">{{ t('quickChat.eyebrow') }}</span>
        </div>
        <h2 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('quickChat.title') }}</h2>
        <p class="max-w-2xl text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('quickChat.description') }}</p>
      </section>

      <section class="grid gap-4 rounded-xl bg-white p-4 shadow-sm ring-1 ring-gray-200/70 dark:bg-dark-800 dark:ring-dark-700 sm:grid-cols-2 lg:grid-cols-3 sm:p-5">
        <div>
          <label class="input-label mb-1.5 block" for="quick-chat-key">{{ t('quickChat.keyLabel') }}</label>
          <Select
            id="quick-chat-key"
            v-model="selectedKeyId"
            :options="keyOptions"
            :loading="keysLoading"
            :disabled="keysLoading || keyOptions.length === 0 || streaming"
            :placeholder="t('quickChat.keyPlaceholder')"
            :aria-label="t('quickChat.keyLabel')"
            @update:model-value="handleKeyChange"
          />
          <p v-if="!keysLoading && keyOptions.length === 0" class="mt-1.5 text-xs text-amber-600 dark:text-amber-300">{{ t('quickChat.noKeys') }}</p>
          <p class="input-hint mt-1.5">{{ t('quickChat.keyHint') }}</p>
        </div>

        <div>
          <label class="input-label mb-1.5 block" for="quick-chat-model">{{ t('quickChat.modelLabel') }}</label>
          <Select
            id="quick-chat-model"
            v-model="selectedModel"
            :options="modelOptions"
            :loading="modelsLoading"
            :disabled="!selectedKey || modelsLoading || modelOptions.length === 0 || streaming"
            :placeholder="t('quickChat.modelPlaceholder')"
            :aria-label="t('quickChat.modelLabel')"
            searchable
          />
          <p class="input-hint mt-1.5">{{ modelsLoading ? t('quickChat.modelsLoading') : t('quickChat.modelHint') }}</p>
        </div>

        <div>
          <label class="input-label mb-1.5 block" for="quick-chat-reasoning-effort">{{ t('quickChat.reasoningLabel') }}</label>
          <Select
            id="quick-chat-reasoning-effort"
            v-model="reasoningEffort"
            :options="reasoningOptions"
            :disabled="!selectedKey || !selectedModel || streaming"
            :placeholder="t('quickChat.reasoningPlaceholder')"
            :aria-label="t('quickChat.reasoningLabel')"
          />
          <p class="input-hint mt-1.5">{{ t('quickChat.reasoningHint') }}</p>
        </div>
      </section>

      <section class="flex min-h-[420px] flex-col overflow-hidden rounded-xl bg-white shadow-sm ring-1 ring-gray-200/70 dark:bg-dark-800 dark:ring-dark-700">
        <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:px-5">
          <div class="flex min-w-0 items-center gap-2">
            <span class="h-2 w-2 rounded-full" :class="streaming ? 'bg-primary-500 animate-pulse' : 'bg-gray-300 dark:bg-dark-500'" />
            <span class="truncate text-sm font-medium text-gray-700 dark:text-dark-200">{{ selectedModel || t('quickChat.noModelSelected') }}</span>
          </div>
          <button
            type="button"
            class="btn btn-secondary px-3 py-1.5 text-xs"
            :disabled="streaming || messages.length === 0"
            @click="clearConversation"
          >
            <Icon name="trash" size="sm" class="mr-1.5" />
            {{ t('quickChat.clear') }}
          </button>
        </div>

        <div ref="messagesRef" class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4 sm:p-6">
          <div v-if="messages.length === 0" class="flex min-h-[280px] flex-col items-center justify-center text-center">
            <div class="mb-3 flex h-12 w-12 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400">
              <Icon name="sparkles" size="lg" />
            </div>
            <p class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('quickChat.emptyTitle') }}</p>
            <p class="mt-1 max-w-sm text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('quickChat.emptyDescription') }}</p>
          </div>

          <article v-for="(message, index) in messages" :key="`${message.role}-${index}`" class="flex gap-3" :class="message.role === 'user' ? 'justify-end' : 'justify-start'">
            <div class="max-w-[min(760px,90%)] rounded-xl px-4 py-3 text-sm leading-6" :class="message.role === 'user' ? 'bg-primary-600 text-white' : 'bg-gray-50 text-gray-800 ring-1 ring-gray-200/70 dark:bg-dark-700 dark:text-dark-100 dark:ring-dark-600'">
              <div v-if="message.role === 'assistant'" class="mb-1 text-xs font-semibold text-primary-600 dark:text-primary-400">{{ t('quickChat.assistant') }}</div>
              <p class="whitespace-pre-wrap break-words">{{ message.content || (streaming && index === messages.length - 1 ? '...' : '') }}</p>
            </div>
          </article>
        </div>

        <div class="border-t border-gray-100 p-4 dark:border-dark-700 sm:p-5">
          <label class="sr-only" for="quick-chat-input">{{ t('quickChat.inputLabel') }}</label>
          <TextArea
            v-model="draft"
            id="quick-chat-input"
            :placeholder="t('quickChat.inputPlaceholder')"
            :disabled="streaming || !selectedKey || !selectedModel"
            :rows="3"
            :aria-label="t('quickChat.inputLabel')"
            @keydown.enter.exact.prevent="sendMessage"
          />
          <div class="mt-3 flex items-center justify-between gap-3">
            <p class="min-w-0 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('quickChat.securityNote') }}</p>
            <button
              v-if="streaming"
              type="button"
              class="btn btn-secondary shrink-0"
              @click="stopStreaming"
            >
              <Icon name="x" size="sm" class="mr-1.5" />
              {{ t('quickChat.stop') }}
            </button>
            <button
              v-else
              type="button"
              class="btn btn-primary shrink-0"
              :disabled="!canSend"
              @click="sendMessage"
            >
              <Icon name="arrowUp" size="sm" class="mr-1.5" />
              {{ t('quickChat.send') }}
            </button>
          </div>
        </div>
      </section>

      <div v-if="errorMessage" class="flex items-start gap-2 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200 dark:bg-red-900/20 dark:text-red-300 dark:ring-red-900/50" role="alert">
        <Icon name="exclamationCircle" size="md" class="mt-0.5 shrink-0" />
        <span>{{ errorMessage }}</span>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import { keysAPI } from '@/api/keys'
import { buildGatewayUrl } from '@/api/client'
import type { ApiKey } from '@/types'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

interface ModelsResponse {
  data?: Array<{ id?: string }>
}

const { t } = useI18n()
const keys = ref<ApiKey[]>([])
const selectedKeyId = ref<string | number | null>(null)
const selectedModel = ref<string | null>(null)
const reasoningEffort = ref('')
const modelOptions = ref<SelectOption[]>([])
const draft = ref('')
const messages = ref<ChatMessage[]>([])
const keysLoading = ref(false)
const modelsLoading = ref(false)
const streaming = ref(false)
const errorMessage = ref('')
const messagesRef = ref<HTMLElement | null>(null)
let abortController: AbortController | null = null

const selectedKey = computed(() => keys.value.find(key => key.id === Number(selectedKeyId.value)))
const keyOptions = computed<SelectOption[]>(() => keys.value.map(key => ({
  value: key.id,
  label: `${key.name} (${maskKey(key.key)})`
})))
const reasoningOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('quickChat.reasoningAuto') },
  { value: 'low', label: t('quickChat.reasoningLow') },
  { value: 'medium', label: t('quickChat.reasoningMedium') },
  { value: 'high', label: t('quickChat.reasoningHigh') },
  { value: 'xhigh', label: t('quickChat.reasoningXhigh') },
  { value: 'max', label: t('quickChat.reasoningMax') },
])
const canSend = computed(() => Boolean(selectedKey.value?.key && selectedModel.value && draft.value.trim()))

function maskKey(key: string): string {
  if (!key) return ''
  if (key.length <= 10) return `${key.slice(0, 3)}...`
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}

async function loadKeys() {
  keysLoading.value = true
  errorMessage.value = ''
  try {
    const response = await keysAPI.list(1, 100, { status: 'active' })
    keys.value = response.items.filter(key => key.status === 'active' && key.key)
    if (keys.value.length > 0) {
      selectedKeyId.value = keys.value[0].id
      await loadModels()
    }
  } catch (error) {
    errorMessage.value = getErrorMessage(error, t('quickChat.keysLoadFailed'))
  } finally {
    keysLoading.value = false
  }
}

async function handleKeyChange() {
  selectedModel.value = null
  modelOptions.value = []
  messages.value = []
  await loadModels()
}

async function loadModels() {
  if (!selectedKey.value?.key) return
  modelsLoading.value = true
  errorMessage.value = ''
  try {
    const response = await fetch(buildGatewayUrl('/v1/models'), {
      headers: { Authorization: `Bearer ${selectedKey.value.key}` }
    })
    const payload = await parseJSONResponse<ModelsResponse>(response)
    const models = (payload.data || []).map(item => String(item.id || '').trim()).filter(Boolean)
    modelOptions.value = Array.from(new Set(models)).sort().map(model => ({ value: model, label: model }))
    if (modelOptions.value.length > 0) selectedModel.value = String(modelOptions.value[0].value)
    if (modelOptions.value.length === 0) errorMessage.value = t('quickChat.noModels')
  } catch (error) {
    errorMessage.value = getErrorMessage(error, t('quickChat.modelsLoadFailed'))
  } finally {
    modelsLoading.value = false
  }
}

async function sendMessage() {
  if (!canSend.value || streaming.value) return
  const content = draft.value.trim()
  draft.value = ''
  errorMessage.value = ''
  messages.value.push({ role: 'user', content })
  messages.value.push({ role: 'assistant', content: '' })
  streaming.value = true
  abortController = new AbortController()
  await scrollToBottom()

  try {
    const response = await fetch(buildGatewayUrl('/v1/chat/completions'), {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${selectedKey.value!.key}`,
        'Content-Type': 'application/json',
        Accept: 'text/event-stream'
      },
      body: JSON.stringify({
        model: selectedModel.value,
        messages: messages.value.slice(0, -1),
        stream: true,
        ...(reasoningEffort.value ? { reasoning_effort: reasoningEffort.value } : {})
      }),
      signal: abortController.signal
    })
    if (!response.ok) {
      const payload = await response.json().catch(() => null)
      throw new Error(payload?.error?.message || payload?.message || `HTTP ${response.status}`)
    }
    if (!response.body) throw new Error(t('quickChat.emptyResponse'))
    await readStream(response.body)
  } catch (error) {
    if (!isAbortError(error)) {
      messages.value.pop()
      errorMessage.value = getErrorMessage(error, t('quickChat.sendFailed'))
    }
  } finally {
    streaming.value = false
    abortController = null
    await scrollToBottom()
  }
}

async function readStream(body: ReadableStream<Uint8Array>) {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const events = buffer.split('\n')
    buffer = events.pop() || ''
    for (const line of events) {
      if (!line.startsWith('data:')) continue
      const raw = line.slice(5).trim()
      if (!raw || raw === '[DONE]') continue
      try {
        const payload = JSON.parse(raw)
        const delta = payload.choices?.[0]?.delta?.content
        if (typeof delta === 'string') {
          messages.value[messages.value.length - 1].content += delta
          await scrollToBottom()
        }
        if (payload.error?.message) throw new Error(payload.error.message)
      } catch (error) {
        if (error instanceof SyntaxError) continue
        throw error
      }
    }
  }
}

function stopStreaming() {
  abortController?.abort()
}

function clearConversation() {
  if (streaming.value) return
  messages.value = []
  errorMessage.value = ''
}

async function scrollToBottom() {
  await nextTick()
  if (messagesRef.value) messagesRef.value.scrollTop = messagesRef.value.scrollHeight
}

async function parseJSONResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const payload = await response.json().catch(() => null)
    throw new Error(payload?.error?.message || payload?.message || `HTTP ${response.status}`)
  }
  return response.json() as Promise<T>
}

function getErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message
  return fallback
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError'
}

watch(selectedKeyId, () => {
  if (!selectedKeyId.value) {
    modelOptions.value = []
    selectedModel.value = null
  }
})

onMounted(loadKeys)
</script>
