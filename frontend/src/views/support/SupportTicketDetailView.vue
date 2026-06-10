<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <!-- 顶部返回 -->
      <button
        type="button"
        class="inline-flex items-center text-sm font-medium text-gray-500 hover:text-primary-600 dark:text-dark-400 dark:hover:text-primary-400"
        @click="goBack"
      >
        <Icon name="arrowLeft" size="sm" class="mr-1" />
        {{ t('support.common.backToList') }}
      </button>

      <!-- 加载中 -->
      <div v-if="loading && !ticket" class="card flex items-center justify-center p-16">
        <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          />
        </svg>
      </div>

      <!-- 加载失败（404 / 无权限） -->
      <div v-else-if="!ticket" class="empty-state card p-16">
        <div class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-red-100 dark:bg-red-900/30">
          <Icon name="exclamationCircle" size="xl" class="text-red-600 dark:text-red-400" />
        </div>
        <p class="text-base font-medium text-gray-900 dark:text-white">
          {{ t('support.detail.notFound') }}
        </p>
      </div>

      <template v-else>
        <!-- 工单元信息卡片 -->
        <div class="card p-6">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
                <span class="font-mono">#{{ ticket.id }}</span>
                <span>·</span>
                <span>{{ ticket.category }}</span>
                <span>·</span>
                <span>{{ formatDateTime(ticket.created_at) }}</span>
              </div>
              <h1 class="mt-2 break-words text-xl font-bold text-gray-900 dark:text-white">
                {{ ticket.title }}
              </h1>
              <div class="mt-3 flex items-center gap-2">
                <SupportStatusBadge :status="ticket.status" />
                <SupportPriorityBadge :priority="ticket.priority" />
                <span
                  v-if="ticket.closed_at"
                  class="text-xs text-gray-500 dark:text-dark-400"
                >
                  {{ t('support.common.closedAt') }}: {{ formatDateTime(ticket.closed_at) }}
                </span>
              </div>
            </div>

            <!-- 关闭工单按钮（仅未关闭时显示） -->
            <button
              v-if="ticket.status !== 'closed'"
              class="btn btn-secondary"
              :disabled="closing"
              @click="closeConfirmOpen = true"
            >
              <Icon name="x" size="sm" class="mr-1" />
              {{ t('support.common.closeTicket') }}
            </button>
          </div>

          <!-- 原始描述（用户提交时的 content） -->
          <div class="mt-5 border-t border-gray-100 pt-5 dark:border-dark-700">
            <h2 class="mb-2 text-sm font-semibold text-gray-700 dark:text-dark-200">
              {{ t('support.common.content') }}
            </h2>
            <pre
              class="whitespace-pre-wrap break-words rounded-xl bg-gray-50 p-4 font-mono text-sm text-gray-800 dark:bg-dark-800 dark:text-dark-100"
            >{{ ticket.content }}</pre>
          </div>

          <!-- chat_context（折叠展示） -->
          <div
            v-if="ticket.chat_context"
            class="mt-4 rounded-xl border border-primary-200 bg-primary-50 dark:border-primary-800/50 dark:bg-primary-900/20"
          >
            <button
              type="button"
              class="flex w-full items-center justify-between px-4 py-3 text-sm font-medium text-primary-800 dark:text-primary-200"
              @click="contextExpanded = !contextExpanded"
            >
              <span class="inline-flex items-center gap-2">
                <Icon name="document" size="sm" />
                {{ t('support.common.chatContextLabel') }}
              </span>
              <Icon :name="contextExpanded ? 'chevronUp' : 'chevronDown'" size="sm" />
            </button>
            <div v-if="contextExpanded" class="border-t border-primary-200 px-4 py-3 dark:border-primary-800/50">
              <pre
                class="max-h-96 overflow-auto whitespace-pre-wrap break-words font-mono text-xs text-primary-900 dark:text-primary-100"
              >{{ ticket.chat_context }}</pre>
            </div>
          </div>
        </div>

        <!-- 回复时间线 -->
        <div class="card p-6">
          <h2 class="mb-4 text-sm font-semibold text-gray-700 dark:text-dark-200">
            {{ t('support.common.timeline') }}
          </h2>

          <div v-if="ticket.replies.length === 0" class="empty-state py-10">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('support.common.noResult') }}</p>
          </div>

          <ul v-else class="space-y-4">
            <li
              v-for="reply in ticket.replies"
              :key="reply.id"
              class="flex gap-3"
              :class="reply.is_admin ? 'flex-row' : 'flex-row-reverse'"
            >
              <div
                class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full"
                :class="
                  reply.is_admin
                    ? 'bg-primary-100 text-primary-600 dark:bg-primary-900/40 dark:text-primary-300'
                    : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'
                "
              >
                <Icon :name="reply.is_admin ? 'shield' : 'user'" size="sm" />
              </div>
              <div
                class="max-w-[80%] flex-1 rounded-2xl px-4 py-3"
                :class="
                  reply.is_admin
                    ? 'bg-primary-50 dark:bg-primary-900/20'
                    : 'bg-gray-50 dark:bg-dark-800'
                "
              >
                <div class="flex items-center justify-between gap-3 text-xs text-gray-500 dark:text-dark-400">
                  <span class="font-medium">
                    {{ reply.is_admin ? t('support.common.authorAdmin') : t('support.common.authorUser') }}
                  </span>
                  <span>{{ formatDateTime(reply.created_at) }}</span>
                </div>
                <pre
                  class="mt-2 whitespace-pre-wrap break-words font-sans text-sm text-gray-800 dark:text-dark-100"
                >{{ reply.content }}</pre>
              </div>
            </li>
          </ul>
        </div>

        <!-- 底部回复输入 -->
        <div class="card p-6">
          <h2 class="mb-3 text-sm font-semibold text-gray-700 dark:text-dark-200">
            {{ t('support.common.reply') }}
          </h2>

          <div
            v-if="ticket.status === 'closed'"
            class="rounded-xl border border-gray-200 bg-gray-50 p-4 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300"
          >
            <Icon name="lock" size="sm" class="mr-1 inline" />
            {{ t('support.common.closedHint') }}
          </div>

          <template v-else>
            <textarea
              v-model="replyContent"
              rows="5"
              maxlength="16384"
              :placeholder="t('support.common.placeholderReply')"
              :disabled="replying"
              class="input font-mono text-sm"
            />
            <div class="mt-3 flex items-center justify-between">
              <p class="text-xs text-gray-500 dark:text-dark-400">{{ replyContent.length }} / 16384</p>
              <button
                type="button"
                class="btn btn-primary"
                :disabled="replying || replyContent.trim() === ''"
                @click="handleAppendReply"
              >
                {{ replying ? t('support.common.sending') : t('support.common.sendReply') }}
              </button>
            </div>
          </template>
        </div>
      </template>

      <!-- 关闭确认对话框 -->
      <BaseDialog :show="closeConfirmOpen" :title="t('support.detail.closeConfirmTitle')" width="narrow" @close="closeConfirmOpen = false">
        <p class="text-sm text-gray-600 dark:text-gray-300">
          {{ t('support.detail.closeConfirmDesc') }}
        </p>
        <template #footer>
          <div class="flex justify-end gap-3">
            <button class="btn btn-secondary" :disabled="closing" @click="closeConfirmOpen = false">
              {{ t('common.cancel') }}
            </button>
            <button class="btn btn-danger" :disabled="closing" @click="confirmClose">
              {{ closing ? t('support.common.closing') : t('support.common.closeTicket') }}
            </button>
          </div>
        </template>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * SupportTicketDetailView —— 用户侧工单详情页。
 *
 * 设计要点：
 *
 * 1) 数据流：getMyTicket(id) -> 包含 chat_context + 回复时间线（按 created_at 升序）。
 *    appendReply 与 closeTicket 调用后均整体 reload 详情，保证状态机切换
 *    （open → in_progress、open|in_progress → closed）能即时反映在 UI。
 *
 * 2) 已关闭语义：status = 'closed' 时
 *      - 隐藏"关闭工单"按钮
 *      - 回复输入框替换成只读提示卡
 *      - 用户即使绕过 UI 调 API 也会被后端 SUPPORT_TICKET_CLOSED 拦下（409）
 *
 * 3) 时间线方向：用户消息靠右、客服消息靠左。这里通过 flex-row-reverse 实现，
 *    保留无障碍意义（DOM 顺序仍然是时间正序）。
 *
 * 4) chat_context 折叠展示：默认收起，避免长文挤压回复区。
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import {
  appendReply,
  closeTicket,
  getMyTicket,
  type SupportTicketWithReplies,
} from '@/api/support'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import SupportStatusBadge from '@/components/support/SupportStatusBadge.vue'
import SupportPriorityBadge from '@/components/support/SupportPriorityBadge.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const ticketId = computed(() => {
  const raw = route.params.id
  const id = Array.isArray(raw) ? raw[0] : raw
  const parsed = Number.parseInt(id ?? '', 10)
  return Number.isFinite(parsed) ? parsed : 0
})

