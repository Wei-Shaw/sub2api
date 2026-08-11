<template>
  <!--
    整块儿是一个"图库式"的输入区：
      - 外框在拖入文件时高亮（虚线 → 实线 + 主色底），给出明确的落点反馈
      - 头部一行：标题图标 + 计数（3 / 5）+ 清空
      - 主体：缩略图网格（可拖拽重排）+ 末尾一个虚线"添加"占位块
      - 底部：三个来源入口（上传 / 素材库 / 粘贴 URL）+ 可折叠的 URL 批量输入
  -->
  <div
    class="image-urls-field rounded-xl border p-3 transition-colors"
    :class="[
      dragOver
        ? 'border-primary-500 bg-primary-50/60 dark:bg-primary-950/30'
        : 'border-dashed border-gray-300 bg-gray-50/60 dark:border-dark-600 dark:bg-dark-800/40',
    ]"
    @dragover.prevent="onDragOver"
    @dragleave.prevent="onDragLeave"
    @drop.prevent="onDrop"
  >
    <!-- ============ 头部：计数 + 操作 ============ -->
    <div class="mb-2 flex items-center justify-between gap-2">
      <div class="flex items-center gap-1.5 text-xs font-medium text-gray-600 dark:text-gray-300">
        <svg class="h-4 w-4 text-primary-500" viewBox="0 0 20 20" fill="currentColor">
          <path
            fill-rule="evenodd"
            d="M4 3h12a1 1 0 011 1v12a1 1 0 01-1 1H4a1 1 0 01-1-1V4a1 1 0 011-1zm1 2v7.6l2.6-2.6a1 1 0 011.4 0l1.8 1.8 2.2-2.2a1 1 0 011.4 0L15 12V5H5zm8.5 1a1.5 1.5 0 100 3 1.5 1.5 0 000-3z"
            clip-rule="evenodd"
          />
        </svg>
        <span>{{ title }}</span>
        <span
          class="rounded-full bg-white px-1.5 py-0.5 font-mono text-[10px] text-gray-500 ring-1 ring-gray-200 dark:bg-dark-900 dark:text-gray-400 dark:ring-dark-700"
        >
          {{ urls.length }}<template v-if="maxItems > 0">/{{ maxItems }}</template>
        </span>
      </div>
      <div class="flex items-center gap-1">
        <span v-if="busy" class="inline-flex items-center gap-1 text-[11px] text-primary-600 dark:text-primary-400">
          <svg class="h-3 w-3 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
          </svg>
          {{ busyText }}
        </span>
        <button
          v-if="urls.length > 0 && !disabled"
          type="button"
          class="rounded px-1.5 py-0.5 text-[11px] text-gray-500 hover:bg-gray-200/70 hover:text-red-600 dark:hover:bg-dark-700"
          @click="clearAll"
        >
          {{ t('materials.clearAll') }}
        </button>
      </div>
    </div>

    <!-- ============ 缩略图网格 ============ -->
    <!-- VueDraggable 直接 v-model 到本地副本，拖拽结束后统一 emit，
         顺序对多图模型（首帧/尾帧、参考图权重）常常是有语义的。
         末尾的"添加"占位块也在同一个 grid 里（视觉上排成一整组），因此必须用
         Sortable 的 `draggable` 选项把可排序项限定为 .img-cell —— 否则那个
         按钮也会被当成一个 item，导致拖拽后的 index 与数组下标错位。 -->
    <VueDraggable
      v-if="urls.length > 0"
      v-model="draggableUrls"
      :animation="180"
      :disabled="disabled"
      handle=".img-drag"
      draggable=".img-cell"
      class="flex flex-wrap gap-2"
      @end="commitDraggable"
    >
      <div
        v-for="(u, i) in draggableUrls"
        :key="u"
        class="img-cell group relative h-28 w-28 shrink-0 overflow-hidden rounded-lg bg-white ring-1 ring-gray-200 dark:bg-dark-900 dark:ring-dark-700"
      >
        <!-- 图片可拖拽排序；单击则在页内打开大图预览。 -->
        <button
          v-if="mediaKind === 'image'"
          type="button"
          class="img-drag h-full w-full cursor-grab active:cursor-grabbing"
          :title="t('materials.previewImage')"
          @click="openImagePreview(i)"
        >
          <img
            :src="u"
            :alt="`${mediaKind} ${i + 1}`"
            loading="lazy"
            class="h-full w-full object-cover"
            @error="onThumbError(u)"
          />
        </button>
        <video
          v-else-if="mediaKind === 'video'"
          :src="u"
          controls
          preload="metadata"
          class="img-drag h-full w-full cursor-grab bg-black object-contain active:cursor-grabbing"
          @error="onThumbError(u)"
        />
        <audio
          v-else
          :src="u"
          controls
          preload="metadata"
          class="img-drag h-full w-full cursor-grab px-1 active:cursor-grabbing"
          @error="onThumbError(u)"
        />
        <!-- 加载失败兜底：显示一个"链接图标 + 失效"占位，仍保留 URL 不清空 -->
        <div
          v-if="brokenUrls.has(u)"
          class="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-gray-100 text-gray-400 dark:bg-dark-800"
        >
          <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path d="M8.5 5.5a3.5 3.5 0 014.95 0l.55.55a1 1 0 01-1.41 1.41l-.55-.55a1.5 1.5 0 00-2.12 0l-.55.55A1 1 0 017.95 6.05l.55-.55zM6.5 8.05a1 1 0 011.41 1.41l-.55.55a1.5 1.5 0 002.12 2.12l.55-.55a1 1 0 011.42 1.42l-.55.55a3.5 3.5 0 01-4.95-4.95l.55-.55z" />
          </svg>
          <span class="text-[10px]">{{ t('materials.thumbBroken') }}</span>
        </div>

        <!-- 左上角序号：多图顺序常有语义（首帧/尾帧），显式标出来 -->
        <span
          class="pointer-events-none absolute left-1 top-1 rounded bg-black/55 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-white"
        >
          {{ i + 1 }}
        </span>

        <button
          v-if="!disabled"
          type="button"
          class="image-remove-button absolute right-1 top-1 z-10 flex h-5 w-5 items-center justify-center rounded-full bg-red-500/90 text-white shadow hover:bg-red-600"
          :title="t('common.remove')"
          @click.stop="removeUrl(u)"
        >
          <Icon name="x" size="xs" :stroke-width="2.5" />
        </button>
      </div>

      <button
        v-if="canAddMore && !disabled"
        type="button"
        class="add-image-tile flex h-28 w-28 shrink-0 flex-col items-center justify-center gap-1 rounded-lg border border-dashed border-gray-300 text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-500 dark:border-dark-600"
        :disabled="busy"
        @click="openPicker"
      >
        <Icon name="plus" size="md" />
        <span class="text-[11px]">{{ t('materials.addMedia') }}</span>
      </button>
    </VueDraggable>

    <!-- ============ 空态：一个大号引导区（也可以直接拖文件进来） ============ -->
    <button
      v-else
      type="button"
      class="flex w-full flex-col items-center justify-center gap-1.5 rounded-lg border border-dashed border-gray-300 py-6 text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-500 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600"
      :disabled="disabled || busy"
      @click="openPicker"
    >
      <svg class="h-7 w-7" viewBox="0 0 20 20" fill="currentColor">
        <path d="M10 3a1 1 0 01.7.29l3 3a1 1 0 01-1.4 1.42L11 6.4V13a1 1 0 11-2 0V6.41L7.7 7.71A1 1 0 016.3 6.3l3-3A1 1 0 0110 3zM4 14a1 1 0 011 1v1h10v-1a1 1 0 112 0v1.5A1.5 1.5 0 0115.5 18h-11A1.5 1.5 0 013 16.5V15a1 1 0 011-1z" />
      </svg>
      <span class="text-xs font-medium">{{ emptyTitle }}</span>
      <span class="text-[11px]">{{ emptyHint }}</span>
    </button>

    <!-- ============ 底部：三种输入源 ============ -->
    <div data-testid="media-group-actions" class="mt-2.5 flex flex-wrap items-center gap-1.5">
      <button
        type="button"
        class="btn btn-secondary btn-xs"
        :disabled="disabled || busy || !canAddMore"
        @click="triggerLocalUpload"
      >
        {{ t('materials.uploadBtn') }}
      </button>
      <!-- multiple：一次可以选多张，逐个上传后按选择顺序追加 -->
      <input
        ref="fileInputEl"
        type="file"
        :accept="fileAccept"
        multiple
        class="hidden"
        @change="onLocalFilesPicked"
      />

      <button
        type="button"
        class="btn btn-secondary btn-xs"
        :disabled="disabled || busy || !canAddMore"
        @click="openPicker"
      >
        {{ t('materials.fromLibrary') }}
      </button>

      <!-- 从 URL 导入：与前两个来源同为按钮样式（此前是 btn-ghost，看起来像
           文字链接，和左边两个按钮不成一组）。点击展开下方的批量 URL 输入区。 -->
      <button
        type="button"
        class="btn btn-secondary btn-xs"
        :disabled="disabled || busy || !canAddMore"
        :aria-expanded="showUrlInput"
        @click="showUrlInput = !showUrlInput"
      >
        {{ t('materials.importUrlBtn') }}
      </button>

      <span v-if="!canAddMore" class="text-[11px] text-amber-600 dark:text-amber-400">
        {{ t('materials.maxItemsReached', { n: maxItems }) }}
      </span>
    </div>

    <!-- URL 批量输入（默认折叠）：一行一个，逐个导入到素材库后追加 -->
    <div v-if="showUrlInput" class="mt-2 space-y-1.5">
      <textarea
        v-model="urlInputValue"
        rows="3"
        class="input font-mono text-xs"
        :disabled="disabled || busy"
        :placeholder="t('materials.pasteUrlsPlaceholder')"
      />
      <div class="flex items-center gap-2">
        <button
          type="button"
          class="btn btn-primary btn-xs"
          :disabled="!urlInputValue.trim() || busy"
          @click="doImportUrls"
        >
          {{ t('materials.importToLibraryBtn') }}
        </button>
        <span class="text-[11px] text-gray-400">{{ t('materials.pasteUrlsHint') }}</span>
      </div>
    </div>

    <!-- 素材库多选弹窗：maxSelect 传剩余额度，避免用户选超了才被拒 -->
    <MaterialPickerModal
      v-model:show="pickerVisible"
      :kind="mediaKind"
      :multiple="true"
      :max-select="remainingSlots"
      :restrict-media-extensions="true"
      @picked-multi="onPickedMulti"
    />

    <Teleport to="body">
      <div
        v-if="previewIndex !== null && previewUrl"
        data-testid="image-preview"
        class="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4"
        role="dialog"
        aria-modal="true"
        @click.self="closeImagePreview"
      >
        <button
          type="button"
          class="absolute right-4 top-4 flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-white hover:bg-white/20"
          :title="t('common.cancel')"
          @click="closeImagePreview"
        >
          <Icon name="x" size="md" />
        </button>
        <button
          v-if="draggableUrls.length > 1"
          type="button"
          class="absolute left-4 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white hover:bg-white/20"
          @click.stop="showPreviousImage"
        >
          <Icon name="chevronLeft" size="md" />
        </button>
        <img
          :src="previewUrl"
          :alt="`image ${previewIndex + 1}`"
          class="max-h-[90vh] max-w-[90vw] object-contain shadow-2xl"
          @click.stop
        />
        <button
          v-if="draggableUrls.length > 1"
          type="button"
          class="absolute right-4 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white hover:bg-white/20"
          @click.stop="showNextImage"
        >
          <Icon name="chevronRight" size="md" />
        </button>
        <span
          v-if="draggableUrls.length > 1"
          class="absolute bottom-4 left-1/2 -translate-x-1/2 rounded-full bg-white/10 px-3 py-1 text-xs text-white"
        >
          {{ previewIndex + 1 }} / {{ draggableUrls.length }}
        </span>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
