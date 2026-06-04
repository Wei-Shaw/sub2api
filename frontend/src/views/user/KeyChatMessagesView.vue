<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-7xl flex-col gap-5 px-6 py-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div class="text-sm text-gray-500 dark:text-dark-400">API Key #{{ apiKeyId }}</div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">聊天记录</h1>
          <div class="mt-2 text-sm text-gray-500 dark:text-dark-400">
            按 session 查看历史记录，每个 session 显示最近 {{ detailLimit }} 条 message
          </div>
        </div>
        <div class="flex items-center gap-3">
          <button class="btn btn-secondary" @click="goBack">返回 API Keys</button>
          <button class="btn btn-secondary" :disabled="sessionsLoading || detailLoading" @click="refresh">
            刷新
          </button>
        </div>
      </div>

      <div class="grid gap-5 lg:grid-cols-[360px_minmax(0,1fr)]">
        <section class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
          <div class="border-b border-gray-200 px-4 py-3 dark:border-dark-700">
            <div class="text-sm font-medium text-gray-900 dark:text-white">历史 Session</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">
              共 {{ sessionPagination.total }} 条
            </div>
          </div>

          <div
            v-if="sessionsLoading"
            class="p-4 text-sm text-gray-500 dark:text-dark-400"
          >
            正在加载 session...
          </div>
          <div
            v-else-if="sessions.length === 0"
            class="p-4 text-sm text-gray-500 dark:text-dark-400"
          >
            当前 API Key 暂无可展示的聊天记录。
          </div>

          <div v-else class="divide-y divide-gray-200 dark:divide-dark-700">
            <button
              v-for="session in sessions"
              :key="session.id"
              type="button"
              :class="[
                'block w-full px-4 py-3 text-left transition',
                session.id === selectedSessionId
                  ? 'bg-primary-50 dark:bg-primary-900/20'
                  : 'hover:bg-gray-50 dark:hover:bg-dark-800'
              ]"
              @click="selectSession(session)"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium text-gray-900 dark:text-white">
                    Session #{{ session.id }}
                  </div>
                  <div class="mt-1 truncate text-xs text-gray-500 dark:text-dark-400">
                    {{ session.requested_model || session.model || 'unknown model' }}
                  </div>
                </div>
                <span
                  class="shrink-0 rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-300"
                >
                  {{ session.message_count }}
                </span>
              </div>
              <div class="mt-2 text-xs text-gray-500 dark:text-dark-400">
                {{ formatDateTime(session.created_at) }}
              </div>
              <div
                v-if="session.user_preview || session.assistant_preview"
                class="mt-2 line-clamp-2 text-xs text-gray-600 dark:text-dark-300"
              >
                {{ session.user_preview || session.assistant_preview }}
              </div>
            </button>
          </div>

          <Pagination
            v-if="sessionPagination.total > sessionPagination.page_size"
            :page="sessionPagination.page"
            :total="sessionPagination.total"
            :page-size="sessionPagination.page_size"
            :show-page-size-selector="false"
            @update:page="handleSessionPageChange"
            @update:page-size="handleSessionPageSizeChange"
          />
        </section>

        <section class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
          <div class="border-b border-gray-200 px-4 py-3 dark:border-dark-700">
            <div class="text-sm font-medium text-gray-900 dark:text-white">
              <span v-if="selectedSessionId">Session #{{ selectedSessionId }}</span>
              <span v-else>Session Messages</span>
            </div>
            <div v-if="sessionMeta" class="mt-1 text-xs text-gray-500 dark:text-dark-400">
              {{ sessionMeta.requested_model || sessionMeta.model || 'unknown model' }} ·
              {{ formatDateTime(sessionMeta.created_at) }} ·
              {{ sessionMeta.message_count }} 条
            </div>
          </div>

          <div
            v-if="detailLoading"
            class="p-5 text-sm text-gray-500 dark:text-dark-400"
          >
            正在加载消息...
          </div>
          <div
            v-else-if="!selectedSessionId"
            class="p-5 text-sm text-gray-500 dark:text-dark-400"
          >
            请选择一个 session。
          </div>
          <div
            v-else-if="messages.length === 0"
            class="p-5 text-sm text-gray-500 dark:text-dark-400"
          >
            当前 session 暂无可展示的 message。
          </div>

          <div v-else class="space-y-4 p-4">
            <div
              v-for="message in messages"
              :key="message.id"
              :class="[
                'rounded-lg border p-4',
                message.direction === 'inbound'
                  ? 'border-blue-200 bg-blue-50 dark:border-blue-900/50 dark:bg-blue-950/20'
                  : 'border-emerald-200 bg-emerald-50 dark:border-emerald-900/50 dark:bg-emerald-950/20'
              ]"
            >
              <div class="mb-2 flex items-center justify-between gap-3 text-xs text-gray-500 dark:text-dark-400">
                <span>{{ message.direction === 'inbound' ? 'User' : 'Assistant' }}</span>
                <span>{{ formatDateTime(message.created_at) }}</span>
              </div>
              <div class="space-y-3">
                <pre
                  v-if="message.content_text"
                  class="whitespace-pre-wrap break-words font-sans text-sm text-gray-900 dark:text-white"
                >{{ message.content_text }}</pre>
                <button
                  v-if="message.has_content_json && !message.content_json"
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="isMessageLoading(message.id)"
                  @click="loadMessagePayload(message)"
                >
                  {{ isMessageLoading(message.id) ? '加载中...' : `加载 JSON${formatContentJSONSize(message.content_json_bytes)}` }}
                </button>
                <div v-if="extractImageSources(message.content_json).length > 0" class="grid gap-3 sm:grid-cols-2">
                  <a
                    v-for="(src, index) in extractImageSources(message.content_json)"
                    :key="`${message.id}-image-${index}`"
                    :href="src"
                    target="_blank"
                    rel="noreferrer"
                    class="block overflow-hidden rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-950"
                  >
                    <img
                      :src="src"
                      alt=""
                      class="max-h-96 w-full object-contain"
                      loading="lazy"
                    />
                  </a>
                </div>
                <details
                  v-if="message.content_json"
                  class="rounded-md border border-gray-200 bg-white/70 dark:border-dark-700 dark:bg-dark-950/70"
                >
                  <summary class="cursor-pointer px-3 py-2 text-xs font-medium text-gray-600 dark:text-dark-300">
                    JSON
                  </summary>
                  <pre class="max-h-[520px] overflow-auto whitespace-pre-wrap break-words px-3 pb-3 font-mono text-xs text-gray-800 dark:text-dark-100">{{ renderJSON(message.content_json) }}</pre>
                </details>
              </div>
            </div>
          </div>

          <Pagination
            v-if="messagePagination.total > messagePagination.page_size"
            :page="messagePagination.page"
            :total="messagePagination.total"
            :page-size="messagePagination.page_size"
            :show-page-size-selector="false"
            @update:page="handleMessagePageChange"
            @update:page-size="handleMessagePageSizeChange"
          />
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import type { ChatMessage, ChatSession } from '@/types'
import { keysAPI } from '@/api'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const apiKeyId = computed(() => Number(route.params.id))
const adminUserId = computed(() => {
  const value = route.query.user_id
  const raw = Array.isArray(value) ? value[0] : value
  const parsed = Number(raw)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
})
const adminMode = computed(() => adminUserId.value !== null)
const sessionsLoading = ref(false)
const detailLoading = ref(false)
const selectedSessionId = ref<number | null>(null)
const sessionMeta = ref<ChatSession | null>(null)
const sessions = ref<ChatSession[]>([])
const messages = ref<ChatMessage[]>([])
const messagePayloadLoading = ref<Set<number>>(new Set())
const detailLimit = computed(() => messagePagination.page_size)
const sessionPagination = reactive({
  page: 1,
  page_size: 10,
  total: 0,
  pages: 0
})
const messagePagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
})