const loading = ref(false)
const ticket = ref<SupportTicketWithReplies | null>(null)
const contextExpanded = ref(false)

const replyContent = ref('')
const replying = ref(false)

const closing = ref(false)
const closeConfirmOpen = ref(false)

async function fetchDetail() {
  if (ticketId.value <= 0) {
    ticket.value = null
    return
  }
  loading.value = true
  try {
    ticket.value = await getMyTicket(ticketId.value)
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('support.detail.notFound')))
    ticket.value = null
  } finally {
    loading.value = false
  }
}

async function handleAppendReply() {
  if (!ticket.value || replying.value) return
  const content = replyContent.value.trim()
  if (content === '') return
  replying.value = true
  try {
    await appendReply(ticket.value.id, content)
    appStore.showSuccess(t('support.detail.replySuccess'))
    replyContent.value = ''
    await fetchDetail()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
  } finally {
    replying.value = false
  }
}

async function confirmClose() {
  if (!ticket.value || closing.value) return
  closing.value = true
  try {
    await closeTicket(ticket.value.id)
    appStore.showSuccess(t('support.detail.closeSuccess'))
    closeConfirmOpen.value = false
    await fetchDetail()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
  } finally {
    closing.value = false
  }
}

function goBack() {
  router.push('/support/tickets')
}

onMounted(() => {
  fetchDetail()
})
</script>