/**
 * ImageUrlsField：视频演练台的媒体 URL 组控件。
 *
 * 与单图控件 ImageInputField 的差别：
 *   - 值是 string[]（每个元素是图片完整 URL），而不是单个 string；
 *   - 把整个图片数组当成一个整体来展示：一个图库式网格 + 统一的计数/清空/
 *     拖拽排序，而不是"每个元素一行输入框 + 一个删除按钮"（后者在多图场景下
 *     会把表单撑得极长，也看不出图之间的顺序关系）；
 *   - 支持 maxItems 上限：达到上限后所有添加入口禁用，批量导入时按剩余额度截断。
 *
 * 三种输入源与单图控件一致，最终写回的都是稳定的 COS URL：
 *   1. 本地上传（支持一次多选，也支持把文件直接拖进整个区域）→ /user/materials/upload
 *   2. 素材库多选 → MaterialPickerModal（multiple 模式，按点击顺序追加）
 *   3. 粘贴 URL（一行一个，批量）→ /user/materials/import-url 后端下载再转存
 *
 * 去重：同一个 URL 不会被重复加入（多图模型里重复图基本都是误操作）。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import MaterialPickerModal from '@/components/materials/MaterialPickerModal.vue'
import Icon from '@/components/icons/Icon.vue'
import userMaterialsAPI, { type UserMaterialItem, type UserMaterialKind } from '@/api/userMaterials'
import { extractApiErrorCode, extractI18nErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'
import {
  hasAllowedMediaExtension,
  mediaExtensions,
  mediaFileAccept,
} from '@/utils/mediaUrlWidget'

const props = withDefaults(
  defineProps<{
    /** 当前值；非数组时按空数组处理（兼容旧数据/JSON 模式里写错类型的情况）。 */
    modelValue: unknown
    disabled?: boolean
    /** 元素个数上限；0 表示不限制。 */
    maxItems?: number
    /** 控件接收的素材类型。 */
    mediaKind?: UserMaterialKind
  }>(),
  { disabled: false, maxItems: 0, mediaKind: 'image' }
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: string[]): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const title = computed(() => t(`materials.${props.mediaKind}UrlsTitle`))
const emptyTitle = computed(() => t(`materials.${props.mediaKind}UrlsEmptyTitle`))
const emptyHint = computed(() =>
  t('materials.mediaUrlsEmptyHint', { extensions: mediaExtensions(props.mediaKind).join(', ') })
)
const fileAccept = computed(() => mediaFileAccept(props.mediaKind))

