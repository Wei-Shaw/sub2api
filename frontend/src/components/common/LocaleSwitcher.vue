<template>
  <div class="flex items-center gap-2">
    <button
      v-for="locale in availableLocales"
      :key="locale.code"
      class="px-2 py-1 text-xs font-semibold rounded border transition-colors"
      :class="currentLocale === locale.code
        ? 'bg-blue-600 text-white border-blue-600 dark:bg-blue-500 dark:border-blue-500'
        : 'bg-transparent text-gray-700 border-gray-300 hover:bg-gray-100 dark:text-gray-300 dark:border-gray-600 dark:hover:bg-gray-800'"
      @click="handleSetLocale(locale.code)"
      :aria-label="'Switch to ' + locale.name"
    >
      {{ locale.name }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { availableLocales, getLocale, setLocale } from '@/i18n'

type LocaleCode = typeof availableLocales[number]['code']

const currentLocale = ref(getLocale())

async function handleSetLocale(code: LocaleCode) {
  if (currentLocale.value === code) return
  await setLocale(code)
  currentLocale.value = code
  localStorage.setItem('sub2api_home_locale', code)
}
</script>