async function fetchSessions(page = sessionPagination.page, pageSize = sessionPagination.page_size) {
  return adminMode.value && adminUserId.value
    ? adminAPI.apiKeys.listChatSessions(apiKeyId.value, adminUserId.value, page, pageSize)
    : keysAPI.listChatSessions(apiKeyId.value, page, pageSize)
}

async function fetchSessionDetail(sessionId: number) {
  return adminMode.value && adminUserId.value
    ? adminAPI.apiKeys.getChatSession(apiKeyId.value, adminUserId.value, sessionId, messagePagination.page, messagePagination.page_size)
    : keysAPI.getChatSession(apiKeyId.value, sessionId, messagePagination.page, messagePagination.page_size)
}

async function fetchMessagePayload(sessionId: number, messageId: number) {
  return adminMode.value && adminUserId.value
    ? adminAPI.apiKeys.getChatMessage(apiKeyId.value, adminUserId.value, sessionId, messageId)
    : keysAPI.getChatMessage(apiKeyId.value, sessionId, messageId)
}

async function loadSessions(selectFirst = false) {
  sessionsLoading.value = true
  try {
    const response = await fetchSessions()
    sessions.value = response.items || []
    sessionPagination.total = response.total || 0
    sessionPagination.page = response.page || sessionPagination.page
    sessionPagination.page_size = response.page_size || sessionPagination.page_size
    sessionPagination.pages = response.pages || 0

    const selectedStillVisible = selectedSessionId.value != null &&
      sessions.value.some((session) => session.id === selectedSessionId.value)
    if ((selectFirst || !selectedStillVisible) && sessions.value.length > 0) {
      await selectSession(sessions.value[0])
    } else if (sessions.value.length === 0) {
      selectedSessionId.value = null
      sessionMeta.value = null
      messages.value = []
    }
  } catch (error: any) {
    sessions.value = []
    sessionPagination.total = 0
    sessionPagination.pages = 0
    selectedSessionId.value = null
    sessionMeta.value = null
    messages.value = []
    appStore.showError(error?.message || '加载 session 列表失败')
  } finally {
    sessionsLoading.value = false
  }
}