const fileInputEl = ref<HTMLInputElement | null>(null)
const pickerVisible = ref(false)
const showUrlInput = ref(false)
const urlInputValue = ref('')
const dragOver = ref(false)
const previewIndex = ref<number | null>(null)
// busy / busyText：上传或导入进行中；文案带进度（3/8），批量操作时用户能看到推进。
const busy = ref(false)
const busyText = ref('')
// brokenUrls：加载失败的缩略图 URL 集合。用 URL 而不是下标做 key，
// 这样拖拽重排后失效标记不会跟着位置错乱。只影响展示（显示失效占位），
// 不清空 URL —— 可能只是当前网络/防盗链问题，值本身仍然有效。
const brokenUrls = ref<Set<string>>(new Set())

/** urls：归一化后的当前值。过滤掉非字符串与空串，保证渲染安全。 */
const urls = computed<string[]>(() => normalizeUrls(props.modelValue))

/**
 * draggableUrls：VueDraggable 需要一个可写数组。
 * 直接 v-model 到 computed 会破坏"单一数据源"，因此保留一份本地副本，
 * 外部值变化时同步过来，拖拽结束（@end）时再统一 emit 回去。
 */
const draggableUrls = ref<string[]>([...urls.value])
const previewUrl = computed(() =>
  previewIndex.value === null ? '' : draggableUrls.value[previewIndex.value] ?? ''
)
watch(
  urls,
  (v) => {
    // 内容相同就不覆盖，避免拖拽过程中被父层回流打断。
    if (v.length === draggableUrls.value.length && v.every((x, i) => x === draggableUrls.value[i])) {
      return
    }
    draggableUrls.value = [...v]
    // 值整体被外部替换（重放历史 / 切换模型）时清空失效标记，重新尝试加载。
    brokenUrls.value = new Set()
  },
  { deep: true }
)

