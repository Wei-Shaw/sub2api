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
          class="group relative flex cursor-pointer flex-col overflow-hidden rounded-lg border bg-white transition hover:shadow dark:bg-dark-800"
          :class="
            isSelected(item)
              ? 'border-primary-500 ring-2 ring-primary-500/40'
              : 'border-gray-200 hover:border-primary-500 dark:border-dark-700'
          "
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
            <!-- 多选模式：右上角勾选角标，显示选中次序（1、2、3…），
                 让用户明确"最终会按点击顺序追加到目标字段"。 -->
            <span
              v-if="multiple"
              class="absolute right-1.5 top-1.5 flex h-5 min-w-5 items-center justify-center rounded-full px-1 text-[10px] font-semibold shadow"
              :class="
                isSelected(item)
                  ? 'bg-primary-600 text-white'
                  : 'bg-white/85 text-gray-400 dark:bg-dark-900/80'
              "
            >
              {{ selectedOrder(item) || '+' }}
            </span>
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
            {{ t('materials.prevPage') }}
          </button>
          <button type="button" class="btn btn-ghost btn-xs" :disabled="page * pageSize >= total" @click="goPage(page + 1)">
            {{ t('materials.nextPage') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 多选模式底部：显示已选数量 + 确认按钮。单选模式不渲染 footer，
         保持"点一下即选中并关闭"的原有交互不变。 -->
    <template v-if="multiple" #footer>
      <span class="mr-auto text-xs text-gray-500">
        {{ t('materials.selectedCount', { n: selectedIds.length }) }}
        <template v-if="remaining !== null">
          · {{ t('materials.remainingSlots', { n: remaining }) }}
        </template>
      </span>
      <button type="button" class="btn btn-secondary" @click="close">{{ t('common.cancel') }}</button>
      <button
        type="button"
        class="btn btn-primary"
        :disabled="selectedIds.length === 0"
        @click="confirmMulti"
      >
        {{ t('materials.confirmPick') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
/**
 * MaterialPickerModal：素材库弹窗选择器。
 *
 * 使用场景：
 *   1) 演练台的图片输入控件 ImageInputField 点击"从素材库选择"时打开（单选）；
 *   2) 演练台的多图输入控件 ImageUrlsField 打开（多选，multiple=true）；
 *   3) （可选）未来其他表单也可以复用（音频/视频输入）。
 *
 * 两种选择模式：
 *   - 单选（默认）：点一下卡片立即 emit('picked', item) 并关闭，交互最短。
 *   - 多选（multiple=true）：点击切换选中态，右上角角标显示选中次序；
 *     底部 footer 出现"确认选择"，确认时 emit('picked-multi', items)（按点击
 *     顺序），让调用方一次性追加多张。maxSelect 用于对齐目标字段的剩余额度。
 *
 * 与独立 UserMaterialsView 的区别：只按传入的 kind 过滤（默认 'image'），
 * 并额外提供上传 / URL 导入的快捷入口。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import userMaterialsAPI, { type UserMaterialItem, type UserMaterialKind } from '@/api/userMaterials'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'
import { formatBytes } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    show: boolean
    /** 只筛选该类型；默认 'image'（图片输入控件专用） */
    kind?: UserMaterialKind
    /** 是否多选。默认 false（保持既有单选行为不变）。 */
    multiple?: boolean
    /**
     * 多选模式下最多还能选几个（通常等于目标字段的剩余额度）。
     * 0 或省略表示不限制；达到上限后继续点击未选中的卡片会给出提示。
     */
    maxSelect?: number
  }>(),
  { kind: 'image', multiple: false, maxSelect: 0 }
)

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
  /** 单选模式：用户点击某条素材时触发；父组件负责把 URL 塞回业务字段 */
  (e: 'picked', item: UserMaterialItem): void
  /** 多选模式：点击"确认选择"时触发，按用户点击顺序给出全部选中项 */
  (e: 'picked-multi', items: UserMaterialItem[]): void
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

// ---------------- 多选状态 ----------------
// selectedIds 保存"点击顺序"（不是 Set），因为多图字段的顺序对用户有意义：
// 确认时按这个顺序追加，用户所见的角标序号就是最终入列顺序。
// selectedItems 同步保存完整记录，避免确认时因翻页/搜索导致 items 里已经找不到。
const selectedIds = ref<number[]>([])
const selectedItems = ref<Map<number, UserMaterialItem>>(new Map())

/** remaining：还能选几个；不限制时为 null（模板里据此决定是否展示剩余额度）。 */
const remaining = computed<number | null>(() => {
  if (!props.multiple || !props.maxSelect || props.maxSelect <= 0) return null
  return Math.max(0, props.maxSelect - selectedIds.value.length)
})

function isSelected(item: UserMaterialItem): boolean {
  return selectedIds.value.includes(item.id)
}

/** selectedOrder：1-based 选中次序；未选中返回 0（模板显示为 '+'）。 */
function selectedOrder(item: UserMaterialItem): number {
  return selectedIds.value.indexOf(item.id) + 1
}

function clearSelection() {
  selectedIds.value = []
  selectedItems.value = new Map()
}

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
      clearSelection()
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

/**
 * onSelect：卡片点击。
 *   - 单选：立即 emit + 关闭（原有行为）
 *   - 多选：切换选中态；已达上限时提示而不是静默忽略
 */
function onSelect(item: UserMaterialItem) {
  if (!props.multiple) {
    emit('picked', item)
    close()
    return
  }
  const idx = selectedIds.value.indexOf(item.id)
  if (idx >= 0) {
    selectedIds.value.splice(idx, 1)
    selectedItems.value.delete(item.id)
    // Map 是浅引用，手动触发一次响应式更新。
    selectedItems.value = new Map(selectedItems.value)
    return
  }
  if (remaining.value !== null && remaining.value <= 0) {
    appStore.showError(t('materials.maxSelectReached', { n: props.maxSelect }))
    return
  }
  selectedIds.value.push(item.id)
  selectedItems.value.set(item.id, item)
  selectedItems.value = new Map(selectedItems.value)
}

/** confirmMulti：多选确认，按点击顺序回吐全部选中项。 */
function confirmMulti() {
  const picked: UserMaterialItem[] = []
  for (const id of selectedIds.value) {
    const it = selectedItems.value.get(id)
    if (it) picked.push(it)
  }
  if (picked.length === 0) return
  emit('picked-multi', picked)
  close()
}

function close() {
  emit('update:show', false)
}

// ---------------- 快捷上传 / URL 导入 ----------------
/**
 * onFilePicked：弹窗内的快捷上传。
 *   - 单选：上传完直接选中并关闭，与"挑一张已有的"体验一致
 *   - 多选：上传完只把它加入选中集并刷新列表，用户可以继续挑更多再确认
 */
async function onFilePicked(ev: Event) {
  const target = ev.target as HTMLInputElement
  const f = target.files?.[0]
  if (!f) return
  try {
    const resp = await userMaterialsAPI.upload(f)
    appStore.showSuccess(t('materials.uploadSuccess'))
    if (props.multiple) {
      onSelect(resp.data)
      await reload()
    } else {
      emit('picked', resp.data)
      close()
    }
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
    importUrl.value = ''
    if (props.multiple) {
      onSelect(resp.data)
      await reload()
    } else {
      emit('picked', resp.data)
      close()
    }
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
    // 已删除的素材若还在多选集里，一并移除，避免确认时回吐一个失效 URL。
    const idx = selectedIds.value.indexOf(item.id)
    if (idx >= 0) {
      selectedIds.value.splice(idx, 1)
      selectedItems.value.delete(item.id)
      selectedItems.value = new Map(selectedItems.value)
    }
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

/**
 * errMessage：把捕获到的错误转成可展示文案。
 *
 * 必须走 extractI18nErrorMessage：apiClient 拦截器 reject 的是**普通对象**
 * （{ status, code, message, reason }）而不是 Error 实例，直接 String(e) 会得到
 * "[object Object]"。该工具会先按 reason 查 materials.errors.<REASON> 给出友好
 * 文案（例如 COS_NOT_CONFIGURED → 提示管理员先配置对象存储），查不到再回落到
 * 后端原始 message。
 */
function errMessage(e: unknown): string {
  return extractI18nErrorMessage(e, t, 'materials.errors', t('common.error'))
}
</script>
