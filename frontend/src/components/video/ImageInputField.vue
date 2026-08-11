<template>
  <div class="image-input-field space-y-2">
    <!-- 顶部按钮组：三种输入源 -->
    <div class="flex flex-wrap items-center gap-2">
      <button
        type="button"
        class="btn btn-secondary btn-xs"
        :disabled="disabled || uploading"
        @click="triggerLocalUpload"
      >
        {{ uploading ? t('common.loading', 'Loading...') : t('materials.uploadBtn') }}
      </button>
      <input ref="fileInputEl" type="file" accept="image/*" class="hidden" @change="onLocalFilePicked" />

      <button
        type="button"
        class="btn btn-secondary btn-xs"
        :disabled="disabled"
        @click="pickerVisible = true"
      >
        {{ t('materials.fromLibrary') }}
      </button>

      <!-- 从 URL 导入：与前两个来源同为按钮样式（此前是 btn-ghost，看起来像
           文字链接，和左边两个按钮不成一组）。点击展开下方的 URL 输入行。 -->
      <button
        type="button"
        class="btn btn-secondary btn-xs"
        :disabled="disabled"
        :aria-expanded="showUrlInput"
        @click="showUrlInput = !showUrlInput"
      >
        {{ t('materials.importUrlBtn') }}
      </button>
    </div>

    <!-- URL 输入（默认折叠）：从 URL 导入到素材库并写回字段 -->
    <div v-if="showUrlInput" class="flex items-center gap-2">
      <input
        v-model="urlInputValue"
        type="text"
        :disabled="disabled"
        class="input h-8 flex-1 text-sm"
        placeholder="https://..."
      />
      <button
        type="button"
        class="btn btn-primary btn-xs"
        :disabled="!urlInputValue.trim() || importingUrl"
        @click="doImportUrlAndSet"
      >
        {{ importingUrl ? t('common.loading', 'Loading...') : t('materials.importToLibraryBtn') }}
      </button>
    </div>

    <!-- 当前值预览 -->
    <div v-if="stringValue" class="space-y-1">
      <div class="group relative inline-block max-w-full overflow-hidden rounded border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
        <img
          :src="stringValue"
          alt="preview"
          class="max-h-40 max-w-full object-contain"
          @error="onPreviewError"
        />
        <button
          v-if="!disabled"
          type="button"
          class="absolute right-1 top-1 rounded bg-black/60 px-1.5 py-0.5 text-[10px] text-white opacity-0 group-hover:opacity-100"
          :title="t('common.clear', 'Clear')"
          @click="clearValue"
        >
          ✕
        </button>
      </div>
      <div class="flex items-center gap-2 text-xs text-gray-500">
        <span class="max-w-[420px] truncate" :title="stringValue">{{ stringValue }}</span>
        <button
          type="button"
          class="rounded px-1.5 py-0.5 text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700"
          :title="t('common.copy', 'Copy')"
          @click="doCopy"
        >
          ⧉
        </button>
      </div>
    </div>

    <!-- 空态提示 -->
    <p v-else class="text-xs text-gray-400">
      {{ t('materials.imageInputEmptyHint') }}
    </p>

    <!-- 素材库选择弹窗（v-model:show 双向绑定；只筛 image） -->
    <MaterialPickerModal
      v-model:show="pickerVisible"
      kind="image"
      @picked="onPicked"
    />
  </div>
</template>

<script setup lang="ts">
/**
 * ImageInputField：视频演练台"图片输入"叶子控件（widget='image'）。
 *
 * 三种输入路径最终都写回一个 COS URL：
 *   1. 本地上传   → /user/materials/upload
 *   2. 素材库选择 → 打开 MaterialPickerModal
 *   3. 粘贴 URL   → /user/materials/import-url 后端下载再转存
 * 这三个入口都会在素材库落库，方便下次复用，且业务侧拿到的都是稳定 COS URL。
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import MaterialPickerModal from '@/components/materials/MaterialPickerModal.vue'
import userMaterialsAPI, { type UserMaterialItem } from '@/api/userMaterials'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{
  modelValue: unknown
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: unknown): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const fileInputEl = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const showUrlInput = ref(false)
const urlInputValue = ref('')
const importingUrl = ref(false)
const pickerVisible = ref(false)

/** 归一化 modelValue 为可显示的字符串（非字符串时视为空）。 */
const stringValue = computed<string>(() => {
  const v = props.modelValue
  if (typeof v === 'string') return v
  return ''
})

function triggerLocalUpload() {
  fileInputEl.value?.click()
}

async function onLocalFilePicked(ev: Event) {
  const target = ev.target as HTMLInputElement
  const f = target.files?.[0]
  if (!f) return
  uploading.value = true
  try {
    const resp = await userMaterialsAPI.upload(f)
    setValue(resp.data.url)
    appStore.showSuccess(t('materials.uploadSuccess'))
  } catch (e: unknown) {
    appStore.showError(errMessage(e))
  } finally {
    uploading.value = false
    target.value = ''
  }
}

async function doImportUrlAndSet() {
  const url = urlInputValue.value.trim()
  if (!url) return
  importingUrl.value = true
  try {
    const resp = await userMaterialsAPI.importFromUrl(url)
    setValue(resp.data.url)
    urlInputValue.value = ''
    showUrlInput.value = false
    appStore.showSuccess(t('materials.uploadSuccess'))
  } catch (e: unknown) {
    appStore.showError(errMessage(e))
  } finally {
    importingUrl.value = false
  }
}

function onPicked(item: UserMaterialItem) {
  setValue(item.url)
}

function setValue(v: string) {
  emit('update:modelValue', v)
}

function clearValue() {
  emit('update:modelValue', '')
}

function doCopy() {
  if (!stringValue.value) return
  void copyToClipboard(stringValue.value, t('common.copied', 'Copied'))
}

function onPreviewError() {
  // 预览失败不清空值：可能仍是有效 URL，只是不支持内嵌预览
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
</script>
