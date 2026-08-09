<template>
  <Teleport to="body">
    <Transition name="popup-fade">
      <div
        v-if="displayedAnnouncement"
        class="fixed inset-0 z-[120] flex items-center justify-center overflow-y-auto bg-black/75 p-3 backdrop-blur-sm sm:p-6"
      >
        <div
          class="max-h-[calc(100vh-1.5rem)] w-full max-w-[680px] overflow-hidden rounded-sm border border-gray-200 bg-[#f8f6f1] shadow-lg dark:border-white/10 dark:bg-[#121214]"
          @click.stop
        >
          <div class="border-b border-gray-200 bg-[#f1ece1] px-5 py-5 dark:border-white/10 dark:bg-[#181715] sm:px-7 sm:py-6">
            <div>
              <!-- Icon and badge -->
              <div class="mb-3 flex items-center gap-2">
                <div class="flex h-9 w-9 items-center justify-center rounded-sm border border-[#b99a5d] bg-[#e6d7b8] text-[#705729] dark:border-[#6f5a31] dark:bg-[#2a2419] dark:text-[#d7bc7e]">
                  <Icon name="bell" size="sm" :stroke-width="1.8" />
                </div>
                <span class="inline-flex items-center gap-1.5 rounded-sm border border-[#b99a5d] bg-[#eee5d2] px-2.5 py-1 text-xs font-medium text-[#705729] dark:border-[#6f5a31] dark:bg-[#2a2419] dark:text-[#d7bc7e]">
                  <span class="relative flex h-2 w-2">
                    <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-[#c8a96a] opacity-45"></span>
                    <span class="relative inline-flex h-2 w-2 rounded-full bg-[#c8a96a]"></span>
                  </span>
                  {{ t('announcements.unread') }}
                </span>
              </div>

              <!-- Title -->
              <h2 class="mb-2 break-words text-xl font-semibold leading-snug tracking-[-0.02em] text-gray-900 dark:text-[#f4f1ea] sm:text-2xl">
                {{ displayedAnnouncement.title }}
              </h2>

              <!-- Time -->
              <div class="flex items-center gap-1.5 font-mono text-xs tracking-wide text-gray-500 dark:text-[#9f9b94] sm:text-sm">
                <Icon name="clock" size="sm" />
                <time>{{ formatRelativeWithDateTime(displayedAnnouncement.created_at) }}</time>
              </div>
            </div>
          </div>

          <!-- Body -->
          <div class="max-h-[52vh] overflow-y-auto bg-[#f8f6f1] px-5 py-6 dark:bg-[#121214] sm:px-7 sm:py-7">
            <div class="relative">
              <div class="absolute bottom-0 left-0 top-0 w-0.5 bg-[#c8a96a]"></div>
              <div class="pl-4 sm:pl-5">
                <div
                  class="markdown-body prose prose-sm max-w-none dark:prose-invert"
                  v-html="renderedContent"
                ></div>
              </div>
            </div>
          </div>

          <!-- Footer -->
          <div class="border-t border-gray-200 bg-[#f1ece1] px-5 py-4 dark:border-white/10 dark:bg-[#181715] sm:px-7">
            <div class="flex items-center justify-end">
              <button
                @click="handleDismiss"
                data-testid="announcement-popup-dismiss"
                class="rounded-sm border border-[#c8a96a] bg-[#c8a96a] px-5 py-2.5 text-sm font-semibold text-[#111112] transition-colors hover:border-[#d7bc7e] hover:bg-[#d7bc7e] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#c8a96a]"
              >
                <span class="flex items-center gap-2">
                  <Icon :name="preview ? 'x' : 'check'" size="sm" :stroke-width="2" />
                  {{ preview ? t('common.close') : t('announcements.markRead') }}
                </span>
              </button>
            </div>
          </div>
        </div>
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

.popup-fade-enter-from > div {
  transform: scale(0.94) translateY(-12px);
  opacity: 0;
}

.popup-fade-leave-to > div {
  transform: scale(0.96) translateY(-8px);
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
  background: #b7b1a7;
  border-radius: 2px;
}

.dark .overflow-y-auto::-webkit-scrollbar-thumb {
  background: #4d4942;
}
</style>