async function selectSession(session: ChatSession) {
  if (!session || detailLoading.value) return
  selectedSessionId.value = session.id
  sessionMeta.value = session
  messagePagination.page = 1
  await loadSelectedSessionMessages()
}

async function loadSelectedSessionMessages() {
  if (!selectedSessionId.value) return
  detailLoading.value = true
  try {
    const detail = await fetchSessionDetail(selectedSessionId.value)
    sessionMeta.value = detail
    const page = detail.messages_page
    messages.value = page?.items || detail.messages || []
    messagePagination.total = page?.total || detail.message_count || 0
    messagePagination.page = page?.page || messagePagination.page
    messagePagination.page_size = page?.page_size || messagePagination.page_size
    messagePagination.pages = page?.pages || 0
    messagePayloadLoading.value = new Set()
  } catch (error: any) {
    messages.value = []
    messagePagination.total = 0
    messagePagination.pages = 0
    appStore.showError(error?.message || '加载 session messages 失败')
  } finally {
    detailLoading.value = false
  }
}

async function loadMessagePayload(message: ChatMessage) {
  if (!message || !selectedSessionId.value || isMessageLoading(message.id)) return
  const next = new Set(messagePayloadLoading.value)
  next.add(message.id)
  messagePayloadLoading.value = next
  try {
    const detail = await fetchMessagePayload(selectedSessionId.value, message.id)
    messages.value = messages.value.map((item) => item.id === message.id ? { ...item, ...detail } : item)
  } catch (error: any) {
    appStore.showError(error?.message || '加载 message JSON 失败')
  } finally {
    const done = new Set(messagePayloadLoading.value)
    done.delete(message.id)
    messagePayloadLoading.value = done
  }
}

function isMessageLoading(messageId: number) {
  return messagePayloadLoading.value.has(messageId)
}

function formatContentJSONSize(bytes?: number) {
  if (!bytes || bytes <= 0) return ''
  if (bytes < 1024) return ` (${bytes} B)`
  if (bytes < 1024 * 1024) return ` (${(bytes / 1024).toFixed(1)} KB)`
  return ` (${(bytes / 1024 / 1024).toFixed(1)} MB)`
}

async function refresh() {
  await loadSessions(false)
  const selected = selectedSessionId.value
  if (selected != null && sessions.value.some((session) => session.id === selected)) {
    const meta = sessions.value.find((session) => session.id === selected)
    if (meta) await selectSession(meta)
  }
}

async function handleSessionPageChange(page: number) {
  sessionPagination.page = page
  await loadSessions(true)
}

async function handleSessionPageSizeChange(pageSize: number) {
  sessionPagination.page_size = pageSize
  sessionPagination.page = 1
  await loadSessions(true)
}

async function handleMessagePageChange(page: number) {
  messagePagination.page = page
  await loadSelectedSessionMessages()
}

async function handleMessagePageSizeChange(pageSize: number) {
  messagePagination.page_size = pageSize
  messagePagination.page = 1
  await loadSelectedSessionMessages()
}

function renderJSON(value: unknown) {
  return JSON.stringify(value || {}, null, 2)
}

function extractImageSources(value: unknown) {
  const sources: string[] = []
  const seen = new Set<string>()
  const visit = (item: unknown) => {
    if (typeof item === 'string') {
      const src = normalizeImageSource(item)
      if (src && !seen.has(src)) {
        seen.add(src)
        sources.push(src)
      }
      return
    }
    if (Array.isArray(item)) {
      item.forEach(visit)
      return
    }
    if (!item || typeof item !== 'object') return
    const record = item as Record<string, unknown>
    if (record.type === 'image_generation_call' && typeof record.result === 'string') {
      const mime = typeof record.output_format === 'string' ? `image/${record.output_format}` : 'image/png'
      const src = normalizeImageSource(record.result, mime)
      if (src && !seen.has(src)) {
        seen.add(src)
        sources.push(src)
      }
    }
    Object.values(record).forEach(visit)
  }
  visit(value)
  return sources
}

function normalizeImageSource(value: string, fallbackMime = 'image/png') {
  const trimmed = value.trim()
  if (!trimmed) return ''
  if (/^data:image\/[a-z0-9.+-]+;base64,/i.test(trimmed)) return trimmed
  if (/^https?:\/\//i.test(trimmed) && /\.(png|jpe?g|gif|webp|avif)(\?|#|$)/i.test(trimmed)) return trimmed
  if (/^[A-Za-z0-9+/=_-]{80,}$/.test(trimmed)) {
    return `data:${fallbackMime};base64,${trimmed}`
  }
  return ''
}

function goBack() {
  router.push(adminMode.value ? { name: 'AdminUsers' } : { name: 'Keys' })
}

onMounted(() => {
  loadSessions(true)
})
</script>
