<template>
  <div class="relative">
    <button
      type="button"
      data-testid="announcement-bell-trigger"
      class="relative flex h-9 w-9 items-center justify-center rounded-lg text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white dark:focus-visible:ring-offset-dark-950"
      :class="{ 'text-blue-600 dark:text-blue-400': unreadCount > 0 }"
      :aria-label="t('announcements.title')"
      @click="openModal"
    >
      <Icon name="bell" size="md" />
      <span
        v-if="unreadCount > 0"
        class="absolute -right-1 -top-1 inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-blue-600 px-1 text-[10px] font-bold leading-none text-white ring-2 ring-white dark:bg-blue-500 dark:ring-dark-950"
      >
        {{ unreadBadgeText }}
      </span>
    </button>

    <Teleport to="body">
      <Transition name="modal-fade">
        <div
          v-if="isModalOpen"
          data-testid="announcement-list-modal"
          class="fixed inset-0 z-[100] flex items-start justify-center overflow-y-auto bg-gray-950/60 p-4 pt-[8vh] backdrop-blur-sm"
          @click="closeModal"
        >
          <section
            class="w-full max-w-[640px] overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900"
            role="dialog"
            aria-modal="true"
            :aria-label="t('announcements.title')"
            @click.stop
          >
            <header class="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4 dark:border-dark-700 sm:px-6 sm:py-5">
              <div class="flex min-w-0 items-start gap-3">
                <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-gray-50 text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-white">
                  <Icon name="bell" size="sm" />
                </div>
                <div class="min-w-0">
                  <h2 class="text-base font-semibold text-gray-950 dark:text-white sm:text-lg">
                    {{ t('announcements.title') }}
                  </h2>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    <template v-if="unreadCount > 0">
                      <span class="font-semibold text-blue-600 dark:text-blue-400">{{ unreadCount }}</span>
                      {{ t('announcements.unread') }}
                    </template>
                    <template v-else>{{ t('announcements.emptyDescription') }}</template>
                  </p>
                </div>
              </div>

              <div class="flex shrink-0 items-center gap-2">
                <button
                  v-if="unreadCount > 0"
                  type="button"
                  class="rounded-lg bg-blue-600 px-3 py-2 text-xs font-semibold text-white transition-colors hover:bg-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600 dark:focus-visible:ring-offset-dark-900"
                  :disabled="loading"
                  @click="markAllAsRead"
                >
                  {{ t('announcements.markAllRead') }}
                </button>
                <button
                  type="button"
                  class="flex h-9 w-9 items-center justify-center rounded-lg border border-gray-200 text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 dark:border-dark-700 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white"
                  :aria-label="t('common.close')"
                  @click="closeModal"
                >
                  <Icon name="x" size="sm" />
                </button>
              </div>
            </header>

            <div class="max-h-[65vh] overflow-y-auto bg-white dark:bg-dark-900">
              <div v-if="loading" class="flex items-center justify-center py-16">
                <div class="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-t-blue-600 dark:border-dark-700 dark:border-t-blue-400"></div>
              </div>

              <div v-else-if="announcements.length > 0" class="divide-y divide-gray-100 dark:divide-dark-700">
                <button
                  v-for="item in announcements"
                  :key="item.id"
                  type="button"
                  class="group relative flex min-h-[76px] w-full items-center gap-3 px-5 py-4 text-left transition-colors hover:bg-gray-50 focus-visible:z-10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-blue-500 dark:hover:bg-dark-800/70 sm:px-6"
                  :class="{ 'bg-blue-50/60 dark:bg-blue-950/20': !item.read_at }"
                  @click="openDetail(item)"
                >
                  <span
                    v-if="!item.read_at"
                    class="absolute inset-y-3 left-0 w-0.5 rounded-r bg-blue-600 dark:bg-blue-400"
                  ></span>

                  <span
                    class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border"
                    :class="item.read_at
                      ? 'border-gray-200 bg-white text-gray-400 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-500'
                      : 'border-blue-200 bg-blue-50 text-blue-600 dark:border-blue-900 dark:bg-blue-950/50 dark:text-blue-400'"
                  >
                    <Icon :name="item.read_at ? 'check' : 'bell'" size="sm" />
                  </span>

                  <span class="min-w-0 flex-1">
                    <span
                      class="block truncate text-sm text-gray-950 dark:text-white"
                      :class="item.read_at ? 'font-medium' : 'font-semibold'"
                    >
                      {{ item.title }}
                    </span>
                    <span class="mt-1 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                      <time>{{ formatRelativeTime(item.created_at) }}</time>
                      <span
                        v-if="!item.read_at"
                        class="inline-flex items-center gap-1 font-medium text-blue-600 dark:text-blue-400"
                      >
                        <span class="h-1.5 w-1.5 rounded-full bg-blue-600 dark:bg-blue-400"></span>
                        {{ t('announcements.unread') }}
                      </span>
                    </span>
                  </span>

                  <Icon
                    name="chevronRight"
                    size="sm"
                    class="shrink-0 text-gray-400 transition-transform group-hover:translate-x-0.5 dark:text-gray-500"
                  />
                </button>
              </div>

              <div v-else class="flex flex-col items-center justify-center px-6 py-16 text-center">
                <div class="flex h-12 w-12 items-center justify-center rounded-xl border border-gray-200 bg-gray-50 text-gray-400 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-500">
                  <Icon name="inbox" size="lg" />
                </div>
                <p class="mt-4 text-sm font-semibold text-gray-950 dark:text-white">
                  {{ t('announcements.empty') }}
                </p>
                <p class="mt-1 max-w-sm text-xs leading-5 text-gray-500 dark:text-gray-400">
                  {{ t('announcements.emptyDescription') }}
                </p>
              </div>
            </div>
          </section>
        </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="modal-fade">
        <div
          v-if="detailModalOpen && selectedAnnouncement"
          data-testid="announcement-detail-modal"
          class="fixed inset-0 z-[110] flex items-start justify-center overflow-y-auto bg-gray-950/60 p-4 pt-[6vh] backdrop-blur-sm"
          @click="closeDetail"
        >
          <article
            class="w-full max-w-[780px] overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900"
            role="dialog"
            aria-modal="true"
            :aria-label="selectedAnnouncement.title"
            @click.stop
          >
            <header class="border-b border-gray-200 px-5 py-5 dark:border-dark-700 sm:px-7 sm:py-6">
              <div class="flex items-start justify-between gap-5">
                <div class="min-w-0 flex-1">
                  <div class="mb-3 flex flex-wrap items-center gap-2">
                    <span class="rounded-md border border-gray-200 bg-gray-50 px-2 py-1 text-xs font-semibold text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
                      {{ t('announcements.title') }}
                    </span>
                    <span
                      v-if="!selectedAnnouncement.read_at"
                      class="inline-flex items-center gap-1.5 rounded-md bg-blue-50 px-2 py-1 text-xs font-semibold text-blue-700 dark:bg-blue-950/50 dark:text-blue-300"
                    >
                      <span class="h-1.5 w-1.5 rounded-full bg-blue-600 dark:bg-blue-400"></span>
                      {{ t('announcements.unread') }}
                    </span>
                  </div>
                  <h2 class="text-xl font-bold leading-tight tracking-tight text-gray-950 dark:text-white sm:text-2xl">
                    {{ selectedAnnouncement.title }}
                  </h2>
                  <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-gray-500 dark:text-gray-400 sm:text-sm">
                    <span class="flex items-center gap-1.5">
                      <Icon name="clock" size="sm" />
                      <time>{{ formatRelativeWithDateTime(selectedAnnouncement.created_at) }}</time>
                    </span>
                    <span class="flex items-center gap-1.5">
                      <span
                        class="h-1.5 w-1.5 rounded-full"
                        :class="selectedAnnouncement.read_at ? 'bg-gray-400' : 'bg-blue-600 dark:bg-blue-400'"
                      ></span>
                      {{ selectedAnnouncement.read_at ? t('announcements.read') : t('announcements.unread') }}
                    </span>
                  </div>
                </div>

                <button
                  type="button"
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-gray-200 text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 dark:border-dark-700 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white"
                  :aria-label="t('common.close')"
                  @click="closeDetail"
                >
                  <Icon name="x" size="sm" />
                </button>
              </div>
            </header>

            <div class="max-h-[60vh] overflow-y-auto bg-white px-5 py-6 dark:bg-dark-900 sm:px-7 sm:py-8">
              <div class="border-l-2 border-blue-600 pl-4 dark:border-blue-400 sm:pl-6">
                <div
                  class="markdown-body prose prose-sm max-w-none dark:prose-invert"
                  v-html="renderMarkdown(selectedAnnouncement.content)"
                ></div>
              </div>
            </div>

            <footer class="flex flex-col-reverse gap-3 border-t border-gray-200 bg-gray-50 px-5 py-4 dark:border-dark-700 dark:bg-dark-950/40 sm:flex-row sm:items-center sm:justify-between sm:px-7">
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ selectedAnnouncement.read_at ? t('announcements.readStatus') : t('announcements.markReadHint') }}
              </p>
              <div class="flex items-center justify-end gap-2">
                <button
                  type="button"
                  class="rounded-lg border border-gray-300 bg-white px-4 py-2 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
                  @click="closeDetail"
                >
                  {{ t('common.close') }}
                </button>
                <button
                  v-if="!selectedAnnouncement.read_at"
                  type="button"
                  class="rounded-lg bg-blue-600 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 dark:bg-blue-500 dark:hover:bg-blue-600 dark:focus-visible:ring-offset-dark-900"
                  @click="markAsReadAndClose(selectedAnnouncement.id)"
                >
                  {{ t('announcements.markRead') }}
                </button>
              </div>
            </footer>
          </article>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import '@/styles/announcement-markdown.css'