const maxItems = computed<number>(() => {
  const n = Number(props.maxItems)
  return Number.isFinite(n) && n > 0 ? Math.trunc(n) : 0
})

/** remainingSlots：还能加几个；不限制时返回 0（picker 侧 0 = 不限制）。 */
const remainingSlots = computed<number>(() => {
  if (maxItems.value <= 0) return 0
  return Math.max(0, maxItems.value - urls.value.length)
})

const canAddMore = computed<boolean>(() => maxItems.value <= 0 || urls.value.length < maxItems.value)

function normalizeUrls(v: unknown): string[] {
  if (!Array.isArray(v)) return []
  const out: string[] = []
  for (const item of v) {
    if (typeof item === 'string' && item.trim()) out.push(item.trim())
  }
  return out
}

/** commit：写回父层（同时同步本地拖拽副本）。 */
function commit(next: string[]) {
  draggableUrls.value = [...next]
  emit('update:modelValue', next)
}

/** commitDraggable：拖拽结束后把新顺序写回父层。 */
function commitDraggable() {
  emit('update:modelValue', [...draggableUrls.value])
}

/**
 * appendUrls：批量追加并去重 + 截断到剩余额度。
 * 返回实际加入的数量，调用方据此决定提示文案（全部加入 / 部分被上限截断）。
 */
