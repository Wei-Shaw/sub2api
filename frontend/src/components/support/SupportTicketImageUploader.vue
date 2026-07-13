<template>
  <div class="space-y-2">
    <!-- 缩略图列表 + 添加按钮 -->
    <div class="flex flex-wrap gap-3">
      <div
        v-for="(img, idx) in modelValue"
        :key="img.key || img.url"
        class="group relative h-20 w-20 overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
      >
        <img
          :src="img.url"
          :alt="`image-${idx}`"
          class="h-full w-full object-cover"
          loading="lazy"
        />
        <!-- 删除按钮 -->
        <button
          type="button"
          class="absolute right-1 top-1 flex h-5 w-5 items-center justify-center rounded-full bg-black/60 text-white opacity-0 transition group-hover:opacity-100 focus:opacity-100"
          :disabled="disabled"
          :title="t('support.attachments.remove')"
          @click="removeAt(idx)"
        >
          <Icon name="x" size="xs" />
        </button>
      </div>

      <!-- 上传中占位（每个项显示进度） -->
      <div
        v-for="job in uploadJobs"
        :key="job.id"
        class="relative flex h-20 w-20 flex-col items-center justify-center gap-1 rounded-lg border border-dashed border-primary-300 bg-primary-50 text-xs text-primary-700 dark:border-primary-700/50 dark:bg-primary-900/20 dark:text-primary-300"
      >
        <svg class="h-5 w-5 animate-spin" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          />
        </svg>
        <span>{{ job.percent }}%</span>
      </div>

      <!-- 添加按钮：达到上限时隐藏 -->
      <button
        v-if="canAddMore"
        type="button"
        class="flex h-20 w-20 flex-col items-center justify-center gap-1 rounded-lg border-2 border-dashed border-gray-300 bg-white text-xs text-gray-500 transition hover:border-primary-400 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-400 dark:hover:border-primary-500 dark:hover:text-primary-300"
        :disabled="disabled || uploading"
        @click="openPicker"
      >
        <Icon name="plus" size="md" />
        <span>{{ t('support.attachments.add') }}</span>
      </button>
    </div>

    <!-- 提示文字（数量 / 大小 / 类型） -->
    <p class="text-xs text-gray-500 dark:text-dark-400">
      {{
        t('support.attachments.hint', {
          count: modelValue.length,
          max: maxCount,
          size: maxSizeMb,
        })
      }}
    </p>

    <!-- 隐藏 file input：accept 与 MIME 白名单同步 -->
    <input
      ref="fileInputRef"
      type="file"
      class="hidden"
      :accept="acceptAttr"
      multiple
      @change="handleFileSelected"
    />
  </div>
</template>

<script setup lang="ts">
/**
 * SupportTicketImageUploader —— 工单图片附件上传控件。
 *
 * 交互：
 *   1. 用户点「+ 添加」按钮触发隐藏 `<input type=file multiple>`。
 *   2. 每个候选文件先做前端校验（MIME 白名单 + ≤ 5 MB + 数量上限），
 *      失败项直接 toast 报错，不阻塞其他项继续上传。
 *   3. 通过校验的文件逐个走 `uploadTicketAttachment()`，展示进度条。
 *      成功后 push 到 v-model 数组；失败 toast 提示但不改数组。
 *   4. 已上传项支持删除（本地移除；后端有独立生命周期，不做在线撤销）。
 *
 * 与后端约束保持一致：
 *   - MIME 白名单：png / jpeg（后端会用 magic bytes 兜底）
 *   - 单张 ≤ 5 MB
 *   - 每条消息 ≤ 5 张
 *
 * 组件仅负责"当前编辑中的附件数组"，不做提交，提交由外层表单驱动。
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import {
  SUPPORT_TICKET_ALLOWED_IMAGE_MIMES,
  SUPPORT_TICKET_IMAGES_MAX_COUNT,
  SUPPORT_TICKET_IMAGE_MAX_BYTES,
  uploadTicketAttachment,
  type SupportTicketImage,
} from '@/api/support'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  /** v-model 双向绑定的图片数组。 */
  modelValue: SupportTicketImage[]
  /** 表单提交/关闭状态下禁用整个控件。 */
  disabled?: boolean
  /** 覆盖数量上限（默认 5，与后端一致）。 */
  maxCount?: number
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  maxCount: SUPPORT_TICKET_IMAGES_MAX_COUNT,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: SupportTicketImage[]): void
  /** 上传状态变化（true = 有正在上传的任务）。外层可用来禁用提交按钮。 */
  (e: 'uploading', value: boolean): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const fileInputRef = ref<HTMLInputElement | null>(null)