const { t } = useI18n()
const appStore = useAppStore()
const announcementStore = useAnnouncementStore()

marked.setOptions({
  breaks: true,
  gfm: true,
})

const { announcements, loading } = storeToRefs(announcementStore)
const unreadCount = computed(() => announcementStore.unreadCount)
const unreadBadgeText = computed(() => (unreadCount.value > 99 ? '99+' : String(unreadCount.value)))

const isModalOpen = ref(false)
const detailModalOpen = ref(false)
const selectedAnnouncement = ref<UserAnnouncement | null>(null)

function renderMarkdown(content: string): string {
  if (!content) return ''
  const html = marked.parse(content) as string
  return DOMPurify.sanitize(html)
}

function openModal() {
  isModalOpen.value = true
}

function closeModal() {
  isModalOpen.value = false
}

function openDetail(announcement: UserAnnouncement) {
  selectedAnnouncement.value = announcement
  detailModalOpen.value = true
  if (!announcement.read_at) {
    void markAsRead(announcement.id)
  }
}

function closeDetail() {
  detailModalOpen.value = false
  selectedAnnouncement.value = null
}

async function markAsRead(id: number) {
  try {
    await announcementStore.markAsRead(id)
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  }
}

async function markAsReadAndClose(id: number) {
  await markAsRead(id)
  appStore.showSuccess(t('announcements.markedAsRead'))
  closeDetail()
}

