<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-6 p-4 sm:p-6 lg:p-8">
      <div class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <p class="text-sm font-medium uppercase tracking-wide text-primary-600 dark:text-primary-400">Playground</p>
            <h1 class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">聊天 / 生图体验台</h1>
            <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-gray-400">
              使用你的 Sub2API Key 直接测试聊天和图片生成接口。请求会发送到下方 Base URL，不会保存 API Key 到后端。
            </p>
          </div>
          <div class="rounded-xl bg-blue-50 px-4 py-3 text-sm text-blue-700 dark:bg-blue-950/40 dark:text-blue-200">
            <div class="font-medium">接口路径</div>
            <div class="mt-1 font-mono text-xs">/v1/chat/completions</div>
            <div class="font-mono text-xs">/v1/images/generations</div>
          </div>
        </div>
      </div>

      <div class="grid gap-4 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 lg:grid-cols-[1fr_1fr]">
        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-200">Base URL</span>
          <input
            v-model.trim="baseUrl"
            class="input mt-2 w-full"
            placeholder="https://example.com"
            spellcheck="false"
          />
          <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">默认使用系统公开设置里的 API 地址，未配置时使用当前站点。</span>
        </label>
        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-200">API Key</span>
          <div class="mt-2 flex gap-2">
            <input
              v-model="apiKey"
              :type="showKey ? 'text' : 'password'"
              class="input w-full font-mono"
              placeholder="sk-..."
              spellcheck="false"
              autocomplete="off"
            />
            <button class="btn btn-secondary shrink-0" type="button" @click="showKey = !showKey">
              {{ showKey ? '隐藏' : '显示' }}
            </button>
          </div>
          <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">仅保存在浏览器 localStorage，方便下次继续测试。</span>
        </label>
      </div>

      <div class="flex gap-2 rounded-2xl border border-gray-200 bg-white p-2 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <button
          type="button"
          class="flex-1 rounded-xl px-4 py-2 text-sm font-medium transition"
          :class="activeTab === 'chat' ? 'bg-primary-600 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
          @click="activeTab = 'chat'"
        >
          聊天
        </button>
        <button
          type="button"
          class="flex-1 rounded-xl px-4 py-2 text-sm font-medium transition"
          :class="activeTab === 'image' ? 'bg-primary-600 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
          @click="activeTab = 'image'"
        >
          生图
        </button>
      </div>

      <section v-if="activeTab === 'chat'" class="grid gap-6 lg:grid-cols-[360px_1fr]">
        <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">聊天参数</h2>
          <div class="mt-4 space-y-4">
            <label class="block">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-200">模型</span>
              <input v-model.trim="chatModel" class="input mt-2 w-full" placeholder="gpt-4o-mini" spellcheck="false" />
            </label>
            <label class="block">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-200">System Prompt</span>
              <textarea v-model="systemPrompt" class="input mt-2 min-h-[110px] w-full resize-y" placeholder="You are a helpful assistant." />
            </label>
            <div class="grid grid-cols-2 gap-3">
              <label class="block">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-200">Temperature</span>
                <input v-model.number="temperature" type="number" min="0" max="2" step="0.1" class="input mt-2 w-full" />
              </label>
              <label class="block">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-200">Max Tokens</span>
                <input v-model.number="maxTokens" type="number" min="1" step="1" class="input mt-2 w-full" />
              </label>
            </div>
            <button class="btn btn-secondary w-full" type="button" :disabled="chatLoading" @click="clearChat">
              清空对话
            </button>
          </div>
        </div>

        <div class="flex min-h-[620px] flex-col rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex-1 space-y-4 overflow-y-auto p-5">
            <div v-if="chatMessages.length === 0" class="flex h-full min-h-[360px] items-center justify-center text-center text-gray-400">
              输入消息后点击发送，直接测试当前 Key 的聊天能力。
            </div>
            <div v-for="(message, index) in chatMessages" :key="index" class="flex" :class="message.role === 'user' ? 'justify-end' : 'justify-start'">
              <div
                class="max-w-[85%] whitespace-pre-wrap rounded-2xl px-4 py-3 text-sm leading-6"
                :class="message.role === 'user'
                  ? 'bg-primary-600 text-white'
                  : 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-gray-100'"
              >
                {{ message.content }}
              </div>
            </div>
            <div v-if="chatError" class="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-200">
              {{ chatError }}
            </div>
          </div>
          <div class="border-t border-gray-200 p-4 dark:border-dark-700">
            <textarea
              v-model="chatInput"
              class="input min-h-[100px] w-full resize-y"
              placeholder="输入你想测试的问题..."
              @keydown.ctrl.enter.prevent="sendChat"
              @keydown.meta.enter.prevent="sendChat"
            />
            <div class="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <span class="text-xs text-gray-500 dark:text-gray-400">快捷键：Ctrl/⌘ + Enter 发送</span>
              <button class="btn btn-primary" type="button" :disabled="chatLoading || !canSendChat" @click="sendChat">
                {{ chatLoading ? '发送中...' : '发送' }}
              </button>
            </div>
          </div>
        </div>
      </section>

      <section v-else class="grid gap-6 lg:grid-cols-[360px_1fr]">
        <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">生图参数</h2>
          <div class="mt-4 space-y-4">
            <label class="block">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-200">模型</span>
              <input v-model.trim="imageModel" class="input mt-2 w-full" placeholder="gpt-image-1" spellcheck="false" />
            </label>
            <label class="block">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-200">尺寸</span>
              <select v-model="imageSize" class="input mt-2 w-full">
                <option value="1024x1024">1024x1024</option>
                <option value="1024x1536">1024x1536</option>
                <option value="1536x1024">1536x1024</option>
                <option value="512x512">512x512</option>
                <option value="256x256">256x256</option>
              </select>
            </label>
            <label class="block">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-200">数量</span>
              <input v-model.number="imageCount" type="number" min="1" max="4" step="1" class="input mt-2 w-full" />
            </label>
            <label class="block">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-200">Prompt</span>
              <textarea v-model="imagePrompt" class="input mt-2 min-h-[180px] w-full resize-y" placeholder="一只戴着护目镜的企鹅程序员，赛博朋克风格，高细节" />
            </label>
            <button class="btn btn-primary w-full" type="button" :disabled="imageLoading || !canGenerateImage" @click="generateImage">
              {{ imageLoading ? '生成中...' : '生成图片' }}
            </button>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center justify-between gap-3">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">生成结果</h2>
            <button v-if="generatedImages.length" class="btn btn-secondary" type="button" @click="generatedImages = []">清空</button>
          </div>
          <div v-if="imageError" class="mt-4 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-200">
            {{ imageError }}
          </div>
          <div v-if="generatedImages.length === 0" class="mt-4 flex min-h-[420px] items-center justify-center rounded-2xl border border-dashed border-gray-200 text-center text-gray-400 dark:border-dark-700">
            图片生成后会显示在这里。
          </div>
          <div v-else class="mt-4 grid gap-4 md:grid-cols-2">
            <div v-for="(image, index) in generatedImages" :key="index" class="overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900">
              <img :src="image" :alt="`Generated image ${index + 1}`" class="aspect-square w-full object-contain" />
              <div class="flex items-center justify-between gap-2 p-3">
                <span class="text-xs text-gray-500">#{{ index + 1 }}</span>
                <a class="btn btn-secondary text-sm" :href="image" target="_blank" rel="noopener noreferrer" download>打开/下载</a>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { authAPI } from '@/api/auth'
