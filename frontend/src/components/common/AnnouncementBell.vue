<template>
  <div>
    <!-- 铃铛按钮 -->
    <button
      @click="openModal"
      class="relative flex h-10 w-10 items-center justify-center rounded-sm text-fg-muted transition-colors hover:bg-accent-weak hover:text-accent-strong"
      :class="{ 'text-accent': unreadCount > 0 }"
      :aria-label="t('announcements.title')"
    >
      <Icon name="bell" size="md" />
      <!-- 未读红点 -->
      <span
        v-if="unreadCount > 0"
        class="absolute right-2 top-2 flex h-2 w-2"
      >
        <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-meter opacity-75"></span>
        <span class="relative inline-flex h-2 w-2 rounded-full bg-meter ring-1 ring-meter-ink/40"></span>
      </span>
    </button>

    <!-- 公告列表 Modal -->
    <!-- Escape 由本组件的 handleEscape 统一处理（详情优先关闭），因此这里关闭 BaseDialog 自带的 Escape -->
    <BaseDialog
      :show="isModalOpen"
      :title="t('announcements.title')"
      :z-index="100"
      :close-on-escape="false"
      close-on-click-outside
      @close="closeModal"
    >
      <!-- Toolbar -->
      <div v-if="unreadCount > 0" class="mb-3 flex items-center justify-between gap-3">
        <p class="text-body text-fg-muted">
          <span class="font-semibold tabular-nums text-accent-strong">{{ unreadCount }}</span>
          {{ t('announcements.unread') }}
        </p>
        <button
          @click="markAllAsRead"
          :disabled="loading"
          class="btn btn-primary btn-sm"
        >
          {{ t('announcements.markAllRead') }}
        </button>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="spinner h-8 w-8 text-accent"></div>
      </div>

      <!-- Announcements List -->
      <div v-else-if="announcements.length > 0" class="divide-y divide-border">
        <div
          v-for="item in announcements"
          :key="item.id"
          class="group relative flex min-h-12 cursor-pointer items-center gap-3 px-2 py-3 transition-colors hover:bg-accent-weak focus-visible:bg-accent-weak focus-visible:-outline-offset-2"
          :class="{ 'shadow-[inset_3px_0_0_rgb(var(--meter))]': !item.read_at }"
          role="button"
          tabindex="0"
          @click="openDetail(item)"
          @keydown.enter.prevent="openDetail(item)"
          @keydown.space.prevent="openDetail(item)"
        >
          <!-- Status Indicator -->
          <Icon
            :name="item.read_at ? 'checkCircle' : 'infoCircle'"
            size="sm"
            :class="['flex-shrink-0', item.read_at ? 'text-fg-subtle' : 'text-accent']"
          />

          <!-- Content -->
          <div class="min-w-0 flex-1">
            <h3 class="truncate text-body font-semibold text-fg group-hover:text-accent-strong">
              {{ item.title }}
            </h3>
            <div class="mt-1 flex items-center gap-2">
              <time class="text-meta text-fg-muted">
                {{ formatRelativeTime(item.created_at) }}
              </time>
              <span v-if="!item.read_at" class="badge badge-primary">
                {{ t('announcements.unread') }}
              </span>
            </div>
          </div>

          <!-- Arrow -->
          <Icon
            name="chevronRight"
            size="sm"
            class="flex-shrink-0 text-fg-subtle group-hover:text-accent-strong"
          />
        </div>
      </div>

      <!-- Empty State -->
      <div v-else class="empty-state">
        <Icon name="inbox" size="xl" class="empty-state-icon" />
        <p class="text-body font-semibold text-fg">{{ t('announcements.empty') }}</p>
        <p class="mt-1 text-meta text-fg-muted">{{ t('announcements.emptyDescription') }}</p>
      </div>
    </BaseDialog>

    <!-- 公告详情 Modal -->
    <BaseDialog
      :show="detailModalOpen && !!selectedAnnouncement"
      :title="selectedAnnouncement?.title ?? ''"
      width="wide"
      :z-index="110"
      :close-on-escape="false"
      close-on-click-outside
      @close="closeDetail"
    >
      <template v-if="selectedAnnouncement">
        <!-- Meta Info -->
        <div class="mb-4 flex flex-wrap items-center gap-3">
          <span class="badge badge-primary">{{ t('announcements.title') }}</span>
          <span v-if="!selectedAnnouncement.read_at" class="badge badge-warning">
            {{ t('announcements.unread') }}
          </span>
          <span class="inline-flex items-center gap-1.5 text-meta text-fg-muted">
            <Icon name="clock" size="sm" />
            <time>{{ formatRelativeWithDateTime(selectedAnnouncement.created_at) }}</time>
          </span>
          <span class="inline-flex items-center gap-1.5 text-meta text-fg-muted">
            <Icon name="eye" size="sm" />
            {{ selectedAnnouncement.read_at ? t('announcements.read') : t('announcements.unread') }}
          </span>
        </div>

        <!-- Body with Markdown -->
        <div class="border border-accent/40 pl-4">
          <div
            class="markdown-body prose prose-sm max-w-none dark:prose-invert"
            v-html="renderMarkdown(selectedAnnouncement.content)"
          ></div>
        </div>
      </template>

      <template #footer>
        <div v-if="selectedAnnouncement" class="flex flex-1 flex-wrap items-center justify-between gap-3">
          <span class="inline-flex items-center gap-1.5 text-meta text-fg-muted">
            <Icon name="infoCircle" size="sm" />
            {{ selectedAnnouncement.read_at ? t('announcements.readStatus') : t('announcements.markReadHint') }}
          </span>
          <div class="flex items-center gap-3">
            <button @click="closeDetail" class="btn btn-secondary">
              {{ t('common.close') }}
            </button>
            <button
              v-if="!selectedAnnouncement.read_at"
              @click="markAsReadAndClose(selectedAnnouncement.id)"
              class="btn btn-primary flex items-center gap-2"
            >
              <Icon name="check" size="sm" />
              {{ t('announcements.markRead') }}
            </button>
          </div>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
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

function handleEscape(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    if (detailModalOpen.value) {
      closeDetail()
    } else if (isModalOpen.value) {
      closeModal()
    }
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
    document.body.style.overflow = (modal || detail || popup) ? 'hidden' : ''
  }
)
</script>
