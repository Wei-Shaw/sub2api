<template>
  <div
    ref="dropdownRef"
    class="relative"
    data-test="locale-switcher"
    @mouseenter="openDropdown"
    @mouseleave="scheduleCloseDropdown"
  >
    <button
      ref="buttonRef"
      type="button"
      :aria-expanded="isOpen"
      aria-haspopup="menu"
      @click.stop="openDropdown"
      :disabled="switching"
      class="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
      :title="currentLocale?.name"
    >
      <span class="text-base">{{ currentLocale?.flag }}</span>
      <span class="hidden sm:inline">{{ currentLocale?.code.toUpperCase() }}</span>
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
          ref="dropdownMenuRef"
          data-test="locale-switcher-menu"
          class="fixed z-50 w-32 overflow-hidden rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
          :style="dropdownStyle"
          role="menu"
          @mouseenter="clearCloseTimer"
          @mouseleave="scheduleCloseDropdown"
        >
          <button
            v-for="locale in availableLocales"
            :key="locale.code"
            type="button"
            :disabled="switching"
            class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
            :class="{
              'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400':
                locale.code === currentLocaleCode
            }"
            role="menuitem"
            @click="selectLocale(locale.code)"
          >
            <span class="text-base">{{ locale.flag }}</span>
            <span>{{ locale.name }}</span>
            <Icon v-if="locale.code === currentLocaleCode" name="check" size="sm" class="ml-auto text-primary-500" />
          </button>
        </div>
      </transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { setLocale, availableLocales } from '@/i18n'

const { locale } = useI18n()

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const buttonRef = ref<HTMLElement | null>(null)
const dropdownMenuRef = ref<HTMLElement | null>(null)
const dropdownStyle = ref<Record<string, string>>({
  top: '40px',
  left: '16px',
})
const switching = ref(false)
let closeTimer: number | null = null

const currentLocaleCode = computed(() => locale.value)
const currentLocale = computed(() => availableLocales.find((l) => l.code === locale.value))

function clearCloseTimer() {
  if (closeTimer) {
    window.clearTimeout(closeTimer)
    closeTimer = null
  }
}

function closeDropdown() {
  clearCloseTimer()
  isOpen.value = false
}

async function openDropdown() {
  if (switching.value) return
  clearCloseTimer()
  isOpen.value = true
  await nextTick()
  updateDropdownPosition()
}

function scheduleCloseDropdown() {
  clearCloseTimer()
  closeTimer = window.setTimeout(() => {
    closeDropdown()
  }, 120)
}

function updateDropdownPosition() {
  const button = buttonRef.value
  if (!button) return

  const rect = button.getBoundingClientRect()
  const menuWidth = dropdownMenuRef.value?.offsetWidth || 128
  const viewportWidth = window.innerWidth || document.documentElement.clientWidth
  const left = Math.max(8, Math.min(rect.right - menuWidth, viewportWidth - menuWidth - 8))

  dropdownStyle.value = {
    top: `${Math.ceil(rect.bottom + 4)}px`,
    left: `${Math.ceil(left)}px`,
  }
}

async function selectLocale(code: string) {
  if (switching.value || code === currentLocaleCode.value) {
    closeDropdown()
    return
  }
  switching.value = true
  try {
    await setLocale(code)
    closeDropdown()
  } finally {
    switching.value = false
  }
}

function handleClickOutside(event: MouseEvent) {
  if (!isOpen.value) return
  const target = event.target as Node
  if (dropdownRef.value?.contains(target)) return
  if (dropdownMenuRef.value?.contains(target)) return
  closeDropdown()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeDropdown()
  }
}

function handleViewportChange() {
  if (isOpen.value) {
    updateDropdownPosition()
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