async function markAllAsRead() {
  try {
    await announcementStore.markAllAsRead()
    appStore.showSuccess(t('announcements.allMarkedAsRead'))
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  }
}

function handleEscape(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  if (detailModalOpen.value) {
    closeDetail()
  } else if (isModalOpen.value) {
    closeModal()
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleEscape)
  document.body.style.overflow = ''
})

watch(
  [isModalOpen, detailModalOpen, () => announcementStore.currentPopup],
  ([modal, detail, popup]) => {
    document.body.style.overflow = modal || detail || popup ? 'hidden' : ''
  },
)
</script>

<style scoped>
.modal-fade-enter-active {
  transition: opacity 180ms ease-out;
}

.modal-fade-leave-active {
  transition: opacity 140ms ease-in;
}

.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}

.modal-fade-enter-from > section,
.modal-fade-enter-from > article {
  transform: translateY(-6px) scale(0.99);
}

.modal-fade-enter-active > section,
.modal-fade-enter-active > article {
  transition: transform 180ms ease-out;
}

.overflow-y-auto::-webkit-scrollbar {
  width: 8px;
}

.overflow-y-auto::-webkit-scrollbar-track {
  background: transparent;
}

.overflow-y-auto::-webkit-scrollbar-thumb {
  border: 2px solid transparent;
  border-radius: 999px;
  background: #9ca3af;
  background-clip: padding-box;
}

.dark .overflow-y-auto::-webkit-scrollbar-thumb {
  background: #4b5563;
  background-clip: padding-box;
}
</style>
