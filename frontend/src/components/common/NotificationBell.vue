<template>
  <div class="relative" ref="rootRef">
    <button
      type="button"
      class="relative flex h-9 w-9 items-center justify-center rounded-lg text-gray-600 transition-all hover:scale-105 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-800"
      :class="{ 'text-primary-600 dark:text-primary-400': unreadCount > 0 }"
      :aria-label="t('notifications.title')"
      @click="toggle"
    >
      <Icon name="inbox" size="md" />
      <span
        v-if="unreadCount > 0"
        class="absolute -right-0.5 -top-0.5 flex h-4 min-w-[16px] items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-semibold text-white shadow"
      >
        {{ unreadCount > 99 ? '99+' : unreadCount }}
      </span>
    </button>

    <transition name="dropdown">
      <div
        v-if="open"
        class="absolute right-0 z-50 mt-2 w-[360px] overflow-hidden rounded-xl border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-300">
            {{ t('notifications.title') }}
          </span>
          <button
            v-if="unreadCount > 0"
            type="button"
            class="text-xs text-primary-600 hover:underline dark:text-primary-400"
            @click="onMarkAllRead"
          >
            {{ t('notifications.markAllRead') }}
          </button>
        </div>

        <div class="max-h-[60vh] overflow-y-auto">
          <div v-if="loading" class="flex items-center justify-center py-8">
            <Icon name="refresh" size="md" class="animate-spin text-gray-400" />
          </div>
          <div v-else-if="items.length === 0" class="flex flex-col items-center gap-2 py-10 text-gray-500 dark:text-gray-400">
            <Icon name="inbox" size="lg" />
            <p class="text-sm">{{ t('notifications.empty') }}</p>
          </div>
          <ul v-else class="divide-y divide-gray-100 dark:divide-dark-700">
            <li
              v-for="item in items"
              :key="item.id"
              class="cursor-pointer p-4 transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/50"
              :class="{ 'bg-primary-50/40 dark:bg-primary-900/10': !item.read_at }"
              @click="onClickItem(item)"
            >
              <div class="flex items-start gap-3">
                <div
                  v-if="!item.read_at"
                  class="mt-1 h-2 w-2 flex-shrink-0 rounded-full bg-primary-500"
                ></div>
                <div v-else class="mt-1 h-2 w-2 flex-shrink-0"></div>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                    {{ item.title }}
                  </p>
                  <p
                    v-if="item.body"
                    class="mt-1 line-clamp-2 text-xs text-gray-600 dark:text-gray-300"
                  >
                    {{ item.body }}
                  </p>
                  <p class="mt-1 text-[11px] text-gray-400 dark:text-gray-500">
                    {{ formatDateTime(item.created_at) }}
                  </p>
                </div>
              </div>
            </li>
          </ul>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { notificationAPI } from '@/api/notifications'
import { formatDateTime } from '@/utils/format'
import type { UserNotification } from '@/types/notification'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const router = useRouter()

const open = ref(false)
const loading = ref(false)
const items = ref<UserNotification[]>([])
const unreadCount = ref(0)
const rootRef = ref<HTMLElement | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null

async function refreshUnreadCount() {
  try {
    const res = await notificationAPI.unreadCount()
    unreadCount.value = res.data.count || 0
  } catch {
    // Silent — bell is best-effort
  }
}

async function loadList() {
  loading.value = true
  try {
    const res = await notificationAPI.list({ page: 1, page_size: 20 })
    items.value = res.data.items || []
  } catch {
    items.value = []
  } finally {
    loading.value = false
  }
}

async function toggle() {
  open.value = !open.value
  if (open.value) {
    await loadList()
  }
}

async function onMarkAllRead() {
  try {
    await notificationAPI.markAllRead()
    items.value = items.value.map((n) => ({ ...n, read_at: n.read_at || new Date().toISOString() }))
    unreadCount.value = 0
  } catch {
    // ignore
  }
}

async function onClickItem(item: UserNotification) {
  if (!item.read_at) {
    try {
      await notificationAPI.markRead(item.id)
      item.read_at = new Date().toISOString()
      unreadCount.value = Math.max(0, unreadCount.value - 1)
    } catch {
      // ignore
    }
  }
  if (item.link) {
    open.value = false
    router.push(item.link).catch(() => undefined)
  }
}

function handleClickOutside(event: MouseEvent) {
  if (!open.value) return
  const target = event.target as Node
  if (rootRef.value && !rootRef.value.contains(target)) {
    open.value = false
  }
}

onMounted(() => {
  refreshUnreadCount()
  document.addEventListener('click', handleClickOutside)
  // Poll every 60s for new notifications
  pollTimer = setInterval(refreshUnreadCount, 60_000)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.15s ease;
}
.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
