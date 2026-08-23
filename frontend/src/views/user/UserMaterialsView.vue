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
    <div v-else class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2">
      <div
        v-for="item in items"
        :key="item.id"
        class="group relative flex min-w-0 flex-col overflow-hidden rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="relative flex aspect-square items-center justify-center overflow-hidden bg-gray-50 dark:bg-dark-900">
          <button
            v-if="item.kind === 'image'"
            type="button"
            class="h-full w-full cursor-zoom-in overflow-hidden focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
            :aria-label="t('materials.previewImage')"
            @click="openPreview(item)"
          >
            <img
              :src="item.url"
              :alt="item.file_name"
              class="h-full w-full object-cover transition-transform duration-200 group-hover:scale-[1.02]"
              loading="lazy"
            />
          </button>
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
        <div class="min-w-0 p-1.5 text-xs">
          <div class="truncate font-medium text-gray-800 dark:text-dark-200" :title="item.file_name">
            {{ item.file_name || `#${item.id}` }}
          </div>
          <div class="mt-0.5 flex min-w-0 items-center justify-between gap-1 text-[10px] text-gray-500">
            <span class="shrink-0">{{ formatBytes(item.size_bytes) }}</span>
            <span class="truncate" :title="formatDateTime(item.created_at)">{{ compactDate(item.created_at) }}</span>
          </div>
          <div class="mt-1 flex items-center justify-between border-t border-gray-100 pt-1 dark:border-dark-700">
            <button
              type="button"
              class="inline-flex h-6 w-6 items-center justify-center rounded text-gray-500 transition hover:bg-gray-100 hover:text-primary-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :title="t('common.copy', 'Copy')"
              :aria-label="t('common.copy', 'Copy')"
              @click="doCopy(item.url)"
            >
              <Icon name="copy" size="xs" />
            </button>
            <a
              :href="item.url"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex h-6 w-6 items-center justify-center rounded text-gray-500 transition hover:bg-gray-100 hover:text-primary-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :title="t('materials.openLink')"
              :aria-label="t('materials.openLink')"
            >
              <Icon name="externalLink" size="xs" />
            </a>
            <button
              type="button"
              class="inline-flex h-6 w-6 items-center justify-center rounded text-gray-500 transition hover:bg-gray-100 hover:text-primary-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :title="t('materials.rename')"
              :aria-label="t('materials.rename')"
              @click="openRename(item)"
            >
              <Icon name="edit" size="xs" />
            </button>
            <button
              type="button"
              class="inline-flex h-6 w-6 items-center justify-center rounded text-gray-500 transition hover:bg-red-50 hover:text-red-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500 dark:text-dark-400 dark:hover:bg-red-950/30 dark:hover:text-red-400"
              :title="t('common.remove')"
              :aria-label="t('common.remove')"
              @click="doRemove(item)"
            >
              <Icon name="trash" size="xs" />
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
          {{ t('materials.prevPage') }}
        </button>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="page * pageSize >= total" @click="goPage(page + 1)">
          {{ t('materials.nextPage') }}
        </button>
      </div>
    </div>

    <BaseDialog
      :show="!!renameTarget"
      :title="t('materials.renameTitle')"
      width="narrow"
      @close="closeRename"
    >
      <div class="space-y-2">
        <label for="material-rename-input" class="block text-sm font-medium text-gray-700 dark:text-dark-200">
          {{ t('materials.rename') }}
        </label>
        <input
          id="material-rename-input"
          ref="renameInput"
          v-model="renameName"
          data-testid="material-rename-input"
          type="text"
          maxlength="512"
          class="input w-full"
          :placeholder="t('materials.renamePlaceholder')"
          :disabled="renaming"
          @keyup.enter="submitRename"
        />
        <p class="text-right text-xs text-gray-400 dark:text-dark-500">{{ renameName.length }} / 512</p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" :disabled="renaming" @click="closeRename">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="!canSubmitRename" @click="submitRename">
            <Icon v-if="renaming" name="refresh" size="sm" class="animate-spin" />
            <span>{{ t('common.save') }}</span>
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="!!previewItem"
      :title="previewItem?.file_name || t('materials.previewImage')"
      width="extra-wide"
      @close="closePreview"
    >
      <div
        v-if="previewItem"
        class="flex min-h-60 items-center justify-center overflow-hidden bg-gray-50 dark:bg-dark-950"
      >
        <img
          data-testid="material-preview-image"
          :src="previewItem.url"
          :alt="previewItem.file_name"
          class="max-h-[70vh] max-w-full object-contain"
        />
      </div>
      <template #footer>
        <div class="flex flex-wrap justify-end gap-2">
          <a
            v-if="previewItem"
            :href="previewItem.url"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary"
          >
            <Icon name="externalLink" size="sm" />
            <span>{{ t('materials.openLink') }}</span>
          </a>
          <button type="button" class="btn btn-primary" @click="closePreview">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * UserMaterialsView：用户菜单里的"素材库"独立页面。
 *
 * - 类型 tabs：image / audio / video 三选一（默认 image）
 * - 支持上传 / 从 URL 导入 / 搜索 / 分页 / 图片预览 / 改名 / 删除 / 复制 URL / 打开新标签页
 * - 所有素材的存储位置：COS 桶下 users/{user_id}/materials/YYYY/MM/{uuid}.{ext}
 */
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import userMaterialsAPI, { type UserMaterialItem, type UserMaterialKind } from '@/api/userMaterials'
import { extractI18nErrorMessage } from '@/utils/apiError'
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
const previewItem = ref<UserMaterialItem | null>(null)
const renameTarget = ref<UserMaterialItem | null>(null)
const renameName = ref('')
const renameInput = ref<HTMLInputElement | null>(null)
const renaming = ref(false)

const canSubmitRename = computed(() => {
  const name = renameName.value.trim()
  return !renaming.value && !!renameTarget.value && name.length > 0 && name !== renameTarget.value.file_name
})

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

async function openRename(item: UserMaterialItem) {
  renameTarget.value = item
  renameName.value = item.file_name
  await nextTick()
  renameInput.value?.focus()
  renameInput.value?.select()
}

function openPreview(item: UserMaterialItem) {
  previewItem.value = item
}

function closePreview() {
  previewItem.value = null
}

function closeRename() {
  if (renaming.value) return
  renameTarget.value = null
  renameName.value = ''
}

async function submitRename() {
  const target = renameTarget.value
  const name = renameName.value.trim()
  if (!target || !name || name === target.file_name || renaming.value) return
  renaming.value = true
  try {
    const response = await userMaterialsAPI.rename(target.id, name)
    const index = items.value.findIndex((item) => item.id === target.id)
    if (index >= 0) items.value[index] = response.data
    appStore.showSuccess(t('materials.renameSuccess'))
    renameTarget.value = null
    renameName.value = ''
  } catch (e: unknown) {
    appStore.showError(errMessage(e))
  } finally {
    renaming.value = false
  }
}

function doCopy(url: string) {
  if (!url) return
  void copyToClipboard(url, t('common.copied', 'Copied'))
}

function compactDate(value: string): string {
  return value ? value.slice(0, 10) : '-'
}

/**
 * errMessage：把捕获到的错误转成可展示文案。
 * 走 extractI18nErrorMessage —— apiClient 拦截器 reject 的是普通对象而非 Error，
 * 直接 String(e) 会显示 "[object Object]"；该工具会按 reason 查 materials.errors
 * 给出友好文案，查不到再回落到后端原始 message。
 */
function errMessage(e: unknown): string {
  return extractI18nErrorMessage(e, t, 'materials.errors', t('common.error'))
}

onMounted(() => {
  void reload()
})
</script>
