<template>
  <footer class="border-t border-gray-100 py-6 dark:border-dark-800">
    <div class="mx-auto flex max-w-7xl flex-col items-center justify-between gap-3 px-6 sm:flex-row">
      <div class="flex items-center gap-3">
        <div class="h-5 w-5 overflow-hidden rounded-md">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
        </div>
        <p class="text-sm text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
      </div>
      <div class="flex flex-wrap items-center justify-center gap-x-5 gap-y-1">
        <router-link to="/legal/terms"
          class="text-sm text-gray-500 transition-colors hover:text-gray-900 dark:text-dark-400 dark:hover:text-white">
          {{ t('home.footer.terms') }}
        </router-link>
        <router-link to="/legal/privacy"
          class="text-sm text-gray-500 transition-colors hover:text-gray-900 dark:text-dark-400 dark:hover:text-white">
          {{ t('home.footer.privacy') }}
        </router-link>
        <router-link to="/legal/usage-policy"
          class="text-sm text-gray-500 transition-colors hover:text-gray-900 dark:text-dark-400 dark:hover:text-white">
          {{ t('home.footer.usagePolicy') }}
        </router-link>
        <router-link to="/legal/supported-regions"
          class="text-sm text-gray-500 transition-colors hover:text-gray-900 dark:text-dark-400 dark:hover:text-white">
          {{ t('home.footer.supportedRegions') }}
        </router-link>
        <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer"
          class="text-sm text-gray-500 transition-colors hover:text-gray-900 dark:text-dark-400 dark:hover:text-white">
          {{ t('home.docs') }}
        </a>
      </div>
    </div>
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const currentYear = new Date().getFullYear()
</script>
