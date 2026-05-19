<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('nav.announcements') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('announcements.subtitle') }}</p>
        </div>
        <button
          v-if="unreadCount > 0"
          @click="markAll"
          :disabled="loading"
          class="rounded bg-primary-400 px-4 py-2 text-sm font-medium text-white hover:bg-primary-500 transition-colors"
        >
          {{ t('announcements.markAllRead') }}
        </button>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-16">
        <LoadingSpinner />
      </div>

      <!-- Empty -->
      <div v-else-if="announcements.length === 0" class="flex flex-col items-center justify-center rounded-lg border border-gray-200 bg-white py-16 dark:border-dark-700 dark:bg-dark-800">
        <Icon name="inbox" size="xl" class="text-gray-300 dark:text-gray-600" />
        <p class="mt-4 text-sm text-gray-500 dark:text-gray-400">{{ t('announcements.empty') }}</p>
      </div>

      <!-- List -->
      <div v-else class="space-y-3">
        <div
          v-for="item in announcements"
          :key="item.id"
          @click="openDetail(item)"
          class="cursor-pointer rounded-lg border bg-white p-5 transition-colors hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:hover:bg-dark-700"
          :class="{ 'border-l-4 border-l-primary-400': !item.read_at }"
        >
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <h3 class="font-medium text-gray-900 dark:text-white">{{ item.title }}</h3>
                <span
                  v-if="!item.read_at"
                  class="inline-flex items-center rounded bg-primary-50 px-2 py-0.5 text-xs font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
                >New</span>
              </div>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ formatRelativeTime(item.created_at) }}</p>
            </div>
            <svg class="h-5 w-5 flex-shrink-0 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </div>
        </div>
      </div>

      <!-- Detail Modal -->
      <Teleport to="body">
        <div
          v-if="selected"
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          @click="selected = null"
        >
          <div
            class="w-full max-w-2xl max-h-[80vh] overflow-y-auto rounded-lg bg-white p-6 dark:bg-dark-800"
            @click.stop
          >
            <div class="flex items-start justify-between">
              <h2 class="text-xl font-bold text-gray-900 dark:text-white">{{ selected.title }}</h2>
              <button @click="selected = null" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
                <Icon name="x" size="md" />
              </button>
            </div>
            <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ formatRelativeWithDateTime(selected.created_at) }}</p>
            <div class="mt-4 prose prose-sm dark:prose-invert" v-html="renderMarkdown(selected.content)"></div>
            <div class="mt-6 flex justify-end">
              <button
                v-if="!selected.read_at"
                @click="markRead(selected.id)"
                class="rounded bg-primary-400 px-4 py-2 text-sm font-medium text-white hover:bg-primary-500 transition-colors"
              >{{ t('announcements.markRead') }}</button>
            </div>
          </div>
        </div>
      </Teleport>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const store = useAnnouncementStore()

const loading = ref(false)
const announcements = computed(() => store.announcements)
const unreadCount = computed(() => store.unreadCount)
const selected = ref<UserAnnouncement | null>(null)

marked.setOptions({ breaks: true, gfm: true })
function renderMarkdown(content: string) {
  return DOMPurify.sanitize(marked.parse(content) as string)
}

function openDetail(item: UserAnnouncement) {
  selected.value = item
  if (!item.read_at) markRead(item.id)
}

async function markRead(id: number) {
  await store.markAsRead(id)
}

async function markAll() {
  await store.markAllAsRead()
}

onMounted(async () => {
  loading.value = true
  await store.fetchAnnouncements(true)
  loading.value = false
})
</script>
