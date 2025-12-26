<template>
  <div class="relative" ref="dropdownRef">
    <button
      @click="toggleDropdown"
      class="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
      :title="currentLocale?.name"
    >
      <span class="text-base">{{ currentLocale?.flag REDACTEDREDACTED</span>
      <span class="hidden sm:inline">{{ currentLocale?.code.toUpperCase() REDACTEDREDACTED</span>
      <svg
        class="h-3.5 w-3.5 text-gray-400 transition-transform duration-200"
        :class="{ 'rotate-180': isOpen REDACTED"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
      </svg>
    </button>

    <transition name="dropdown">
      <div
        v-if="isOpen"
        class="absolute right-0 z-50 mt-1 w-32 overflow-hidden rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
      >
        <button
          v-for="locale in availableLocales"
          :key="locale.code"
          @click="selectLocale(locale.code)"
          class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
          :class="{
            'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400':
              locale.code === currentLocaleCode
          REDACTED"
        >
          <span class="text-base">{{ locale.flag REDACTEDREDACTED</span>
          <span>{{ locale.name REDACTEDREDACTED</span>
          <svg
            v-if="locale.code === currentLocaleCode"
            class="ml-auto h-4 w-4 text-primary-500"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
        </button>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { setLocale, availableLocales REDACTED from '@/i18n'

const { locale REDACTED = useI18n()

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)

const currentLocaleCode = computed(() => locale.value)
const currentLocale = computed(() => availableLocales.find((l) => l.code === locale.value))

function toggleDropdown() {
  isOpen.value = !isOpen.value
REDACTED

function selectLocale(code: string) {
  setLocale(code)
  isOpen.value = false
REDACTED

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    isOpen.value = false
  REDACTED
REDACTED

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
REDACTED)

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
REDACTED)
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.15s ease;
REDACTED

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
REDACTED
</style>
