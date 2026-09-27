<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="modal-overlay"
        :style="zIndexStyle"
        :aria-labelledby="dialogId"
        role="dialog"
        aria-modal="true"
        @click.self="handleClose"
      >
        <!-- Modal panel -->
        <div
          ref="dialogRef"
          :class="['modal-content', widthClasses]"
          @click.stop
          @keydown.tab="trapFocus"
          @input="markDirty"
          @change="markDirty"
        >
          <!-- Header -->
          <div class="modal-header">
            <h3 :id="dialogId" class="modal-title">
              {{ title }}
            </h3>
            <button
              v-if="showCloseButton"
              @click="requestClose"
              class="-mr-2 rounded-sm p-2 text-fg-subtle transition-colors hover:bg-accent-weak hover:text-accent-strong focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              :aria-label="t('common.close')"
            >
              <Icon name="x" size="md" />
            </button>
          </div>

          <!-- Body -->
          <div ref="modalBodyRef" class="modal-body">
            <slot></slot>
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="modal-footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
    <ConfirmDialog
      v-if="confirmDiscard"
      :show="confirmingDiscard"
      :title="t('common.unsavedChangesTitle')"
      :message="t('common.unsavedChangesMessage')"
      :confirm-text="t('common.discard')"
      danger
      @confirm="discardAndClose"
      @cancel="confirmingDiscard = false"
    />
  </Teleport>
</template>

<script lang="ts">
let dialogIdCounter = 0
// Stack of open dialogs, topmost last: only the top one reacts to Escape.
const openDialogs: string[] = []
</script>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted, ref, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ConfirmDialog from './ConfirmDialog.vue'

const { t } = useI18n()

// 生成唯一ID以避免多个对话框时ID冲突
const dialogId = `modal-title-${++dialogIdCounter}`

// 焦点管理
const dialogRef = ref<HTMLElement | null>(null)
const modalBodyRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  zIndex?: number
  /** Ask before Esc / X / backdrop closes a form the user has typed into. */
  confirmDiscard?: boolean
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  zIndex: 50,
  confirmDiscard: false
})

const emit = defineEmits<Emits>()

// Custom z-index style (overrides the default z-50 from CSS)
const zIndexStyle = computed(() => {
  return props.zIndex !== 50 ? { zIndex: props.zIndex } : undefined
})

const widthClasses = computed(() => {
  // Width guidance: narrow=confirm/short prompts, normal=standard forms,
  // wide=multi-section forms or rich content, extra-wide=analytics/tables,
  // full=full-screen or very dense layouts.
  const widths: Record<DialogWidth, string> = {
    narrow: 'max-w-md',
    normal: 'max-w-lg',
    wide: 'w-full sm:max-w-2xl md:max-w-3xl lg:max-w-4xl',
    'extra-wide': 'w-full sm:max-w-3xl md:max-w-4xl lg:max-w-5xl xl:max-w-6xl',
    full: 'w-full sm:max-w-4xl md:max-w-5xl lg:max-w-6xl xl:max-w-7xl'
  }
  return widths[props.width]
})

// ponytail: dirty = any native input/change event inside the panel since opening. Custom widgets
// (Toggle, Select) emit none, so flipping only those skips the prompt; add a dirty prop if that matters.
let dirty = false
const confirmingDiscard = ref(false)

const markDirty = () => {
  dirty = true
}

// Esc, the X button and the backdrop all close through here.
const requestClose = () => {
  if (props.confirmDiscard && dirty) {
    confirmingDiscard.value = true
  } else {
    emit('close')
  }
}

const discardAndClose = () => {
  const opener = previousActiveElement
  confirmingDiscard.value = false
  emit('close')
  // The prompt closes in the same flush and refocuses a node inside this leaving panel;
  // hand focus back to our opener once that settles.
  nextTick(() => {
    if (!props.show) opener?.focus()
  })
}

const handleClose = () => {
  if (props.closeOnClickOutside) {
    requestClose()
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (openDialogs[openDialogs.length - 1] !== dialogId) return
  if (props.show && props.closeOnEscape && event.key === 'Escape') {
    requestClose()
  }
}

const FOCUSABLE = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'

// Keep Tab / Shift+Tab cycling inside the dialog panel.
// ponytail: elements hidden by display:none still count; filter by layout if a dialog hides its first/last control.
const trapFocus = (event: KeyboardEvent) => {
  if (!dialogRef.value) return
  const focusable = Array.from(dialogRef.value.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (el) => !el.hasAttribute('disabled')
  )
  if (focusable.length === 0) return
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  const active = document.activeElement
  if (event.shiftKey && (active === first || !dialogRef.value.contains(active))) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && active === last) {
    event.preventDefault()
    first.focus()
  }
}

const updateScrollLock = (isOpen: boolean) => {
  const index = openDialogs.indexOf(dialogId)
  if (index !== -1) openDialogs.splice(index, 1)
  if (isOpen) openDialogs.push(dialogId)
  document.body.classList.toggle('modal-open', openDialogs.length > 0)
}

// Prevent body scroll when modal is open and manage focus
watch(
  () => props.show,
  async (isOpen) => {
    dirty = false
    confirmingDiscard.value = false
    if (isOpen) {
      // 保存当前焦点元素
      previousActiveElement = document.activeElement as HTMLElement
      // 使用CSS类而不是直接操作style,更易于管理多个对话框
      updateScrollLock(true)

      // 等待DOM更新后设置焦点到对话框
      await nextTick()
      if (modalBodyRef.value) {
        modalBodyRef.value.scrollTop = 0
      }
      if (dialogRef.value) {
        const firstFocusable = dialogRef.value.querySelector<HTMLElement>(FOCUSABLE)
        firstFocusable?.focus()
      }
    } else {
      updateScrollLock(false)
      // 恢复之前的焦点
      if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
        previousActiveElement.focus()
      }
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  // 确保组件卸载时移除滚动锁定
  updateScrollLock(false)
})
</script>