interface UploadJob {
  id: number
  percent: number
}
const uploadJobs = ref<UploadJob[]>([])
let jobSeq = 0

const uploading = computed(() => uploadJobs.value.length > 0)
const maxSizeMb = computed(() =>
  Math.round(SUPPORT_TICKET_IMAGE_MAX_BYTES / (1024 * 1024))
)
const acceptAttr = SUPPORT_TICKET_ALLOWED_IMAGE_MIMES.join(',')

const canAddMore = computed(
  () => props.modelValue.length + uploadJobs.value.length < props.maxCount
)

function openPicker() {
  fileInputRef.value?.click()
}

function updateUploadingState() {
  emit('uploading', uploading.value)
}

/**
 * 前端 fast-path 校验：MIME 白名单 + 体积。
 * 白名单命中失败 → 视为不合法（后端 magic bytes 会再校验一次）。
 */
function validateFile(file: File): string | null {
  const mimeOk = (SUPPORT_TICKET_ALLOWED_IMAGE_MIMES as readonly string[]).includes(
    file.type
  )
  if (!mimeOk) {
    return t('support.attachments.errorUnsupportedType', { name: file.name })
  }
  if (file.size > SUPPORT_TICKET_IMAGE_MAX_BYTES) {
    return t('support.attachments.errorTooLarge', {
      name: file.name,
      size: maxSizeMb.value,
    })
  }
  return null
}

async function handleFileSelected(evt: Event) {
  const input = evt.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  // 允许用户重复选同一张文件（浏览器会 dedupe，重置 value 才能再次触发 change）
  input.value = ''

  if (files.length === 0) return

  // 逐张过校验；数量上限动态计算，兼容前面校验失败的 case
  const currentCount = props.modelValue.length + uploadJobs.value.length
  const remaining = props.maxCount - currentCount
  if (remaining <= 0) {
    appStore.showError(
      t('support.attachments.errorTooMany', { max: props.maxCount })
    )
    return
  }

  const toUpload: File[] = []
  for (const f of files) {
    if (toUpload.length >= remaining) {
      appStore.showWarning(
        t('support.attachments.errorTooMany', { max: props.maxCount })
      )
      break
    }
    const err = validateFile(f)
    if (err) {
      appStore.showError(err)
      continue
    }
    toUpload.push(f)
  }

  // 逐张并行上传即可（≤5 张）。任何一张失败不影响其他。
  await Promise.all(toUpload.map(uploadOne))
}

async function uploadOne(file: File) {
  const job: UploadJob = { id: ++jobSeq, percent: 0 }
  uploadJobs.value.push(job)
  updateUploadingState()
  try {
    const img = await uploadTicketAttachment(file, (percent) => {
      job.percent = percent
    })
    emit('update:modelValue', [...props.modelValue, img])
  } catch (err: unknown) {
    appStore.showError(
      extractI18nErrorMessage(err, t, 'support.errors', t('support.attachments.errorUploadFailed'))
    )
  } finally {
    uploadJobs.value = uploadJobs.value.filter((j) => j.id !== job.id)
    updateUploadingState()
  }
}

function removeAt(idx: number) {
  if (props.disabled) return
  const next = [...props.modelValue]
  next.splice(idx, 1)
  emit('update:modelValue', next)
}
</script>
