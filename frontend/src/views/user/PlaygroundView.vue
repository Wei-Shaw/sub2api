<template>
  <AppLayout>
    <div v-if="!loadingKeys && keys.length === 0" class="card p-6 text-body text-fg-muted">
      {{ t('playground.noKey') }}
      <router-link to="/keys" class="text-accent">{{ t('playground.manageKeys') }}</router-link>
    </div>

    <div v-else class="grid gap-4 lg:grid-cols-[18rem_1fr]">
      <section class="card space-y-3 self-start p-4">
        <div>
          <label class="input-label">{{ t('playground.apiKey') }}</label>
          <Select v-model="form.apiKeyId" :options="keyOptions" :disabled="sending" />
        </div>
        <div>
          <label class="input-label">{{ t('playground.model') }}</label>
          <Select v-model="form.model" :options="modelOptions" :disabled="sending" creatable searchable />
        </div>
        <div>
          <label class="input-label" for="playground-system">{{ t('playground.systemPrompt') }}</label>
          <textarea id="playground-system" v-model="form.system" rows="4" class="input" :disabled="sending" />
        </div>
        <div>
          <label class="input-label" for="playground-temperature">{{ t('playground.temperature') }}</label>
          <input
            id="playground-temperature"
            v-model="form.temperature"
            type="number"
            min="0"
            max="2"
            step="0.1"
            class="input"
            :placeholder="t('playground.auto')"
            :disabled="sending"
          />
        </div>
        <button class="btn btn-secondary btn-sm w-full" :disabled="sending || messages.length === 0" @click="messages = []">
          <Icon name="trash" size="sm" class="mr-1" />
          {{ t('playground.clear') }}
        </button>
      </section>

      <section class="card flex min-h-[32rem] flex-col p-4">
        <div ref="thread" class="max-h-[60vh] flex-1 space-y-3 overflow-y-auto" aria-live="polite">
          <p v-if="messages.length === 0" class="text-body text-fg-muted">{{ t('playground.empty') }}</p>
          <div
            v-for="(message, index) in messages"
            :key="index"
            :class="['rounded-md p-3 text-body', message.role === 'user' ? 'ml-8 bg-surface-sunken' : 'mr-8 border border-border']"
          >
            <div class="mb-1 flex items-center justify-between text-meta text-fg-subtle">
              <span>{{ t(`playground.role.${message.role}`) }}</span>
              <button
                v-if="message.content"
                class="btn btn-ghost btn-sm btn-icon"
                :title="t('playground.copy')"
                :aria-label="t('playground.copy')"
                @click="copyToClipboard(message.content)"
              >
                <Icon name="copy" size="xs" />
              </button>
            </div>
            <div
              v-if="message.role === 'assistant' && message.content"
              class="markdown-body prose prose-sm max-w-none break-words text-fg dark:prose-invert"
              v-html="renderMarkdown(message)"
            />
            <p v-else class="whitespace-pre-wrap break-words text-fg">{{ message.content || (sending ? '…' : '') }}</p>
          </div>
        </div>

        <div class="mt-3 flex gap-2">
          <textarea
            v-model="draft"
            rows="2"
            class="input flex-1"
            :placeholder="t('playground.placeholder')"
            :aria-label="t('playground.placeholder')"
            :disabled="sending"
            @keydown.enter.exact.prevent="send"
          />
          <button v-if="sending" class="btn btn-secondary self-end" @click="controller?.abort()">{{ t('playground.stop') }}</button>
          <button v-else class="btn btn-primary self-end" :disabled="!canSend" @click="send">{{ t('playground.send') }}</button>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { keysAPI } from '@/api/keys'
import { useAppStore } from '@/stores'
import { useClipboard } from '@/composables/useClipboard'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { ApiKey } from '@/types'
import { readChatDeltas, responseError } from './playgroundStream'
import '@/styles/announcement-markdown.css'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const loadingKeys = ref(true)
const keys = ref<ApiKey[]>([])
const models = ref<string[]>([])
const form = reactive({ apiKeyId: 0, model: '', system: '', temperature: '' as string | number })
const messages = ref<ChatMessage[]>([])
const draft = ref('')
const sending = ref(false)
const controller = ref<AbortController>()
const thread = ref<HTMLElement>()

