<template>
  <div
    v-if="items.length > 0"
    ref="rootRef"
    class="relative inline-flex"
    data-test="home-products-menu"
    @mouseenter="openDropdown"
    @mouseleave="scheduleClose"
  >
    <button
      ref="buttonRef"
      type="button"
      :aria-expanded="isOpen"
      aria-haspopup="menu"
      class="flex items-center gap-1.5 whitespace-nowrap rounded-lg px-2 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
      @click.stop="toggleDropdown"
    >
      <Icon name="grid" size="sm" />
      <span :class="labelClass">{{ t('home.products') }}</span>
      <Icon
        name="chevronDown"
        size="xs"
        class="text-gray-400 transition-transform duration-200"
        :class="{ 'rotate-180': isOpen }"
      />
    </button>

    <Teleport to="body">
      <transition name="dropdown">
        <div
          v-if="isOpen"
          ref="menuRef"
          class="fixed z-50 max-w-[calc(100vw-2rem)] rounded-lg border border-gray-200 bg-white py-2 shadow-lg dark:border-dark-700 dark:bg-dark-900"
          :style="menuStyle"
          role="menu"
          @mouseenter="clearCloseTimer"
          @mouseleave="scheduleClose"
        >
          <a
            v-for="item in items"
            :key="item.id || item.url || item.label"
            :href="item.url"
            :target="item.action === 'new_tab' ? '_blank' : undefined"
            :rel="item.action === 'new_tab' ? 'noopener noreferrer' : undefined"
            class="flex items-center gap-2 px-4 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:text-dark-200 dark:hover:bg-dark-800 dark:hover:text-white"
            role="menuitem"
            @click="closeDropdown"
          >
            <span
              v-if="item.icon_svg"
              class="h-4 w-4 shrink-0 [&>svg]:h-4 [&>svg]:w-4"
              v-html="item.icon_svg"
            ></span>
            <Icon v-else name="grid" size="sm" class="shrink-0" />
            <span class="truncate">{{ item.label }}</span>
          </a>
        </div>
      </transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  /** 是否隐藏文本标签（只显示图标 + 箭头），常用于紧凑布局 */
  hideLabel?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  hideLabel: false,
})

const { t } = useI18n()
const appStore = useAppStore()

const items = computed(() =>
  (appStore.cachedPublicSettings?.home_product_menu_items || []).filter(
    (item) => item.label && item.url,
  ),
)

const labelClass = computed(() => (props.hideLabel ? 'hidden sm:inline' : ''))

const isOpen = ref(false)
const rootRef = ref<HTMLElement | null>(null)
const buttonRef = ref<HTMLElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const menuStyle = ref<Record<string, string>>({
  top: '48px',
  left: '16px',
  minWidth: '12rem',
})
let closeTimer: number | null = null

function clearCloseTimer() {
  if (closeTimer) {
    window.clearTimeout(closeTimer)
    closeTimer = null
  }
}

function scheduleClose() {
  clearCloseTimer()
  closeTimer = window.setTimeout(() => {
    closeDropdown()
  }, 120)
}

function supportsHover(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return true
  }
  return window.matchMedia('(hover: hover) and (pointer: fine)').matches
}

async function openDropdown(event?: Event) {
  // Skip synthetic mouseenter fired right before click on touch devices,
  // which would immediately re-toggle the menu and swallow the first tap.
  if (event && event.type === 'mouseenter' && !supportsHover()) {
    return
  }
  clearCloseTimer()
  if (isOpen.value) {
    // Already open — just refresh position (e.g. viewport changed)
    await nextTick()
    updateMenuPosition()
    return
  }
  isOpen.value = true
  await nextTick()
  updateMenuPosition()
}

function closeDropdown() {
  clearCloseTimer()
  isOpen.value = false
}

function toggleDropdown() {
  if (isOpen.value) {
    closeDropdown()
  } else {
    openDropdown()
  }
}

function updateMenuPosition() {
  const button = buttonRef.value
  if (!button) return
  const rect = button.getBoundingClientRect()
  const minWidth = Math.max(192, Math.ceil(rect.width))
  const viewportWidth = window.innerWidth || document.documentElement.clientWidth
  const left = Math.max(8, Math.min(rect.right - minWidth, viewportWidth - minWidth - 8))
  menuStyle.value = {
    top: `${Math.ceil(rect.bottom + 4)}px`,
    left: `${Math.ceil(left)}px`,
    minWidth: `${minWidth}px`,
  }
}

function handleClickOutside(event: MouseEvent) {
  if (!isOpen.value) return
  const target = event.target as Node | null
  if (!target) return
  if (rootRef.value?.contains(target)) return
  if (menuRef.value?.contains(target)) return
  closeDropdown()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeDropdown()
  }
}

function handleViewportChange() {
  if (isOpen.value) {
    updateMenuPosition()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeydown)
  window.addEventListener('resize', handleViewportChange)
  window.addEventListener('scroll', handleViewportChange, true)
})

onBeforeUnmount(() => {
  clearCloseTimer()
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('resize', handleViewportChange)
  window.removeEventListener('scroll', handleViewportChange, true)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.15s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}
</style>
