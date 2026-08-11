<template>
  <AppLayout>
    <div class="space-y-4">
    <!-- 页头 -->
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('admin.files.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.files.subtitle') }}
        </p>
      </div>
    </div>

    <!-- 未启用：引导去系统设置。文件管理完全依赖对象存储，没配置时列表毫无意义 -->
    <div
      v-if="statusLoaded && !status?.enabled"
      class="rounded-lg border border-yellow-300 bg-yellow-50 p-6 dark:border-yellow-800 dark:bg-yellow-950/30"
    >
      <h2 class="text-base font-semibold text-yellow-900 dark:text-yellow-200">
        {{ t('admin.files.disabledTitle') }}
      </h2>
      <p class="mt-2 text-sm text-yellow-800 dark:text-yellow-300">
        {{ t('admin.files.disabledHint') }}
      </p>
      <router-link to="/admin/settings" class="btn btn-primary btn-sm mt-4">
        {{ t('admin.files.gotoSettings') }}
      </router-link>
    </div>

    <template v-else-if="status?.enabled">
      <!-- 桶信息 + 工具栏 -->
      <div class="card space-y-3 p-4">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
          <span>
            {{ t('admin.files.bucket') }}:
            <code class="font-mono text-gray-700 dark:text-gray-200">{{ status.bucket }}</code>
          </span>
          <span v-if="status.prefix">
            {{ t('admin.files.configuredPrefix') }}:
            <code class="font-mono text-gray-700 dark:text-gray-200">{{ status.prefix }}</code>
          </span>
        </div>

        <div data-testid="file-toolbar" class="flex flex-wrap items-center gap-2">
          <button type="button" class="btn btn-secondary btn-sm h-9" :disabled="loading" @click="reload">
            {{ t('common.refresh') }}
          </button>
          <button type="button" class="btn btn-secondary btn-sm h-9" :disabled="uploading" @click="triggerUpload">
            {{ t('admin.files.upload') }}
          </button>
          <input ref="fileInputEl" type="file" multiple class="hidden" @change="onFilesPicked" />
          <button
            type="button"
            class="btn btn-secondary btn-sm h-9"
            :disabled="importingUrl"
            :aria-expanded="showUrlImport"
            @click="showUrlImport = !showUrlImport"
          >
            {{ t('admin.files.importUrl') }}
          </button>
          <input
            v-model="searchInput"
            type="text"
            class="input ml-auto h-9 max-w-xs py-1.5"
            :placeholder="t('admin.files.searchPlaceholder')"
            @keyup.enter="applySearch"
          />
          <button type="button" class="btn btn-secondary btn-sm h-9" @click="applySearch">
            {{ t('common.search') }}
          </button>
          <button v-if="search" type="button" class="btn btn-ghost btn-sm h-9" @click="clearSearch">
            {{ t('common.reset') }}
          </button>
          <span v-if="search" class="text-xs text-gray-500">{{ t('admin.files.searchHint') }}</span>
        </div>

        <div
          v-if="showUrlImport"
          class="flex flex-wrap items-end gap-2 border-t border-gray-200 pt-3 dark:border-dark-700"
        >
          <div class="min-w-64 flex-1">
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('admin.files.importUrlLabel') }}
            </label>
            <input
              v-model="importUrl"
              type="url"
              class="input h-9 py-1.5"
              :disabled="importingUrl"
              placeholder="https://..."
              @keyup.enter="submitUrlImport"
            />
          </div>
          <div class="w-56 max-w-full">
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('admin.files.importNameLabel') }}
            </label>
            <input
              v-model="importName"
              type="text"
              class="input h-9 py-1.5"
              :disabled="importingUrl"
              :placeholder="t('admin.files.importNamePlaceholder')"
              @keyup.enter="submitUrlImport"
            />
          </div>
          <button
            type="button"
            class="btn btn-primary btn-sm h-9 shrink-0 self-end"
            :disabled="importingUrl || !importUrl.trim()"
            @click="submitUrlImport"
          >
            {{ importingUrl ? t('common.loading') : t('admin.files.importConfirm') }}
          </button>
          <span class="w-full text-xs text-gray-500">
            {{ t('admin.files.importCurrentDirectory', { path: currentDirectoryLabel }) }}
          </span>
        </div>
      </div>

      <!-- 批量操作条 -->
      <div
        v-if="selectedKeys.length"
        class="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-primary-300 bg-primary-50 px-4 py-2 dark:border-primary-800 dark:bg-primary-950/30"
      >
        <span class="text-sm text-primary-800 dark:text-primary-200">
          {{ t('admin.files.selectedCount', { n: selectedKeys.length }) }}
        </span>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-ghost btn-xs" @click="clearSelection">
            {{ t('admin.files.clearSelection') }}
          </button>
          <button type="button" class="btn btn-danger btn-xs" @click="confirmDeleteSelected">
            {{ t('admin.files.deleteSelected') }}
          </button>
        </div>
      </div>

      <!-- 上传进度 -->
      <div v-if="uploading" class="card p-4">
        <p class="mb-2 text-sm text-gray-700 dark:text-gray-200">
          {{ t('admin.files.uploadingProgress', { i: uploadIndex, n: uploadTotal, name: uploadName }) }}
        </p>
        <div class="h-2 w-full overflow-hidden rounded bg-gray-200 dark:bg-dark-700">
          <div class="h-full bg-primary-500 transition-all" :style="{ width: uploadPercent + '%' }" />
        </div>
      </div>

      <!-- 列表 -->
      <div
        data-testid="file-list"
        class="card relative overflow-hidden transition-colors"
        :class="{ 'ring-2 ring-primary-500 ring-offset-2 dark:ring-offset-dark-950': dragActive }"
        @dragenter.prevent="handleDragEnter"
        @dragover.prevent="handleDragOver"
        @dragleave.prevent="handleDragLeave"
        @drop.prevent="handleDrop"
      >
        <!-- 面包屑紧贴目录树。对象存储没有真目录，这里按 "/" 聚合逻辑层级。 -->
        <div
          data-testid="directory-breadcrumbs"
          class="flex flex-wrap items-center gap-1 border-b border-gray-200 bg-gray-50 px-4 py-3 text-sm dark:border-dark-700 dark:bg-dark-800"
        >
          <button type="button" class="text-primary-600 hover:underline dark:text-primary-400" @click="goPrefix('')">
            {{ t('admin.files.root') }}
          </button>
          <template v-for="(seg, i) in breadcrumbs" :key="seg.prefix">
            <span class="text-gray-400">/</span>
            <button
              v-if="i < breadcrumbs.length - 1"
              type="button"
              class="text-primary-600 hover:underline dark:text-primary-400"
              @click="goPrefix(seg.prefix)"
            >
              {{ seg.name }}
            </button>
            <span v-else class="font-medium text-gray-900 dark:text-white">{{ seg.name }}</span>
          </template>
        </div>
        <div
          v-if="dragActive"
          class="pointer-events-none absolute inset-0 z-20 flex items-center justify-center bg-primary-50/95 p-6 text-center dark:bg-primary-950/95"
        >
          <div class="flex flex-col items-center gap-2 text-primary-700 dark:text-primary-300">
            <Icon name="upload" size="xl" :stroke-width="2" />
            <span class="text-sm font-semibold">{{ t('admin.files.dropToUpload') }}</span>
          </div>
        </div>
        <div v-if="loading" class="p-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
        <div v-else-if="!entries.length" class="p-8 text-center text-sm text-gray-500">
          {{ t('admin.files.empty') }}
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[920px] divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="w-10 px-3 py-2">
                  <input
                    type="checkbox"
                    class="rounded"
                    :checked="allFilesSelected"
                    :indeterminate="someFilesSelected"
                    :disabled="!fileEntries.length"
                    @change="toggleSelectAll"
                  />
                </th>
                <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.files.colName') }}
                </th>
                <th class="w-28 px-3 py-2 text-right font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.files.colSize') }}
                </th>
                <th class="w-44 px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.files.colModified') }}
                </th>
                <th class="w-72 whitespace-nowrap px-3 py-2 text-right font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.files.colActions') }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr
                v-for="entry in entries"
                :key="entry.key"
                class="hover:bg-gray-50 dark:hover:bg-dark-800/60"
              >
              <td class="px-3 py-2">
                <input
                  v-if="!entry.is_dir"
                  type="checkbox"
                  class="rounded"
                  :checked="selectedKeys.includes(entry.key)"
                  @change="toggleSelect(entry.key)"
                />
              </td>
              <td class="px-3 py-2">
                <!-- 目录：点进去。文件：显示缩略图（图片）+ 名称 -->
                <button
                  v-if="entry.is_dir"
                  type="button"
                  class="flex items-center gap-2 text-left text-primary-600 hover:underline dark:text-primary-400"
                  @click="goPrefix(entry.key)"
                >
                  <svg class="h-4 w-4 shrink-0" viewBox="0 0 20 20" fill="currentColor">
                    <path d="M2 5a2 2 0 012-2h3.2a2 2 0 011.4.6L10 5h6a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V5z" />
                  </svg>
                  <span class="break-all font-medium">{{ entry.name }}</span>
                </button>
                <div v-else class="flex items-center gap-2">
                  <img
                    v-if="isImageKey(entry.key) && entry.public_url && !brokenThumbs.has(entry.key)"
                    :src="entry.public_url"
                    :alt="entry.name"
                    loading="lazy"
                    class="h-8 w-8 shrink-0 rounded object-cover ring-1 ring-gray-200 dark:ring-dark-700"
                    @error="markThumbBroken(entry.key)"
                  />
                  <span v-else class="w-8 shrink-0 text-center text-base">{{ iconForKey(entry.key) }}</span>
                  <span class="break-all" :title="entry.key">{{ entry.name }}</span>
                </div>
              </td>
              <td class="px-3 py-2 text-right text-gray-500 dark:text-gray-400">
                {{ entry.is_dir ? '-' : formatBytes(entry.size) }}
              </td>
              <td class="px-3 py-2 text-gray-500 dark:text-gray-400">
                {{ entry.is_dir ? '-' : formatDateTime(entry.last_modified) }}
              </td>
              <td class="whitespace-nowrap px-3 py-2">
                <div v-if="!entry.is_dir" class="flex flex-nowrap items-center justify-end gap-1">
                  <button type="button" class="btn btn-ghost btn-xs" @click="doDownload(entry)">
                    {{ t('admin.files.download') }}
                  </button>
                  <button type="button" class="btn btn-ghost btn-xs" @click="openRename(entry)">
                    {{ t('admin.files.rename') }}
                  </button>
                  <button type="button" class="btn btn-ghost btn-xs" @click="copyURL(entry)">
                    {{ t('admin.files.copyUrl') }}
                  </button>
                  <button
                    type="button"
                    class="btn btn-ghost btn-xs text-red-600 dark:text-red-400"
                    @click="confirmDeleteOne(entry)"
                  >
                    {{ t('common.delete') }}
                  </button>
                </div>
              </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 游标分页：对象存储只支持"下一页"，无法跳页也拿不到总数 -->
        <div
          v-if="entries.length"
          class="flex flex-wrap items-center justify-between gap-2 border-t border-gray-200 px-4 py-3 dark:border-dark-700"
        >
          <span class="text-xs text-gray-500">
            {{ t('admin.files.loadedCount', { n: entries.length }) }}
          </span>
          <button
            v-if="nextToken"
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="loadingMore"
            @click="loadMore"
          >
            {{ loadingMore ? t('common.loading') : t('admin.files.loadMore') }}
          </button>
          <span v-else class="text-xs text-gray-400">{{ t('admin.files.noMore') }}</span>
        </div>
      </div>
    </template>

    <!-- 重命名弹窗 -->
    <BaseDialog :show="showRename" :title="t('admin.files.renameTitle')" @close="showRename = false">
      <div class="space-y-3">
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.files.currentKey') }}
          </label>
          <code class="block break-all rounded bg-gray-100 p-2 font-mono text-xs dark:bg-dark-800">
            {{ renameTarget?.key }}
          </code>
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.files.newName') }} <span class="text-red-500">*</span>
          </label>
          <input
            v-model="renameName"
            type="text"
            class="input"
            :disabled="renaming"
            @keyup.enter="submitRename"
          />
          <p class="mt-1 text-xs text-gray-500">{{ t('admin.files.renameHint') }}</p>
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="showRename = false">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="renaming || !renameName.trim() || renameName.trim() === renameTarget?.name"
          @click="submitRename"
        >
          {{ renaming ? t('common.submitting') : t('common.confirm') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.files.deleteTitle')"
      :message="deleteMessage"
      :confirm-text="t('common.delete')"
      danger
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
    />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * AdminFilesView：管理员「文件管理」。
 *
 * 直接管理图片转存（COS / S3 兼容）桶里的对象：逐层浏览、上传、下载、改名、
 * 批量删除。图片转存开启后桶里会不断堆积 fal 出图/出片的转存件与用户素材，
 * 此前除了登云控制台没有任何办法查看和清理。
 *
 * 几个与"普通列表页"不同的约束，都来自对象存储本身：
 *   - **没有真目录**：层级是按 "/" 聚合出来的（S3 CommonPrefixes），
 *     所以"目录"不能改名/删除，只能进入。
 *   - **游标分页**：只有 next_token，没有总数、也无法跳页 ——
 *     因此 UI 用"加载更多"追加，而不是页码器。
 *   - **只能前缀搜索**：不支持"包含"匹配。搜索框语义是"当前目录下按名称前缀
 *     过滤"，并自动开启递归平铺，这样能搜到子目录里的文件。
 *   - **没有 rename**：改名 = 服务端 copy + 删源（后端实现），目标已存在时拒绝覆盖。
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import adminFilesAPI, { type AdminFileEntry, type AdminFileStatus } from '@/api/admin/files'
import { extractApiErrorCode, extractI18nErrorMessage } from '@/utils/apiError'
import { formatBytes, formatDateTime } from '@/utils/format'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

const status = ref<AdminFileStatus | null>(null)
const statusLoaded = ref(false)

const entries = ref<AdminFileEntry[]>([])
const loading = ref(false)
const loadingMore = ref(false)
const nextToken = ref('')

/** prefix：当前浏览的目录前缀（始终以 "/" 结尾，或为空表示桶根）。 */
const prefix = ref('')
const searchInput = ref('')
const search = ref('')
const showUrlImport = ref(false)
const importUrl = ref('')
const importName = ref('')
const importingUrl = ref(false)

const selectedKeys = ref<string[]>([])
// brokenThumbs：缩略图加载失败的 key 集合（桶为私有读时 public_url 取不到图，
// 这时退回图标展示）。用 key 而不是下标，避免翻页追加后标记错位。
const brokenThumbs = ref(new Set<string>())

/**
 * markThumbBroken：标记缩略图失效。
 * 必须整体替换 Set —— 直接 .add() 不会改变 ref 的引用，Vue 侦测不到变化，
 * 失效的图会一直卡在破图状态。
 */
function markThumbBroken(key: string) {
  if (brokenThumbs.value.has(key)) return
  const next = new Set(brokenThumbs.value)
  next.add(key)
  brokenThumbs.value = next
}

const fileInputEl = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const uploadIndex = ref(0)
const uploadTotal = ref(0)
const uploadName = ref('')
const uploadPercent = ref(0)
const dragDepth = ref(0)
const dragActive = computed(() => dragDepth.value > 0)

const showRename = ref(false)
const renameTarget = ref<AdminFileEntry | null>(null)
const renameName = ref('')
const renaming = ref(false)

const showDeleteConfirm = ref(false)
const deleteTargets = ref<string[]>([])

/** fileEntries：只含文件（不含目录），用于全选逻辑 —— 目录不可删。 */
const fileEntries = computed(() => entries.value.filter((e) => !e.is_dir))

const allFilesSelected = computed(
  () => fileEntries.value.length > 0 && selectedKeys.value.length === fileEntries.value.length
)
const someFilesSelected = computed(
  () => selectedKeys.value.length > 0 && selectedKeys.value.length < fileEntries.value.length
)

/** breadcrumbs：把 prefix 拆成可点击的层级。 */
const breadcrumbs = computed(() => {
  const segs = prefix.value.split('/').filter((s) => s.length > 0)
  let acc = ''
  return segs.map((name) => {
    acc += name + '/'
    return { name, prefix: acc }
  })
})
const currentDirectoryLabel = computed(() => prefix.value || t('admin.files.root'))

const deleteMessage = computed(() => {
  if (deleteTargets.value.length === 1) {
    return t('admin.files.deleteOneConfirm', { name: deleteTargets.value[0] })
  }
  return t('admin.files.deleteManyConfirm', { n: deleteTargets.value.length })
})

function errMsg(e: unknown): string {
  return extractI18nErrorMessage(e, t, 'admin.files.errors', t('common.error'))
}

async function loadStatus() {
  try {
    status.value = await adminFilesAPI.getStatus()
  } catch {
    status.value = { enabled: false, bucket: '', prefix: '' }
  } finally {
    statusLoaded.value = true
  }
}

/**
 * reload：重新拉第一页。
 *
 * 搜索态下把"当前前缀 + 搜索词"拼成列举前缀，并开启 flat 递归平铺 ——
 * 对象存储只支持前缀匹配，这是唯一能"搜到子目录内容"的做法。
 */
async function reload() {
  loading.value = true
  clearSelection()
  try {
    const res = await adminFilesAPI.list({
      prefix: search.value ? prefix.value + search.value : prefix.value,
      flat: !!search.value,
    })
    entries.value = res.entries
    nextToken.value = res.next_token
    brokenThumbs.value = new Set()
  } catch (e: unknown) {
    entries.value = []
    nextToken.value = ''
    appStore.showError(errMsg(e))
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (!nextToken.value || loadingMore.value) return
  loadingMore.value = true
  try {
    const res = await adminFilesAPI.list({
      prefix: search.value ? prefix.value + search.value : prefix.value,
      token: nextToken.value,
      flat: !!search.value,
    })
    entries.value = [...entries.value, ...res.entries]
    nextToken.value = res.next_token
  } catch (e: unknown) {
    appStore.showError(errMsg(e))
  } finally {
    loadingMore.value = false
  }
}

function goPrefix(p: string) {
  prefix.value = p
  // 切目录时清掉搜索：否则会用旧关键词去过滤新目录，结果为空很让人困惑。
  search.value = ''
  searchInput.value = ''
  void reload()
}

function applySearch() {
  search.value = searchInput.value.trim()
  void reload()
}

function clearSearch() {
  searchInput.value = ''
  search.value = ''
  void reload()
}

async function submitUrlImport() {
  const url = importUrl.value.trim()
  if (!url || importingUrl.value) return
  importingUrl.value = true
  try {
    const options = { prefix: prefix.value, name: importName.value.trim() }
    let entry: AdminFileEntry
    try {
      entry = await adminFilesAPI.importFromUrl(url, options)
    } catch (e: unknown) {
      if (!isObjectKeyConflict(e)) throw e
      const name = options.name || fileNameFromUrl(url)
      if (!window.confirm(t('admin.files.overwriteConfirm', { name }))) return
      entry = await adminFilesAPI.importFromUrl(url, { ...options, overwrite: true })
    }
    appStore.showSuccess(t('admin.files.importSuccess', { name: entry.name }))
    importUrl.value = ''
    importName.value = ''
    showUrlImport.value = false
    await reload()
  } catch (e: unknown) {
    appStore.showError(errMsg(e))
  } finally {
    importingUrl.value = false
  }
}

// ── 选择 ──
function toggleSelect(key: string) {
  const i = selectedKeys.value.indexOf(key)
  if (i >= 0) selectedKeys.value.splice(i, 1)
  else selectedKeys.value.push(key)
}

function toggleSelectAll() {
  if (allFilesSelected.value) selectedKeys.value = []
  else selectedKeys.value = fileEntries.value.map((e) => e.key)
}

function clearSelection() {
  selectedKeys.value = []
}

// ── 上传 ──
function triggerUpload() {
  if (uploading.value) return
  fileInputEl.value?.click()
}

/**
 * onFilesPicked：上传到当前目录。
 * 串行上传：顺序可控、进度可读，也避免一次并发几十个大文件把后端打满。
 */
async function onFilesPicked(ev: Event) {
  const target = ev.target as HTMLInputElement
  const files = Array.from(target.files ?? [])
  target.value = ''
  await uploadFiles(files)
}

/** uploadFiles：文件选择器与拖放入口共用同一套串行上传和结果汇总。 */
async function uploadFiles(files: File[]) {
  if (!files.length || uploading.value) return

  uploading.value = true
  uploadTotal.value = files.length
  let ok = 0
  let skipped = 0
  let firstError = ''
  try {
    for (let i = 0; i < files.length; i++) {
      uploadIndex.value = i + 1
      uploadName.value = files[i].name
      uploadPercent.value = 0
      try {
        const options = {
          prefix: prefix.value,
          onProgress: (p: number) => (uploadPercent.value = p),
        }
        try {
          await adminFilesAPI.upload(files[i], options)
        } catch (e: unknown) {
          if (!isObjectKeyConflict(e)) throw e
          if (!window.confirm(t('admin.files.overwriteConfirm', { name: files[i].name }))) {
            skipped++
            continue
          }
          await adminFilesAPI.upload(files[i], { ...options, overwrite: true })
        }
        ok++
      } catch (e: unknown) {
        if (!firstError) firstError = errMsg(e)
      }
    }
  } finally {
    uploading.value = false
    uploadPercent.value = 0
    uploadName.value = ''
  }

  if (ok > 0) appStore.showSuccess(t('admin.files.uploadSuccess', { n: ok }))
  const failed = files.length - ok - skipped
  if (failed > 0) {
    appStore.showError(t('admin.files.uploadFailed', { n: failed, msg: firstError }))
  }
  if (skipped > 0) appStore.showError(t('admin.files.overwriteSkipped', { n: skipped }))
  if (ok > 0) await reload()
}

function isObjectKeyConflict(e: unknown): boolean {
  return extractApiErrorCode(e) === 'OBJECT_KEY_EXISTS'
}

function fileNameFromUrl(raw: string): string {
  try {
    const name = new URL(raw).pathname.split('/').filter(Boolean).pop()
    return name || t('admin.files.unknownFileName')
  } catch {
    return t('admin.files.unknownFileName')
  }
}

function isFileDrag(ev: DragEvent): boolean {
  return Array.from(ev.dataTransfer?.types ?? []).includes('Files')
}

function handleDragEnter(ev: DragEvent) {
  if (uploading.value || !isFileDrag(ev)) return
  dragDepth.value += 1
  if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'copy'
}

function handleDragOver(ev: DragEvent) {
  if (uploading.value || !isFileDrag(ev)) return
  if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'copy'
}

function handleDragLeave() {
  dragDepth.value = Math.max(0, dragDepth.value - 1)
}

async function handleDrop(ev: DragEvent) {
  dragDepth.value = 0
  if (uploading.value) return
  await uploadFiles(Array.from(ev.dataTransfer?.files ?? []))
}

// ── 下载 / 复制 ──
/**
 * doDownload：取预签名直链后同页导航触发下载。
 * 后端已在直链里带上 attachment 的 Content-Disposition，所以不会变成页面跳转；
 * 用 location.assign 而不是 window.open，避免被浏览器弹窗拦截。
 */
async function doDownload(entry: AdminFileEntry) {
  try {
    const { url } = await adminFilesAPI.getDownloadURL(entry.key)
    window.location.assign(url)
  } catch (e: unknown) {
    appStore.showError(errMsg(e))
  }
}

async function copyURL(entry: AdminFileEntry) {
  const url = entry.public_url || entry.key
  try {
    await navigator.clipboard.writeText(url)
    appStore.showSuccess(t('admin.files.urlCopied'))
  } catch {
    appStore.showError(t('admin.files.copyFailed'))
  }
}

// ── 重命名 ──
function openRename(entry: AdminFileEntry) {
  renameTarget.value = entry
  renameName.value = entry.name
  showRename.value = true
}

async function submitRename() {
  const name = renameName.value.trim()
  const target = renameTarget.value
  if (!target || !name || renaming.value) return
  renaming.value = true
  try {
    await adminFilesAPI.rename(target.key, { name })
    showRename.value = false
    appStore.showSuccess(t('admin.files.renameSuccess'))
    await reload()
  } catch (e: unknown) {
    appStore.showError(errMsg(e))
  } finally {
    renaming.value = false
  }
}

// ── 删除 ──
function confirmDeleteOne(entry: AdminFileEntry) {
  deleteTargets.value = [entry.key]
  showDeleteConfirm.value = true
}

function confirmDeleteSelected() {
  if (!selectedKeys.value.length) return
  deleteTargets.value = [...selectedKeys.value]
  showDeleteConfirm.value = true
}

async function doDelete() {
  const keys = deleteTargets.value
  showDeleteConfirm.value = false
  if (!keys.length) return
  try {
    const res = await adminFilesAPI.remove(keys)
    if (res.deleted > 0) appStore.showSuccess(t('admin.files.deleteSuccess', { n: res.deleted }))
    if (res.failed > 0) {
      // 逐条原因可能很长，只取第一条展示，其余靠刷新后仍在列表里体现。
      const first = Object.values(res.failures)[0] ?? ''
      appStore.showError(t('admin.files.deleteFailed', { n: res.failed, msg: first }))
    }
    await reload()
  } catch (e: unknown) {
    appStore.showError(errMsg(e))
  } finally {
    deleteTargets.value = []
  }
}

// ── 展示辅助 ──
function extOf(key: string): string {
  const base = key.split('/').pop() ?? ''
  const i = base.lastIndexOf('.')
  return i >= 0 ? base.slice(i + 1).toLowerCase() : ''
}

function isImageKey(key: string): boolean {
  return ['png', 'jpg', 'jpeg', 'webp', 'gif', 'bmp', 'svg', 'avif'].includes(extOf(key))
}

function iconForKey(key: string): string {
  const e = extOf(key)
  if (['mp4', 'mov', 'webm', 'mkv', 'avi'].includes(e)) return '🎬'
  if (['mp3', 'wav', 'flac', 'aac', 'ogg', 'm4a'].includes(e)) return '🎵'
  if (['zip', 'gz', 'tar', 'rar', '7z'].includes(e)) return '🗜️'
  if (['json', 'txt', 'md', 'log', 'csv', 'yaml', 'yml'].includes(e)) return '📄'
  return '📁'
}

onMounted(async () => {
  await loadStatus()
  if (status.value?.enabled) await reload()
})
</script>
