<template>
  <div v-if="images.length > 0">
    <!-- 缩略图网格 -->
    <div class="flex flex-wrap gap-2">
      <button
        v-for="(img, idx) in images"
        :key="img.key || img.url"
        type="button"
        class="block h-20 w-20 overflow-hidden rounded-lg border border-gray-200 bg-gray-50 transition hover:opacity-80 focus:outline-none focus:ring-2 focus:ring-primary-400 dark:border-dark-700 dark:bg-dark-800"
        :title="t('support.attachments.viewFull')"
        @click="openAt(idx)"
      >
        <img
          :src="img.url"
          :alt="`image-${idx}`"
          class="h-full w-full object-cover"
          loading="lazy"
        />
      </button>
    </div>

    <!-- Lightbox：使用 Teleport 挂到 body 顶层，避免被父级 max-width / overflow 裁剪 -->
    <Teleport to="body">
      <div
        v-if="activeIdx !== null"
        class="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4"
        role="dialog"
        aria-modal="true"
        @click.self="close"
        @keydown.esc="close"
      >
        <!-- 关闭按钮 -->
        <button
          type="button"
          class="absolute right-4 top-4 flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20"
          :title="t('common.cancel')"
          @click="close"
        >
          <Icon name="x" size="md" />
        </button>

        <!-- 上一张 -->
        <button
          v-if="images.length > 1"
          type="button"
          class="absolute left-4 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20"
          @click.stop="prev"
        >
          <Icon name="chevronLeft" size="md" />
        </button>

        <img
          :src="activeImage!.url"
          :alt="`image-${activeIdx}`"
          class="max-h-[90vh] max-w-[90vw] rounded-lg object-contain shadow-2xl"
          @click.stop
        />

        <!-- 下一张 -->
        <button
          v-if="images.length > 1"
          type="button"
          class="absolute right-4 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20"
          @click.stop="next"
        >
          <Icon name="chevronRight" size="md" />
        </button>

        <!-- 底部计数 -->
        <div
          v-if="images.length > 1"
          class="absolute bottom-4 left-1/2 -translate-x-1/2 rounded-full bg-white/10 px-3 py-1 text-xs text-white"
        >
          {{ activeIdx! + 1 }} / {{ images.length }}
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
/**
 * SupportTicketImageGallery —— 工单消息里的图片附件展示器。
 *
 * 只用于「读」路径：主帖 content 下方、每条回复气泡下方各挂一份。
 * 点击缩略图打开一个全屏 Lightbox（Teleport 到 body）用于放大 + 上下切换。
 *
 * 与 Uploader 组件配对，输入类型完全对齐：`SupportTicketImage[]`。
 * 空数组时整个组件不渲染，方便调用方 `:images="ticket.images ?? []"` 直接用。
 */
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SupportTicketImage } from '@/api/support'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  images: SupportTicketImage[]
}

const props = defineProps<Props>()

const { t } = useI18n()
const activeIdx = ref<number | null>(null)

const activeImage = computed(() =>
  activeIdx.value !== null ? props.images[activeIdx.value] ?? null : null
)

function openAt(idx: number) {
  activeIdx.value = idx
}

function close() {
  activeIdx.value = null
}

function prev() {
  if (activeIdx.value === null || props.images.length === 0) return
  activeIdx.value =
    (activeIdx.value - 1 + props.images.length) % props.images.length
}

function next() {
  if (activeIdx.value === null || props.images.length === 0) return
  activeIdx.value = (activeIdx.value + 1) % props.images.length
}

// 全局键盘控制（Esc / ← / →），仅在 Lightbox 打开时挂
function onKeydown(e: KeyboardEvent) {
  if (activeIdx.value === null) return
  if (e.key === 'Escape') close()
  else if (e.key === 'ArrowLeft') prev()
  else if (e.key === 'ArrowRight') next()
}

watch(activeIdx, (v) => {
  if (v !== null) {
    window.addEventListener('keydown', onKeydown)
  } else {
    window.removeEventListener('keydown', onKeydown)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>
