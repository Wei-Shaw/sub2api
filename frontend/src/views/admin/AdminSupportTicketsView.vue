<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- 顶部过滤栏 -->
      <div class="card p-4">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-6">
          <!-- status -->
          <div>
            <label class="input-label">{{ t('support.common.status') }}</label>
            <select v-model="filter.status" class="input mt-1" @change="reloadFromFirstPage">
              <option value="">{{ t('admin.tickets.filters.statusAll') }}</option>
              <option value="open">{{ t('support.statusLabel.open') }}</option>
              <option value="in_progress">{{ t('support.statusLabel.in_progress') }}</option>
              <option value="closed">{{ t('support.statusLabel.closed') }}</option>
            </select>
          </div>
          <!-- priority -->
          <div>
            <label class="input-label">{{ t('support.common.priority') }}</label>
            <select v-model="filter.priority" class="input mt-1" @change="reloadFromFirstPage">
              <option value="">{{ t('admin.tickets.filters.priorityAll') }}</option>
              <option value="high">{{ t('support.priorityLabel.high') }}</option>
              <option value="normal">{{ t('support.priorityLabel.normal') }}</option>
              <option value="low">{{ t('support.priorityLabel.low') }}</option>
            </select>
          </div>
          <!-- category -->
          <div>
            <label class="input-label">{{ t('support.common.category') }}</label>
            <select
              v-if="categories.length > 0"
              v-model="filter.category"
              class="input mt-1"
              @change="reloadFromFirstPage"
            >
              <option value="">{{ t('admin.tickets.filters.categoryAll') }}</option>
              <option v-for="cat in categories" :key="cat" :value="cat">{{ cat }}</option>
            </select>
            <input
              v-else
              v-model.trim="filter.category"
              type="text"
              :placeholder="t('admin.tickets.filters.categoryAll')"
              class="input mt-1"
              @keyup.enter="reloadFromFirstPage"
            />
          </div>
          <!-- user_id -->
          <div>
            <label class="input-label">{{ t('admin.tickets.filters.userIdLabel') }}</label>
            <input
              v-model.number="filter.user_id"
              type="number"
              min="0"
              :placeholder="t('admin.tickets.filters.userIdPlaceholder')"
              class="input mt-1"
              @keyup.enter="reloadFromFirstPage"
            />
          </div>
          <!-- q -->
          <div class="sm:col-span-2 lg:col-span-2">
            <label class="input-label">{{ t('admin.tickets.filters.searchLabel') }}</label>
            <input
              v-model.trim="filter.q"
              type="text"
              maxlength="200"
              :placeholder="t('admin.tickets.filters.searchPlaceholder')"
              class="input mt-1"
              @keyup.enter="reloadFromFirstPage"
            />
          </div>
        </div>
        <div class="mt-3 flex items-center justify-end gap-2">
          <button class="btn btn-secondary" @click="resetFilter">
            {{ t('admin.tickets.filters.reset') }}
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

        <div v-else-if="tickets.length === 0" class="empty-state py-16">
          <div class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-800">
            <Icon name="inbox" size="xl" class="text-gray-400 dark:text-dark-500" />
          </div>
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.tickets.empty') }}</p>
        </div>

        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="th-cell">#</th>
                <th class="th-cell">{{ t('admin.tickets.drawer.userId') }}</th>
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
                @click="openDrawer(row.id)"
              >
                <td class="td-cell font-mono text-xs text-gray-500 dark:text-dark-400">#{{ row.id }}</td>
                <td class="td-cell font-mono text-xs text-gray-700 dark:text-dark-200">{{ row.user_id }}</td>
                <td class="td-cell">
                  <div class="max-w-md truncate text-sm font-medium text-gray-900 dark:text-white">
                    {{ row.title }}
                  </div>
                </td>
                <td class="td-cell text-sm text-gray-700 dark:text-dark-200">{{ row.category }}</td>
                <td class="td-cell"><SupportStatusBadge :status="row.status" /></td>
                <td class="td-cell"><SupportPriorityBadge :priority="row.priority" /></td>
                <td class="td-cell text-sm text-gray-500 dark:text-dark-400">
                  {{ formatDateTime(row.created_at) }}
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

      <!-- Detail Drawer (作为 wide Dialog 实现) -->
      <BaseDialog
        :show="drawerOpen"
        :title="detail ? t('admin.tickets.drawer.title', { id: detail.id }) : '#'"
        width="extra-wide"
        @close="closeDrawer"
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
          <!-- meta -->
          <section>
            <h4 class="mb-2 text-sm font-semibold text-gray-700 dark:text-dark-200">
              {{ t('admin.tickets.drawer.meta') }}
            </h4>
            <div class="rounded-xl bg-gray-50 p-4 text-sm dark:bg-dark-800">
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <span class="text-gray-500 dark:text-dark-400">{{ t('support.common.title') }}：</span>
                  <span class="font-medium text-gray-900 dark:text-white">{{ detail.title }}</span>
                </div>
                <div>
                  <span class="text-gray-500 dark:text-dark-400">{{ t('admin.tickets.drawer.userId') }}：</span>
                  <span class="font-mono text-gray-900 dark:text-white">{{ detail.user_id }}</span>
                </div>
                <div>
                  <span class="text-gray-500 dark:text-dark-400">{{ t('support.common.createdAt') }}：</span>
                  <span class="text-gray-900 dark:text-white">{{ formatDateTime(detail.created_at) }}</span>
                </div>
                <div>
                  <span class="text-gray-500 dark:text-dark-400">{{ t('support.common.updatedAt') }}：</span>
                  <span class="text-gray-900 dark:text-white">{{ formatDateTime(detail.updated_at) }}</span>
                </div>
                <div v-if="detail.closed_at">
                  <span class="text-gray-500 dark:text-dark-400">{{ t('support.common.closedAt') }}：</span>
                  <span class="text-gray-900 dark:text-white">{{ formatDateTime(detail.closed_at) }}</span>
                </div>
              </div>
            </div>
          </section>

          <!-- 描述 -->
          <section>
            <h4 class="mb-2 text-sm font-semibold text-gray-700 dark:text-dark-200">
              {{ t('support.common.content') }}
            </h4>
            <pre
              class="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-xl bg-gray-50 p-4 font-mono text-xs text-gray-800 dark:bg-dark-800 dark:text-dark-100"
            >{{ detail.content }}</pre>
          </section>

          <!-- chat_context -->
          <section v-if="detail.chat_context">
            <h4 class="mb-2 text-sm font-semibold text-gray-700 dark:text-dark-200">
              {{ t('support.common.chatContextLabel') }}
            </h4>
            <pre
              class="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-xl border border-primary-200 bg-primary-50 p-4 font-mono text-xs text-primary-900 dark:border-primary-800/50 dark:bg-primary-900/20 dark:text-primary-100"
            >{{ detail.chat_context }}</pre>
          </section>

          <!-- 时间线 -->
          <section>
            <h4 class="mb-2 text-sm font-semibold text-gray-700 dark:text-dark-200">
              {{ t('support.common.timeline') }}
            </h4>
            <ul v-if="detail.replies.length > 0" class="space-y-3">
              <li
                v-for="r in detail.replies"
                :key="r.id"
                class="flex gap-3"
                :class="r.is_admin ? 'flex-row-reverse' : 'flex-row'"
              >
                <div
                  class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full"
                  :class="r.is_admin ? 'bg-primary-100 text-primary-600 dark:bg-primary-900/40 dark:text-primary-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'"
                >
                  <Icon :name="r.is_admin ? 'shield' : 'user'" size="sm" />
                </div>
                <div
                  class="max-w-[80%] flex-1 rounded-2xl px-3 py-2"
                  :class="r.is_admin ? 'bg-primary-50 dark:bg-primary-900/20' : 'bg-gray-50 dark:bg-dark-800'"
                >
                  <div class="flex items-center justify-between gap-3 text-xs text-gray-500 dark:text-dark-400">
                    <span class="font-medium">
                      {{ r.is_admin ? t('support.common.authorAdmin') : t('support.common.authorUser') }}
                    </span>
                    <span>{{ formatDateTime(r.created_at) }}</span>
                  </div>
                  <pre class="mt-1 whitespace-pre-wrap break-words font-sans text-sm text-gray-800 dark:text-dark-100">{{ r.content }}</pre>
                </div>
              </li>
            </ul>
            <p v-else class="text-sm text-gray-500 dark:text-dark-400">
              {{ t('support.common.noResult') }}
            </p>
          </section>

          <!-- 回复输入（admin 不卡 closed？后端实际仍 409，所以禁用） -->
          <section>
            <h4 class="mb-2 text-sm font-semibold text-gray-700 dark:text-dark-200">
              {{ t('admin.tickets.drawer.replyHeading') }}
            </h4>
            <div
              v-if="detail.status === 'closed'"
              class="rounded-xl border border-gray-200 bg-gray-50 p-3 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300"
            >
              <Icon name="lock" size="sm" class="mr-1 inline" />
              {{ t('support.common.closedHint') }}
            </div>
            <template v-else>
              <textarea
                v-model="replyContent"
                rows="4"
                maxlength="16384"
                :placeholder="t('admin.tickets.drawer.replyPlaceholder')"
                :disabled="replying"
                class="input font-mono text-sm"
              />
              <div class="mt-2 flex items-center justify-between">
                <p class="text-xs text-gray-500 dark:text-dark-400">{{ replyContent.length }} / 16384</p>
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="replying || replyContent.trim() === ''"
                  @click="handleAdminReply"
                >
                  {{ replying ? t('support.common.sending') : t('admin.tickets.drawer.sendReply') }}
                </button>
              </div>
            </template>
          </section>

          <!-- 修改字段（status / priority / category） -->
          <section>
            <h4 class="mb-2 text-sm font-semibold text-gray-700 dark:text-dark-200">
              {{ t('admin.tickets.drawer.editTitle') }}
            </h4>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div>
                <label class="input-label">{{ t('admin.tickets.drawer.statusLabel') }}</label>
                <select v-model="patchForm.status" class="input mt-1">
                  <option value="open">{{ t('support.statusLabel.open') }}</option>
                  <option value="in_progress">{{ t('support.statusLabel.in_progress') }}</option>
                  <option value="closed">{{ t('support.statusLabel.closed') }}</option>
                </select>
              </div>
              <div>
                <label class="input-label">{{ t('admin.tickets.drawer.priorityLabel') }}</label>
                <select v-model="patchForm.priority" class="input mt-1">
                  <option value="high">{{ t('support.priorityLabel.high') }}</option>
                  <option value="normal">{{ t('support.priorityLabel.normal') }}</option>
                  <option value="low">{{ t('support.priorityLabel.low') }}</option>
                </select>
              </div>
              <div>
                <label class="input-label">{{ t('admin.tickets.drawer.categoryLabel') }}</label>
                <select v-if="categories.length > 0" v-model="patchForm.category" class="input mt-1">
                  <option v-for="cat in categories" :key="cat" :value="cat">{{ cat }}</option>
                </select>
                <input v-else v-model.trim="patchForm.category" type="text" class="input mt-1" />
              </div>
            </div>
            <div class="mt-3 flex justify-end">
              <button
                type="button"
                class="btn btn-primary"
                :disabled="saving || !patchDirty"
                @click="handleSavePatch"
              >
                {{ saving ? t('admin.tickets.drawer.saving') : t('admin.tickets.drawer.save') }}
              </button>
            </div>
          </section>
        </div>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * AdminSupportTicketsView —— admin 端工单管理页。
 *
 * 设计要点：
 *
 * 1) 过滤栏：status/priority/category/user_id/q 五个维度。
 *    - status/priority 用 enum select；category 优先用 settings 配置生成 select，
 *      失败时降级为自由文本（admin 仍可按历史分类查询）。
 *    - user_id 是数字输入框；q 走 ILIKE on (title, content)，后端截断到 200 字符。
 *
 * 2) 表格行点击 → 打开 BaseDialog (extra-wide) 当 Drawer 用，复用现成 modal 设施
 *    避免新引入 Drawer 组件。Drawer 内：
 *    - 顶部 meta + 用户原始 content + chat_context（折叠展示）
 *    - 中部回复时间线（与用户视图一致风格）
 *    - 底部回复输入（closed 工单禁用并提示）
 *    - 最下方 PATCH form：status / priority / category
 *
 * 3) 不卡 feature_disabled：admin 路由后端不校验开关，前端的 sidebar 入口仍由
 *    `support_ticket_enabled` 控制（见 §10.3），这里直接信任路由可达就允许操作。
 *
 * 4) PATCH 时只提交 dirty 字段（避免触发 NO_FIELDS_TO_UPDATE 与无意义写）。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import {
  adminAppendReply,
  adminGetTicket,
  adminListTickets,
  adminPatchTicket,
  listCategories,
  type AdminTicketFilter,
  type AdminTicketPatch,
  type SupportTicket,
  type SupportTicketWithReplies,
  type TicketPriority,
  type TicketStatus,
} from '@/api/support'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import SupportStatusBadge from '@/components/support/SupportStatusBadge.vue'
import SupportPriorityBadge from '@/components/support/SupportPriorityBadge.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const tickets = ref<SupportTicket[]>([])
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const categories = ref<string[]>([])

