<template>
  <Teleport to="body">
    <Transition name="popup-fade">
      <div
        v-if="displayedAnnouncement"
        class="fixed inset-0 z-[120] flex items-start justify-center overflow-y-auto bg-overlay/70 p-4 pt-[8vh]"
      >
        <div
          class="w-full max-w-[680px] overflow-hidden rounded border border-line bg-surface-raised shadow-modal"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          @click.stop
        >
          <!--
            The header used to run an amber-to-orange-to-yellow gradient behind
            two blurred colour blobs, a gradient icon tile with a coloured glow,
            and a gradient "unread" pill containing an `animate-ping` dot. Six
            decorative layers in a box whose job is to show one title and one
            paragraph. What is left says the same things: that it is unread,
            what it is called, and when it arrived.
          -->
          <div class="border-b border-line px-6 py-4">
            <Badge tone="warn" caps>{{ t('announcements.unread') }}</Badge>

            <h2 :id="titleId" class="mt-2 text-md font-semibold leading-tight text-ink">
              {{ displayedAnnouncement.title }}
            </h2>

            <time class="mt-1 block font-mono text-2xs tabular-nums text-ink-tertiary">
              {{ formatRelativeWithDateTime(displayedAnnouncement.created_at) }}
            </time>
          </div>

          <!--
            Body. The 4px gradient spine down the left of the prose is gone: it
            marked nothing, since there is only ever one block here.
          -->
          <div class="max-h-[50vh] overflow-y-auto px-6 py-5">
            <div
              class="markdown-body prose prose-sm max-w-none dark:prose-invert"
              v-html="renderedContent"
            ></div>
          </div>

          <div class="border-t border-line bg-surface-sunken px-6 py-3">
            <div class="flex items-center justify-end">
              <Button
                tone="accent"
                variant="solid"
                data-testid="announcement-popup-dismiss"
                @click="handleDismiss"
              >
                {{ preview ? t('common.close') : t('announcements.markRead') }}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeWithDateTime } from '@/utils/format'
import type { Announcement, UserAnnouncement } from '@/types'
// By path rather than through the common barrel: the barrel re-exports
// LocaleSwitcher, which drags `createI18n` into the graph of every spec that
// mocks vue-i18n with a partial factory — including this component's own.
import Badge from './Badge.vue'
import Button from './Button.vue'
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
const titleId = useId()
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
/*
 * Opacity and a 4px rise. The old transition scaled the dialog up from 0.94
 * and animated `all` over 300ms — both a longer duration than this system
 * allows and a gesture that reads as a notification lunging at the reader.
 */
.popup-fade-enter-active {
  transition:
    opacity var(--ds-dur-base) var(--ds-ease-out),
    transform var(--ds-dur-base) var(--ds-ease-out);
}

.popup-fade-leave-active {
  transition:
    opacity var(--ds-dur-fast) var(--ds-ease-in),
    transform var(--ds-dur-fast) var(--ds-ease-in);
}

.popup-fade-enter-from,
.popup-fade-leave-to {
  opacity: 0;
}

.popup-fade-enter-from > div,
.popup-fade-leave-to > div {
  transform: translateY(4px);
  opacity: 0;
}
</style>
