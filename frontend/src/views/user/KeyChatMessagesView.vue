<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-5 px-6 py-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div class="text-sm text-gray-500 dark:text-dark-400">API Key #{{ apiKeyId }}</div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">聊天记录</h1>
          <div class="mt-2 text-sm text-gray-500 dark:text-dark-400">
            按会话分页展示，点选会话查看最近 {{ detailLimit }} 条主消息
          </div>
        </div>
        <div class="flex items-center gap-3">
          <button class="btn btn-secondary" @click="goBack">返回 API Keys</button>
          <button class="btn btn-secondary" :disabled="loadingSessions" @click="loadSessions">
            刷新
          </button>
        </div>
      </div>

      <div v-if="loadingSessions" class="rounded-lg border border-gray-200 bg-white p-5 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400">
        正在加载会话...
      </div>

      <div v-else-if="sessions.length === 0" class="rounded-lg border border-gray-200 bg-white p-5 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400">
        当前 API Key 暂无可展示的聊天记录。
      </div>

      <template v-else>
        <div class="grid gap-4 lg:grid-cols-[minmax(0,420px)_1fr]">
          <section class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <div class="border-b border-gray-200 px-4 py-3 text-sm font-medium text-gray-700 dark:border-dark-700 dark:text-dark-200">
              会话列表
            </div>
            <button
              v-for="session in sessions"
              :key="session.id"
              type="button"
              :class="[
                'block w-full border-b border-gray-100 px-4 py-3 text-left transition-colors last:border-b-0 dark:border-dark-800',
                selectedSessionId === session.id
                  ? 'bg-primary-50 dark:bg-primary-950/30'
                  : 'hover:bg-gray-50 dark:hover:bg-dark-800'
              ]"
              @click="selectSession(session.id)"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium text-gray-900 dark:text-white">
                    {{ session.requested_model || session.model || 'unknown model' }}
                  </div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                    {{ formatDateTime(session.created_at) }} · {{ session.message_count }} 条
                  </div>
                </div>
                <span
                  :class="[
                    'shrink-0 rounded px-2 py-0.5 text-xs',
                    session.http_status_code >= 400
                      ? 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300'
                      : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
                  ]"
                >
                  {{ session.http_status_code }}
                </span>
              </div>
              <div v-if="session.user_preview" class="mt-2 line-clamp-2 text-xs text-gray-600 dark:text-dark-300">
                User: {{ session.user_preview }}
              </div>
              <div v-if="session.assistant_preview" class="mt-1 line-clamp-2 text-xs text-gray-500 dark:text-dark-400">
                Assistant: {{ session.assistant_preview }}
              </div>
            </button>
          </section>

          <section class="min-h-[360px] rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-4 py-3 dark:border-dark-700">
              <div>
                <div class="text-sm font-medium text-gray-900 dark:text-white">
                  {{ selectedSession ? `Session #${selectedSession.id}` : '会话详情' }}
                </div>
                <div v-if="selectedSession" class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                  {{ selectedSession.inbound_endpoint || '-' }} · {{ selectedSession.status }}
                </div>
              </div>
              <button
                class="btn btn-secondary btn-sm"
                :disabled="!selectedSessionId || loadingDetail"
                @click="selectedSessionId && loadSessionDetail(selectedSessionId)"
              >
                刷新详情
              </button>
            </div>

            <div v-if="loadingDetail" class="p-5 text-sm text-gray-500 dark:text-dark-400">
              正在加载消息...
            </div>
            <div v-else-if="messages.length === 0" class="p-5 text-sm text-gray-500 dark:text-dark-400">
              当前会话暂无可展示的主消息。
            </div>
            <div v-else class="space-y-4 p-4">
              <div
                v-for="message in messages"
                :key="message.id"
                :class="[
                  'rounded-lg border p-4',
                  message.direction === 'inbound'
                    ? 'mr-8 border-blue-200 bg-blue-50 dark:border-blue-900/50 dark:bg-blue-950/20'
                    : 'ml-8 border-emerald-200 bg-emerald-50 dark:border-emerald-900/50 dark:bg-emerald-950/20'
                ]"
              >
                <div class="mb-2 flex items-center justify-between gap-3 text-xs uppercase tracking-wide text-gray-500 dark:text-dark-400">
                  <span>{{ message.direction === 'inbound' ? 'User' : 'Assistant' }} / {{ message.role }}</span>
                  <span>{{ formatDateTime(message.created_at) }}</span>
                </div>
                <pre class="whitespace-pre-wrap break-words font-sans text-sm text-gray-900 dark:text-white">{{ renderMessage(message) }}</pre>
              </div>
            </div>
          </section>
        </div>

        <Pagination
          v-if="pagination.total > 0"
          :total="pagination.total"
          :page="pagination.page"
          :page-size="pagination.page_size"
          :show-jump="true"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
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
const loadingSessions = ref(false)
const loadingDetail = ref(false)
const sessions = ref<ChatSession[]>([])
const selectedSessionId = ref<number | null>(null)
const messages = ref<ChatMessage[]>([])
const detailLimit = 80

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
})

const selectedSession = computed(() =>
  sessions.value.find((session) => session.id === selectedSessionId.value) || null
)

async function loadSessions() {
  loadingSessions.value = true
  try {
    const response = adminMode.value && adminUserId.value
      ? await adminAPI.apiKeys.listChatSessions(apiKeyId.value, adminUserId.value, pagination.page, pagination.page_size)
      : await keysAPI.listChatSessions(apiKeyId.value, pagination.page, pagination.page_size)
    sessions.value = response.items || []
    pagination.total = response.total
    pagination.pages = response.pages

    if (sessions.value.length === 0) {
      selectedSessionId.value = null
      messages.value = []
      return
    }

    const stillVisible = sessions.value.some((session) => session.id === selectedSessionId.value)
    const nextSessionId = stillVisible ? selectedSessionId.value : sessions.value[0].id
    if (nextSessionId && nextSessionId !== selectedSessionId.value) {
      selectedSessionId.value = nextSessionId
    }
    if (nextSessionId) {
      await loadSessionDetail(nextSessionId)
    }
  } catch (error: any) {
    appStore.showError(error?.message || '加载聊天会话失败')
  } finally {
    loadingSessions.value = false
  }
}

async function loadSessionDetail(sessionId: number) {
  loadingDetail.value = true
  try {
    const detail = adminMode.value && adminUserId.value
      ? await adminAPI.apiKeys.getChatSession(apiKeyId.value, adminUserId.value, sessionId, detailLimit)
      : await keysAPI.getChatSession(apiKeyId.value, sessionId, detailLimit)
    messages.value = detail.messages || []
  } catch (error: any) {
    messages.value = []
    appStore.showError(error?.message || '加载会话详情失败')
  } finally {
    loadingDetail.value = false
  }
}

function selectSession(sessionId: number) {
  if (selectedSessionId.value === sessionId) return
  selectedSessionId.value = sessionId
  loadSessionDetail(sessionId)
}

function handlePageChange(page: number) {
  pagination.page = page
  loadSessions()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadSessions()
}

function renderMessage(message: ChatMessage) {
  if (message.content_text) return message.content_text
  return JSON.stringify(message.content_json || {}, null, 2)
}

function goBack() {
  router.push(adminMode.value ? { name: 'AdminUsers' } : { name: 'Keys' })
}

onMounted(() => {
  loadSessions()
})
</script>