interface FilterModel {
  status: TicketStatus | ''
  priority: TicketPriority | ''
  category: string
  user_id: number | null
  q: string
}
const initialFilter = (): FilterModel => ({
  status: '',
  priority: '',
  category: '',
  user_id: null,
  q: '',
})
const filter = reactive<FilterModel>(initialFilter())

function buildFilterPayload(): AdminTicketFilter {
  const out: AdminTicketFilter = {}
  if (filter.status) out.status = filter.status
  if (filter.priority) out.priority = filter.priority
  if (filter.category && filter.category.trim() !== '') out.category = filter.category.trim()
  if (typeof filter.user_id === 'number' && filter.user_id > 0) out.user_id = filter.user_id
  if (filter.q && filter.q.trim() !== '') out.q = filter.q.trim()
  return out
}

async function fetchList() {
  loading.value = true
  try {
    const res = await adminListTickets(buildFilterPayload(), pagination.page, pagination.page_size)
    tickets.value = res.items || []
    pagination.total = res.total || 0
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
    tickets.value = []
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

function handlePageChange(page: number) {
  pagination.page = page
  fetchList()
}
function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  fetchList()
}

// ============================================================
// Drawer (BaseDialog) 相关
// ============================================================

const drawerOpen = ref(false)
const detail = ref<SupportTicketWithReplies | null>(null)
const detailLoading = ref(false)
const replyContent = ref('')
const replying = ref(false)

interface PatchModel {
  status: TicketStatus
  priority: TicketPriority
  category: string
}
const patchForm = reactive<PatchModel>({ status: 'open', priority: 'normal', category: '' })
const patchOriginal = reactive<PatchModel>({ status: 'open', priority: 'normal', category: '' })
const saving = ref(false)

const patchDirty = computed(() => {
  return (
    patchForm.status !== patchOriginal.status ||
    patchForm.priority !== patchOriginal.priority ||
    patchForm.category !== patchOriginal.category
  )
})

async function openDrawer(id: number) {
  drawerOpen.value = true
  detail.value = null
  detailLoading.value = true
  replyContent.value = ''
  try {
    const d = await adminGetTicket(id)
    detail.value = d
    patchForm.status = d.status
    patchForm.priority = d.priority
    patchForm.category = d.category
    patchOriginal.status = d.status
    patchOriginal.priority = d.priority
    patchOriginal.category = d.category
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
    drawerOpen.value = false
  } finally {
    detailLoading.value = false
  }
}

function closeDrawer() {
  drawerOpen.value = false
  detail.value = null
}

async function refreshDetail() {
  if (!detail.value) return
  try {
    const d = await adminGetTicket(detail.value.id)
    detail.value = d
    patchForm.status = d.status
    patchForm.priority = d.priority
    patchForm.category = d.category
    patchOriginal.status = d.status
    patchOriginal.priority = d.priority
    patchOriginal.category = d.category
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
  }
}

async function handleAdminReply() {
  if (!detail.value || replying.value) return
  const content = replyContent.value.trim()
  if (content === '') return
  replying.value = true
  try {
    await adminAppendReply(detail.value.id, content)
    appStore.showSuccess(t('admin.tickets.drawer.replySuccess'))
    replyContent.value = ''
    await refreshDetail()
    // 列表中的 status 也会从 open → in_progress 翻转，重新拉一遍
    fetchList()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
  } finally {
    replying.value = false
  }
}

async function handleSavePatch() {
  if (!detail.value || !patchDirty.value || saving.value) return
  saving.value = true
  // 仅提交 dirty 字段，避免触发 NO_FIELDS_TO_UPDATE 与无意义写
  const payload: AdminTicketPatch = {}
  if (patchForm.status !== patchOriginal.status) payload.status = patchForm.status
  if (patchForm.priority !== patchOriginal.priority) payload.priority = patchForm.priority
  if (patchForm.category !== patchOriginal.category) payload.category = patchForm.category
  try {
    await adminPatchTicket(detail.value.id, payload)
    appStore.showSuccess(t('admin.tickets.drawer.saveSuccess'))
    await refreshDetail()
    fetchList()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
  } finally {
    saving.value = false
  }
}

// ============================================================
// 初始加载
// ============================================================

async function tryLoadCategories() {
  // feature_disabled 时该接口 404，直接吞掉错误并降级到自由文本输入即可。
  try {
    const res = await listCategories()
    categories.value = res.categories || []
  } catch {
    categories.value = []
  }
}

onMounted(() => {
  tryLoadCategories()
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
