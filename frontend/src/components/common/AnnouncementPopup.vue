<template>
  <Teleport to="body">
    <Transition name="popup-fade">
      <div
        v-if="displayedAnnouncement"
        data-testid="announcement-popup"
        class="fixed inset-0 z-[120] flex items-start justify-center overflow-y-auto bg-gray-950/75 p-4 pt-[7vh] backdrop-blur-sm sm:p-6 sm:pt-[9vh]"
      >
        <article
          class="w-full max-w-[720px] overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900"
          role="dialog"
          aria-modal="true"
          :aria-label="displayedAnnouncement.title"
          @click.stop
        >
          <header
            class="relative border-b border-gray-200 bg-white px-6 py-6 dark:border-dark-700 dark:bg-dark-900 sm:px-8 sm:py-7"
          >
            <div class="absolute inset-y-0 left-0 w-1 bg-blue-500"></div>
            <div class="flex items-start justify-between gap-5">
              <div class="min-w-0 flex-1">
                <div class="mb-5 flex flex-wrap items-center gap-3">
                  <span
                    class="inline-flex items-center gap-2 rounded-full border border-blue-200 bg-blue-50 px-3 py-1 text-xs font-semibold tracking-wide text-blue-700 dark:border-blue-400/30 dark:bg-blue-500/10 dark:text-blue-300"
                  >
                    <span class="h-1.5 w-1.5 rounded-full bg-blue-600 dark:bg-blue-400"></span>
                    TokenSupply Notice
                  </span>
                  <span
                    v-if="!preview"
                    class="inline-flex items-center gap-1.5 text-xs font-medium text-gray-500 dark:text-gray-400"
                  >
                    <span class="h-1.5 w-1.5 rounded-full bg-blue-600 dark:bg-blue-400"></span>
                    {{ t('announcements.unread') }}
                  </span>
                </div>
                <h2
                  class="text-2xl font-semibold leading-tight tracking-tight text-gray-950 dark:text-white sm:text-3xl"
                >
                  {{ displayedAnnouncement.title }}
                </h2>
                <div class="mt-4 flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
                  <Icon name="clock" size="sm" />
                  <time>{{ formatRelativeWithDateTime(displayedAnnouncement.created_at) }}</time>
                </div>
              </div>
              <div
                class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-gray-200 bg-gray-50 text-blue-600 dark:border-dark-600 dark:bg-dark-800 dark:text-blue-400"
              >
                <Icon name="bell" size="sm" />
              </div>
            </div>
          </header>

          <div class="max-h-[52vh] overflow-y-auto bg-white px-6 py-7 dark:bg-dark-900 sm:px-8 sm:py-9">
            <div
              class="markdown-body prose prose-sm max-w-none dark:prose-invert"
              v-html="renderedContent"
            ></div>
          </div>

          <footer class="flex flex-col gap-4 border-t border-gray-200 bg-gray-50 px-6 py-5 dark:border-dark-700 dark:bg-dark-950/50 sm:flex-row sm:items-center sm:justify-between sm:px-8">
            <div class="flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
              <span class="flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-950 dark:border-dark-600">
                <span class="flex flex-col gap-[2px]" aria-hidden="true">
                  <span class="block h-[2px] w-3 rounded-full bg-current"></span>
                  <span class="block h-[2px] w-2.5 rounded-full bg-current"></span>
                  <span class="block h-[2px] w-1.5 rounded-full bg-current"></span>
                </span>
              </span>
              <span>TokenSupply</span>
            </div>
            <button
              @click="handleDismiss"
              data-testid="announcement-popup-dismiss"
              class="inline-flex min-h-10 items-center justify-center gap-2 rounded-lg bg-blue-600 px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 dark:bg-blue-500 dark:hover:bg-blue-600 dark:focus-visible:ring-offset-dark-950"
            >
              <Icon :name="preview ? 'x' : 'check'" size="sm" />
                  {{ preview ? t('common.close') : t('announcements.markRead') }}
            </button>
          </footer>
        </article>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeWithDateTime } from '@/utils/format'
import type { Announcement, UserAnnouncement } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import '@/styles/announcement-markdown.css'

type PreviewAnnouncement = Pick<Announcement | UserAnnouncement, 'title' | 'content' | 'created_at'>

const props = withDefaults(defineProps<{
  announcement?: PreviewAnnouncement | null
  preview?: boolean
}>(), {
  announcement: null,
  preview: false,
})

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const announcementStore = useAnnouncementStore()
const displayedAnnouncement = computed(() => (
  props.preview ? props.announcement : announcementStore.currentPopup
))

marked.setOptions({
  breaks: true,
  gfm: true,
})

const renderedContent = computed(() => {
  const content = displayedAnnouncement.value?.content
  if (!content) return ''
  const html = marked.parse(content) as string
  return DOMPurify.sanitize(html)
})

function handleDismiss() {
  if (props.preview) {
    emit('close')
    return
  }
  announcementStore.dismissPopup()
}

// Manage body overflow — only set, never unset (bell component handles restore)
watch(
  displayedAnnouncement,
  (popup) => {
    if (popup) {
      document.body.style.overflow = 'hidden'
    } else if (props.preview) {
      document.body.style.overflow = ''
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  if (props.preview) {
    document.body.style.overflow = ''
  }
})
</script>

<style scoped>
.popup-fade-enter-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}

.popup-fade-leave-active {
  transition: all 0.2s cubic-bezier(0.4, 0, 1, 1);
}

.popup-fade-enter-from,
.popup-fade-leave-to {
  opacity: 0;
}

.popup-fade-enter-from > article {
  transform: scale(0.97) translateY(-10px);
  opacity: 0;
}

.popup-fade-leave-to > article {
  transform: scale(0.98) translateY(-6px);
  opacity: 0;
}

/* Scrollbar Styling */
.overflow-y-auto::-webkit-scrollbar {
  width: 8px;
}

.overflow-y-auto::-webkit-scrollbar-track {
  background: transparent;
}

.overflow-y-auto::-webkit-scrollbar-thumb {
  background: #9ca3af;
  border-radius: 4px;
}

.dark .overflow-y-auto::-webkit-scrollbar-thumb {
  background: #4b5563;
}
</style>
