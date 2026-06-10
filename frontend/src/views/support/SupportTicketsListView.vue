<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- 顶部操作条：标题 + 新建工单按钮 -->
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('support.list.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
            {{ t('support.list.description') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <button
            class="btn btn-secondary"
            :disabled="loading"
            :title="t('common.refresh')"
            @click="fetchList"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button class="btn btn-primary" @click="goNew">
            <Icon name="plus" size="md" class="mr-1" />
            {{ t('support.list.newButton') }}
          </button>
        </div>
      </div>

      <!-- 工单列表卡片 -->
      <div class="card overflow-hidden">
        <!-- Loading 占位 -->
        <div v-if="loading && tickets.length === 0" class="flex items-center justify-center py-16">
          <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
        </div>

        <!-- 空态 -->
        <div v-else-if="tickets.length === 0" class="empty-state py-16">
          <div
            class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-800"
          >
            <Icon name="inbox" size="xl" class="text-gray-400 dark:text-dark-500" />
          </div>
          <p class="text-base font-medium text-gray-900 dark:text-white">
            {{ t('support.list.empty') }}
          </p>
          <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
            {{ t('support.list.emptyHint') }}
          </p>
        </div>

        <!-- 列表表格 -->
        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="th-cell">#</th>
                <th class="th-cell">{{ t('support.common.title') }}</th>
                <th class="th-cell">{{ t('support.common.category') }}</th>
                <th class="th-cell">{{ t('support.common.status') }}</th>
                <th class="th-cell">{{ t('support.common.priority') }}</th>
                <th class="th-cell">{{ t('support.common.createdAt') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
              <tr
                v-for="row in tickets"
                :key="row.id"
                class="cursor-pointer transition hover:bg-gray-50 dark:hover:bg-dark-800/60"
                @click="goDetail(row.id)"
              >
                <td class="td-cell font-mono text-xs text-gray-500 dark:text-dark-400">
                  #{{ row.id }}
                </td>
                <td class="td-cell">
                  <div class="max-w-md truncate text-sm font-medium text-gray-900 dark:text-white">
                    {{ row.title }}
                  </div>
                </td>
                <td class="td-cell text-sm text-gray-700 dark:text-dark-200">
                  {{ row.category }}
                </td>
                <td class="td-cell">
                  <SupportStatusBadge :status="row.status" />
                </td>
                <td class="td-cell">
                  <SupportPriorityBadge :priority="row.priority" />
                </td>
                <td class="td-cell text-sm text-gray-500 dark:text-dark-400">
                  {{ formatDateTime(row.created_at) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 分页器 -->
        <Pagination
          v-if="pagination.total > 0"
          :total="pagination.total"
          :page="pagination.page"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * SupportTicketsListView —— 用户侧"我的工单"列表页。
 *
 * 设计要点：
 *   - 仅消费 `listMyTickets`（返回的 SupportTicket 类型在编译期已经不含 chat_context）。
 *   - 行级点击跳详情，沿用 RedeemView / UserOrdersView 的 card 与 Pagination 模式。
 *   - feature_disabled 时后端返回 404，由 axios interceptor 抛错，本页通过
 *     extractI18nErrorMessage 翻译成友好提示并停留在空态。
 */
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { listMyTickets, type SupportTicket } from '@/api/support'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import SupportStatusBadge from '@/components/support/SupportStatusBadge.vue'
import SupportPriorityBadge from '@/components/support/SupportPriorityBadge.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const loading = ref(false)
const tickets = ref<SupportTicket[]>([])
const pagination = reactive({ page: 1, page_size: 20, total: 0 })

async function fetchList() {
  loading.value = true
  try {
    const res = await listMyTickets(pagination.page, pagination.page_size)
    tickets.value = res.items || []
    pagination.total = res.total || 0
  } catch (err: unknown) {
    // feature_disabled / 网络错误统一走 i18n 映射
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
    tickets.value = []
    pagination.total = 0
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  fetchList()
}

function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  fetchList()
}

function goNew() {
  router.push('/support/tickets/new')
}

function goDetail(id: number) {
  router.push(`/support/tickets/${id}`)
}

onMounted(() => {
  fetchList()
})
</script>

<style scoped>
.th-cell {
  @apply px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400;
}
.td-cell {
  @apply whitespace-nowrap px-4 py-3;
}
</style>
