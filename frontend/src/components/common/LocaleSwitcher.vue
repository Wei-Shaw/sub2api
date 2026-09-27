<template>
  <div class="relative" ref="dropdownRef" @keydown.esc="closeOnEscape">
    <button
      ref="triggerRef"
      @click="toggleDropdown"
      :disabled="switching"
      class="flex min-h-10 items-center gap-1.5 rounded-sm px-2 py-1.5 text-label font-semibold text-fg-muted transition-colors hover:bg-accent-weak hover:text-accent-strong"
      :title="currentLocale?.name"
      aria-haspopup="true"
      :aria-expanded="isOpen"
    >
      <span class="text-base" aria-hidden="true">{{ currentLocale?.flag }}</span>
      <span class="hidden sm:inline" aria-hidden="true">{{ currentLocale?.code.toUpperCase() }}</span>
      <span class="sr-only" :lang="currentLocale?.code">{{ currentLocale?.name }}</span>
      <Icon
        name="chevronDown"
        size="xs"
        class="text-fg-subtle transition-transform duration-150"
        :class="{ 'rotate-180': isOpen }"
      />
    </button>

    <transition name="dropdown">
      <div
        v-if="isOpen"
        class="absolute right-0 z-50 mt-1 w-36 overflow-hidden rounded-none border border-border-strong border-t-2 border-t-accent bg-surface-raised py-1 shadow-overlay"
      >
        <button
          v-for="locale in availableLocales"
          :key="locale.code"
          :disabled="switching"
          :lang="locale.code"
          :aria-current="locale.code === currentLocaleCode ? 'true' : undefined"
          @click="selectLocale(locale.code)"
          class="flex w-full items-center gap-2 px-3 py-2 text-body text-fg transition-colors hover:bg-accent-weak hover:text-accent-strong"
          :class="{
            'bg-accent-weak font-semibold text-accent-strong':
              locale.code === currentLocaleCode
          }"
        >
          <span class="text-base">{{ locale.flag }}</span>
          <span>{{ locale.name }}</span>
          <Icon v-if="locale.code === currentLocaleCode" name="check" size="sm" class="ml-auto text-accent" />
        </button>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { setLocale, availableLocales } from '@/i18n'

const { locale } = useI18n()

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const switching = ref(false)

const currentLocaleCode = computed(() => locale.value)
const currentLocale = computed(() => availableLocales.find((l) => l.code === locale.value))

function toggleDropdown() {
  isOpen.value = !isOpen.value
}

async function selectLocale(code: string) {
  if (switching.value || code === currentLocaleCode.value) {
    isOpen.value = false
    return
  }
  switching.value = true
  try {
    await setLocale(code)
    isOpen.value = false
  } finally {
    switching.value = false
  }
}

function closeOnEscape() {
  if (!isOpen.value) return
  isOpen.value = false
  triggerRef.value?.focus()
}

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    isOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.12s ease, transform 0.12s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