import AppLayout from '@/components/layout/AppLayout.vue'
import type { PublicSettings } from '@/types'

type PlaygroundTab = 'chat' | 'image'
type ChatRole = 'user' | 'assistant'
interface ChatMessage {
  role: ChatRole
  content: string
}

const activeTab = ref<PlaygroundTab>('chat')
const publicSettings = ref<PublicSettings | null>(null)
const baseUrl = ref('')
const apiKey = ref(localStorage.getItem('playground_api_key') || '')
const showKey = ref(false)

const chatModel = ref(localStorage.getItem('playground_chat_model') || 'gpt-4o-mini')
const systemPrompt = ref(localStorage.getItem('playground_system_prompt') || 'You are a helpful assistant.')
const temperature = ref(0.7)
const maxTokens = ref(1024)
const chatInput = ref('')
const chatMessages = ref<ChatMessage[]>([])
const chatLoading = ref(false)
const chatError = ref('')

const imageModel = ref(localStorage.getItem('playground_image_model') || 'gpt-image-1')
const imageSize = ref(localStorage.getItem('playground_image_size') || '1024x1024')
const imageCount = ref(1)
const imagePrompt = ref('')
const generatedImages = ref<string[]>([])
const imageLoading = ref(false)
const imageError = ref('')

const normalizedBaseUrl = computed(() => baseUrl.value.replace(/\/+$/, ''))
const canSendChat = computed(() => Boolean(apiKey.value.trim() && chatModel.value.trim() && chatInput.value.trim() && normalizedBaseUrl.value))
const canGenerateImage = computed(() => Boolean(apiKey.value.trim() && imageModel.value.trim() && imagePrompt.value.trim() && normalizedBaseUrl.value))

