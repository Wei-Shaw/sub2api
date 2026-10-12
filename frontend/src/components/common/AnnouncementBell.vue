<template>
  <div ref="root" class="relative">
    <button class="relative flex h-9 w-9 items-center justify-center rounded-lg text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-800" :aria-label="t('announcements.title')" @click="toggle">
      <Icon name="bell" size="md" />
      <span v-if="unreadCount > 0" class="absolute -right-1 -top-1 flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-semibold text-white ring-2 ring-white dark:ring-dark-900">{{ unreadCount > 99 ? '99+' : unreadCount }}</span>
    </button>
    <Transition name="notice-drop">
      <section v-if="open" class="fixed left-2 right-2 top-14 z-[120] flex max-h-[calc(100vh-4rem)] flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-800 sm:absolute sm:left-auto sm:right-0 sm:top-full sm:mt-2 sm:w-[420px]" @click.stop>
        <header class="flex items-start justify-between gap-3 px-4 pb-3 pt-4">
          <div><h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('announcements.systemTitle') }}</h2><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('announcements.systemDescription') }}</p></div>
          <button class="newapi-icon-button" @click="close"><Icon name="x" size="sm" /></button>
        </header>
        <div class="mx-4 grid grid-cols-2 rounded-lg bg-gray-100 p-1 dark:bg-dark-900">
          <button v-for="tab in tabs" :key="tab.value" class="flex h-9 items-center justify-center gap-2 rounded-md text-sm font-medium" :class="activeTab === tab.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 dark:text-gray-400'" @click="selectTab(tab.value)"><Icon :name="tab.icon" size="sm" />{{ t(tab.label) }}</button>
        </div>
        <div class="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          <div v-if="loading" class="flex h-56 items-center justify-center"><div class="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-t-blue-600"></div></div>
          <template v-else-if="activeTab === 'notice'">
            <LongNoticeRenderer :document="longNotice" />
            <p v-if="longNotice.blocks.length === 0" class="py-14 text-center text-sm text-gray-400">{{ t('announcements.empty') }}</p>
          </template>
          <template v-else-if="selected">
            <button class="mb-3 inline-flex items-center text-xs font-medium text-blue-600" @click="selected = null">← {{ t('announcements.timeline') }}</button>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ selected.title }}</h3><p class="mt-1 text-xs text-gray-400">{{ formatRelativeWithDateTime(selected.created_at) }}</p>
            <div class="markdown-body prose prose-sm mt-4 max-w-none dark:prose-invert" v-html="renderMarkdown(selected.content)"></div>
          </template>
          <template v-else>
            <button v-for="item in announcements" :key="item.id" class="flex w-full items-start gap-3 border-b border-gray-100 py-3 text-left last:border-0 dark:border-dark-700" @click="openItem(item)">
              <span class="mt-1.5 h-2 w-2 flex-shrink-0 rounded-full" :class="item.read_at ? 'bg-gray-300 dark:bg-gray-600' : 'bg-emerald-500'"></span>
              <span class="min-w-0 flex-1"><span class="line-clamp-2 text-sm font-medium text-gray-900 dark:text-white">{{ item.title }}</span><span class="mt-1 block line-clamp-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ plainText(item.content) }}</span><span class="mt-1 block text-xs text-gray-400">{{ formatRelativeTime(item.created_at) }} · {{ formatDateTime(item.created_at) }}</span></span>
            </button>
            <p v-if="announcements.length === 0" class="py-14 text-center text-sm text-gray-400">{{ t('announcements.noHistory') }}</p>
          </template>
        </div>
        <footer class="flex items-center justify-between border-t border-gray-100 px-4 py-3 dark:border-dark-700"><button type="button" :disabled="unreadCount === 0" class="text-xs font-medium text-blue-600 disabled:cursor-default disabled:text-gray-400 dark:disabled:text-gray-500" @click="markAllAsRead">{{ t(unreadCount ? 'announcements.markAllRead' : 'announcements.noUnread') }}</button><button class="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white dark:bg-white dark:text-gray-900" @click="close">{{ t('common.close') }}</button></footer>
      </section>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'
import { parseLongNoticeDocument } from '@/types/longNotice'
import type { UserAnnouncement } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import LongNoticeRenderer from './LongNoticeRenderer.vue'
import '@/styles/announcement-markdown.css'
type TabValue = 'notice' | 'timeline'
type IconName = InstanceType<typeof Icon>['$props']['name']
const { t } = useI18n(); const appStore = useAppStore(); const store = useAnnouncementStore(); const { announcements, loading, currentPopup } = storeToRefs(store)
const root = ref<HTMLElement | null>(null); const open = ref(false); const activeTab = ref<TabValue>('notice'); const selected = ref<UserAnnouncement | null>(null)
const unreadCount = computed(() => store.unreadCount)
const longNotice = computed(() => parseLongNoticeDocument(appStore.cachedPublicSettings?.long_notice_content))
const tabs: Array<{ value: TabValue; label: string; icon: IconName }> = [{ value: 'notice', label: 'announcements.notice', icon: 'bell' }, { value: 'timeline', label: 'announcements.timeline', icon: 'clock' }]
marked.setOptions({ breaks: true, gfm: true })
function renderMarkdown(content: string) { return DOMPurify.sanitize(marked.parse(content || '') as string) }
function plainText(content: string) { const el = document.createElement('div'); el.innerHTML = renderMarkdown(content); return el.textContent?.trim() || '' }
function formatDateTime(value: string) { return new Intl.DateTimeFormat(undefined, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value)) }
function toggle() { open.value ? close() : show('notice') }
function show(tab: TabValue) { activeTab.value = tab; selected.value = null; open.value = true }
function close() { open.value = false; selected.value = null; if (currentPopup.value) store.acknowledgeCurrentPopup() }
function selectTab(tab: TabValue) { activeTab.value = tab; selected.value = null }
function openItem(item: UserAnnouncement) { selected.value = item; if (!item.read_at) store.markAsRead(item.id) }
async function markAllAsRead() { try { await store.markAllAsRead(); appStore.showSuccess(t('announcements.allMarkedAsRead')) } catch { appStore.showError(t('common.unknownError')) } }
function outside(event: MouseEvent) { if (open.value && root.value && !root.value.contains(event.target as Node)) close() }
function escape(event: KeyboardEvent) { if (event.key === 'Escape') close() }
onMounted(() => { document.addEventListener('click', outside); document.addEventListener('keydown', escape) })
onBeforeUnmount(() => { document.removeEventListener('click', outside); document.removeEventListener('keydown', escape) })
watch(currentPopup, (item) => { if (item) show('timeline') }, { immediate: true })
</script>
<style scoped>.notice-drop-enter-active,.notice-drop-leave-active{transition:opacity .16s ease,transform .16s ease}.notice-drop-enter-from,.notice-drop-leave-to{opacity:0;transform:translateY(-6px)}</style>