function appendUrls(candidates: string[]): number {
  const cur = [...urls.value]
  const seen = new Set(cur)
  let added = 0
  for (const raw of candidates) {
    const u = (raw || '').trim()
    if (!u || seen.has(u)) continue
    if (maxItems.value > 0 && cur.length >= maxItems.value) break
    cur.push(u)
    seen.add(u)
    added++
  }
  if (added > 0) commit(cur)
  return added
}

/**
 * removeUrl：按 URL 删除（而不是按下标）。
 * 拖拽过程中本地副本 draggableUrls 与外部值可能短暂不同步，按下标删有删错的
 * 风险；URL 在本控件里是去重后唯一的，按值删除永远删的是用户点的那一张。
 */
function removeUrl(url: string) {
  const cur = draggableUrls.value.filter((u) => u !== url)
  commit(cur)
}

function clearAll() {
  commit([])
}

function openImagePreview(index: number) {
  previewIndex.value = index
}

function closeImagePreview() {
  previewIndex.value = null
}

function showPreviousImage() {
  if (previewIndex.value === null || draggableUrls.value.length === 0) return
  previewIndex.value =
    (previewIndex.value - 1 + draggableUrls.value.length) % draggableUrls.value.length
}

function showNextImage() {
  if (previewIndex.value === null || draggableUrls.value.length === 0) return
  previewIndex.value = (previewIndex.value + 1) % draggableUrls.value.length
}

function openPicker() {
  if (props.disabled || !canAddMore.value) return
  pickerVisible.value = true
}

function onPickedMulti(items: UserMaterialItem[]) {
  const matchingItems = items.filter((it) => it.kind === props.mediaKind)
  if (matchingItems.length !== items.length) appStore.showError(t('materials.mediaKindMismatch'))
  const added = appendUrls(matchingItems.map((it) => it.url))
  reportAdded(added, matchingItems.length)
}

