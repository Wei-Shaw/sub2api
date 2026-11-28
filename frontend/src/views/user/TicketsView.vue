<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('tickets.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('tickets.description') }}</p>
        </div>
        <button class="btn btn-primary" @click="showCreate = true">
          <Icon name="plus" size="md" class="mr-1" />
          {{ t('tickets.create') }}
        </button>
      </div>

      <div class="flex flex-wrap gap-3">
        <input
          v-model="search"
          type="text"
          class="input sm:max-w-64"
          :placeholder="t('tickets.searchPlaceholder')"
          @input="scheduleReload"
        />
        <Select v-model="statusFilter" :options="statusOptions" class="w-40" @change="reload" />
      </div>

      <div class="card overflow-hidden">
        <div v-if="loading" class="p-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
        <div v-else-if="tickets.length === 0" class="p-8 text-center text-sm text-gray-500">
          {{ t('tickets.empty') }}
        </div>
        <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
          <button
            v-for="item in tickets"
            :key="item.id"
            type="button"
            class="flex w-full items-start justify-between gap-4 px-4 py-4 text-left transition hover:bg-gray-50 dark:hover:bg-dark-700/50"
            @click="openDetail(item.id)"
          >
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ item.subject }}</span>
                <span class="badge" :class="statusBadgeClass(item.status)">{{ statusLabel(item.status) }}</span>
              </div>
              <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                #{{ item.id }} · {{ t('tickets.messagesCount', { count: item.message_count }) }} ·
                {{ formatRelativeTime(new Date(item.updated_at * 1000)) }}
              </div>
            </div>
            <Icon name="chevronRight" size="sm" class="mt-1 text-gray-400" />
          </button>
        </div>
        <div v-if="total > pageSize" class="flex items-center justify-between border-t border-gray-100 px-4 py-3 dark:border-dark-700">
          <button class="btn btn-secondary btn-sm" :disabled="page <= 1" @click="page--; reload()">{{ t('pagination.previous') }}</button>
          <span class="text-xs text-gray-500">{{ page }} / {{ pages }}</span>
          <button class="btn btn-secondary btn-sm" :disabled="page >= pages" @click="page++; reload()">{{ t('pagination.next') }}</button>
        </div>
      </div>
    </div>

    <!-- Create modal -->
    <div v-if="showCreate" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="showCreate = false">
      <div class="card w-full max-w-lg p-6">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('tickets.create') }}</h2>
        <div class="mt-4 space-y-4">
          <div>
            <label class="input-label">{{ t('tickets.subject') }}</label>
            <input v-model="createForm.subject" class="input mt-1" maxlength="200" />
          </div>
          <div>
            <label class="input-label">{{ t('tickets.content') }}</label>
            <textarea v-model="createForm.content" class="input mt-1 min-h-32" maxlength="10000" />
          </div>
        </div>
        <div class="mt-6 flex justify-end gap-2">
          <button class="btn btn-secondary" @click="showCreate = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="creating" @click="handleCreate">
            {{ creating ? t('common.submitting') : t('common.submit') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Detail modal -->
    <div v-if="detail" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="detail = null">
      <div class="card flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <div class="flex items-start justify-between gap-3">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ detail.subject }}</h2>
              <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500">
                <span>#{{ detail.id }}</span>
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
              <span class="font-medium">{{ msg.is_admin ? t('tickets.admin') : t('tickets.me') }}</span>
              <span>{{ formatDateTime(new Date(msg.created_at * 1000)) }}</span>
            </div>
            <p class="whitespace-pre-wrap text-sm text-gray-800 dark:text-gray-200">{{ msg.content }}</p>
          </div>
        </div>
        <div class="border-t border-gray-100 px-6 py-4 dark:border-dark-700">
          <template v-if="detail.status !== 'closed'">
            <textarea v-model="replyContent" class="input min-h-24" :placeholder="t('tickets.replyPlaceholder')" />
            <div class="mt-3 flex flex-wrap justify-between gap-2">
              <button class="btn btn-secondary" :disabled="acting" @click="handleClose">{{ t('tickets.closeTicket') }}</button>
              <button class="btn btn-primary" :disabled="acting || !replyContent.trim()" @click="handleReply">
                {{ t('tickets.reply') }}
              </button>
            </div>
          </template>
          <p v-else class="text-sm text-gray-500">{{ t('tickets.closedHint') }}</p>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores/app'
import { formatDateTime, formatRelativeTime } from '@/utils/format'
import ticketsAPI, { type Ticket, type TicketStatus } from '@/api/tickets'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const creating = ref(false)
const acting = ref(false)
const tickets = ref<Ticket[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const search = ref('')
const statusFilter = ref('')
const showCreate = ref(false)
const detail = ref<Ticket | null>(null)
const replyContent = ref('')
const createForm = reactive({ subject: '', content: '' })
let searchTimer: ReturnType<typeof setTimeout> | null = null

const pages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const statusOptions = computed(() => [
  { value: '', label: t('tickets.statusAll') },
  { value: 'open', label: t('tickets.status.open') },
  { value: 'waiting_admin', label: t('tickets.status.waiting_admin') },
  { value: 'answered', label: t('tickets.status.answered') },
  { value: 'closed', label: t('tickets.status.closed') }
])

const statusLabel = (status: string) => t(`tickets.status.${status}` as const)
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
    const data = await ticketsAPI.listMyTickets({
      page: page.value,
      page_size: pageSize,
      status: statusFilter.value || undefined,
      search: search.value.trim() || undefined
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

const handleCreate = async () => {
  if (!createForm.subject.trim() || !createForm.content.trim()) {
    appStore.showError(t('tickets.fillRequired'))
    return
  }
  creating.value = true
  try {
    const ticket = await ticketsAPI.createTicket({
      subject: createForm.subject.trim(),
      content: createForm.content.trim()
    })
    showCreate.value = false
    createForm.subject = ''
    createForm.content = ''
    appStore.showSuccess(t('tickets.createSuccess'))
    await reload()
    detail.value = ticket
    replyContent.value = ''
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e) || t('common.error'))
  } finally {
    creating.value = false
  }
}

const openDetail = async (id: number) => {
  try {
    detail.value = await ticketsAPI.getMyTicket(id)
    replyContent.value = ''
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e) || t('common.error'))
  }
}

const handleReply = async () => {
  if (!detail.value || !replyContent.value.trim()) return
  acting.value = true
  try {
    detail.value = await ticketsAPI.replyMyTicket(detail.value.id, replyContent.value.trim())
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
    detail.value = await ticketsAPI.closeMyTicket(detail.value.id)
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
