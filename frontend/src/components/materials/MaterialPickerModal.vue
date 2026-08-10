<template>
  <BaseDialog :show="show" :title="t('materials.pickerTitle')" width="wide" @close="close">
    <div class="space-y-3">
      <!-- 顶部工具栏：搜索 + 上传 + 从 URL 导入 -->
      <div class="flex flex-wrap items-center gap-2">
        <input
          v-model="keyword"
          type="text"
          class="input h-8 flex-1 min-w-[180px] text-sm"
          :placeholder="t('materials.searchPlaceholder')"
          @keyup.enter="reload"
        />
        <button type="button" class="btn btn-secondary btn-xs" :disabled="loading" @click="reload">
          {{ t('common.search', 'Search') }}
        </button>
        <label class="btn btn-secondary btn-xs cursor-pointer">
          <input type="file" :accept="acceptMime" class="hidden" @change="onFilePicked" />
          {{ t('materials.uploadBtn') }}
        </label>
        <button type="button" class="btn btn-secondary btn-xs" @click="showUrlImport = !showUrlImport">
          {{ t('materials.importUrlBtn') }}
        </button>
      </div>

      <!-- URL 导入区（默认折叠）-->
      <div v-if="showUrlImport" class="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-800">
        <input
          v-model="importUrl"
          type="text"
          class="input h-8 flex-1 text-sm"
          placeholder="https://..."
        />
        <button type="button" class="btn btn-primary btn-xs" :disabled="!importUrl || importing" @click="doImportFromUrl">
          {{ importing ? t('common.loading', 'Loading...') : t('materials.importUrlConfirm') }}
        </button>
      </div>

      <!-- Loading / Empty / List -->
      <div v-if="loading" class="py-8 text-center text-sm text-gray-500">
        {{ t('common.loading', 'Loading...') }}
      </div>
      <div v-else-if="items.length === 0" class="py-8 text-center text-sm text-gray-500">
        {{ t('materials.empty') }}
      </div>
      <div v-else class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
        <div
          v-for="item in items"
          :key="item.id"
          class="group relative flex cursor-pointer flex-col overflow-hidden rounded-lg border border-gray-200 bg-white transition hover:border-primary-500 hover:shadow dark:border-dark-700 dark:bg-dark-800"
          @click="onSelect(item)"
        >
          <div class="relative flex aspect-square items-center justify-center overflow-hidden bg-gray-50 dark:bg-dark-900">
            <!-- image：直接渲染缩略图 -->
            <img
              v-if="item.kind === 'image'"
              :src="item.url"
              :alt="item.file_name"
              class="h-full w-full object-cover"
              loading="lazy"
              @error="onThumbError"
            />
            <!-- audio / video：占位图标 -->
            <div v-else class="flex flex-col items-center gap-1 text-gray-400">
              <span class="text-2xl">
                {{ item.kind === 'audio' ? '🎵' : item.kind === 'video' ? '🎬' : '📄' }}
              </span>
              <span class="text-[10px] uppercase">{{ item.kind }}</span>
            </div>
          </div>
          <div class="p-2 text-xs">
            <div class="truncate font-medium text-gray-800 dark:text-dark-200" :title="item.file_name">
              {{ item.file_name || `#${item.id}` }}
            </div>
            <div class="mt-0.5 flex items-center justify-between text-[11px] text-gray-500">
              <span>{{ formatBytes(item.size_bytes) }}</span>
              <button
                type="button"
                class="text-red-500 hover:underline"
                :title="t('common.remove')"
                @click.stop="doRemove(item)"
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
          <button type="button" class="btn btn-ghost btn-xs" :disabled="page <= 1" @click="goPage(page - 1)">
            {{ t('common.prev', 'Prev') }}
          </button>
          <button type="button" class="btn btn-ghost btn-xs" :disabled="page * pageSize >= total" @click="goPage(page + 1)">
            {{ t('common.next', 'Next') }}
          </button>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
/**
 * MaterialPickerModal：素材库弹窗选择器。
 *
 * 使用场景：
 *   1) 演练台的图片输入控件 ImageInputField 点击"从素材库选择"时打开；
 *   2) （可选）未来其他表单也可以复用（音频/视频输入）。
 *
 * 与独立 UserMaterialsView 的区别：
 *   - 只支持"单选"，选中后 emit('picked', item) 并关闭
 *   - 只按传入的 kind 过滤（默认 'image'）
 *   - 同时提供上传 / URL 导入的快捷入口
 */
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import userMaterialsAPI, { type UserMaterialItem, type UserMaterialKind } from '@/api/userMaterials'
import { useAppStore } from '@/stores/app'
import { formatBytes } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    show: boolean
    /** 只筛选该类型；默认 'image'（图片输入控件专用） */
    kind?: UserMaterialKind
  }>(),
  { kind: 'image' }
)

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
  /** 用户在网格上点击某条素材时触发；父组件负责把 URL 塞回业务字段 */
  (e: 'picked', item: UserMaterialItem): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const items = ref<UserMaterialItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 12
const loading = ref(false)
const keyword = ref('')

const importUrl = ref('')
const importing = ref(false)
const showUrlImport = ref(false)

// accept 属性：image → image/*，其他类型对应展开。用户还是可以选择任意文件，
// 后端会按 Content-Type 校验并拒绝不匹配的（防止误传变成"通用网盘"）。
const acceptMime = computedAccept(props.kind)
function computedAccept(kind: UserMaterialKind): string {
  switch (kind) {
    case 'image':
      return 'image/*'
    case 'audio':
      return 'audio/*'
    case 'video':
      return 'video/*'
    default:
      return '*/*'
  }
}

watch(
  () => props.show,
  (v) => {
    if (v) {
      // 每次打开都刷新第一页，避免上次浏览到第 N 页干扰选取。
      page.value = 1
      keyword.value = ''
      importUrl.value = ''
      showUrlImport.value = false
      void reload()
    }
  }
)

onMounted(() => {
  if (props.show) void reload()
})

async function reload() {
  loading.value = true
  try {
    const resp = await userMaterialsAPI.list({
      kind: props.kind,
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

function goPage(p: number) {
  page.value = p
  void reload()
}

function onSelect(item: UserMaterialItem) {
  emit('picked', item)
  close()
}

function close() {
  emit('update:show', false)
}

// ---------------- 快捷上传 / URL 导入 ----------------
async function onFilePicked(ev: Event) {
  const target = ev.target as HTMLInputElement
  const f = target.files?.[0]
  if (!f) return
  try {
    const resp = await userMaterialsAPI.upload(f)
    appStore.showSuccess(t('materials.uploadSuccess'))
    // 上传成功直接选中并关闭，与"从素材库挑一张已有的"体验一致
    emit('picked', resp.data)
    close()
  } catch (e: unknown) {
    appStore.showError(errMessage(e))
  } finally {
    // 复原 input.value 让"同一文件再次上传"能触发 change
    target.value = ''
  }
}

async function doImportFromUrl() {
  const url = importUrl.value.trim()
  if (!url) return
  importing.value = true
  try {
    const resp = await userMaterialsAPI.importFromUrl(url)
    appStore.showSuccess(t('materials.uploadSuccess'))
    emit('picked', resp.data)
    close()
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

function onThumbError(ev: Event) {
  // 缩略图加载失败时用一个透明像素兜底，避免 404 图形
  const img = ev.target as HTMLImageElement
  img.style.display = 'none'
}

function errMessage(e: unknown): string {
  if (e instanceof Error) return e.message
  return String(e)
}
</script>
