<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex justify-end gap-3">
          <button
            @click="loadAnnouncements"
            :disabled="loading"
            class="btn btn-secondary"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="unreadOnly" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
            <span>{{ t('announcements.unreadOnly') REDACTEDREDACTED</span>
          </label>
        </div>
      </template>

      <template #table>
        <div v-if="loading" class="flex items-center justify-center py-10">
          <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
        </div>

        <div v-else-if="announcements.length === 0" class="py-12 text-center text-gray-500 dark:text-gray-400">
          {{ unreadOnly ? t('announcements.emptyUnread') : t('announcements.empty') REDACTEDREDACTED
        </div>

        <div v-else class="space-y-4">
          <div
            v-for="item in announcements"
            :key="item.id"
            class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex items-start justify-between gap-4">
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <h3 class="truncate text-base font-semibold text-gray-900 dark:text-white">
                    {{ item.title REDACTEDREDACTED
                  </h3>
                  <span v-if="!item.read_at" class="badge badge-warning">
                    {{ t('announcements.unread') REDACTEDREDACTED
                  </span>
                  <span v-else class="badge badge-success">
                    {{ t('announcements.read') REDACTEDREDACTED
                  </span>
                </div>
                <div class="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
                  <span>{{ formatDateTime(item.created_at) REDACTEDREDACTED</span>
                  <span v-if="item.starts_at">
                    {{ t('announcements.startsAt') REDACTEDREDACTED: {{ formatDateTime(item.starts_at) REDACTEDREDACTED
                  </span>
                  <span v-if="item.ends_at">
                    {{ t('announcements.endsAt') REDACTEDREDACTED: {{ formatDateTime(item.ends_at) REDACTEDREDACTED
                  </span>
                </div>
              </div>

              <div class="flex flex-shrink-0 items-center gap-2">
                <button
                  v-if="!item.read_at"
                  class="btn btn-secondary"
                  :disabled="markingReadId === item.id"
                  @click="markRead(item.id)"
                >
                  {{ markingReadId === item.id ? t('common.processing') : t('announcements.markRead') REDACTEDREDACTED
                </button>
                <span v-else class="text-xs text-gray-500 dark:text-dark-400">
                  {{ t('announcements.readAt') REDACTEDREDACTED: {{ formatDateTime(item.read_at) REDACTEDREDACTED
                </span>
              </div>
            </div>

            <div class="mt-4 whitespace-pre-wrap text-sm text-gray-700 dark:text-gray-200">
              {{ item.content REDACTEDREDACTED
            </div>
          </div>
        </div>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { announcementsAPI REDACTED from '@/api'
import { useAppStore REDACTED from '@/stores/app'
import { formatDateTime REDACTED from '@/utils/format'
import type { UserAnnouncement REDACTED from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'

const { t REDACTED = useI18n()
const appStore = useAppStore()

const announcements = ref<UserAnnouncement[]>([])
const loading = ref(false)
const unreadOnly = ref(false)
const markingReadId = ref<number | null>(null)

async function loadAnnouncements() {
  try {
    loading.value = true
    announcements.value = await announcementsAPI.list(unreadOnly.value)
  REDACTED catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

async function markRead(id: number) {
  if (markingReadId.value) return
  try {
    markingReadId.value = id
    await announcementsAPI.markRead(id)
    await loadAnnouncements()
  REDACTED catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  REDACTED finally {
    markingReadId.value = null
  REDACTED
REDACTED

watch(unreadOnly, () => {
  loadAnnouncements()
REDACTED)

onMounted(() => {
  loadAnnouncements()
REDACTED)
</script>