const selectedKey = computed(() => keys.value.find((key) => key.id === form.apiKeyId))
const keyOptions = computed(() => keys.value.map((key) => ({ value: key.id, label: `${key.name} · ${key.group?.name ?? ''}` })))
const modelOptions = computed(() => {
  const ids = form.model && !models.value.includes(form.model) ? [form.model, ...models.value] : models.value
  return ids.map((id) => ({ value: id, label: id }))
})
const canSend = computed(() => !sending.value && !!selectedKey.value && form.model.trim() !== '' && draft.value.trim() !== '')

// Requests go straight to this site's gateway with the user's own key, so billing and limits match real API use.
function gatewayFetch(path: string, init: Parameters<typeof fetch>[1] = {}) {
  return fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${selectedKey.value?.key ?? ''}` }
  })
}

async function loadKeys() {
  loadingKeys.value = true
  try {
    const res = await keysAPI.list(1, 100, { status: 'active', sort_by: 'created_at', sort_order: 'desc' })
    keys.value = res.items
    if (keys.value.length && !form.apiKeyId) form.apiKeyId = keys.value[0].id
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.unknownError')))
  } finally {
    loadingKeys.value = false
  }
}

async function loadModels() {
  models.value = []
  if (!selectedKey.value) return
  try {
    const res = await gatewayFetch('/v1/models')
    if (!res.ok) return
    const body = await res.json()
    models.value = (body.data ?? []).map((item: { id: string }) => item.id).filter(Boolean)
    if (!models.value.includes(form.model)) form.model = models.value[0] ?? ''
  } catch {
    // Model list is a convenience; the picker still accepts a typed model name.
  }
}

async function send() {
  if (!canSend.value) return
  const history: ChatMessage[] = [...messages.value, { role: 'user', content: draft.value.trim() }]
  messages.value = [...history, { role: 'assistant', content: '' }]
  const reply = messages.value[messages.value.length - 1]
  draft.value = ''
  sending.value = true
  controller.value = new AbortController()

  const temperature = form.temperature === '' ? undefined : Number(form.temperature)
  const system = form.system.trim()
  // Apply stream deltas at most once per animation frame; each apply re-renders the reply's markdown.
  let pending = ''
  let frame = 0
  const flush = () => {
    frame = 0
    if (!pending) return
    reply.content += pending
    pending = ''
    scrollToBottom()
  }
  try {
    const res = await gatewayFetch('/v1/chat/completions', {
      method: 'POST',
      signal: controller.value.signal,
      body: JSON.stringify({
        model: form.model.trim(),
        stream: true,
        temperature,
        messages: system ? [{ role: 'system', content: system }, ...history] : history
      })
    })
    if (!res.ok || !res.body) throw new Error(await responseError(res))
    for await (const delta of readChatDeltas(res.body)) {
      pending += delta
      if (!frame) frame = requestAnimationFrame(flush)
    }
  } catch (err) {
    if ((err as Error).name !== 'AbortError') appStore.showError((err as Error).message || t('common.unknownError'))
  } finally {
    cancelAnimationFrame(frame)
    flush()
    // Drop an empty reply so a failed turn leaves no blank bubble.
    if (!reply.content) messages.value = messages.value.filter((message) => message !== reply)
    sending.value = false
    controller.value = undefined
  }
}

// Rendered HTML per message: finished turns hit the cache, so a stream delta only re-parses the streaming reply.
const renderedMarkdown = new WeakMap<ChatMessage, { content: string; html: string }>()

// Model output is untrusted: always sanitize the rendered HTML.
function renderMarkdown(message: ChatMessage) {
  const cached = renderedMarkdown.get(message)
  if (cached?.content === message.content) return cached.html
  const html = DOMPurify.sanitize(marked.parse(message.content, { breaks: true, gfm: true, async: false }))
  renderedMarkdown.set(message, { content: message.content, html })
  return html
}

function scrollToBottom() {
  void nextTick(() => thread.value?.scrollTo({ top: thread.value.scrollHeight }))
}

watch(() => form.apiKeyId, loadModels)
onMounted(loadKeys)
onBeforeUnmount(() => controller.value?.abort())
</script>