function triggerLocalUpload() {
  fileInputEl.value?.click()
}

/**
 * onLocalFilesPicked：一次可选多张。逐个串行上传（而不是 Promise.all）：
 *   - 顺序可控，追加进数组的次序 == 用户在文件选择器里的次序；
 *   - 避免一次并发几十个请求把后端/COS 打满；
 *   - 单张失败不影响已成功的（失败计数汇总后一次性提示）。
 * 超过剩余额度的部分直接不再上传，省流量。
 */
async function onLocalFilesPicked(ev: Event) {
  const target = ev.target as HTMLInputElement
  const files = Array.from(target.files ?? [])
  target.value = ''
  await uploadLocalFiles(files)
}

async function uploadLocalFiles(files: File[]) {
  if (files.length === 0) return

  const allowedFiles = files.filter((file) => hasAllowedMediaExtension(file.name, props.mediaKind))
  const invalidCount = files.length - allowedFiles.length
  if (invalidCount > 0) {
    appStore.showError(
      t('materials.invalidMediaFiles', {
        n: invalidCount,
        extensions: mediaExtensions(props.mediaKind).join(', '),
      })
    )
  }
  if (allowedFiles.length === 0) return

  const slots = maxItems.value > 0 ? remainingSlots.value : allowedFiles.length
  const picked = allowedFiles.slice(0, slots)
  const skipped = allowedFiles.length - picked.length

  busy.value = true
  const uploaded: string[] = []
  let failed = 0
  // firstError：批量场景下只提示"N 张失败"没法排查（到底是文件太大、类型不支持，
  // 还是管理员压根没配 COS？）。这里留住第一条失败原因一起展示。
  let firstError = ''
  try {
    for (let i = 0; i < picked.length; i++) {
      busyText.value = t('materials.uploadingProgress', { i: i + 1, n: picked.length })
      try {
        const resp = await userMaterialsAPI.upload(picked[i])
        if (resp.data.kind !== props.mediaKind) {
          failed++
          if (!firstError) firstError = t('materials.mediaKindMismatch')
          continue
        }
        uploaded.push(resp.data.url)
      } catch (e: unknown) {
        failed++
        if (!firstError) firstError = errMessage(e)
        // COS 未配置这类"整体性失败"对后续每个文件都会复现，继续循环只是白等，
        // 直接中断把剩下的算作失败。
        if (isFatalMaterialError(e)) {
          failed += picked.length - i - 1
          break
        }
      }
    }
  } finally {
    busy.value = false
    busyText.value = ''
  }

  const added = appendUrls(uploaded)
  if (added > 0) appStore.showSuccess(t('materials.addedCount', { n: added }))
  if (failed > 0) {
    appStore.showError(
      firstError
        ? t('materials.uploadPartialFailedWithReason', { n: failed, msg: firstError })
        : t('materials.uploadPartialFailed', { n: failed })
    )
  }
  if (skipped > 0) appStore.showError(t('materials.maxItemsSkipped', { n: skipped }))
}

/**
 * doImportUrls：把 textarea 里的多个 URL（换行 / 空格 / 逗号分隔）逐个交给后端
 * 导入到素材库，再把返回的 COS URL 追加进来。
 * 与单图控件一致地走 import-url，而不是直接把外链塞进字段 —— 外链会失效、
 * 也可能有防盗链，转存后拿到的才是稳定地址。
 */
