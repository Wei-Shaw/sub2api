<template>
  <div>
    <!--
      铃铛按钮. The unread marker was an 8px red dot under an `animate-ping`
      halo running forever, on a button that also scaled on hover. A dot that
      pulses until you read it is an interface demanding to be obeyed; the
      count is the actual fact, so the count is what it shows.
    -->
    <button
      @click="openModal"
      class="relative flex h-9 items-center gap-1.5 rounded px-2 text-ink-secondary transition-colors duration-fast hover:bg-surface-hover hover:text-ink"
      :aria-label="
        unreadCount > 0
          ? `${t('announcements.title')} — ${unreadCount} ${t('announcements.unread')}`
          : t('announcements.title')
      "
    >
      <Icon name="bell" size="md" />
      <span
        v-if="unreadCount > 0"
        class="font-mono text-2xs tabular-nums text-accent"
        aria-hidden="true"
        >{{ unreadCount }}</span
      >
    </button>

    <!-- 公告列表 -->
    <BaseDialog
      :show="isModalOpen"
      :title="t('announcements.title')"
      width="normal"
      close-on-click-outside
      @close="closeModal"
    >
      <div
        v-if="unreadCount > 0"
        class="mb-3 flex items-center justify-between gap-4 border-b border-line pb-3"
      >
        <p class="text-xs text-ink-tertiary">
          <span class="font-mono tabular-nums text-ink">{{ unreadCount }}</span>
          {{ t('announcements.unread') }}
        </p>
        <Button :loading="loading" @click="markAllAsRead">
          {{ t('announcements.markAllRead') }}
        </Button>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <!--
        Rows, not cards. Every row used to carry a 40px gradient tile with a
        pulse ring, a gradient spine down its left edge, a tinted ground and an
        arrow that slid on hover — five treatments to express one boolean.
        Unread is the accent rule at the left, and it is written in words too.
      -->
      <div v-else-if="announcements.length > 0" class="-mx-4 divide-y divide-line-subtle">
        <button
          v-for="item in announcements"
          :key="item.id"
          type="button"
          class="relative flex w-full items-start gap-3 px-4 py-3 text-left transition-colors duration-fast hover:bg-surface-hover"
          @click="openDetail(item)"
        >
          <span
            v-if="!item.read_at"
            class="absolute inset-y-0 left-0 w-0.5 bg-accent"
            aria-hidden="true"
          ></span>

          <span class="min-w-0 flex-1">
            <span class="block truncate text-sm font-medium text-ink">{{ item.title }}</span>
            <span class="mt-1 flex items-center gap-2">
              <time class="font-mono text-2xs tabular-nums text-ink-tertiary">
                {{ formatRelativeTime(item.created_at) }}
              </time>
              <Badge v-if="!item.read_at" tone="accent" caps>
                {{ t('announcements.unread') }}
              </Badge>
            </span>
          </span>

          <span class="shrink-0 pt-0.5 font-mono text-xs text-ink-tertiary" aria-hidden="true"
            >→</span
          >
        </button>
      </div>

      <EmptyState
        v-else
        :message="t('announcements.empty')"
        :description="t('announcements.emptyDescription')"
      />
    </BaseDialog>

    <!-- 公告详情 -->
    <BaseDialog
      :show="detailModalOpen && !!selectedAnnouncement"
      :title="selectedAnnouncement?.title || ''"
      width="wide"
      :z-index="110"
      @close="closeDetail"
    >
      <template v-if="selectedAnnouncement">
        <div class="mb-4 flex flex-wrap items-center gap-2 border-b border-line pb-3">
          <Badge v-if="!selectedAnnouncement.read_at" tone="accent" caps>
            {{ t('announcements.unread') }}
          </Badge>
          <Badge v-else caps>{{ t('announcements.read') }}</Badge>
          <time class="font-mono text-2xs tabular-nums text-ink-tertiary">
            {{ formatRelativeWithDateTime(selectedAnnouncement.created_at) }}
          </time>
        </div>

        <div
          class="markdown-body prose prose-sm max-w-none dark:prose-invert"
          v-html="renderMarkdown(selectedAnnouncement.content)"
        ></div>
      </template>

      <template #footer>
        <div class="flex items-center justify-between gap-4">
          <span class="min-w-0 truncate text-2xs text-ink-tertiary">
            {{
              selectedAnnouncement?.read_at
                ? t('announcements.readStatus')
                : t('announcements.markReadHint')
            }}
          </span>
          <div class="flex shrink-0 items-center gap-2">
            <Button @click="closeDetail">{{ t('common.close') }}</Button>
            <Button
              v-if="selectedAnnouncement && !selectedAnnouncement.read_at"
              tone="accent"
              variant="solid"
              @click="markAsReadAndClose(selectedAnnouncement.id)"
            >
              {{ t('announcements.markRead') }}
            </Button>
          </div>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import Icon from '@/components/icons/Icon.vue'
// By path, not through the barrel: it re-exports LocaleSwitcher, which pulls
// `createI18n` into the graph of any spec that mocks vue-i18n partially.
import BaseDialog from './BaseDialog.vue'
import Badge from './Badge.vue'
import Button from './Button.vue'
import EmptyState from './EmptyState.vue'
import LoadingSpinner from './LoadingSpinner.vue'
import '@/styles/announcement-markdown.css'

const { t } = useI18n()
const appStore = useAppStore()
const announcementStore = useAnnouncementStore()

// Configure marked
marked.setOptions({
  breaks: true,
  gfm: true,
})

// Use store state (storeToRefs for reactivity)
const { announcements, loading } = storeToRefs(announcementStore)
const unreadCount = computed(() => announcementStore.unreadCount)

// Local modal state
const isModalOpen = ref(false)
const detailModalOpen = ref(false)
const selectedAnnouncement = ref<UserAnnouncement | null>(null)

// Methods
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
    markAsRead(announcement.id)
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

/*
 * Escape, scroll locking and the backdrop all belong to BaseDialog, which owns
 * the dialog stack. This component used to hand-roll all three: a
 * document-level keydown listener, a `document.body.style.overflow` watcher
 * shared with the popup, and its own backdrop element — three copies of
 * behaviour that has to agree with every other dialog in the app, and the
 * reason a nested announcement dialog could leave the page unscrollable.
 */
</script>
