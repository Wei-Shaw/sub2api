<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-5xl flex-col gap-5 px-6 py-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div class="text-sm text-gray-500 dark:text-dark-400">API Key #{{ apiKeyId }}</div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">聊天记录</h1>
          <div class="mt-2 text-sm text-gray-500 dark:text-dark-400">
            显示最近一个 session 的 {{ detailLimit }} 条 message
          </div>
        </div>
        <div class="flex items-center gap-3">
          <button class="btn btn-secondary" @click="goBack">返回 API Keys</button>
          <button class="btn btn-secondary" :disabled="loading" @click="loadLatestSessionMessages">
            刷新
          </button>
        </div>
      </div>

      <div
        v-if="loading"
        class="rounded-lg border border-gray-200 bg-white p-5 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400"
      >
        正在加载消息...
      </div>

      <div
        v-else-if="messages.length === 0"
        class="rounded-lg border border-gray-200 bg-white p-5 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400"
      >
        当前 API Key 暂无可展示的聊天记录。
      </div>

      <section
        v-else
        class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900"
      >
        <div class="border-b border-gray-200 px-4 py-3 dark:border-dark-700">
          <div class="text-sm font-medium text-gray-900 dark:text-white">
            Session #{{ selectedSessionId }}
          </div>
          <div v-if="sessionMeta" class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ sessionMeta.requested_model || sessionMeta.model || 'unknown model' }} ·
            {{ formatDateTime(sessionMeta.created_at) }} ·
            {{ sessionMeta.message_count }} 条
          </div>
        </div>

        <div class="space-y-4 p-4">
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
            <pre class="whitespace-pre-wrap break-words font-sans text-sm text-gray-900 dark:text-white">{{ renderMessage(message) }}</pre>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
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
const loading = ref(false)
const selectedSessionId = ref<number | null>(null)
const sessionMeta = ref<ChatSession | null>(null)
const messages = ref<ChatMessage[]>([])
const detailLimit = 80

async function loadLatestSessionMessages() {
  loading.value = true
  try {
    const sessions = adminMode.value && adminUserId.value
      ? await adminAPI.apiKeys.listChatSessions(apiKeyId.value, adminUserId.value, 1, 1)
      : await keysAPI.listChatSessions(apiKeyId.value, 1, 1)

    const latest = sessions.items?.[0]
    if (!latest) {
      selectedSessionId.value = null
      sessionMeta.value = null
      messages.value = []
      return
    }

    selectedSessionId.value = latest.id
    sessionMeta.value = latest

    const detail = adminMode.value && adminUserId.value
      ? await adminAPI.apiKeys.getChatSession(apiKeyId.value, adminUserId.value, latest.id, detailLimit)
      : await keysAPI.getChatSession(apiKeyId.value, latest.id, detailLimit)
    messages.value = detail.messages || []
  } catch (error: any) {
    messages.value = []
    appStore.showError(error?.message || '加载 session messages 失败')
  } finally {
    loading.value = false
  }
}

function renderMessage(message: ChatMessage) {
  if (message.content_text) return message.content_text
  return JSON.stringify(message.content_json || {}, null, 2)
}

function goBack() {
  router.push(adminMode.value ? { name: 'AdminUsers' } : { name: 'Keys' })
}

onMounted(() => {
  loadLatestSessionMessages()
})
</script>
