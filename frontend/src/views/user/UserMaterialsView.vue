<template>
  <AppLayout>
  <div class="user-materials-view space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div>
        <h1 class="text-lg font-semibold text-gray-900 dark:text-dark-100">
          {{ t('materials.title') }}
        </h1>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
          {{ t('materials.description') }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <label class="btn btn-primary btn-sm cursor-pointer">
          <input type="file" :accept="acceptForKind(activeKind)" class="hidden" @change="onFilePicked" />
          {{ t('materials.uploadBtn') }}
        </label>
        <button type="button" class="btn btn-secondary btn-sm" @click="showUrlImport = !showUrlImport">
          {{ t('materials.importUrlBtn') }}
        </button>
      </div>
    </div>

    <!-- URL 导入区（默认折叠）-->
    <div v-if="showUrlImport" class="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800">
      <input v-model="importUrl" type="text" class="input h-9 flex-1 text-sm" placeholder="https://..." />
      <button type="button" class="btn btn-primary btn-sm" :disabled="!importUrl || importing" @click="doImportUrl">
        {{ importing ? t('common.loading', 'Loading...') : t('materials.importUrlConfirm') }}
      </button>
    </div>

    <!-- 类型切换 tabs -->
    <div class="flex flex-wrap items-center gap-2 border-b border-gray-200 dark:border-dark-700">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        type="button"
        class="border-b-2 px-3 py-2 text-sm font-medium transition"
        :class="activeKind === tab.value
          ? 'border-primary-500 text-primary-600 dark:text-primary-400'
          : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200'"
        @click="switchKind(tab.value)"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- 搜索 -->
    <div class="flex items-center gap-2">
      <input
        v-model="keyword"
        type="text"
        class="input h-9 max-w-sm flex-1 text-sm"
        :placeholder="t('materials.searchPlaceholder')"
        @keyup.enter="reload"
      />
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="reload">
        {{ t('common.search', 'Search') }}
      </button>
    </div>

    <!-- 主体 -->
    <div v-if="loading" class="py-10 text-center text-sm text-gray-500">
      {{ t('common.loading', 'Loading...') }}
    </div>
    <div v-else-if="items.length === 0" class="py-10 text-center text-sm text-gray-500">
      {{ t('materials.empty') }}
    </div>
    <div v-else class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-6">
      <div
        v-for="item in items"
        :key="item.id"
        class="group relative flex flex-col overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="relative flex aspect-square items-center justify-center overflow-hidden bg-gray-50 dark:bg-dark-900">
          <img
            v-if="item.kind === 'image'"
            :src="item.url"
            :alt="item.file_name"
            class="h-full w-full object-cover"
            loading="lazy"
          />
          <video
            v-else-if="item.kind === 'video'"
            :src="item.url"
            class="h-full w-full object-cover"
            muted
            preload="metadata"
          />
          <audio
            v-else-if="item.kind === 'audio'"
            :src="item.url"
            controls
            class="w-full px-2"
          />
          <span v-else class="text-3xl text-gray-400">📄</span>
        </div>
        <div class="p-2 text-xs">
          <div class="truncate font-medium text-gray-800 dark:text-dark-200" :title="item.file_name">
            {{ item.file_name || `#${item.id}` }}
          </div>
          <div class="mt-1 flex items-center justify-between text-[11px] text-gray-500">
            <span>{{ formatBytes(item.size_bytes) }}</span>
            <span>{{ formatDateTime(item.created_at) }}</span>
          </div>
          <div class="mt-1 flex items-center gap-2">
            <button
              type="button"
              class="text-primary-600 hover:underline dark:text-primary-400"
              @click="doCopy(item.url)"
            >
              {{ t('common.copy', 'Copy') }}
            </button>
            <a :href="item.url" target="_blank" rel="noopener noreferrer" class="text-gray-500 hover:underline">
              {{ t('materials.openLink') }}
            </a>
            <button
              type="button"
              class="ml-auto text-red-500 hover:underline"
              @click="doRemove(item)"
            >
              {{ t('common.remove') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 分页 -->
    <div v-if="total > pageSize" class="flex items-center justify-between text-xs text-gray-500">
      <span>
        {{ t('materials.pageInfo', { page, total: Math.ceil(total / pageSize) }) }}
      </span>
      <div class="flex gap-2">
        <button type="button" class="btn btn-ghost btn-sm" :disabled="page <= 1" @click="goPage(page - 1)">
          {{ t('common.prev', 'Prev') }}
        </button>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="page * pageSize >= total" @click="goPage(page + 1)">
          {{ t('common.next', 'Next') }}
        </button>
      </div>
    </div>
  </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * UserMaterialsView：用户菜单里的"素材库"独立页面。
 *
 * - 类型 tabs：image / audio / video 三选一（默认 image）
 * - 支持上传 / 从 URL 导入 / 搜索 / 分页 / 删除 / 复制 URL / 打开新标签页
 * - 所有素材的存储位置：COS 桶下 users/{user_id}/materials/YYYY/MM/{uuid}.{ext}
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import userMaterialsAPI, { type UserMaterialItem, type UserMaterialKind } from '@/api/userMaterials'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { formatBytes, formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const activeKind = ref<UserMaterialKind>('image')
const keyword = ref('')
const items = ref<UserMaterialItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 24
const loading = ref(false)

const importUrl = ref('')
const importing = ref(false)
const showUrlImport = ref(false)

const tabs = computed<{ value: UserMaterialKind; label: string }[]>(() => [
  { value: 'image', label: t('materials.kindImage') },
  { value: 'audio', label: t('materials.kindAudio') },
  { value: 'video', label: t('materials.kindVideo') },
])

function acceptForKind(k: UserMaterialKind): string {
  switch (k) {
    case 'image': return 'image/*'
    case 'audio': return 'audio/*'
    case 'video': return 'video/*'
    default: return '*/*'
  }
}

async function reload() {
  loading.value = true
  try {
    const resp = await userMaterialsAPI.list({
      kind: activeKind.value,
      keyword: keyword.value.trim(),
      page: page.value,
      pageSize,
    })
    items.value = resp.data.items || []
    total.value = resp.data.total || 0
  } catch (e: unknown) {
    appStore.showError(errMessage(e))
  } finally {
    loading.value = false
  }
}

function switchKind(k: UserMaterialKind) {
  activeKind.value = k
  page.value = 1
  void reload()
}

function goPage(p: number) {
  page.value = p
  void reload()
}

async function onFilePicked(ev: Event) {
  const target = ev.target as HTMLInputElement
  const f = target.files?.[0]
  if (!f) return
  try {
    await userMaterialsAPI.upload(f)
    appStore.showSuccess(t('materials.uploadSuccess'))
    // 上传成功后回到第一页刷新（最新素材会在顶部）
    page.value = 1
    void reload()
  } catch (e: unknown) {
    appStore.showError(errMessage(e))
  } finally {
    target.value = ''
  }
}

async function doImportUrl() {
  const url = importUrl.value.trim()
  if (!url) return
  importing.value = true
  try {
    await userMaterialsAPI.importFromUrl(url)
    appStore.showSuccess(t('materials.uploadSuccess'))
    importUrl.value = ''
    showUrlImport.value = false
    page.value = 1
    void reload()
  } catch (e: unknown) {
    appStore.showError(errMessage(e))
  } finally {
    importing.value = false
  }
}

async function doRemove(item: UserMaterialItem) {
  if (!window.confirm(t('materials.confirmRemove', { name: item.file_name }))) return
  try {
    await userMaterialsAPI.remove(item.id)
    appStore.showSuccess(t('common.removeSuccess', 'Removed'))
    void reload()
  } catch (e: unknown) {
    appStore.showError(errMessage(e))
  }
}

function doCopy(url: string) {
  if (!url) return
  void copyToClipboard(url, t('common.copied', 'Copied'))
}

function errMessage(e: unknown): string {
  if (e instanceof Error) return e.message
  return String(e)
}

onMounted(() => {
  void reload()
})
</script>
