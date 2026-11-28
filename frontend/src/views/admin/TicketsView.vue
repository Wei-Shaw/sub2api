<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 sm:max-w-64">
            <input
              v-model="search"
              type="text"
              class="input"
              :placeholder="t('admin.tickets.searchPlaceholder')"
              @input="scheduleReload"
            />
          </div>
          <Select v-model="statusFilter" :options="statusOptions" class="w-44" @change="reload" />
          <div class="flex flex-1 justify-end">
            <button class="btn btn-secondary" :disabled="loading" @click="reload">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="tickets"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="updated_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-subject="{ value, row }">
            <button type="button" class="text-left" @click="openDetail(row.id)">
              <div class="font-medium text-gray-900 dark:text-white">{{ value }}</div>
              <div class="mt-1 text-xs text-gray-500">#{{ row.id }} · {{ row.message_count }} {{ t('admin.tickets.messages') }}</div>
            </button>
          </template>
          <template #cell-user="{ row }">
            <div class="text-sm text-gray-700 dark:text-gray-300">{{ row.user_name || row.user_email || row.user_id }}</div>
            <div v-if="row.user_email && row.user_name" class="text-xs text-gray-500">{{ row.user_email }}</div>
          </template>
          <template #cell-status="{ value }">
            <span class="badge" :class="statusBadgeClass(value)">{{ statusLabel(value) }}</span>
          </template>
          <template #cell-updated_at="{ value }">
            <span class="text-sm text-gray-500">{{ formatDateTime(new Date(value * 1000)) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <button class="btn btn-secondary btn-sm" @click="openDetail(row.id)">{{ t('common.view') }}</button>
          </template>
          <template #empty>
            <EmptyState :title="t('admin.tickets.empty')" />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="(p: number) => { page = p; reload() }"
          @update:pageSize="(s: number) => { pageSize = s; page = 1; reload() }"
        />
      </template>
    </TablePageLayout>

    <div v-if="detail" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="detail = null">
      <div class="card flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <div class="flex items-start justify-between gap-3">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ detail.subject }}</h2>
              <div class="mt-1 text-xs text-gray-500">
                #{{ detail.id }} · {{ detail.user_name || detail.user_email || detail.user_id }} ·
                <span class="badge" :class="statusBadgeClass(detail.status)">{{ statusLabel(detail.status) }}</span>
              </div>
            </div>
            <button class="btn btn-secondary btn-sm" @click="detail = null">{{ t('common.close') }}</button>
          </div>
        </div>
        <div class="flex-1 space-y-3 overflow-y-auto px-6 py-4">
          <div
            v-for="msg in detail.messages || []"
            :key="msg.id"
            class="rounded-xl px-4 py-3"
            :class="msg.is_admin ? 'bg-blue-50 dark:bg-blue-900/20' : 'bg-gray-50 dark:bg-dark-700/60'"
          >
            <div class="mb-1 flex items-center justify-between gap-2 text-xs text-gray-500">
              <span class="font-medium">{{ msg.is_admin ? t('admin.tickets.admin') : (msg.user_name || msg.user_email || t('admin.tickets.user')) }}</span>
              <span>{{ formatDateTime(new Date(msg.created_at * 1000)) }}</span>
            </div>
            <p class="whitespace-pre-wrap text-sm text-gray-800 dark:text-gray-200">{{ msg.content }}</p>
          </div>
        </div>
        <div class="border-t border-gray-100 px-6 py-4 dark:border-dark-700">
          <template v-if="detail.status !== 'closed'">
            <textarea v-model="replyContent" class="input min-h-24" :placeholder="t('admin.tickets.replyPlaceholder')" />
            <div class="mt-3 flex flex-wrap justify-between gap-2">
              <button class="btn btn-secondary" :disabled="acting" @click="handleClose">{{ t('admin.tickets.closeTicket') }}</button>
              <button class="btn btn-primary" :disabled="acting || !replyContent.trim()" @click="handleReply">
                {{ t('admin.tickets.reply') }}
              </button>
            </div>
          </template>
          <div v-else class="flex items-center justify-between gap-2">
            <p class="text-sm text-gray-500">{{ t('admin.tickets.closedHint') }}</p>
            <button class="btn btn-primary" :disabled="acting" @click="handleReopen">{{ t('admin.tickets.reopen') }}</button>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'
import ticketsAPI, { type Ticket, type TicketStatus } from '@/api/tickets'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const acting = ref(false)
const tickets = ref<Ticket[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const search = ref('')
const statusFilter = ref('')
const sortBy = ref('updated_at')
const sortOrder = ref<'asc' | 'desc'>('desc')
const detail = ref<Ticket | null>(null)
const replyContent = ref('')
let searchTimer: ReturnType<typeof setTimeout> | null = null

const columns = computed(() => [
  { key: 'subject', label: t('admin.tickets.columns.subject'), sortable: true },
  { key: 'user', label: t('admin.tickets.columns.user'), sortable: false },
  { key: 'status', label: t('admin.tickets.columns.status'), sortable: true },
  { key: 'updated_at', label: t('admin.tickets.columns.updatedAt'), sortable: true },
  { key: 'actions', label: t('common.actions'), sortable: false }
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.tickets.statusAll') },
  { value: 'open', label: t('admin.tickets.status.open') },
  { value: 'waiting_admin', label: t('admin.tickets.status.waiting_admin') },
  { value: 'answered', label: t('admin.tickets.status.answered') },
  { value: 'closed', label: t('admin.tickets.status.closed') }
])

const statusLabel = (status: string) => t(`admin.tickets.status.${status}` as const)
const statusBadgeClass = (status: TicketStatus | string) => {
  switch (status) {
    case 'open':
      return 'badge-warning'
    case 'waiting_admin':
      return 'badge-primary'
    case 'answered':
      return 'badge-success'
    case 'closed':
      return 'badge-gray'
    default:
      return 'badge-gray'
  }
}

const reload = async () => {
  loading.value = true
  try {
    const data = await ticketsAPI.listAdminTickets({
      page: page.value,
      page_size: pageSize.value,
      status: statusFilter.value || undefined,
      search: search.value.trim() || undefined,
      sort_by: sortBy.value,
      sort_order: sortOrder.value
    })
    tickets.value = data.items || []
    total.value = data.total || 0
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e) || t('common.error'))
  } finally {
    loading.value = false
  }
}

const scheduleReload = () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    void reload()
  }, 300)
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortBy.value = key
  sortOrder.value = order
  void reload()
}

const openDetail = async (id: number) => {
  try {
    detail.value = await ticketsAPI.getAdminTicket(id)
    replyContent.value = ''
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e) || t('common.error'))
  }
}

const handleReply = async () => {
  if (!detail.value || !replyContent.value.trim()) return
  acting.value = true
  try {
    detail.value = await ticketsAPI.replyAdminTicket(detail.value.id, replyContent.value.trim())
    replyContent.value = ''
    await reload()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e) || t('common.error'))
  } finally {
    acting.value = false
  }
}

const handleClose = async () => {
  if (!detail.value) return
  acting.value = true
  try {
    detail.value = await ticketsAPI.closeAdminTicket(detail.value.id)
    await reload()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e) || t('common.error'))
  } finally {
    acting.value = false
  }
}

const handleReopen = async () => {
  if (!detail.value) return
  acting.value = true
  try {
    detail.value = await ticketsAPI.reopenAdminTicket(detail.value.id)
    await reload()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e) || t('common.error'))
  } finally {
    acting.value = false
  }
}

onMounted(() => {
  void reload()
})
</script>
