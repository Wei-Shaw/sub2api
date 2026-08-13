<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="modal-overlay"
        :style="zIndexStyle"
        :aria-labelledby="dialogId"
        role="dialog"
        :aria-modal="isTopmost ? 'true' : 'false'"
        :inert="isTopmost ? undefined : true"
        @click.self="handleClose"
      >
        <!-- Modal panel -->
        <div ref="dialogRef" :class="['modal-content', widthClasses]" @click.stop>
          <!-- Header -->
          <div class="modal-header">
            <h3 :id="dialogId" class="modal-title">
              {{ title }}
            </h3>
            <button
              v-if="showCloseButton"
              @click="emit('close')"
              class="-mr-2 rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 focus-visible:ring-offset-2 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-300 dark:focus-visible:ring-offset-dark-900"
              :aria-label="closeLabel"
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
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import {
  isTopDialog,
  registerDialog,
  topDialog,
  unregisterDialog,
  type DialogStackEntry
} from '@/composables/useDialogStack'

// 生成唯一ID以避免多个对话框时ID冲突
let dialogIdCounter = 0
const dialogId = `modal-title-${++dialogIdCounter}`

// 焦点管理
const dialogRef = ref<HTMLElement | null>(null)
const modalBodyRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

// This primitive is mounted from ~70 call sites, some of which are unit-tested
// without the app-level i18n plugin installed. `useI18n()` throws when no i18n
// instance is available, so resolve the label defensively: a missing instance
// degrades to English instead of breaking every consumer's spec.
const i18n = (() => {
  try {
    return useI18n()
  } catch {
    return null
  }
})()

const closeLabel = computed(() => {
  if (!i18n) return 'Close'
  try {
    return i18n.t('common.close')
  } catch {
    return 'Close'
  }
})

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  zIndex?: number
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  zIndex: 50
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

const handleClose = () => {
  // Only the topmost dialog is a real modal; an overlay click that lands on a
  // dialog underneath must not close it.
  if (props.closeOnClickOutside && isTopmost.value) {
    emit('close')
  }
}

const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

const focusableElements = (): HTMLElement[] =>
  dialogRef.value ? Array.from(dialogRef.value.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)) : []

const focusSelf = () => {
  focusableElements()[0]?.focus()
}

// Keeps Tab inside this dialog. The stack only routes keydown to the topmost
// entry, so the trap follows the stack for free.
const trapTab = (event: KeyboardEvent) => {
  const focusables = focusableElements()
  if (focusables.length === 0) return

  const first = focusables[0] as HTMLElement
  const last = focusables[focusables.length - 1] as HTMLElement
  const active = document.activeElement as HTMLElement | null
  const inside = active !== null && dialogRef.value?.contains(active) === true

  if (event.shiftKey) {
    if (!inside || active === first) {
      event.preventDefault()
      last.focus()
    }
    return
  }

  if (!inside || active === last) {
    event.preventDefault()
    first.focus()
  }
}

// Entry identity is what the stack removes by, so it must be a stable object.
const stackEntry: DialogStackEntry = {
  element: () => dialogRef.value,
  focus: focusSelf,
  onKeydown: (event: KeyboardEvent) => {
    if (!props.show) return

    if (event.key === 'Escape') {
      // Swallowed even when closeOnEscape is false: the topmost modal owns the
      // key, it must not fall through to the dialog underneath.
      if (props.closeOnEscape) {
        emit('close')
      }
      return
    }

    if (event.key === 'Tab') {
      trapTab(event)
    }
  }
}

const isTopmost = computed(() => isTopDialog(stackEntry))

const releaseDialog = () => {
  const ownedFocus = document.activeElement !== null && dialogRef.value?.contains(document.activeElement) === true
  const restoreTarget = previousActiveElement
  previousActiveElement = null

  // Removing from the stack releases the scroll lock only when nothing is open.
  unregisterDialog(stackEntry)

  // A dialog closed out of order (still-open dialog above it) must not steal focus.
  if (!ownedFocus) return

  const below = topDialog()
  if (below) {
    // Focus goes back to the dialog underneath, not to whatever was focused
    // before *that* dialog opened.
    if (restoreTarget?.isConnected && below.element()?.contains(restoreTarget) === true) {
      restoreTarget.focus()
    } else {
      below.focus()
    }
    return
  }

  if (restoreTarget?.isConnected && typeof restoreTarget.focus === 'function') {
    restoreTarget.focus()
  }
}

// Register on open / unregister on close: the stack owns the body scroll lock,
// the document keydown listener and which dialog is the active modal.
watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      // 保存当前焦点元素
      previousActiveElement = document.activeElement as HTMLElement | null
      registerDialog(stackEntry)

      // 等待DOM更新后设置焦点到对话框
      await nextTick()
      if (modalBodyRef.value) {
        modalBodyRef.value.scrollTop = 0
      }
      focusSelf()
    } else {
      releaseDialog()
    }
  },
  { immediate: true }
)

onUnmounted(() => {
  // A dialog destroyed while still open (route change, parent v-if) must not
  // leave an orphan entry behind — the stack would never empty and the scroll
  // lock would stick forever. unregisterDialog is a no-op when already removed.
  unregisterDialog(stackEntry)
})
</script>