watch(apiKey, value => localStorage.setItem('playground_api_key', value))
watch(chatModel, value => localStorage.setItem('playground_chat_model', value))
watch(systemPrompt, value => localStorage.setItem('playground_system_prompt', value))
watch(imageModel, value => localStorage.setItem('playground_image_model', value))
watch(imageSize, value => localStorage.setItem('playground_image_size', value))
watch(baseUrl, value => localStorage.setItem('playground_base_url', value))

onMounted(async () => {
  const savedBaseUrl = localStorage.getItem('playground_base_url')
  try {
    publicSettings.value = await authAPI.getPublicSettings()
  } catch {
    publicSettings.value = null
  }
  baseUrl.value = savedBaseUrl || publicSettings.value?.api_base_url || window.location.origin
})

function buildUrl(path: string): string {
  return `${normalizedBaseUrl.value}${path}`
}

function extractErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  return '请求失败，请检查 Base URL、API Key、模型名或服务端日志。'
}

async function requestJson(url: string, body: Record<string, unknown>): Promise<unknown> {
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${apiKey.value.trim()}`,
    },
    body: JSON.stringify(body),
  })

  const text = await response.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }

  if (!response.ok) {
    const message =
      typeof data === 'object' && data !== null && 'error' in data
        ? JSON.stringify((data as { error: unknown }).error)
        : typeof data === 'object' && data !== null && 'message' in data
          ? String((data as { message: unknown }).message)
          : text || `HTTP ${response.status}`
    throw new Error(message)
  }

  return data
}

async function sendChat() {
  if (!canSendChat.value || chatLoading.value) return
  chatError.value = ''
  chatLoading.value = true
  const userMessage = chatInput.value.trim()
  chatMessages.value.push({ role: 'user', content: userMessage })
  chatInput.value = ''

  try {
    const messages = [
      ...(systemPrompt.value.trim() ? [{ role: 'system', content: systemPrompt.value.trim() }] : []),
      ...chatMessages.value.map(message => ({ role: message.role, content: message.content })),
    ]
    const data = await requestJson(buildUrl('/v1/chat/completions'), {
      model: chatModel.value.trim(),
      messages,
      temperature: temperature.value,
      max_tokens: maxTokens.value || undefined,
    })

    const content =
      typeof data === 'object' && data !== null && 'choices' in data
        ? ((data as { choices?: Array<{ message?: { content?: string }, text?: string }> }).choices?.[0]?.message?.content ||
          (data as { choices?: Array<{ message?: { content?: string }, text?: string }> }).choices?.[0]?.text ||
          JSON.stringify(data, null, 2))
        : String(data)
    chatMessages.value.push({ role: 'assistant', content })
  } catch (error) {
    chatError.value = extractErrorMessage(error)
  } finally {
    chatLoading.value = false
  }
}

function clearChat() {
  chatMessages.value = []
  chatError.value = ''
}

async function generateImage() {
  if (!canGenerateImage.value || imageLoading.value) return
  imageError.value = ''
  imageLoading.value = true

  try {
    const data = await requestJson(buildUrl('/v1/images/generations'), {
      model: imageModel.value.trim(),
      prompt: imagePrompt.value.trim(),
      size: imageSize.value,
      n: Math.max(1, Math.min(4, Number(imageCount.value) || 1)),
    })

    const items = typeof data === 'object' && data !== null && 'data' in data
      ? (data as { data?: Array<{ url?: string, b64_json?: string }> }).data || []
      : []

    const images = items
      .map(item => item.url || (item.b64_json ? `data:image/png;base64,${item.b64_json}` : ''))
      .filter(Boolean)

    if (images.length === 0) {
      throw new Error('接口已返回，但没有找到图片 URL 或 b64_json。')
    }
    generatedImages.value = images
  } catch (error) {
    imageError.value = extractErrorMessage(error)
  } finally {
    imageLoading.value = false
  }
}
</script>