async function doImportUrls() {
  const candidates = urlInputValue.value
    .split(/[\s,，;；]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
  if (candidates.length === 0) return

  const allowedCandidates = candidates.filter((url) => hasAllowedMediaExtension(url, props.mediaKind))
  const invalidCount = candidates.length - allowedCandidates.length
  if (invalidCount > 0) {
    appStore.showError(
      t('materials.invalidMediaUrls', {
        n: invalidCount,
        extensions: mediaExtensions(props.mediaKind).join(', '),
      })
    )
  }
  if (allowedCandidates.length === 0) return

  const slots = maxItems.value > 0 ? remainingSlots.value : allowedCandidates.length
  const picked = allowedCandidates.slice(0, slots)
  const skipped = allowedCandidates.length - picked.length

  busy.value = true
  const imported: string[] = []
  let failed = 0
  let firstError = ''
  try {
    for (let i = 0; i < picked.length; i++) {
      busyText.value = t('materials.importingProgress', { i: i + 1, n: picked.length })
      try {
        const resp = await userMaterialsAPI.importFromUrl(picked[i])
        if (resp.data.kind !== props.mediaKind) {
          failed++
          if (!firstError) firstError = t('materials.mediaKindMismatch')
          continue
        }
        imported.push(resp.data.url)
      } catch (e: unknown) {
        failed++
        if (!firstError) firstError = errMessage(e)
        if (isFatalMaterialError(e)) {
          failed += picked.length - i - 1
          break
        }
      }
    }
  } finally {
    busy.value = false
    busyText.value = ''
  }

  const added = appendUrls(imported)
  if (added > 0) {
    appStore.showSuccess(t('materials.addedCount', { n: added }))
    urlInputValue.value = ''
    showUrlInput.value = false
  }
  if (failed > 0) {
    appStore.showError(
      firstError
        ? t('materials.importPartialFailedWithReason', { n: failed, msg: firstError })
        : t('materials.importPartialFailed', { n: failed })
    )
  }
  if (skipped > 0) appStore.showError(t('materials.maxItemsSkipped', { n: skipped }))
}

// ---------------- 拖拽上传 ----------------
function onDragOver() {
  if (props.disabled || !canAddMore.value) return
  dragOver.value = true
}

function onDragLeave() {
  dragOver.value = false
}

/**
 * onDrop：把拖入文件交给统一的后缀校验和上传流程。
 */
async function onDrop(ev: DragEvent) {
  dragOver.value = false
  if (props.disabled || !canAddMore.value) return
  const files = Array.from(ev.dataTransfer?.files ?? [])
  if (files.length === 0) return
  await uploadLocalFiles(files)
}

/** reportAdded：素材库多选后的提示；被上限截断时额外说明。 */function reportAdded(added: number, requested: number) {
  if (added > 0) appStore.showSuccess(t('materials.addedCount', { n: added }))
  const skipped = requested - added
  if (skipped > 0) appStore.showError(t('materials.maxItemsSkipped', { n: skipped }))
}

function onThumbError(url: string) {
  const next = new Set(brokenUrls.value)
  next.add(url)
  brokenUrls.value = next
}

/**
 * errMessage：把捕获到的错误转成可展示文案。
 * apiClient 拦截器 reject 的是普通对象而非 Error，直接 String(e) 会得到
 * "[object Object]"；extractI18nErrorMessage 会按 reason 查 materials.errors
 * 给出友好文案，查不到再回落到后端原始 message。
 */
function errMessage(e: unknown): string {
  return extractI18nErrorMessage(e, t, 'materials.errors', t('common.error'))
}

/**
 * isFatalMaterialError：判断该错误是否"对后续每个文件都会同样失败"。
 *
 * 例如 COS 压根没配置（COS_NOT_CONFIGURED）或已超配额，继续循环剩下的文件
 * 只是让用户白等 N 个必然失败的请求。命中这些 reason 时提前中断，把剩余数量
 * 算作失败一次性提示。
 */
function isFatalMaterialError(e: unknown): boolean {
  const code = extractApiErrorCode(e)
  return (
    code === 'COS_NOT_CONFIGURED' ||
    code === 'MATERIAL_COUNT_QUOTA_EXCEEDED' ||
    code === 'MATERIAL_SIZE_QUOTA_EXCEEDED'
  )
}
</script>

<style scoped>
.image-urls-field {
  width: 100%;
}
</style>
