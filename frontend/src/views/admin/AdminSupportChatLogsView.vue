<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- 顶部过滤栏 -->
      <div class="card p-4">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-6">
          <div>
            <label class="input-label">{{ t('admin.supportChatLogs.filter.status') }}</label>
            <select v-model="filter.status" class="input mt-1" @change="reloadFromFirstPage">
              <option value="">{{ t('admin.supportChatLogs.filter.statusAll') }}</option>
              <option v-for="s in STATUSES" :key="s" :value="s">
                {{ t('admin.supportChatLogs.status.' + s) }}
              </option>
            </select>
          </div>
          <div>
            <label class="input-label">{{ t('admin.supportChatLogs.filter.userId') }}</label>
            <input
              v-model.number="filter.user_id"
              type="number"
              min="0"
              :placeholder="t('admin.supportChatLogs.filter.userIdPlaceholder')"
              class="input mt-1"
              @keyup.enter="reloadFromFirstPage"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.supportChatLogs.filter.ip') }}</label>
            <input
              v-model.trim="filter.ip"
              type="text"
              :placeholder="t('admin.supportChatLogs.filter.ipPlaceholder')"
              class="input mt-1"
              @keyup.enter="reloadFromFirstPage"
            />
          </div>
          <div class="lg:col-span-3">
            <label class="input-label">{{ t('admin.supportChatLogs.filter.keyword') }}</label>
            <input
              v-model.trim="filter.q"
              type="text"
              maxlength="200"
              :placeholder="t('admin.supportChatLogs.filter.keywordPlaceholder')"
              class="input mt-1"
              @keyup.enter="reloadFromFirstPage"
            />
          </div>
        </div>
        <div class="mt-3 flex items-center justify-end gap-2">
          <button class="btn btn-secondary" @click="resetFilter">
            {{ t('admin.supportChatLogs.filter.reset') }}
          </button>
          <button class="btn btn-primary" :disabled="loading" @click="reloadFromFirstPage">
            <Icon name="search" size="sm" class="mr-1" />
            {{ t('common.search') }}
          </button>
          <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="fetchList">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>
      <!-- 表格 -->
      <div class="card overflow-hidden">
        <div v-if="loading && rows.length === 0" class="flex items-center justify-center py-16">
          <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
        </div>
        <div v-else-if="rows.length === 0" class="empty-state py-16">
          <div class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-800">
            <Icon name="inbox" size="xl" class="text-gray-400 dark:text-dark-500" />
          </div>
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.supportChatLogs.empty') }}</p>
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="th-cell">#</th>
                <th class="th-cell">{{ t('admin.supportChatLogs.col.user') }}</th>
                <th class="th-cell">{{ t('admin.supportChatLogs.col.ip') }}</th>
                <th class="th-cell">{{ t('admin.supportChatLogs.col.turns') }}</th>
                <th class="th-cell">{{ t('admin.supportChatLogs.col.status') }}</th>
                <th class="th-cell">{{ t('admin.supportChatLogs.col.lastAt') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
              <tr
                v-for="row in rows"
                :key="row.id"
                class="cursor-pointer transition hover:bg-gray-50 dark:hover:bg-dark-800/60"
                @click="openDetail(row.id)"
              >
                <td class="td-cell font-mono text-xs text-gray-500 dark:text-dark-400">#{{ row.id }}</td>
                <td class="td-cell font-mono text-xs text-gray-700 dark:text-dark-200">
                  {{ row.user_id ?? t('admin.supportChatLogs.anonymous') }}
                </td>
                <td class="td-cell font-mono text-xs text-gray-500 dark:text-dark-400">{{ row.client_ip || '-' }}</td>
                <td class="td-cell text-sm text-gray-700 dark:text-dark-200">{{ row.turn_count }}</td>
                <td class="td-cell"><span :class="statusClass(row.last_status)">{{ statusLabel(row.last_status) }}</span></td>
                <td class="td-cell text-sm text-gray-500 dark:text-dark-400">
                  {{ row.last_at ? formatDateTime(row.last_at) : '-' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="pagination.total > 0"
          :total="pagination.total"
          :page="pagination.page"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </div>
      <!-- 详情 Dialog：整段对话时间线 -->
      <BaseDialog
        :show="detailOpen"
        :title="t('admin.supportChatLogs.detail.title')"
        width="extra-wide"
        @close="closeDetail"
      >
        <div v-if="detailLoading" class="flex items-center justify-center py-16">
          <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
        </div>
        <div v-else-if="detail" class="space-y-5">
          <section>
            <div class="rounded-xl bg-gray-50 p-4 text-sm dark:bg-dark-800">
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <span class="text-gray-500 dark:text-dark-400">{{ t('admin.supportChatLogs.col.user') }}：</span>
                  <span class="font-mono text-gray-900 dark:text-white">
                    {{ detail.user_id ?? t('admin.supportChatLogs.anonymous') }}
                  </span>
                </div>
                <div>
                  <span class="text-gray-500 dark:text-dark-400">{{ t('admin.supportChatLogs.col.ip') }}：</span>
                  <span class="font-mono text-gray-900 dark:text-white">{{ detail.client_ip || '-' }}</span>
                </div>
                <div>
                  <span class="text-gray-500 dark:text-dark-400">{{ t('admin.supportChatLogs.col.turns') }}：</span>
                  <span class="text-gray-900 dark:text-white">{{ detail.turn_count }}</span>
                </div>
                <div>
                  <span class="text-gray-500 dark:text-dark-400">{{ t('admin.supportChatLogs.col.status') }}：</span>
                  <span :class="statusClass(detail.last_status)">{{ statusLabel(detail.last_status) }}</span>
                </div>
              </div>
            </div>
          </section>
          <section>
            <ul v-if="detail.messages.length > 0" class="space-y-3">
              <li
                v-for="m in detail.messages"
                :key="m.id"
                class="flex gap-3"
                :class="m.role === 'assistant' ? 'flex-row-reverse' : 'flex-row'"
              >
                <div
                  class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full"
                  :class="m.role === 'assistant' ? 'bg-primary-100 text-primary-600 dark:bg-primary-900/40 dark:text-primary-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'"
                >
                  <Icon :name="m.role === 'assistant' ? 'shield' : 'user'" size="sm" />
                </div>
                <div class="max-w-[80%] flex-1 rounded-2xl bg-gray-50 px-3 py-2 dark:bg-dark-800">
                  <div class="mb-1 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
                    <span>{{ m.role === 'assistant' ? t('admin.supportChatLogs.detail.roleAssistant') : t('admin.supportChatLogs.detail.roleUser') }}</span>
                    <span v-if="m.status" :class="statusClass(m.status)">{{ statusLabel(m.status) }}</span>
                    <span>{{ formatDateTime(m.created_at) }}</span>
                  </div>
                  <pre class="whitespace-pre-wrap break-words font-sans text-sm text-gray-900 dark:text-white">{{ m.content || '—' }}</pre>
                  <div v-if="m.error_message" class="mt-1 text-xs text-red-500">
                    {{ t('admin.supportChatLogs.detail.error') }}: {{ m.error_message }}
                  </div>
                  <div v-if="m.model || m.latency_ms != null" class="mt-1 flex gap-3 text-xs text-gray-400 dark:text-dark-500">
                    <span v-if="m.model">{{ t('admin.supportChatLogs.detail.model') }}: {{ m.model }}</span>
                    <span v-if="m.latency_ms != null">{{ t('admin.supportChatLogs.detail.latency') }}: {{ m.latency_ms }}ms</span>
                  </div>
                </div>
              </li>
            </ul>
            <p v-else class="py-8 text-center text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.supportChatLogs.detail.empty') }}
            </p>
          </section>
        </div>
      </BaseDialog>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
/**
 * AdminSupportChatLogsView —— admin 端客服对话记录（只读审计）。
 *
 * - 过滤栏：status / user_id / ip / q（消息正文 ILIKE）。
 * - 列表：会话头（不含正文），点击行打开详情看整段消息时间线。
 * - 纯只读：不提供介入/回复（要人工介入走工单）。
 * - 不卡 feature_disabled：sidebar 入口由 support_chat_enabled 控制，路由可达即允许查看。
 */
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import {
  adminGetChatConversation,
  adminListChatConversations,
  type AdminChatLogFilter,
  type ChatConversationDetail,
  type ChatConversationListItem,
} from '@/api/support'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()

const STATUSES = [
  'success',
  'upstream_auth',
  'upstream_error',
  'interrupted',
  'rate_limited',
  'config_error',
] as const

const loading = ref(false)
const rows = ref<ChatConversationListItem[]>([])
const pagination = reactive({ page: 1, page_size: 20, total: 0 })

interface FilterModel {
  status: string
  user_id: number | null
  ip: string
  q: string
}
const initialFilter = (): FilterModel => ({ status: '', user_id: null, ip: '', q: '' })
const filter = reactive<FilterModel>(initialFilter())

function statusLabel(s: string): string {
  return STATUSES.includes(s as (typeof STATUSES)[number])
    ? t('admin.supportChatLogs.status.' + s)
    : s || '-'
}

// 状态徽章配色：success 绿；限流/配置/中断 黄；上游/鉴权错误 红。
function statusClass(s: string): string {
  const base = 'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium'
  switch (s) {
    case 'success':
      return `${base} bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300`
    case 'upstream_auth':
    case 'upstream_error':
      return `${base} bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300`
    case 'rate_limited':
    case 'config_error':
    case 'interrupted':
      return `${base} bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300`
    default:
      return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200`
  }
}

function buildFilterPayload(): AdminChatLogFilter {
  const out: AdminChatLogFilter = {}
  if (filter.status) out.status = filter.status
  if (typeof filter.user_id === 'number' && filter.user_id > 0) out.user_id = filter.user_id
  if (filter.ip && filter.ip.trim() !== '') out.ip = filter.ip.trim()
  if (filter.q && filter.q.trim() !== '') out.q = filter.q.trim()
  return out
}

async function fetchList() {
  loading.value = true
  try {
    const res = await adminListChatConversations(buildFilterPayload(), pagination.page, pagination.page_size)
    rows.value = res.items || []
    pagination.total = res.total || 0
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'admin.supportChatLogs', t('admin.supportChatLogs.loadFailed')))
    rows.value = []
    pagination.total = 0
  } finally {
    loading.value = false
  }
}

function reloadFromFirstPage() {
  pagination.page = 1
  fetchList()
}
function resetFilter() {
  Object.assign(filter, initialFilter())
  reloadFromFirstPage()
}
function handlePageChange(p: number) {
  pagination.page = p
  fetchList()
}
function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  fetchList()
}

const detailOpen = ref(false)
const detailLoading = ref(false)
const detail = ref<ChatConversationDetail | null>(null)

async function openDetail(id: number) {
  detailOpen.value = true
  detailLoading.value = true
  detail.value = null
  try {
    detail.value = await adminGetChatConversation(id)
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'admin.supportChatLogs', t('admin.supportChatLogs.loadFailed')))
    detailOpen.value = false
  } finally {
    detailLoading.value = false
  }
}
function closeDetail() {
  detailOpen.value = false
  detail.value = null
}

onMounted(fetchList)
</script>

