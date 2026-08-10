<template>
  <div data-testid="compact-home" class="flex min-h-screen min-w-0 flex-col overflow-x-hidden bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img :src="context.siteLogo || '/logo.svg'" alt="Logo" class="h-9 w-9 shrink-0 rounded-lg object-contain" />
          <span class="min-w-0 truncate text-base font-semibold">{{ context.siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a v-if="context.docUrl" :href="context.docUrl" target="_blank" rel="noopener noreferrer" class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800" :title="t('home.viewDocs')"><Icon name="book" size="md" /></a>
          <button class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800" :title="context.isDark ? t('home.switchToLight') : t('home.switchToDark')" @click="context.toggleTheme">
            <Icon v-if="context.isDark" name="sun" size="md" /><Icon v-else name="moon" size="md" />
          </button>
          <router-link :to="destination" class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200">
            {{ context.isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>
    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img :src="context.siteLogo || '/logo.svg'" alt="Logo" class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain" />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ context.siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ context.siteSubtitle }}</p>
        <router-link :to="destination" class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700">
          {{ context.isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>
    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ context.currentYear }} {{ context.siteName }}
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import type { HomeStyleContext } from './types'

const props = defineProps<{ context: HomeStyleContext }>()
const { t } = useI18n()
const destination = computed(() => props.context.isAuthenticated ? props.context.dashboardPath : '/login')
</script>
